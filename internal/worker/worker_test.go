package worker

// NOTE SUR LES HYPOTHÈSES (internal/k8s et internal/targz non fournis) :
//
// Les tests de RunScaleDown/RunScaleUp utilisent un fake clientset
// (k8s.io/client-go/kubernetes/fake) et un petit "reconciler" maison qui
// simule le comportement du vrai contrôleur StatefulSet (que le fake
// clientset ne fait pas tourner tout seul). Ce reconciler suppose que
// internal/k8s.WaitForStatefulSetReplicas regarde StatefulSet.Status.Replicas
// / ReadyReplicas, et que WaitForStatefulSetPodReady regarde un Pod nommé
// "<statefulset>-0" (ordinal 0). Si l'implémentation réelle regarde autre
// chose (ex: un champ de status différent, un autre nom de pod), il suffit
// d'ajuster reconcileOnce() en conséquence — le reste des tests n'a pas à
// changer.
//
// Les tests "TimesOut" et "StatefulSetNotFound" ne dépendent d'aucune de ces
// hypothèses : ils vérifient juste la propagation d'erreur, qui doit tenir
// quelle que soit l'implémentation de internal/k8s.
//
// Les tests de RunBackup appellent la vraie fonction targz.TarGzDirectory
// (elle n'est pas mockée) et vérifient le résultat observable : un fichier
// .tar.gz valide contenant les fichiers de la source. Aucune hypothèse sur
// son fonctionnement interne n'est nécessaire, seulement sur le contrat
// (archive tar.gz valide, erreur si le répertoire source n'existe pas).

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// -----------------------------------------------------------------------
// Helpers communs
// -----------------------------------------------------------------------

func int32Ptr(i int32) *int32 { return &i }

// newFakeStatefulSet crée un fake clientset contenant un unique StatefulSet
// avec `replicas` à la fois dans le spec et le status.
func newFakeStatefulSet(replicas int32) (client kubernetes.Interface, namespace, name string) {
	namespace, name = "plex-ns", "plex"
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(replicas),
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:        replicas,
			ReadyReplicas:   replicas,
			CurrentReplicas: replicas,
		},
	}
	return fake.NewSimpleClientset(sts), namespace, name
}

// startFakeReconciler simule, en tâche de fond, le sous-ensemble du
// contrôleur StatefulSet dont RunScaleDown/RunScaleUp ont besoin pour
// converger : recopier spec.Replicas vers le status, et créer/supprimer le
// pod "<name>-0" en le marquant Ready. Voir la note d'hypothèses en tête de
// fichier.
func startFakeReconciler(t *testing.T, client kubernetes.Interface, namespace, name string) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(15 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcileOnce(client, namespace, name)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func reconcileOnce(client kubernetes.Interface, namespace, name string) {
	ctx := context.Background()

	sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return
	}

	desired := int32(1)
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}

	if sts.Status.Replicas != desired || sts.Status.ReadyReplicas != desired {
		sts.Status.Replicas = desired
		sts.Status.ReadyReplicas = desired
		sts.Status.CurrentReplicas = desired
		_, _ = client.AppsV1().StatefulSets(namespace).UpdateStatus(ctx, sts, metav1.UpdateOptions{})
	}

	podName := fmt.Sprintf("%s-0", name)
	pod, getPodErr := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})

	switch {
	case desired >= 1 && getPodErr != nil:
		newPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}
		_, _ = client.CoreV1().Pods(namespace).Create(ctx, newPod, metav1.CreateOptions{})

	case desired >= 1 && getPodErr == nil && !podIsReady(pod):
		pod.Status.Phase = corev1.PodRunning
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		_, _ = client.CoreV1().Pods(namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})

	case desired == 0 && getPodErr == nil:
		_ = client.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	}
}

func podIsReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// RunScaleDown
// -----------------------------------------------------------------------

func TestRunScaleDown_ScalesToZero(t *testing.T) {
	client, ns, name := newFakeStatefulSet(1)
	stop := startFakeReconciler(t, client, ns, name)
	defer stop()

	if err := RunScaleDown(client, ns, name, 5*time.Second); err != nil {
		t.Fatalf("RunScaleDown a renvoyé une erreur inattendue : %v", err)
	}

	sts, err := client.AppsV1().StatefulSets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("impossible de relire le StatefulSet : %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Errorf("attendu spec.Replicas = 0, obtenu %v", sts.Spec.Replicas)
	}
}

func TestRunScaleDown_TimesOutWhenReplicasNeverDrop(t *testing.T) {
	client, ns, name := newFakeStatefulSet(1)
	// Pas de reconciler ici : le status ne convergera jamais vers 0,
	// donc l'attente doit finir par expirer.
	err := RunScaleDown(client, ns, name, 300*time.Millisecond)
	if err == nil {
		t.Fatal("attendu une erreur quand les replicas ne redescendent jamais à 0, obtenu nil")
	}
}

func TestRunScaleDown_StatefulSetNotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	if err := RunScaleDown(client, "plex-ns", "does-not-exist", time.Second); err == nil {
		t.Fatal("attendu une erreur quand le StatefulSet n'existe pas")
	}
}

// -----------------------------------------------------------------------
// RunScaleUp
// -----------------------------------------------------------------------

func TestRunScaleUp_ScalesToOneAndWaitsForPodReady(t *testing.T) {
	client, ns, name := newFakeStatefulSet(0)
	stop := startFakeReconciler(t, client, ns, name)
	defer stop()

	if err := RunScaleUp(client, ns, name, 5*time.Second); err != nil {
		t.Fatalf("RunScaleUp a renvoyé une erreur inattendue : %v", err)
	}

	sts, err := client.AppsV1().StatefulSets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("impossible de relire le StatefulSet : %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Errorf("attendu spec.Replicas = 1, obtenu %v", sts.Spec.Replicas)
	}
}

func TestRunScaleUp_TimesOutWhenReplicasNeverRise(t *testing.T) {
	client, ns, name := newFakeStatefulSet(0)
	// L'attente sur les replicas (qui utilise `timeout`) échouera avant
	// même d'atteindre l'attente de pod ready (codée en dur à 10 minutes
	// dans RunScaleUp), donc ce test reste rapide.
	err := RunScaleUp(client, ns, name, 300*time.Millisecond)
	if err == nil {
		t.Fatal("attendu une erreur quand les replicas ne montent jamais à 1, obtenu nil")
	}
}

func TestRunScaleUp_StatefulSetNotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	if err := RunScaleUp(client, "plex-ns", "does-not-exist", time.Second); err == nil {
		t.Fatal("attendu une erreur quand le StatefulSet n'existe pas")
	}
}

// -----------------------------------------------------------------------
// RunBackup
// -----------------------------------------------------------------------

var backupNamePattern = regexp.MustCompile(`^plex_backup_\d{8}_\d{6}\.tar\.gz$`)

func TestRunBackup_CreatesArchiveWithSourceContents(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "config.xml"), "<config/>")
	writeFile(t, filepath.Join(srcDir, "sub", "data.db"), "fake-db-bytes")

	if err := RunBackup("plex-ns", "plex", srcDir, destDir); err != nil {
		t.Fatalf("RunBackup a renvoyé une erreur inattendue : %v", err)
	}

	archivePath := findBackupArchive(t, destDir)

	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("l'archive devrait exister : %v", err)
	}
	if info.Size() == 0 {
		t.Error("l'archive ne devrait pas être vide")
	}

	entries := listTarGzEntries(t, archivePath)
	for _, want := range []string{"config.xml", filepath.Join("sub", "data.db")} {
		if !containsSuffix(entries, want) {
			t.Errorf("attendu une entrée se terminant par %q dans l'archive, entrées trouvées : %v", want, entries)
		}
	}
}

func TestRunBackup_ReturnsErrorForMissingSourceDirectory(t *testing.T) {
	destDir := t.TempDir()
	missingSrc := filepath.Join(destDir, "does-not-exist")

	if err := RunBackup("plex-ns", "plex", missingSrc, destDir); err == nil {
		t.Fatal("attendu une erreur quand le répertoire source n'existe pas")
	}
}

func findBackupArchive(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("échec de lecture du répertoire de destination : %v", err)
	}
	var matches []string
	for _, e := range entries {
		if !e.IsDir() && backupNamePattern.MatchString(e.Name()) {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) != 1 {
		t.Fatalf("attendu exactement une archive de backup dans %s, trouvé %v", dir, matches)
	}
	return filepath.Join(dir, matches[0])
}

func listTarGzEntries(t *testing.T, archivePath string) []string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("échec d'ouverture de l'archive : %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("l'archive n'est pas un gzip valide : %v", err)
	}
	defer gz.Close()

	var names []string
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("échec de lecture d'une entrée tar : %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

func containsSuffix(entries []string, suffix string) bool {
	suffix = filepath.ToSlash(suffix)
	for _, e := range entries {
		if strings.HasSuffix(filepath.ToSlash(e), suffix) {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// CleanupBackupsByAge
// -----------------------------------------------------------------------

func backupFileName(when time.Time) string {
	return "plex_backup_" + when.Format("20060102_150405") + ".tar.gz"
}

func TestCleanupBackupsByAge(t *testing.T) {
	now := time.Now()

	t.Run("supprime les fichiers plus vieux que la rétention", func(t *testing.T) {
		dir := t.TempDir()
		oldFile := filepath.Join(dir, backupFileName(now.AddDate(0, 0, -20)))
		recentFile := filepath.Join(dir, backupFileName(now.AddDate(0, 0, -5)))
		writeFile(t, oldFile, "old")
		writeFile(t, recentFile, "recent")

		if err := CleanupBackupsByAge(dir, 14); err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}

		assertNotExists(t, oldFile)
		assertExists(t, recentFile)
	})

	t.Run("ignore les fichiers qui ne correspondent pas au pattern", func(t *testing.T) {
		dir := t.TempDir()
		other := filepath.Join(dir, "notes.txt")
		writeFile(t, other, "keep me")

		if err := CleanupBackupsByAge(dir, 0); err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		assertExists(t, other)
	})

	t.Run("ignore les répertoires même s'ils matchent le pattern", func(t *testing.T) {
		dir := t.TempDir()
		fakeDir := filepath.Join(dir, backupFileName(now.AddDate(0, 0, -30)))
		if err := os.Mkdir(fakeDir, 0o755); err != nil {
			t.Fatalf("échec de création du répertoire : %v", err)
		}

		if err := CleanupBackupsByAge(dir, 1); err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		assertExists(t, fakeDir)
	})

	t.Run("garde les fichiers dont la date est imparsable", func(t *testing.T) {
		dir := t.TempDir()
		malformed := filepath.Join(dir, "plex_backup_not-a-date.tar.gz")
		writeFile(t, malformed, "???")

		if err := CleanupBackupsByAge(dir, 0); err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		assertExists(t, malformed)
	})

	t.Run("ne fait rien sur un répertoire vide", func(t *testing.T) {
		dir := t.TempDir()
		if err := CleanupBackupsByAge(dir, 7); err != nil {
			t.Fatalf("erreur inattendue sur répertoire vide : %v", err)
		}
	})

	t.Run("renvoie une erreur si le répertoire n'existe pas", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		if err := CleanupBackupsByAge(missing, 7); err == nil {
			t.Fatal("attendu une erreur pour un répertoire inexistant")
		}
	})
}

// -----------------------------------------------------------------------
// Helpers fichiers
// -----------------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("échec de création du dossier pour %s : %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("échec d'écriture de %s : %v", path, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s devrait toujours exister : %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s aurait dû être supprimé, stat err = %v", path, err)
	}
}

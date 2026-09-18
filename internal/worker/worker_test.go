package worker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// --- Helpers ---

func int32Ptr(i int32) *int32 {
	return &i
}

func newFakeStatefulSet(namespace, name string, replicas, readyReplicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(replicas)},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: readyReplicas},
	}
}

func newFakeReadyPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

// runSubprocessTest re-executes the current test binary running only the named
// test, with GO_TEST_SUBPROCESS=caseName set in the environment. It is used to
// exercise the log.Fatalf(...) branches of worker functions, since os.Exit
// cannot be recovered from within the same test process.
func runSubprocessTest(t *testing.T, testName, caseName string) (exitCode int, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), "GO_TEST_SUBPROCESS="+caseName)
	var stderrBuf, stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf

	err := cmd.Run()
	if err == nil {
		return 0, stderrBuf.String() + stdoutBuf.String()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), stderrBuf.String() + stdoutBuf.String()
	}
	t.Fatalf("failed to run subprocess: %v", err)
	return -1, ""
}

// --- RunScaleDown ---

func TestRunScaleDown_Success(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	// ReadyReplicas already at 0 so WaitForStatefulSetReplicas(target=0) succeeds immediately.
	sts := newFakeStatefulSet(namespace, name, 3, 0)
	clientset := fake.NewSimpleClientset(sts)

	done := make(chan struct{})
	go func() {
		RunScaleDown(clientset, namespace, name, time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunScaleDown did not return in time")
	}

	updated, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get statefulset: %v", err)
	}
	if updated.Spec.Replicas == nil || *updated.Spec.Replicas != 0 {
		t.Fatalf("expected replicas to be 0, got %v", updated.Spec.Replicas)
	}
}

func TestRunScaleDown_FatalOnScaleDownError(t *testing.T) {
	if os.Getenv("GO_TEST_SUBPROCESS") == "scaledown_notfound" {
		client := fake.NewSimpleClientset() // no StatefulSet present -> ScaleDown fails
		RunScaleDown(client, "default", "missing-sts", time.Second)
		return
	}

	exitCode, output := runSubprocessTest(t, "TestRunScaleDown_FatalOnScaleDownError", "scaledown_notfound")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code from log.Fatalf, got 0. Output: %s", output)
	}
	if !strings.Contains(output, "Error when ScaleDown the StatefulSet") {
		t.Errorf("expected output to contain fatal log message, got: %q", output)
	}
}

func TestRunScaleDown_FatalOnWaitTimeout(t *testing.T) {
	if os.Getenv("GO_TEST_SUBPROCESS") == "scaledown_wait_timeout" {
		// ReadyReplicas will never drop to 0 in the fake clientset (no controller),
		// so the wait step times out and RunScaleDown must call log.Fatalf.
		sts := newFakeStatefulSet("default", "my-sts", 3, 3)
		client := fake.NewSimpleClientset(sts)
		RunScaleDown(client, "default", "my-sts", 50*time.Millisecond)
		return
	}

	exitCode, output := runSubprocessTest(t, "TestRunScaleDown_FatalOnWaitTimeout", "scaledown_wait_timeout")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code from log.Fatalf, got 0. Output: %s", output)
	}
	if !strings.Contains(output, "Timeout waiting for StatefulSet to scale down") {
		t.Errorf("expected output to contain timeout fatal message, got: %q", output)
	}
}

// --- RunScaleUp ---

func TestRunScaleUp_Success(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	// ReadyReplicas already at 1 and the ordinal-0 pod is already Ready, so both
	// wait steps succeed immediately.
	sts := newFakeStatefulSet(namespace, name, 0, 1)
	pod := newFakeReadyPod(namespace, name+"-0")
	clientset := fake.NewSimpleClientset(sts, pod)

	done := make(chan struct{})
	go func() {
		RunScaleUp(clientset, namespace, name, time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunScaleUp did not return in time")
	}

	updated, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get statefulset: %v", err)
	}
	if updated.Spec.Replicas == nil || *updated.Spec.Replicas != 1 {
		t.Fatalf("expected replicas to be 1, got %v", updated.Spec.Replicas)
	}
}

func TestRunScaleUp_FatalOnScaleUpError(t *testing.T) {
	if os.Getenv("GO_TEST_SUBPROCESS") == "scaleup_notfound" {
		client := fake.NewSimpleClientset() // no StatefulSet present -> ScaleUp fails
		RunScaleUp(client, "default", "missing-sts", time.Second)
		return
	}

	exitCode, output := runSubprocessTest(t, "TestRunScaleUp_FatalOnScaleUpError", "scaleup_notfound")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code from log.Fatalf, got 0. Output: %s", output)
	}
	if !strings.Contains(output, "Error when ScaleUp the StatefulSet") {
		t.Errorf("expected output to contain fatal log message, got: %q", output)
	}
}

func TestRunScaleUp_FatalOnReplicasWaitTimeout(t *testing.T) {
	if os.Getenv("GO_TEST_SUBPROCESS") == "scaleup_replicas_wait_timeout" {
		// ReadyReplicas stays at 0 in the fake clientset (no controller simulating
		// pod startup), so waiting for it to reach 1 times out.
		sts := newFakeStatefulSet("default", "my-sts", 0, 0)
		client := fake.NewSimpleClientset(sts)
		RunScaleUp(client, "default", "my-sts", 50*time.Millisecond)
		return
	}

	exitCode, output := runSubprocessTest(t, "TestRunScaleUp_FatalOnReplicasWaitTimeout", "scaleup_replicas_wait_timeout")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code from log.Fatalf, got 0. Output: %s", output)
	}
	if !strings.Contains(output, "Timeout waiting for StatefulSet to scale up") {
		t.Errorf("expected output to contain timeout fatal message, got: %q", output)
	}
}

// --- RunBackup ---

func TestRunBackup_Success(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()

	// Create a small file tree to archive.
	if err := os.WriteFile(filepath.Join(sourceDir, "hello.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	subDir := filepath.Join(sourceDir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("failed to create source subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0o644); err != nil {
		t.Fatalf("failed to create nested source file: %v", err)
	}

	RunBackup("default", "my-sts", sourceDir, destDir)

	matches, err := filepath.Glob(filepath.Join(destDir, "plex_backup_*.tar.gz"))
	if err != nil {
		t.Fatalf("failed to glob destination directory: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup archive, found %d: %v", len(matches), matches)
	}

	archivePath := matches[0]
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("failed to stat archive: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected archive to be non-empty")
	}

	// Sanity-check that the archive is a valid tar.gz containing our file.
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("archive is not valid gzip: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	foundHello := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read tar entry: %v", err)
		}
		if strings.HasSuffix(hdr.Name, "hello.txt") {
			foundHello = true
		}
	}
	if !foundHello {
		t.Error("expected archive to contain hello.txt")
	}
}

func TestRunBackup_FatalOnInvalidSourceDirectory(t *testing.T) {
	if os.Getenv("GO_TEST_SUBPROCESS") == "backup_bad_source" {
		destDir, err := os.MkdirTemp("", "backup-dest-")
		if err != nil {
			panic(err)
		}
		RunBackup("default", "my-sts", "/path/does/not/exist", destDir)
		return
	}

	exitCode, output := runSubprocessTest(t, "TestRunBackup_FatalOnInvalidSourceDirectory", "backup_bad_source")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code from log.Fatalf, got 0. Output: %s", output)
	}
	if !strings.Contains(output, "Error when creating the archive") {
		t.Errorf("expected output to contain fatal log message, got: %q", output)
	}
}

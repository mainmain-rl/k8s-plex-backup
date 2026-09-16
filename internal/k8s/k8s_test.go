package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// --- Helpers de fixtures ---

func newStatefulSet(namespace, name string, replicas, readyReplicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: readyReplicas,
		},
	}
}

func newPod(namespace, name string, phase corev1.PodPhase, ready bool) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: phase,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: status},
			},
		},
	}
}

// --- IsPodReady ---

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{"running et ready", newPod("ns", "pod-0", corev1.PodRunning, true), true},
		{"running mais pas ready", newPod("ns", "pod-0", corev1.PodRunning, false), false},
		{"pending", newPod("ns", "pod-0", corev1.PodPending, true), false},
		{"running sans condition Ready", &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPodReady(tt.pod); got != tt.want {
				t.Errorf("IsPodReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- ScaleDown / ScaleUp ---

func TestScaleDown(t *testing.T) {
	ns, name := "apps", "plex"
	clientset := fake.NewSimpleClientset(newStatefulSet(ns, name, 1, 1))

	if err := ScaleDown(context.Background(), clientset, ns, name); err != nil {
		t.Fatalf("ScaleDown() error = %v", err)
	}

	got, err := clientset.AppsV1().StatefulSets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() après ScaleDown a échoué: %v", err)
	}
	if got.Spec.Replicas == nil || *got.Spec.Replicas != 0 {
		t.Errorf("replicas attendu = 0, obtenu = %v", got.Spec.Replicas)
	}
}

func TestScaleUp(t *testing.T) {
	ns, name := "apps", "plex"
	clientset := fake.NewSimpleClientset(newStatefulSet(ns, name, 0, 0))

	if err := ScaleUp(context.Background(), clientset, ns, name); err != nil {
		t.Fatalf("ScaleUp() error = %v", err)
	}

	got, err := clientset.AppsV1().StatefulSets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() après ScaleUp a échoué: %v", err)
	}
	if got.Spec.Replicas == nil || *got.Spec.Replicas != 1 {
		t.Errorf("replicas attendu = 1, obtenu = %v", got.Spec.Replicas)
	}
}

func TestScaleDown_StatefulSetNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset() // aucun statefulset enregistré

	if err := ScaleDown(context.Background(), clientset, "apps", "inexistant"); err == nil {
		t.Fatal("erreur attendue quand le statefulset n'existe pas, obtenu nil")
	}
}

// --- WaitForStatefulSetReplicas ---

func TestWaitForStatefulSetReplicas_Success(t *testing.T) {
	ns, name := "apps", "plex"
	clientset := fake.NewSimpleClientset(newStatefulSet(ns, name, 1, 1)) // déjà à la cible

	err := WaitForStatefulSetReplicas(context.Background(), clientset, ns, name, 1, 10*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForStatefulSetReplicas() error = %v", err)
	}
}

func TestWaitForStatefulSetReplicas_Timeout(t *testing.T) {
	ns, name := "apps", "plex"
	clientset := fake.NewSimpleClientset(newStatefulSet(ns, name, 1, 0)) // n'atteindra jamais la cible

	err := WaitForStatefulSetReplicas(context.Background(), clientset, ns, name, 1, 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("erreur de timeout attendue, obtenu nil")
	}
}

// --- WaitForPodReady / WaitForStatefulSetPodReady ---

func TestWaitForPodReady_Success(t *testing.T) {
	ns, podName := "apps", "plex-0"
	clientset := fake.NewSimpleClientset(newPod(ns, podName, corev1.PodRunning, true))

	err := WaitForPodReady(context.Background(), clientset, ns, podName, 10*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForPodReady() error = %v", err)
	}
}

func TestWaitForPodReady_NeverAppears(t *testing.T) {
	clientset := fake.NewSimpleClientset() // aucun pod enregistré

	err := WaitForPodReady(context.Background(), clientset, "apps", "absent", 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("erreur de timeout attendue quand le pod n'apparaît jamais, obtenu nil")
	}
}

func TestWaitForStatefulSetPodReady(t *testing.T) {
	ns, name := "apps", "plex"
	clientset := fake.NewSimpleClientset(newPod(ns, name+"-0", corev1.PodRunning, true))

	err := WaitForStatefulSetPodReady(context.Background(), clientset, ns, name, 0, 10*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForStatefulSetPodReady() error = %v", err)
	}
}

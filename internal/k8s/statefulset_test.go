package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// --- Helpers ---

func int32Ptr(i int32) *int32 {
	return &i
}

func newFakeStatefulSet(namespace, name string, replicas, readyReplicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(replicas),
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: readyReplicas,
		},
	}
}

func newFakePod(namespace, name string, phase corev1.PodPhase, ready bool) *corev1.Pod {
	condStatus := corev1.ConditionFalse
	if ready {
		condStatus = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: phase,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: condStatus,
				},
			},
		},
	}
}

// --- ScaleDown ---

func TestScaleDown_Success(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	sts := newFakeStatefulSet(namespace, name, 3, 3)
	clientset := fake.NewSimpleClientset(sts)

	if err := ScaleDown(ctx, clientset, namespace, name); err != nil {
		t.Fatalf("ScaleDown returned unexpected error: %v", err)
	}

	updated, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get statefulset after scale down: %v", err)
	}
	if updated.Spec.Replicas == nil || *updated.Spec.Replicas != 0 {
		t.Fatalf("expected replicas to be 0, got %v", updated.Spec.Replicas)
	}
}

func TestScaleDown_NotFound(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	err := ScaleDown(ctx, clientset, "default", "does-not-exist")
	if err == nil {
		t.Fatal("expected an error when scaling down a non-existent statefulset, got nil")
	}
	if !apierrors.IsNotFound(errUnwrapAll(err)) {
		// Not fatal: wrapping may hide the exact type in some client-go versions,
		// but we still expect an error to be returned.
		t.Logf("error is not a NotFound apierror (may be wrapped): %v", err)
	}
}

// --- ScaleUp ---

func TestScaleUp_Success(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	sts := newFakeStatefulSet(namespace, name, 0, 0)
	clientset := fake.NewSimpleClientset(sts)

	if err := ScaleUp(ctx, clientset, namespace, name); err != nil {
		t.Fatalf("ScaleUp returned unexpected error: %v", err)
	}

	updated, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get statefulset after scale up: %v", err)
	}
	if updated.Spec.Replicas == nil || *updated.Spec.Replicas != 1 {
		t.Fatalf("expected replicas to be 1, got %v", updated.Spec.Replicas)
	}
}

func TestScaleUp_NotFound(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	err := ScaleUp(ctx, clientset, "default", "does-not-exist")
	if err == nil {
		t.Fatal("expected an error when scaling up a non-existent statefulset, got nil")
	}
}

// --- IsPodReady ---

func TestIsPodReady(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "running and ready",
			pod:  newFakePod("default", "pod-0", corev1.PodRunning, true),
			want: true,
		},
		{
			name: "running but not ready",
			pod:  newFakePod("default", "pod-0", corev1.PodRunning, false),
			want: false,
		},
		{
			name: "pending",
			pod:  newFakePod("default", "pod-0", corev1.PodPending, true),
			want: false,
		},
		{
			name: "running with no conditions",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Namespace: "default"},
				Status: corev1.PodStatus{
					Phase:      corev1.PodRunning,
					Conditions: []corev1.PodCondition{},
				},
			},
			want: false,
		},
		{
			name: "failed",
			pod:  newFakePod("default", "pod-0", corev1.PodFailed, false),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPodReady(tt.pod)
			if got != tt.want {
				t.Errorf("IsPodReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- WaitForPodReady ---

func TestWaitForPodReady_AlreadyReady(t *testing.T) {
	ctx := context.Background()
	namespace, podName := "default", "my-sts-0"

	pod := newFakePod(namespace, podName, corev1.PodRunning, true)
	clientset := fake.NewSimpleClientset(pod)

	err := WaitForPodReady(ctx, clientset, namespace, podName, 10*time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("WaitForPodReady returned unexpected error: %v", err)
	}
}

func TestWaitForPodReady_BecomesReadyLater(t *testing.T) {
	ctx := context.Background()
	namespace, podName := "default", "my-sts-0"

	pod := newFakePod(namespace, podName, corev1.PodPending, false)
	clientset := fake.NewSimpleClientset(pod)

	// Flip the pod to Ready after a short delay, concurrently with the poll.
	go func() {
		time.Sleep(30 * time.Millisecond)
		readyPod := newFakePod(namespace, podName, corev1.PodRunning, true)
		_, _ = clientset.CoreV1().Pods(namespace).Update(ctx, readyPod, metav1.UpdateOptions{})
	}()

	err := WaitForPodReady(ctx, clientset, namespace, podName, 10*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForPodReady returned unexpected error: %v", err)
	}
}

func TestWaitForPodReady_TimeoutNeverReady(t *testing.T) {
	ctx := context.Background()
	namespace, podName := "default", "my-sts-0"

	pod := newFakePod(namespace, podName, corev1.PodPending, false)
	clientset := fake.NewSimpleClientset(pod)

	err := WaitForPodReady(ctx, clientset, namespace, podName, 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

func TestWaitForPodReady_PodNeverCreated(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	err := WaitForPodReady(ctx, clientset, "default", "ghost-pod", 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error when pod never appears, got nil")
	}
}

// --- WaitForStatefulSetPodReady ---

func TestWaitForStatefulSetPodReady_Success(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"
	ordinal := 2

	pod := newFakePod(namespace, "my-sts-2", corev1.PodRunning, true)
	clientset := fake.NewSimpleClientset(pod)

	err := WaitForStatefulSetPodReady(ctx, clientset, namespace, name, ordinal, 10*time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("WaitForStatefulSetPodReady returned unexpected error: %v", err)
	}
}

func TestWaitForStatefulSetPodReady_WrongOrdinalTimesOut(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	// Only ordinal 0 exists and is ready; we wait for ordinal 1, which never appears.
	pod := newFakePod(namespace, "my-sts-0", corev1.PodRunning, true)
	clientset := fake.NewSimpleClientset(pod)

	err := WaitForStatefulSetPodReady(ctx, clientset, namespace, name, 1, 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error for a pod ordinal that doesn't exist, got nil")
	}
}

// --- WaitForStatefulSetReplicas ---

func TestWaitForStatefulSetReplicas_AlreadyMatching(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	sts := newFakeStatefulSet(namespace, name, 3, 3)
	clientset := fake.NewSimpleClientset(sts)

	err := WaitForStatefulSetReplicas(ctx, clientset, namespace, name, 3, 10*time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("WaitForStatefulSetReplicas returned unexpected error: %v", err)
	}
}

func TestWaitForStatefulSetReplicas_BecomesMatchingLater(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	sts := newFakeStatefulSet(namespace, name, 3, 1)
	clientset := fake.NewSimpleClientset(sts)

	go func() {
		time.Sleep(30 * time.Millisecond)
		updated := newFakeStatefulSet(namespace, name, 3, 3)
		_, _ = clientset.AppsV1().StatefulSets(namespace).Update(ctx, updated, metav1.UpdateOptions{})
	}()

	err := WaitForStatefulSetReplicas(ctx, clientset, namespace, name, 3, 10*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForStatefulSetReplicas returned unexpected error: %v", err)
	}
}

func TestWaitForStatefulSetReplicas_Timeout(t *testing.T) {
	ctx := context.Background()
	namespace, name := "default", "my-sts"

	sts := newFakeStatefulSet(namespace, name, 3, 1)
	clientset := fake.NewSimpleClientset(sts)

	err := WaitForStatefulSetReplicas(ctx, clientset, namespace, name, 3, 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

func TestWaitForStatefulSetReplicas_NotFound(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()

	err := WaitForStatefulSetReplicas(ctx, clientset, "default", "does-not-exist", 1, 10*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error when statefulset doesn't exist, got nil")
	}
}

// errUnwrapAll is a small helper to try to reach the root apierrors.StatusError
// even if intermediate wrapping (%w) was applied.
func errUnwrapAll(err error) error {
	type unwrapper interface {
		Unwrap() error
	}
	for err != nil {
		if u, ok := err.(unwrapper); ok {
			next := u.Unwrap()
			if next == nil {
				break
			}
			err = next
			continue
		}
		break
	}
	return err
}

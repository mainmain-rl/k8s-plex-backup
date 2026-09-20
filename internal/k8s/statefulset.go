package k8s

import (
	"context"
	"fmt"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// Func allow to scale down a StatefulSet with his namespace and name.
// clientset: Kubernetes clientset
// namespace: Namespace of the StatefulSet
// name: Name of the StatefulSet
func ScaleDown(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	log.Printf("Trying to scale down %s/%s", namespace, name)
	_, err := clientset.AppsV1().StatefulSets(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		[]byte(`{"spec":{"replicas":0}}`),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("scale down failed for statefulset %s/%s: %w", namespace, name, err)
	}
	log.Printf("%s/%s scaled down", namespace, name)
	return nil
}

// Func allow to scale up a StatefulSet with his namespace and name.
// clientset: Kubernetes clientset
// namespace: Namespace of the StatefulSet
// name: Name of the StatefulSet
func ScaleUp(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	log.Printf("Trying to scale up %s/%s", namespace, name)
	_, err := clientset.AppsV1().StatefulSets(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		[]byte(`{"spec":{"replicas":1}}`),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("scale up failed for statefulset %s/%s: %w", namespace, name, err)
	}
	log.Printf("%s/%s scaled up", namespace, name)
	return nil
}

// Find the State of the pod. Return True if it is Ready, else return False.
// pod: Pod resource to check
func IsPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// Pull (with interval and timeout) a Pod resource until it's Ready.
// clientset: Kubernetes clientset
// namespace: Namespace of the Pod
// podName: Name of the Pod
// interval: Interval between checks
// timeout: Timeout for waiting for the pod to be ready
func WaitForPodReady(ctx context.Context, clientset kubernetes.Interface, namespace, podName string, interval, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return IsPodReady(pod), nil
	})
}

// Pull (with interval and timeout) a Pod resource until it's Ready trough the WaitForPodReady Func.
// clientset: Kubernetes clientset
// namespace: Namespace of the StatefulSet
// name: Name of the StatefulSet
// ordinal: Ordinal of the pod to wait for readiness
// interval: Interval between checks
// timeout: Timeout for waiting for the pod to be ready
func WaitForStatefulSetPodReady(ctx context.Context, clientset kubernetes.Interface, namespace, name string, ordinal int, interval, timeout time.Duration) error {
	log.Printf("Waiting %s/%s StatefulSet Pod Readiness", namespace, name)
	podName := fmt.Sprintf("%s-%d", name, ordinal)
	return WaitForPodReady(ctx, clientset, namespace, podName, interval, timeout)
}

// Pull (with interval and timeout) a StatefulSet resource until the ReadyReplicas match the target.
// clientset: Kubernetes clientset
// namespace: Namespace of the StatefulSet
// name: Name of the StatefulSet
// target: Target number of ready replicas
// interval: Interval between checks
// timeout: Timeout for waiting for the StatefulSet to reach the target number of ready replicas
func WaitForStatefulSetReplicas(ctx context.Context, clientset kubernetes.Interface, namespace, name string, target int32, interval, timeout time.Duration) error {
	log.Printf("Waiting %s/%s StatefulSet ReplicaSet as %d", namespace, name, target)
	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		sts, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return sts.Status.ReadyReplicas == target, nil
	})
}

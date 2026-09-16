package worker

import (
	"context"
	"fmt"
	"k8s-plex-backup/internal/k8s"
	"k8s-plex-backup/internal/targz"
	"log"

	"time"

	"k8s.io/client-go/kubernetes"
)

// RunScaleDown scales down the StatefulSet and waits for the pod to be terminated.
// kubernetesClient: Kubernetes clientset
// plexNamespace: Namespace of the StatefulSet
// plexStatefulsetName: Name of the StatefulSet
// timeout: Timeout for waiting for the pod to be terminated
func RunScaleDown(kubernetesClient kubernetes.Interface, plexNamespace string, plexStatefulsetName string, timeout time.Duration) {
	ctx := context.Background()
	if err := k8s.ScaleDown(ctx, kubernetesClient, plexNamespace, plexStatefulsetName); err != nil {
		log.Fatalf("Error when ScaleDown the StatefulSet: %s", err)
	}
	if err := k8s.WaitForStatefulSetReplicas(ctx, kubernetesClient, plexNamespace, plexStatefulsetName, 0, 2*time.Second, timeout); err != nil {
		log.Fatalf("Timeout waiting for StatefulSet to scale down: %s", err)
	}

}

// RunScaleUp scales up the StatefulSet and waits for the pod to become ready.
// kubernetesClient: Kubernetes clientset
// plexNamespace: Namespace of the StatefulSet
// plexStatefulsetName: Name of the StatefulSet
// timeout: Timeout for waiting for the pod to become ready
func RunScaleUp(kubernetesClient kubernetes.Interface, plexNamespace string, plexStatefulsetName string, timeout time.Duration) {
	ctx := context.Background()
	if err := k8s.ScaleUp(ctx, kubernetesClient, plexNamespace, plexStatefulsetName); err != nil {
		log.Fatalf("Error when ScaleUp the StatefulSet: %s", err)
	}
	if err := k8s.WaitForStatefulSetReplicas(ctx, kubernetesClient, plexNamespace, plexStatefulsetName, 1, 2*time.Second, timeout); err != nil {
		log.Fatalf("Timeout waiting for StatefulSet to scale up: %s", err)
	}
	if err := k8s.WaitForStatefulSetPodReady(ctx, kubernetesClient, plexNamespace, plexStatefulsetName, 0, 2*time.Second, 10*time.Minute); err != nil {
		log.Fatalf("Timeout waiting for pod to become ready: %s", err)
	}
	log.Printf("%s/%s resource scaled back up and ready", plexNamespace, plexStatefulsetName)
}

// RunBackup creates a tar.gz archive of the sourceDirectory and saves it to destinationArchive.
// plexNamespace: Namespace of the StatefulSet
// plexStatefulsetName: Name of the StatefulSet
// sourceDirectory: Directory to be archived
// destinationArchive: Destination directory for the archive
func RunBackup(plexNamespace string, plexStatefulsetName string, sourceDirectory string, destinationArchive string) {
	log.Printf(
		"Backup for StafulSet%s/%s\nSource directory: %s\nDestination archive: %s",
		plexNamespace, plexStatefulsetName, sourceDirectory, destinationArchive,
	)
	log.Printf("Archive will begin to be created from %s to %s", sourceDirectory, destinationArchive)
	plexBackupFileName := fmt.Sprintf("plex_backup_%s.tar.gz", time.Now().Format("20060102_150405"))
	log.Printf("Backup name: %s", plexBackupFileName)
	destinationArchive = fmt.Sprintf("%s/%s", destinationArchive, plexBackupFileName)
	if err := targz.TarGzDirectory(sourceDirectory, destinationArchive); err != nil {
		log.Fatalf("Error when creating the archive: %s", err)
	}
	fmt.Printf("Archive created: %s\n", destinationArchive)
}

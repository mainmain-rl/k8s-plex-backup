package worker

import (
	"context"
	"fmt"
	"k8s-plex-backup/internal/k8s"
	"k8s-plex-backup/internal/targz"
	"log"
	"os"
	"path/filepath"
	"strings"

	"time"

	"k8s.io/client-go/kubernetes"
)

// RunScaleDown scales down the StatefulSet and waits for the pod to be terminated.
// kubernetesClient: Kubernetes clientset
// plexNamespace: Namespace of the StatefulSet
// plexStatefulsetName: Name of the StatefulSet
// timeout: Timeout for waiting for the pod to be terminated
func RunScaleDown(kubernetesClient kubernetes.Interface, plexNamespace string, plexStatefulsetName string, timeout time.Duration) error {
	ctx := context.Background()
	log.Printf("Starting scale down for StatefulSet %s/%s", plexNamespace, plexStatefulsetName)
	if err := k8s.ScaleDown(ctx, kubernetesClient, plexNamespace, plexStatefulsetName); err != nil {
		fmt.Errorf("Error when ScaleDown the StatefulSet: %s", err)
		return err
	}
	if err := k8s.WaitForStatefulSetReplicas(ctx, kubernetesClient, plexNamespace, plexStatefulsetName, 0, 2*time.Second, timeout); err != nil {
		fmt.Errorf("Timeout waiting for StatefulSet to scale down: %s", err)
		return err
	}
	log.Printf("StatefulSet %s/%s scaled down", plexNamespace, plexStatefulsetName)
	return nil

}

// RunScaleUp scales up the StatefulSet and waits for the pod to become ready.
// kubernetesClient: Kubernetes clientset
// plexNamespace: Namespace of the StatefulSet
// plexStatefulsetName: Name of the StatefulSet
// timeout: Timeout for waiting for the pod to become ready
func RunScaleUp(kubernetesClient kubernetes.Interface, plexNamespace string, plexStatefulsetName string, timeout time.Duration) error {
	ctx := context.Background()
	log.Printf("Starting scale up for StatefulSet %s/%s", plexNamespace, plexStatefulsetName)
	if err := k8s.ScaleUp(ctx, kubernetesClient, plexNamespace, plexStatefulsetName); err != nil {
		fmt.Errorf("Error when ScaleUp the StatefulSet: %s", err)
		return err
	}
	if err := k8s.WaitForStatefulSetReplicas(ctx, kubernetesClient, plexNamespace, plexStatefulsetName, 1, 2*time.Second, timeout); err != nil {
		fmt.Errorf("Timeout waiting for StatefulSet to scale up: %s", err)
		return err
	}
	if err := k8s.WaitForStatefulSetPodReady(ctx, kubernetesClient, plexNamespace, plexStatefulsetName, 0, 2*time.Second, 10*time.Minute); err != nil {
		fmt.Errorf("Timeout waiting for pod to become ready: %s", err)
		return err
	}
	log.Printf("StatefulSet %s/%s resource scaled up and ready", plexNamespace, plexStatefulsetName)
	return nil
}

// RunBackup creates a tar.gz archive of the sourceDirectory and saves it to destinationArchive.
// plexNamespace: Namespace of the StatefulSet
// plexStatefulsetName: Name of the StatefulSet
// sourceDirectory: Directory to be archived
// destinationDirectory: Destination directory for the archive
func RunBackup(plexNamespace string, plexStatefulsetName string, sourceDirectory string, destinationDirectory string) error {
	plexBackupFileName := fmt.Sprintf("plex_backup_%s.tar.gz", time.Now().Format("20060102_150405"))
	log.Printf(
		"Starting backup %s for StafulSet %s/%s",
		plexBackupFileName, plexNamespace, plexStatefulsetName,
	)

	PathAndFullFileName := filepath.Join(destinationDirectory, plexBackupFileName)

	skipped, err := targz.TarGzDirectory(sourceDirectory, PathAndFullFileName)
	if err != nil {
		return fmt.Errorf("error when creating the archive: %w", err)
	}
	if len(skipped) > 0 {
		log.Printf("WARNING: backup completed but %d file(s) were skipped due to permission errors:", len(skipped))
		for _, path := range skipped {
			log.Printf("  - %s", path)
		}
	}
	log.Printf("Backup completed: %s\n", PathAndFullFileName)
	return nil
}

// CleanupBackupsByAge deletes backup files in the destinationDirectory that are older than retentionDays.
// destinationDirectory: Directory where backups are stored
// retentionDays: Number of days to retain backups
func CleanupBackupsByAge(destinationDirectory string, retentionDays int) error {
	entries, err := os.ReadDir(destinationDirectory)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	// Calculate the cutoff date (e.g., 14 days ago)
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	// Define the time format that matches your filename
	timeFormat := "20060102_150405"

	for _, entry := range entries {
		fileName := entry.Name()

		// 1. Only process files that match your backup naming pattern
		if !entry.IsDir() && strings.HasPrefix(fileName, "plex_backup_") && strings.HasSuffix(fileName, ".tar.gz") {

			// 2. Extract the date string from the filename
			// Removes "plex_backup_" and ".tar.gz" to leave just "20231025_150405"
			dateStr := strings.TrimPrefix(fileName, "plex_backup_")
			dateStr = strings.TrimSuffix(dateStr, ".tar.gz")

			// 3. Parse the extracted string back into a Go time object
			fileTime, err := time.Parse(timeFormat, dateStr)
			if err != nil {
				log.Printf("Warning: could not parse date from filename %s: %v\n", fileName, err)
				continue
			}

			// 4. Compare the file's time to your cutoff time
			if fileTime.Before(cutoffTime) {
				fileToDelete := filepath.Join(destinationDirectory, fileName)

				err := os.Remove(fileToDelete)
				if err != nil {
					log.Printf("Error deleting old backup %s: %v\n", fileToDelete, err)
				} else {
					log.Printf("Deleted old backup: %s (Age: older than %d days)\n", fileToDelete, retentionDays)
				}
			}
		}
	}

	return nil
}

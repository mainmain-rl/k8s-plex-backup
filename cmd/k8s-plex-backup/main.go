package main

import (
	"k8s-plex-backup/internal/config"
	"k8s-plex-backup/internal/worker"
	"log"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error when loading configuration: %s", err)
	}

	log.Printf("Starting scale down for StatefulSet %s/%s", config.Namespace, config.StatefulSetName)
	worker.RunScaleDown(
		config.KubernetesClient,
		config.Namespace,
		config.StatefulSetName,
		config.ScaleDownTimeout,
	)
	log.Printf("StatefulSet %s/%s scaled down", config.Namespace, config.StatefulSetName)

	log.Printf("Starting backup for StatefulSet %s/%s", config.Namespace, config.StatefulSetName)
	worker.RunBackup(
		config.Namespace,
		config.StatefulSetName,
		config.SourceDirectory,
		config.DestinationDirectory,
	)
	log.Printf("Backup for StatefulSet %s/%s completed", config.Namespace, config.StatefulSetName)

	log.Printf("Starting scale up for StatefulSet %s/%s", config.Namespace, config.StatefulSetName)
	worker.RunScaleUp(
		config.KubernetesClient,
		config.Namespace,
		config.StatefulSetName,
		config.ScaleUpTimeout,
	)
	log.Printf("StatefulSet %s/%s scaled up", config.Namespace, config.StatefulSetName)
}

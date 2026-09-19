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

	worker.RunScaleDown(
		config.KubernetesClient,
		config.Namespace,
		config.StatefulSetName,
		config.ScaleDownTimeout,
	)

	worker.RunBackup(
		config.Namespace,
		config.StatefulSetName,
		config.SourceDirectory,
		config.DestinationDirectory,
	)

	worker.RunScaleUp(
		config.KubernetesClient,
		config.Namespace,
		config.StatefulSetName,
		config.ScaleUpTimeout,
	)
}

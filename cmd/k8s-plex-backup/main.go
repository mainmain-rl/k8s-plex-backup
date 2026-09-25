package main

import (
	"fmt"
	"k8s-plex-backup/internal/config"
	"k8s-plex-backup/internal/worker"
	"log"
	"os"
)

func main() {
	if err := run(); err != nil {
		log.Printf("CronJob has failed: %s", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error when loading configuration: %s", err)
	}
	log.Printf(
		"Starting k8s-plex-backup",
	)
	defer func() {
		if err := worker.RunScaleUp(
			config.KubernetesClient,
			config.Namespace,
			config.StatefulSetName,
			config.ScaleUpTimeout,
		); err != nil {
			log.Printf("Error during scale-up : %s", err)
		}
	}()
	if err := worker.RunScaleDown(
		config.KubernetesClient,
		config.Namespace,
		config.StatefulSetName,
		config.ScaleDownTimeout,
	); err != nil {
		return fmt.Errorf("Error during scale-down : %w", err)
	}

	if err := worker.RunBackup(
		config.Namespace,
		config.StatefulSetName,
		config.SourceDirectory,
		config.DestinationDirectory,
	); err != nil {
		return fmt.Errorf("Error during backup : %w", err)
	}

	return nil
}

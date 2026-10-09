package main

import (
	"fmt"
	"log"
	"os"

	"k8s-plex-backup/internal/config"
	"k8s-plex-backup/internal/k8s"
	"k8s-plex-backup/internal/worker"
)

func main() {
	if err := run(); err != nil {
		log.Printf("CronJob has failed: %s", err)
		os.Exit(1)
	}

	log.Printf(
		"k8s-plex-backup job is done",
	)
}

func run() error {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error when loading configuration: %s", err)
	}
	log.Printf(
		"Starting k8s-plex-backup",
	)

	log.Printf(
		"Configuration:\nNamespace=%s\nStatefulSetName=%s\nSourceDirectory=%s\nDestinationDirectory=%s\nScaleDownTimeout=%s\nScaleUpTimeout=%s\nRetentionDays=%d\nFluxCDOption=%t\nArgoCDOption=%t",
		config.Namespace,
		config.StatefulSetName,
		config.SourceDirectory,
		config.DestinationDirectory,
		config.ScaleDownTimeout,
		config.ScaleUpTimeout,
		config.RetentionDays,
		config.FluxCDOption,
		config.ArgoCDOption,
	)

	defer func() {
		if err := worker.RunScaleUp(
			config.KubernetesClient,
			config.Namespace,
			config.StatefulSetName,
			config.ScaleUpTimeout,
			k8s.GitOpsOptions{FluxCD: config.FluxCDOption, ArgoCD: config.ArgoCDOption},
		); err != nil {
			log.Printf("Error during scale-up : %s", err)
		}
	}()

	if err := worker.RunScaleDown(
		config.KubernetesClient,
		config.Namespace,
		config.StatefulSetName,
		config.ScaleDownTimeout,
		k8s.GitOpsOptions{FluxCD: config.FluxCDOption, ArgoCD: config.ArgoCDOption},
	); err != nil {
		return fmt.Errorf("Error during scale-down : %w", err) //nolint:all
	}

	if err := worker.RunBackup(
		config.Namespace,
		config.StatefulSetName,
		config.SourceDirectory,
		config.DestinationDirectory,
		config.PlexExcludedDirs,
	); err != nil {
		return fmt.Errorf("Error during backup : %w", err) //nolint:all
	}

	if err := worker.CleanupBackupsByAge(config.DestinationDirectory, config.RetentionDays); err != nil {
		return fmt.Errorf("Error during backup cleanup : %w", err) //nolint:all
	}

	return nil
}

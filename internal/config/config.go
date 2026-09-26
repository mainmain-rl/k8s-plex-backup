package config

import (
	"fmt"
	"k8s-plex-backup/internal/k8s"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
)

// Config holds the configuration for the backup process.
type Config struct {
	Namespace            string
	StatefulSetName      string
	SourceDirectory      string
	DestinationDirectory string
	ScaleDownTimeout     time.Duration
	ScaleUpTimeout       time.Duration
	KubernetesClient     kubernetes.Interface
	RETENTION_DAYS       int
}

// LoadConfig loads the configuration from environment variables and returns a Config struct.
func LoadConfig() (Config, error) {
	cfg := Config{}
	var err error

	cfg.ScaleDownTimeout, err = getDurationOrDefault("SCALE_DOWN_TIMEOUT", 5*time.Minute)
	if err != nil {
		return cfg, err
	}

	cfg.ScaleUpTimeout, err = getDurationOrDefault("SCALE_UP_TIMEOUT", 10*time.Minute)
	if err != nil {
		return cfg, err
	}

	cfg.RETENTION_DAYS, err = getIntOrDefault("RETENTION_DAYS", 14)
	if err != nil {
		return cfg, err
	}

	cfg.KubernetesClient, err = k8s.BuildClientset()
	if err != nil {
		return cfg, err
	}

	if cfg.SourceDirectory, err = requireEnv("SOURCE_DIRECTORY"); err != nil {
		return cfg, err
	}

	if cfg.DestinationDirectory, err = requireEnv("DESTINATION_DIRECTORY"); err != nil {
		return cfg, err
	}

	if cfg.Namespace, err = requireEnv("PLEX_NAMESPACE"); err != nil {
		return cfg, err
	}

	if cfg.StatefulSetName, err = requireEnv("PLEX_STATEFULSET_NAME"); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// requireEnv retrieves the value of an environment variable and returns an error if it is not set.
// key: The name of the environment variable to retrieve.
// Returns the value of the environment variable or an error if it is not set.
func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return v, nil
}

// getDurationOrDefault retrieves a duration from an environment variable, or returns a default value if the variable is not set.
// It returns an error if the value is set but cannot be parsed as a duration.
// key: The name of the environment variable to retrieve.
// def: The default duration to return if the environment variable is not set.
// Returns the parsed duration or the default value, and an error if parsing fails.
func getDurationOrDefault(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}
	return d, nil
}

// getIntOrDefault retrieves an integer from an environment variable, or returns a default value if the variable is not set.
// It returns an error if the value is set but cannot be parsed as an integer.
// key: The name of the environment variable to retrieve.
// def: The default integer to return if the environment variable is not set.
// Returns the parsed integer or the default value, and an error if parsing fails.
func getIntOrDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	var i int
	_, err := fmt.Sscanf(v, "%d", &i)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}
	return i, nil
}

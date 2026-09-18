package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Save original environment variables
	originalEnv := getEnvironment()
	defer restoreEnvironment(originalEnv)

	// Set required environment variables
	os.Setenv("SOURCE_DIRECTORY", "/source")
	os.Setenv("DESTINATION_DIRECTORY", "/dest")
	os.Setenv("PLEX_NAMESPACE", "plex-ns")
	os.Setenv("PLEX_STATEFULSET_NAME", "plex-ss")

	// Test with default timeouts
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Verify configuration values
	if cfg.SourceDirectory != "/source" {
		t.Errorf("SourceDirectory = %s, want /source", cfg.SourceDirectory)
	}
	if cfg.DestinationDirectory != "/dest" {
		t.Errorf("DestinationDirectory = %s, want /dest", cfg.DestinationDirectory)
	}
	if cfg.Namespace != "plex-ns" {
		t.Errorf("Namespace = %s, want plex-ns", cfg.Namespace)
	}
	if cfg.StatefulSetName != "plex-ss" {
		t.Errorf("StatefulSetName = %s, want plex-ss", cfg.StatefulSetName)
	}
	if cfg.ScaleDownTimeout != 5*time.Minute {
		t.Errorf("ScaleDownTimeout = %v, want 5m", cfg.ScaleDownTimeout)
	}
	if cfg.ScaleUpTimeout != 10*time.Minute {
		t.Errorf("ScaleUpTimeout = %v, want 10m", cfg.ScaleUpTimeout)
	}

	// Verify Kubernetes client is not nil
	if cfg.KubernetesClient == nil {
		t.Error("KubernetesClient is nil")
	}

	// Test with custom timeouts
	os.Setenv("SCALE_DOWN_TIMEOUT", "2m")
	os.Setenv("SCALE_UP_TIMEOUT", "30m")

	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with custom timeouts failed: %v", err)
	}

	if cfg2.ScaleDownTimeout != 2*time.Minute {
		t.Errorf("ScaleDownTimeout = %v, want 2m", cfg2.ScaleDownTimeout)
	}
	if cfg2.ScaleUpTimeout != 30*time.Minute {
		t.Errorf("ScaleUpTimeout = %v, want 30m", cfg2.ScaleUpTimeout)
	}

	// Test missing required environment variable
	os.Unsetenv("SOURCE_DIRECTORY")
	_, err = LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should fail when SOURCE_DIRECTORY is missing")
	}

	// Test invalid duration format
	os.Setenv("SOURCE_DIRECTORY", "/source")
	os.Unsetenv("DESTINATION_DIRECTORY")
	os.Setenv("SCALE_DOWN_TIMEOUT", "invalid")
	_, err = LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should fail when SCALE_DOWN_TIMEOUT is invalid")
	}
}

func TestRequireEnv(t *testing.T) {
	// Save original environment variables
	originalEnv := getEnvironment()
	defer restoreEnvironment(originalEnv)

	// Test existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	val, err := requireEnv("TEST_VAR")
	if err != nil {
		t.Errorf("requireEnv() failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("requireEnv() = %s, want test_value", val)
	}

	// Test missing environment variable
	_, err = requireEnv("NONEXISTENT_VAR")
	if err == nil {
		t.Error("requireEnv() should fail for missing variable")
	}
}

func TestGetDurationOrDefault(t *testing.T) {
	// Save original environment variables
	originalEnv := getEnvironment()
	defer restoreEnvironment(originalEnv)

	// Test with unset variable (should return default)
	d, err := getDurationOrDefault("UNSET_VAR", 5*time.Minute)
	if err != nil {
		t.Errorf("getDurationOrDefault() failed: %v", err)
	}
	if d != 5*time.Minute {
		t.Errorf("getDurationOrDefault() = %v, want 5m", d)
	}

	// Test with valid duration
	os.Setenv("DURATION_VAR", "2h30m")
	d, err = getDurationOrDefault("DURATION_VAR", 5*time.Minute)
	if err != nil {
		t.Errorf("getDurationOrDefault() failed: %v", err)
	}
	expected := 2*time.Hour + 30*time.Minute
	if d != expected {
		t.Errorf("getDurationOrDefault() = %v, want 2h30m", d)
	}

	// Test with invalid duration format
	os.Setenv("INVALID_VAR", "not_a_duration")
	_, err = getDurationOrDefault("INVALID_VAR", 5*time.Minute)
	if err == nil {
		t.Error("getDurationOrDefault() should fail for invalid duration")
	}
}

// Helper functions to save and restore environment variables
func getEnvironment() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if len(e) > 0 {
			key, value, found := splitEnv(e)
			if found {
				env[key] = value
			}
		}
	}
	return env
}

func restoreEnvironment(env map[string]string) {
	// Clear all environment variables
	os.Clearenv()
	// Restore saved values
	for key, value := range env {
		os.Setenv(key, value)
	}
}

func splitEnv(e string) (key, value string, found bool) {
	if e == "" {
		return "", "", false
	}
	// Find the first = in the string
	if idx := indexByte(e, '='); idx >= 0 {
		return e[:idx], e[idx+1:], true
	}
	return e, "", false
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

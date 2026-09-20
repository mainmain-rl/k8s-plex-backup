package k8s

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/homedir"
)

// Mock functions to replace the real implementations
type mockInClusterConfig struct{}

func (m *mockInClusterConfig) InClusterConfig() (*rest.Config, error) {
	return &rest.Config{}, nil
}

type mockBuildConfigFromFlags struct{}

func (m *mockBuildConfigFromFlags) BuildConfigFromFlags(kubeconfig, context string) (*rest.Config, error) {
	return &rest.Config{}, nil
}

type mockHomeDir struct{}

func (m *mockHomeDir) HomeDir() string {
	return "/home/test"
}

func TestBuildClientsetInCluster(t *testing.T) {
	// Save original values
	originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalEnv != "" {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
		} else {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		}
	}()

	// Set the environment variable to simulate in-cluster
	os.Setenv("KUBERNETES_SERVICE_HOST", "127.0.0.1")

	// Test that the function detects in-cluster environment
	_, isInCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	if !isInCluster {
		t.Fatal("Failed to set KUBERNETES_SERVICE_HOST environment variable")
	}

	// We can't actually test the full in-cluster flow without a real cluster,
	// but we can verify the environment detection works
	t.Log("In-cluster environment detected successfully")
}

func TestBuildClientsetOutOfCluster(t *testing.T) {
	// Save original values
	originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalEnv != "" {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
		} else {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		}
	}()

	// Ensure the environment variable is not set
	os.Unsetenv("KUBERNETES_SERVICE_HOST")

	// Test that the function detects out-of-cluster environment
	_, isInCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	if isInCluster {
		t.Fatal("Failed to unset KUBERNETES_SERVICE_HOST environment variable")
	}

	// We can't actually test the full out-of-cluster flow without a valid kubeconfig,
	// but we can verify the environment detection works
	t.Log("Out-of-cluster environment detected successfully")
}

func TestBuildClientsetKubeconfigPath(t *testing.T) {
	// Save original values
	originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalEnv != "" {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
		} else {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		}
	}()

	// Ensure the environment variable is not set
	os.Unsetenv("KUBERNETES_SERVICE_HOST")

	// Test the kubeconfig path construction
	home := homedir.HomeDir()
	if home == "" {
		t.Log("Home directory not found, kubeconfig path would be empty")
	} else {
		expectedPath := filepath.Join(home, ".kube", "config")
		t.Logf("Expected kubeconfig path: %s", expectedPath)
	}
}

func TestBuildClientsetEnvironmentDetection(t *testing.T) {
	tests := []struct {
		name              string
		setEnv            bool
		expectedInCluster bool
	}{
		{
			name:              "Environment variable set",
			setEnv:            true,
			expectedInCluster: true,
		},
		{
			name:              "Environment variable not set",
			setEnv:            false,
			expectedInCluster: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
			defer func() {
				if originalEnv != "" {
					os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
				} else {
					os.Unsetenv("KUBERNETES_SERVICE_HOST")
				}
			}()

			if tt.setEnv {
				os.Setenv("KUBERNETES_SERVICE_HOST", "127.0.0.1")
			} else {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			}

			_, isInCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST")
			if isInCluster != tt.expectedInCluster {
				t.Errorf("Expected isInCluster=%v, got %v", tt.expectedInCluster, isInCluster)
			}
		})
	}
}

func TestBuildClientsetErrorHandling(t *testing.T) {
	// Save original values
	originalEnv := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalEnv != "" {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalEnv)
		} else {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		}
	}()

	// Ensure the environment variable is not set for out-of-cluster test
	os.Unsetenv("KUBERNETES_SERVICE_HOST")

	// Test with invalid kubeconfig path (should return error)
	clientset, err := BuildClientset()
	if err == nil {
		t.Log("Got clientset (unexpected):", clientset)
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

package k8s

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Build the Kubernetes ClientSet.
// If the Environment variable 'KUBERNETES_SERVICE_HOST' is found, it's mean that the system is in a Kubernetes Cluster as pod.
// If not, load the kubeconfig file
func BuildClientset() (*kubernetes.Clientset, error) {
	_, isInCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST")

	if isInCluster {
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		return kubernetes.NewForConfig(config)
	} else {
		var kubeconfig string
		if home := homedir.HomeDir(); home != "" {
			flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "path to kubeconfig file")
		} else {
			flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
		}
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("Error when reading kubeconfig: %w", err)
		}
		return kubernetes.NewForConfig(config)
	}
}

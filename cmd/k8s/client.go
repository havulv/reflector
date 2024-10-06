package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateK8sClient creates a kubernetes client based on a config passed to it.
// If a kube config is not passed to the function, we assume we are inside a cluster
// and try to construct an in cluster configuration
func CreateK8sClient(
	kubeconfig *string,
) (kubernetes.Interface, error) {
	var err error
	var config *rest.Config
	if kubeconfig == nil || *kubeconfig == "" {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to get cluster config: %w", err)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("unable to create config from kubeconfig: %w", err)
		}
	}
	// creates the clientset -- we panic here, because the configuration is loaded
	// and vetted far before it gets to the registration steps and further validation.
	// A panic is a much better signal to the user than an error here, because something
	// is really wrong.
	return kubernetes.NewForConfigOrDie(config), nil
}

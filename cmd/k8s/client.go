package k8s

import (
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateK8sClient creates a kubernetes client based on a config passed to it.
// If a kube config is not passed to the function, we assume we are inside a cluster
// and try to construct an in cluster configuration
// TODO: we can test this, but it is really troublesome because it takes a lot
// of closures and mocking to do for little gain. The TODO is to actually test it though
func CreateK8sClient(
	kubeconfig *string,
) (kubernetes.Interface, error) {
	var err error
	var config *rest.Config
	if kubeconfig == nil || *kubeconfig == "" {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "unable to get cluster config")
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create config from kubeconfig")
		}
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create clientset with in cluster config")
	}
	return clientset, nil
}

package annotations

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	// Prefix is the prefix for annotations
	Prefix = "reflector.havulv.io"
	// ReflectAnnotation is the annotation used to determine if
	// the secret should be reflected or not
	ReflectAnnotation = Prefix + "/reflect"

	// NamespaceAnnotation is the annotation which determines which
	// namespaces to reflect to
	NamespaceAnnotation = Prefix + "/namespaces"

	// ReflectedFromAnnotation indicates what the originating namespace of the secret was
	ReflectedFromAnnotation = Prefix + "/reflected-from"
	// ReflectedAtAnnotation indicates when the annotation was originally reflected
	ReflectedAtAnnotation = Prefix + "/reflected-at"
	// ReflectionHashAnnotation is a hash of the reflected secret for quick comparison
	ReflectionHashAnnotation = Prefix + "/hash"
)

var (
	// ErrorNoNamespace is used when no namespaces are supplied
	ErrorNoNamespace = errors.New("no namespace given")
)

// GetAnnotations fetches the relevant annotations from a secret
func GetAnnotations(secret *v1.Secret) map[string]string {
	return map[string]string{}
}

// ParseOrFetchNamespaces parses the namespaces of a secret from the specified
// annotation and retrives either all namespaces (if `*` is in the
// field of the annotation) or the specified namespaces. An empty annotation yields no namespaces.
func ParseOrFetchNamespaces(
	ctx context.Context,
	client corev1.CoreV1Interface,
	objAnnotations map[string]string,
) ([]string, error) {
	// parse the annotations
	namespaces, err := parseNamespaces(
		objAnnotations[NamespaceAnnotation])
	if errors.Is(err, ErrorNoNamespace) {
		return []string{}, nil
	} else if len(namespaces) == 0 {
		found, err := client.Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return []string{}, errors.Wrap(err, "unable to list namespaces")
		}
		for _, namespace := range found.Items {
			namespaces = append(namespaces, namespace.Name)
		}
	}
	return namespaces, nil
}

// parseNamespaces fetches the list of namespaces from the correct annotation
func parseNamespaces(str string) ([]string, error) {
	if str == "" {
		return []string{}, ErrorNoNamespace
	}

	if str == "*" {
		return []string{}, nil
	}

	return strings.Split(str, ","), nil
}

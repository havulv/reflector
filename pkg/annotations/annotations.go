package annotations

import (
	"errors"
	"strings"

	v1 "k8s.io/api/core/v1"
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

// ParseNamespaces fetches the list of namespaces from the correct annotation
func ParseNamespaces(str string) ([]string, error) {
	if str == "" {
		return []string{}, ErrorNoNamespace
	}

	if str == "*" {
		return []string{}, nil
	}

	return strings.Split(str, ","), nil
}

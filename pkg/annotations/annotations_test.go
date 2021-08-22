package annotations

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestParseOrFetchNamespaces(t *testing.T) {
	tests := []struct {
		descrip     string
		annotations map[string]string
		namespaces  []string
		fetched     []string
		err         error
		listErr     error
	}{
		{
			"fails to fetch namespaces",
			map[string]string{
				NamespaceAnnotation: "",
			},
			[]string{},
			[]string{},
			nil,
			nil,
		},
		{
			"fetches namespaces from annotations",
			map[string]string{
				NamespaceAnnotation: "default,monitoring,logging",
			},
			[]string{"default", "monitoring", "logging"},
			[]string{},
			nil,
			nil,
		},
		{
			"fetches all namespaces",
			map[string]string{
				NamespaceAnnotation: "*",
			},
			[]string{"default", "kube-system"},
			[]string{"default", "kube-system"},
			nil,
			nil,
		},
		{
			"fails to fetch all namespaces",
			map[string]string{
				NamespaceAnnotation: "*",
			},
			[]string{},
			[]string{},
			nil,
			errors.New("some err"),
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			client := fake.NewSimpleClientset()
			if len(test.fetched) > 0 {
				nsList := &v1.NamespaceList{Items: []v1.Namespace{}}

				for _, i := range test.fetched {
					ns := v1.Namespace{}
					ns.Name = i
					nsList.Items = append(nsList.Items, ns)
				}
				client = fake.NewSimpleClientset(nsList)
			}

			if test.listErr != nil {
				client.PrependReactor("*", "*",
					func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, test.listErr
					})
			}

			fakeCore := client.CoreV1()
			namespaces, err := ParseOrFetchNamespaces(
				context.Background(),
				fakeCore,
				test.annotations)
			assert.Equal(t, test.namespaces, namespaces)
			if test.listErr != nil {
				assert.NotNil(t, err)
				return
			}
			require.Equal(t, test.err, err)
		})
	}
}

func TestParseNamespaces(t *testing.T) {
	tests := []struct {
		d   string
		in  string
		out []string
		err error
	}{
		{
			"tests empty annotation is error",
			"",
			[]string{},
			ErrorNoNamespace,
		},
		{
			"tests `*` returns empty with no error",
			"*",
			[]string{},
			nil,
		},
		{
			"tests annotations are split",
			"logging,monitoring,other",
			[]string{"logging", "monitoring", "other"},
			nil,
		},
		{
			"tests that spaces are trimmed",
			"logging, monitoring, otherss ,default",
			[]string{"logging", "monitoring", "otherss", "default"},
			nil,
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			s, err := parseNamespaces(test.in)
			assert.Equal(t, s, test.out)
			assert.Equal(t, err, test.err)
		})
	}
}

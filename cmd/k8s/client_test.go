package k8s

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateK8sClient(t *testing.T) {
	t.Parallel()
	t.Run("creates a clientset from a kubeconfig", func(t *testing.T) {
		t.Parallel()
		f, err := os.CreateTemp(t.TempDir(), ".*kubeconfig")
		require.NoError(t, err)

		_, err = f.Write([]byte(`
apiVersion: v1
kind: Config
preferences: {}
current-context: k8s.example.com
clusters:
- cluster:
    server: https://api.example.com
  name: k8s.example.com
contexts:
- context:
    cluster: k8s.example.com
    user: example
  name: k8s.example.com
users:
- name: example
  user:
    password: foobar
    username: admin
`))
		require.Nil(t, err)
		require.Nil(t, f.Close())

		kubeconfig := f.Name()
		client, err := CreateK8sClient(&kubeconfig)

		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("fails to create an in cluster config", func(t *testing.T) {
		t.Parallel()
		client, err := CreateK8sClient(nil)
		assert.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("fails to create an out of cluster config", func(t *testing.T) {
		t.Parallel()
		f, err := os.CreateTemp(t.TempDir(), ".*kubeconfig")
		require.NoError(t, err)
		_, err = f.Write([]byte("apiVersion: v1"))
		require.Nil(t, err)
		require.Nil(t, f.Close())

		kubeconfig := f.Name()
		client, err := CreateK8sClient(&kubeconfig)

		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

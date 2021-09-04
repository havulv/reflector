package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/havulv/reflector/pkg/mocks"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		descrip string
		obj     *v1.Secret
		rl      *mocks.RateLimiter
	}{
		{
			"adds the key if no error",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "this",
				},
			},
			&mocks.RateLimiter{},
		},
		{
			"does not add the key if error",
			&v1.Secret{},
			&mocks.RateLimiter{},
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			f := add(test.rl)
			if test.obj != nil {
				test.rl.On("AddRateLimited", test.obj.Name)
			}
			f(test.obj)
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		descrip string
		obj     *v1.Secret
		rl      *mocks.RateLimiter
	}{
		{
			"updates the key if no error",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "this",
				},
			},
			&mocks.RateLimiter{},
		},
		{
			"does not update the key if error",
			&v1.Secret{},
			&mocks.RateLimiter{},
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			f := update(test.rl)
			if test.obj != nil {
				test.rl.On("AddRateLimited", test.obj.Name)
			}
			f(nil, test.obj)
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		descrip string
		obj     *v1.Secret
		rl      *mocks.RateLimiter
	}{
		{
			"removes the key if no error",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "this",
				},
			},
			&mocks.RateLimiter{},
		},
		{
			"does not remove the key if error",
			&v1.Secret{},
			&mocks.RateLimiter{},
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			f := remove(test.rl)
			if test.obj != nil {
				test.rl.On("AddRateLimited", test.obj.Name)
			}
			f(test.obj)
		})
	}
}

func TestCreateSecretsWorkQueue(t *testing.T) {
	t.Parallel()
	// this doesn't work because I don't understand how to add
	// to the informer queue
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()
	client := fake.NewSimpleClientset()
	watcherStarted := make(chan struct{})
	client.PrependWatchReactor("*", func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
		t.Log("starting watch")
		gvr := action.GetResource()
		ns := action.GetNamespace()
		watch, err := client.Tracker().Watch(gvr, ns)
		if err != nil {
			return false, nil, err
		}
		t.Log("closing blocking channel")
		close(watcherStarted)
		return true, watch, nil
	})
	queue, indexer, informer := CreateSecretsWorkQueue(client.CoreV1(), "kube-system")
	require.NotNil(t, queue)
	require.NotNil(t, indexer)
	require.NotNil(t, informer)
}

func TestParseWorkQueueKey(t *testing.T) {
	tests := []struct {
		descrip   string
		in        string
		name      string
		namespace string
	}{
		{
			"parses key without slash",
			"secret",
			"secret",
			"",
		},
		{
			"parses key with slash",
			"ns/secret",
			"secret",
			"ns",
		},
		{
			"parses key with multiple slashes",
			"ns/secret/with/slash/in/it",
			"secret/with/slash/in/it",
			"ns",
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			ns, name := ParseWorkQueueKey(test.in)
			assert.Equal(t, name, test.name)
			assert.Equal(t, ns, test.namespace)
		})
	}
}

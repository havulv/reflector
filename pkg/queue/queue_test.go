package queue

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestCreateSecretsWorkQueue(t *testing.T) {
	// this doesn't work because I don't understand how to add
	// to the informer queue
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	t.Log("starting informer")
	go informer.Run(ctx.Done())
	t.Log("waiting for cache sync")
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)

	<-watcherStarted
	p := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret"}}
	_, err := client.CoreV1().Secrets("test-ns").Create(context.TODO(), p, metav1.CreateOptions{})
	require.Nil(t, err)

	found := make(chan interface{})
	go func() {
		sec, _ := queue.Get()
		found <- sec
	}()

	select {
	case sec := <-found:
		t.Logf("Got sec from channel: %v", sec)
	case <-time.After(wait.ForeverTestTimeout):
		t.Error("Informer did not get the added pod")
	}
}

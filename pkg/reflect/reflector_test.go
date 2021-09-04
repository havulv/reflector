package reflect

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/havulv/reflector/pkg/annotations"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"
	"k8s.io/client-go/util/workqueue"
)

func TestNewReflector(t *testing.T) {
	tests := []struct {
		descrip string
		rCon    int
		wCon    int
	}{
		{
			"creates a new reflector",
			5,
			5,
		},
		{
			"sets the reflect concurrency to at least 1",
			0,
			3,
		},
		{
			"sets the worker concurrency to at least 1",
			3,
			0,
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			r, err := NewReflector(
				zerolog.New(bytes.NewBuffer([]byte{})),
				fake.NewSimpleClientset(),
				test.rCon,
				test.wCon,
				12,
				false,
				"namespace",
			)
			assert.Nil(t, err)
			if test.rCon < 1 {
				assert.Equal(t, r.(*reflector).reflectConcurrency, 1)
				return
			}

			if test.wCon < 1 {
				assert.Equal(t, r.(*reflector).workerConcurrency, 1)
				return
			}
		})
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		descrip  string
		shutdown bool
	}{
		{
			"queue shut down shuts down",
			true,
		},
		{
			"key is processed",
			false,
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			limiter := workqueue.NewItemExponentialFailureRateLimiter(
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewRateLimitingQueue(limiter)
			source := fcache.NewFakeControllerSource()
			indexer, informer := cache.NewIndexerInformer(
				source, &v1.Secret{}, 0,
				cache.ResourceEventHandlerFuncs{
					AddFunc:    func(obj interface{}) {},
					UpdateFunc: func(old interface{}, new interface{}) {},
					DeleteFunc: func(obj interface{}) {},
				}, cache.Indexers{})

			r := reflector{
				ctx:        context.Background(),
				queue:      queue,
				indexer:    indexer,
				controller: informer,
			}

			if test.shutdown {
				r.queue.ShutDown()
			} else {
				defer r.queue.ShutDown()
			}

			require.Nil(t, indexer.Add(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "thing",
				},
			}))

			r.queue.AddRateLimited(interface{}("thing"))

			if test.shutdown {
				assert.False(t, r.next())
				return
			}
			assert.True(t, r.next())
		})
	}

}

func TestProcess(t *testing.T) {
	tests := []struct {
		descrip       string
		item          string
		secret        *v1.Secret
		cascadeDelete bool
		err           error
		nsErr         error
		rm            bool
		checkLogs     string
	}{
		{
			"does not update if deleted and no cascade delete",
			"thing/secret",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "thing",
				},
			},
			false,
			nil,
			nil,
			true,
			"cascadeDelete",
		},
		{
			"deletes if deleted and cascade delete",
			"thing/secret",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "thing",
				},
			},
			true,
			nil,
			nil,
			true,
			"",
		},
		{
			"fails to cascade delete due to namespace error",
			"thing/secret",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "thing",
				},
			},
			true,
			errors.New("some error"),
			errors.New("some error"),
			true,
			"",
		},
		{
			"reflects a secret with the right namespaces",
			"thing/secret",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "thing",
					Annotations: map[string]string{
						annotations.ReflectAnnotation:   "true",
						annotations.NamespaceAnnotation: "ns1,ns2",
					},
				},
			},
			false,
			nil,
			nil,
			false,
			"",
		},
		{
			"fails to reflect a secret with a bad annotation",
			"thing/secret",
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "thing",
					Annotations: map[string]string{
						annotations.ReflectAnnotation:   "true",
						annotations.NamespaceAnnotation: "",
					},
				},
			},
			false,
			errors.New("some error"),
			errors.New("some error"),
			false,
			"",
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			limiter := workqueue.NewItemExponentialFailureRateLimiter(
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewRateLimitingQueue(limiter)
			source := fcache.NewFakeControllerSource()
			indexer, informer := cache.NewIndexerInformer(
				source, &v1.Secret{}, 0,
				cache.ResourceEventHandlerFuncs{
					AddFunc: func(obj interface{}) {
						key, err := cache.MetaNamespaceKeyFunc(obj)
						if err == nil {
							queue.AddRateLimited(key)
						}
					},
					UpdateFunc: func(old interface{}, new interface{}) {
						key, err := cache.MetaNamespaceKeyFunc(new)
						if err == nil {
							queue.AddRateLimited(key)
						}
					},
					DeleteFunc: func(obj interface{}) {
						t.Logf("deletion for queue %v", obj)
						key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
						if err == nil {
							t.Logf("adding to queue %s", key)
							queue.AddRateLimited(key)
						}
					},
				}, cache.Indexers{})

			client := fake.NewSimpleClientset()
			if test.nsErr != nil {
				client.PrependReactor("*", "*",
					func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, errors.New("some error")
					})
			}

			buf := bytes.NewBuffer([]byte{})
			r := reflector{
				ctx:           context.Background(),
				logger:        zerolog.New(buf),
				core:          client.CoreV1(),
				queue:         queue,
				indexer:       indexer,
				controller:    informer,
				cascadeDelete: test.cascadeDelete,
			}

			if test.rm {
				require.Nil(t, indexer.Delete(test.secret))
			} else {
				require.Nil(t, indexer.Add(test.secret))
				t.Log(indexer)
			}

			if test.err != nil {
				assert.NotNil(t, r.process(test.item))
			} else {
				assert.Nil(t, r.process(test.item))
			}

			if test.checkLogs != "" {
				assert.Contains(t, buf.String(), test.checkLogs)
				return
			}
			t.Log(buf.String())
		})
	}

}

func TestHandleErr(t *testing.T) {
	tests := []struct {
		descrip string
		retries int
		err     error
	}{
		{
			"nil error forgets the key",
			3,
			nil,
		},
		{
			"error less than retries requeues key",
			3,
			errors.New("something"),
		},
		{
			"errors over retries drops from the queue",
			-1,
			errors.New("something"),
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			buf := bytes.NewBuffer([]byte{})
			limiter := workqueue.NewItemExponentialFailureRateLimiter(
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewRateLimitingQueue(limiter)
			r := &reflector{
				logger:  zerolog.New(buf),
				queue:   queue,
				retries: test.retries,
			}

			if test.retries > 0 && test.err != nil {
				limiter.When(interface{}("thing"))
			}

			r.handleErr(test.err, interface{}("thing"))

			if test.retries < 0 && test.err != nil {
				assert.Contains(t, buf.String(), "Dropping")
				return
			}

			if test.err != nil {
				assert.Contains(t, buf.String(), "requeueing")
				return
			}
		})
	}

}

func TestStart(t *testing.T) {
	tests := []struct {
		descrip string
		timeout bool
		cancel  bool
		failNow bool
	}{
		{
			"runs until context cancellation",
			false,
			true,
			false,
		},
		{
			"returns an error that is not cancellation",
			true,
			false,
			false,
		},
		{
			"fails to sync the cache",
			false,
			false,
			true,
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			buf := bytes.NewBuffer([]byte{})
			limiter := workqueue.NewItemExponentialFailureRateLimiter(
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewRateLimitingQueue(limiter)
			source := fcache.NewFakeControllerSource()
			indexer, informer := cache.NewIndexerInformer(
				source, &v1.Pod{}, 0,
				cache.ResourceEventHandlerFuncs{
					AddFunc:    func(obj interface{}) {},
					UpdateFunc: func(old interface{}, new interface{}) {},
					DeleteFunc: func(obj interface{}) {},
				}, cache.Indexers{})

			r := &reflector{
				logger:            zerolog.New(buf),
				queue:             queue,
				indexer:           indexer,
				controller:        informer,
				hasSynced:         func() bool { return true },
				workerConcurrency: 1,
			}

			// on timeout or cancel, we want to bypass cache syncing
			if test.failNow {
				r.hasSynced = func() bool { return false }
			}

			ctx := context.Background()

			var cancel func()
			if test.timeout {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}

			if test.cancel {
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
			}

			if test.failNow {
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			errChan := make(chan error)
			go func() {
				errChan <- r.Start(ctx)
			}()

			if test.cancel {
				cancel()
			}

			var err error
			timer := time.NewTimer(1 * time.Second)
			select {
			case err = <-errChan:
			case <-timer.C:
				t.Log("timer timed out while waiting for error")
				t.Log(buf.String())
				t.FailNow()
			}
			if test.timeout || test.failNow {
				assert.NotNil(t, err)
				return
			}
			assert.Nil(t, err)
		})
	}

}

func TestWorker(t *testing.T) {
	tests := []struct {
		descrip string
	}{
		{
			"runs `next`",
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			t.Parallel()
			limiter := workqueue.NewItemExponentialFailureRateLimiter(
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewRateLimitingQueue(limiter)
			r := reflector{
				queue: queue,
			}
			go func() {
				r.queue.ShutDown()
			}()
			r.worker()
		})
	}
}

package reflect

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

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
				zerolog.New(zerolog.NewTestWriter(t)),
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
			limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewTypedRateLimitingQueue[string](limiter)
			source := fcache.NewFakeControllerSource()
			store, informer := cache.NewInformerWithOptions(
				cache.InformerOptions{
					ListerWatcher: source,
					ObjectType:    &v1.Secret{},
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc:    func(_ any) {},
						UpdateFunc: func(_ any, _ any) {},
						DeleteFunc: func(_ any) {},
					},
					Indexers: cache.Indexers{},
				})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			r := reflector{
				ctx:        ctx,
				queue:      queue,
				store:      store,
				controller: informer,
			}

			if test.shutdown {
				r.queue.ShutDown()
			} else {
				defer r.queue.ShutDown()
			}

			require.Nil(t, store.Add(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "thing",
				},
			}))

			r.queue.AddRateLimited("thing")

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
			limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewTypedRateLimitingQueue[string](limiter)
			source := fcache.NewFakeControllerSource()
			store, informer := cache.NewInformerWithOptions(
				cache.InformerOptions{
					ListerWatcher: source,
					ObjectType:    &v1.Secret{},
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc: func(obj any) {
							key, err := cache.MetaNamespaceKeyFunc(obj)
							if err == nil {
								queue.AddRateLimited(key)
							}
						},
						UpdateFunc: func(_ any, n any) {
							key, err := cache.MetaNamespaceKeyFunc(n)
							if err == nil {
								queue.AddRateLimited(key)
							}
						},
						DeleteFunc: func(obj any) {
							t.Logf("deletion for queue %v", obj)
							key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
							if err == nil {
								t.Logf("adding to queue %s", key)
								queue.AddRateLimited(key)
							}
						},
					},
					Indexers: cache.Indexers{},
				})

			client := fake.NewSimpleClientset()
			if test.nsErr != nil {
				client.PrependReactor("*", "*",
					func(_ clienttesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, errors.New("some error")
					})
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			buf := bytes.NewBuffer([]byte{})
			r := reflector{
				ctx:                ctx,
				logger:             zerolog.New(buf),
				core:               client.CoreV1(),
				queue:              queue,
				store:              store,
				controller:         informer,
				cascadeDelete:      test.cascadeDelete,
				reflectConcurrency: 1,
			}

			if test.rm {
				require.Nil(t, store.Delete(test.secret))
			} else {
				require.Nil(t, store.Add(test.secret))
				t.Log(store)
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
			limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewTypedRateLimitingQueue[string](limiter)
			r := &reflector{
				logger:  zerolog.New(buf),
				queue:   queue,
				retries: test.retries,
			}

			if test.retries > 0 && test.err != nil {
				limiter.When("thing")
			}

			r.handleErr(test.err, "thing")

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
			limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewTypedRateLimitingQueue[string](limiter)
			source := fcache.NewFakeControllerSource()
			store, informer := cache.NewInformerWithOptions(
				cache.InformerOptions{
					ListerWatcher: source,
					ObjectType:    &v1.Secret{},
					Handler: cache.ResourceEventHandlerFuncs{
						AddFunc:    func(_ any) {},
						UpdateFunc: func(_ any, _ any) {},
						DeleteFunc: func(_ any) {},
					},
					Indexers: cache.Indexers{},
				})

			r := &reflector{
				logger:            zerolog.New(zerolog.NewTestWriter(t)),
				queue:             queue,
				store:             store,
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
			limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](
				1*time.Millisecond, 1*time.Millisecond)
			queue := workqueue.NewTypedRateLimitingQueue[string](limiter)
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

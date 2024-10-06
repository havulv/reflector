package queue

import (
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type RateLimiter interface {
	AddRateLimited(string)
}

func add(
	queue RateLimiter,
) func(any) {
	return func(obj any) {
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err == nil {
			queue.AddRateLimited(key)
		}
	}
}

func update(
	queue RateLimiter,
) func(any, any) {
	return func(_ any, updated any) {
		key, err := cache.MetaNamespaceKeyFunc(updated)
		if err == nil {
			queue.AddRateLimited(key)
		}
	}
}

func remove(
	queue RateLimiter,
) func(any) {
	return func(obj any) {
		// IndexerInformer uses a delta queue, therefore for deletes we have to use this
		// key function.
		key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
		if err == nil {
			queue.AddRateLimited(key)
		}
	}
}

// CreateSecretsWorkQueue creates a secrets work queue for a given namespace
func CreateSecretsWorkQueue(
	core corev1.CoreV1Interface,
	namespace string,
) (workqueue.TypedRateLimitingInterface[string], cache.Store, cache.Controller) {
	// create the secret watcher
	// We must grab everything because we can't filter by labels or
	// annotations
	secretListWatcher := cache.NewListWatchFromClient(
		core.RESTClient(),
		"secrets",
		namespace,
		fields.Everything(),
	)

	// create the workqueue
	queue := workqueue.NewTypedRateLimitingQueue[string](workqueue.DefaultTypedControllerRateLimiter[string]())

	// Bind the workqueue to a cache with the help of an informer. This way we make sure that
	// whenever the cache is updated, the secret key is added to the workqueue.
	// Note that when we finally process the item from the workqueue, we might see a newer version
	// of the Pod than the version which was responsible for triggering the update.
	store, informer := cache.NewInformerWithOptions(
		cache.InformerOptions{
			ListerWatcher: secretListWatcher,
			ObjectType:    &v1.Secret{},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    add(queue),
				UpdateFunc: update(queue),
				DeleteFunc: remove(queue),
			},
			Indexers: cache.Indexers{},
		})
	return queue, store, informer
}

// ParseWorkQueueKey parses a key from the workqueue into its namespace
// and name.
func ParseWorkQueueKey(key string) (string, string) {
	if strings.Contains(key, "/") {
		keySplit := strings.Split(key, "/")
		return keySplit[0], strings.Join(keySplit[1:], "/")
	}
	return "", key
}

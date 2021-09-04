package queue

import (
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/havulv/reflector/pkg/annotations"
)

// RateLimiter is the minimal interface needed for a rate limiting
// queue.
type RateLimiter interface {
	AddRateLimited(interface{})
}

func add(
	queue RateLimiter,
) func(interface{}) {
	return func(obj interface{}) {
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err == nil {
			queue.AddRateLimited(key)
		}
	}
}

func update(
	queue RateLimiter,
) func(interface{}, interface{}) {
	return func(old interface{}, updated interface{}) {
		key, err := cache.MetaNamespaceKeyFunc(updated)
		if err == nil {
			queue.AddRateLimited(key)
		}
	}
}

func remove(
	queue RateLimiter,
) func(interface{}) {
	return func(obj interface{}) {
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
) (workqueue.RateLimitingInterface, cache.Indexer, cache.Controller) {
	// create the pod watcher
	secretListWatcher := cache.NewListWatchFromClient(
		core.RESTClient(),
		"secrets",
		namespace,
		fields.SelectorFromSet(fields.Set{annotations.ReflectAnnotation: "true"}),
	)

	// create the workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Bind the workqueue to a cache with the help of an informer. This way we make sure that
	// whenever the cache is updated, the secret key is added to the workqueue.
	// Note that when we finally process the item from the workqueue, we might see a newer version
	// of the Pod than the version which was responsible for triggering the update.
	indexer, informer := cache.NewIndexerInformer(secretListWatcher, &v1.Secret{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    add(queue),
		UpdateFunc: update(queue),
		DeleteFunc: remove(queue),
	}, cache.Indexers{})
	return queue, indexer, informer
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

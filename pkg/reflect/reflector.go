package reflect

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/havulv/reflector/pkg/annotations"
	"github.com/havulv/reflector/pkg/queue"
)

// Reflector is the core reflector interface which takes care of
// watching and syncing secrets
type Reflector interface {
	Start(ctx context.Context) error
}

type reflector struct {
	ctx                context.Context
	core               corev1.CoreV1Interface
	logger             zerolog.Logger
	workerConcurrency  int
	reflectConcurrency int
	queue              workqueue.RateLimitingInterface
	indexer            cache.Indexer
	controller         cache.Controller
}

// NewReflector creates a new reflector for reflecting secrets to other namespaces
func NewReflector(
	logger zerolog.Logger,
	reflectConcurrency int,
	workerConcurrency int,
	namespace string,
) (Reflector, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get cluster config")
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create clientset with in cluster config")
	}

	queue, indexer, controller := queue.CreateSecretsWorkQueue(
		clientset.CoreV1(), namespace)

	return &reflector{
		core:               clientset.CoreV1(),
		logger:             logger,
		queue:              queue,
		indexer:            indexer,
		controller:         controller,
		reflectConcurrency: reflectConcurrency,
		workerConcurrency:  workerConcurrency,
	}, nil
}

func (r *reflector) next() bool {
	// Wait until there is a new item in the working queue
	key, quit := r.queue.Get()
	if quit {
		return false
	}
	defer r.queue.Done(key)

	// Invoke the method containing the business logic
	err := r.process(key.(string))

	r.handleErr(err, key)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the pod to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (r *reflector) process(key string) error {
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	obj, exists, err := r.indexer.GetByKey(key)
	if err != nil {
		r.logger.Error().
			Str("key", key).
			Err(err).
			Msg("Fetching object from store failed")
		return err
	}
	sec := obj.(*v1.Secret)

	// TODO cascade delete
	if !exists {
		return nil
	}

	// fetch the secret object's annotations
	objAnnotations := annotations.GetAnnotations(sec)
	if objAnnotations[annotations.ReflectAnnotation] != "true" {
		return nil
	}

	namespaces, err := parseNamespaces(ctx, r.core, objAnnotations)
	if err != nil {
		return errors.Wrap(err, "unable to parse namespaces")
	}
	return reflectToNamespaces(
		ctx,
		r.logger,
		r.core,
		sec,
		namespaces,
		r.reflectConcurrency)
}

// handleErr checks if an error happened and makes sure we will retry later.
func (r *reflector) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		r.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if r.queue.NumRequeues(key) < 5 {

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		r.queue.AddRateLimited(key)
		return
	}

	r.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	r.logger.Error().
		Str("key", key.(string)).
		Err(err).
		Msg("Dropping secret out of the queue")
}

func (r *reflector) Start(ctx context.Context) error {
	// set the root context to this context, so all
	// work queue processing inherits it.
	r.ctx = ctx
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer r.queue.ShutDown()

	go r.controller.Run(ctx.Done())

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(ctx.Done(), r.controller.HasSynced) {
		err := fmt.Errorf("Timed out waiting for caches to sync")
		return err
	}

	// start workers which will continually check the work queue for
	// items to process
	// The duration given is the time that we will wait before re-running
	// the worker in the event that we panic.
	for i := 0; i < r.workerConcurrency; i++ {
		go wait.Until(r.worker, 1*time.Second, ctx.Done())
	}

	<-ctx.Done()

	if err := ctx.Err(); err != nil || !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (r *reflector) worker() {
	for r.next() {
	}
}

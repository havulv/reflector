package reflect

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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
	retries            int
	cascadeDelete      bool
	queue              workqueue.RateLimitingInterface
	indexer            cache.Indexer
	controller         cache.Controller
	hasSynced          func() bool
}

// NewReflector creates a new reflector for reflecting secrets to other namespaces
func NewReflector(
	logger zerolog.Logger,
	clientset kubernetes.Interface,
	reflectConcurrency int,
	workerConcurrency int,
	retries int,
	cascadeDelete bool,
	namespace string,
) (Reflector, error) {
	if reflectConcurrency < 1 {
		reflectConcurrency = 1
	}

	if workerConcurrency < 1 {
		workerConcurrency = 1
	}

	queue, indexer, controller := queue.CreateSecretsWorkQueue(
		clientset.CoreV1(), namespace)

	return &reflector{
		core:               clientset.CoreV1(),
		cascadeDelete:      cascadeDelete,
		logger:             logger,
		queue:              queue,
		retries:            retries,
		indexer:            indexer,
		controller:         controller,
		reflectConcurrency: reflectConcurrency,
		workerConcurrency:  workerConcurrency,
		hasSynced:          controller.HasSynced,
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

	// In the implementation of the cache, the returned error of GetByKey is always nil
	obj, exists, _ := r.indexer.GetByKey(key)

	var name, namespace string
	if strings.Contains(key, "/") {
		keySplit := strings.Split(key, "/")
		namespace = keySplit[0]
		name = strings.Join(keySplit[1:], "/")
	}

	ctxLogger := r.logger.With().
		Str("rootNamespace", namespace).
		Str("secret", name).Logger()

	// Secret was deleted so we have to reconstruct the object in case cascadeDelete is set.
	if !exists {
		if !r.cascadeDelete {
			ctxLogger.Info().Msg("secret deleted and `cascadeDelete` not set, not attempting to delete reflected secrets")
			return nil
		}
		namespaces, err := findExistingSecretNamespaces(ctx, r.core, name, namespace)
		if err != nil {
			return errors.Wrap(err, "unable to find namespaces secret existed in")
		}

		return cascadeDelete(
			ctx,
			ctxLogger,
			r.core,
			name,
			namespaces,
			r.reflectConcurrency)
	}

	sec, ok := obj.(*v1.Secret)
	if !ok {
		return errors.New("could not convert object to secret")
	}

	// fetch the secret object's annotations
	if shouldReflect, ok := sec.Annotations[annotations.ReflectAnnotation]; !ok || shouldReflect != "true" {
		return nil
	}

	namespaces, err := annotations.ParseOrFetchNamespaces(
		ctx, r.core, sec.Annotations)
	if err != nil {
		return errors.Wrap(err, "unable to parse namespaces")
	}

	return reflectToNamespaces(
		ctx,
		ctxLogger,
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

	// This controller retries r.retries times if something goes wrong. After that, it stops trying.
	requeues := r.queue.NumRequeues(key)
	if requeues < r.retries {
		r.logger.Error().
			Str("secret", key.(string)).
			Err(err).
			Msg("reflection failed; requeueing")

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		r.queue.AddRateLimited(key)
		return
	}

	r.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	r.logger.Error().
		Str("secret", key.(string)).
		Int("requeues", requeues).
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

	r.logger.Info().Msg("Spinning off controller")
	go r.controller.Run(ctx.Done())

	// Wait for all involved caches to be synced, before processing items from the queue is started
	r.logger.Info().Msg("Syncing cache before starting controller loop")
	if !cache.WaitForCacheSync(ctx.Done(), r.hasSynced) {
		return errors.New("Timed out waiting for caches to sync")
	}

	// start workers which will continually check the work queue for
	// items to process
	// The duration given is the time that we will wait before re-running
	// the worker in the event that we panic.
	for i := 0; i < r.workerConcurrency; i++ {
		go wait.Until(r.worker, 1*time.Second, ctx.Done())
	}

	<-ctx.Done()

	r.logger.Info().Msg("Shutting down")
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (r *reflector) worker() {
	for r.next() {
	}
}

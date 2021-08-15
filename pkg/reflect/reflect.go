package reflect

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/havulv/reflector/pkg/annotations"
	"github.com/havulv/reflector/pkg/queue"
)

// Reflector is the core reflector interface which takes care of
// watching and syncing secrets
type Reflector interface {
	Start(ctx context.Context) error
}

type reflector struct {
	rootCtx     context.Context
	core        corev1.CoreV1Interface
	logger      zerolog.Logger
	concurrency int
	queue       workqueue.RateLimitingInterface
	indexer     cache.Indexer
	controller  cache.Controller
}

// NewReflector creates a new reflector for reflecting secrets to other namespaces
func NewReflector(ctx context.Context, logger zerolog.Logger, namespace string) (Reflector, error) {
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
		rootCtx:    ctx,
		core:       clientset.CoreV1(),
		logger:     logger,
		queue:      queue,
		indexer:    indexer,
		controller: controller,
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
	err := r.sync(key.(string))

	// Handle the error if something went wrong during the execution of the business logic
	r.handleErr(err, key)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the pod to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (r *reflector) sync(key string) error {
	ctx, cancel := context.WithCancel(r.rootCtx)
	defer cancel()
	obj, exists, err := r.indexer.GetByKey(key)
	if err != nil {
		r.logger.Error().
			Str("key", key).
			Err(err).
			Msg("Fetching object from store failed")
		return err
	}

	// TODO cascade delete
	if !exists {
		return nil
	}

	// fetch the secret object's annotations
	objAnnotations := annotations.GetAnnotations(obj.(*v1.Secret))
	if objAnnotations[annotations.ReflectAnnotation] != "true" {
		return nil
	}

	namespaces, err := parseNamespaces(ctx, r.core, objAnnotations)
	if err != nil {
		return errors.Wrap(err, "unable to parse namespaces")
	}

	for _, namespace := range namespaces {
		if err := r.reflect(ctx, obj.(*v1.Secret), namespace); err != nil {
			return err
		}
	}
	return nil
}

func parseNamespaces(
	ctx context.Context,
	client corev1.CoreV1Interface,
	objAnnotations map[string]string,
) ([]string, error) {
	// parse the annotations
	namespaces, err := annotations.ParseNamespaces(
		objAnnotations[annotations.NamespaceAnnotation])
	if errors.Is(err, annotations.ErrorNoNamespace) {
		// TODO log something
		return []string{}, nil
	} else if len(namespaces) == 0 {
		// TODO fetch namespaces
		found, err := client.Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return []string{}, errors.Wrap(err, "unable to list namespaces")
		}
		for _, namespace := range found.Items {
			namespaces = append(namespaces, namespace.Name)
		}
	}
	return namespaces, nil
}

func (r *reflector) reflect(ctx context.Context, og *v1.Secret, namespace string) error {
	secretsClient := r.core.Secrets(namespace)
	// reflect to the new namespace

	// if it exists, then pull the resource and check if we own it
	reflected, err := secretsClient.Get(ctx, og.Name, metav1.GetOptions{})
	exists := !apierrors.IsNotFound(err)
	if err != nil && exists {
		return errors.Wrap(err, "error while getting reflected secret")
	}
	// hash the og -- TODO is crc64 good enough here?
	ogHash := fmt.Sprintf("%x", sha256.Sum256([]byte(og.String())))

	// if it does exist, check the hash to see if we need to update
	if exists {
		reflectedAnn := annotations.GetAnnotations(reflected)

		// if we can't find the hash annotation, then we don't own it
		reflectHash, ok := reflectedAnn[annotations.ReflectionHashAnnotation]
		if !ok {
			r.logger.Info().Msg("We don't own this secret: not updating")
			return nil
		}

		if reflectHash == ogHash {
			r.logger.Info().Str("hash", ogHash).Msg("No changes to secret, not updating")
			return nil
		}
	}

	// DeepCopy and fix the annotations
	toReflect := og.DeepCopy()

	// remove the reflection annotation so we don't get recursive reflection somewhere
	delete(toReflect.Annotations, annotations.ReflectAnnotation)
	toReflect.Annotations[annotations.ReflectedFromAnnotation] = og.Namespace
	toReflect.Annotations[annotations.ReflectedAtAnnotation] = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	toReflect.Annotations[annotations.ReflectionHashAnnotation] = ogHash

	// update if it does exist, create if it does not
	if exists {
		_, err := secretsClient.Update(ctx, toReflect, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "error while updating secret")
		}
	} else {
		_, err := secretsClient.Create(ctx, toReflect, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "error while creating secret")
		}
	}

	return nil
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

	stop := anyChan(ctx.Done(), r.rootCtx.Done())
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer r.queue.ShutDown()

	go r.controller.Run(stop)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stop, r.controller.HasSynced) {
		err := fmt.Errorf("Timed out waiting for caches to sync")
		return err
	}

	for i := 0; i < r.concurrency; i++ {
		go wait.Until(r.worker, time.Second, stop)
	}

	<-stop
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.rootCtx.Err()
}

func (r *reflector) worker() {
	for r.next() {
	}
}

func anyChan(c1, c2 <-chan struct{}) <-chan struct{} {
	c := make(chan struct{})
	go func() {
		select {
		case <-c1:
		case <-c2:
		}
		c <- struct{}{}
	}()
	return c
}

package reflect

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/havulv/reflector/pkg/annotations"
)

func reflectToNamespaces(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	sec *v1.Secret,
	namespaces []string,
	concurrency int,
) error {
	// shortcircuit if we have the best case of `do nothing`
	if len(namespaces) == 0 {
		logger.Info().
			Msg("no namespaces in annotation, skipping")
		return nil
	}

	start := time.Now()
	defer func() {
		reflectorReflectionLatency.
			WithLabelValues(sec.Name).
			Observe(start.Sub(time.Now()).Seconds())
	}()

	// hash the og -- TODO is crc64 good enough here?
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(sec.String())))

	return batchOverNamespaces(
		concurrency,
		namespaces,
		reflectLambda(ctx, logger, client, sec, hash))
}

func reflectLambda(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	sec *v1.Secret,
	hash string,
) func(wg *sync.WaitGroup, ns string, errChan chan error) {
	return func(wg *sync.WaitGroup, ns string, errChan chan error) {
		reflectSecret(
			ctx, logger.With().Str("reflectionNamespace", ns).Logger(),
			wg, client, sec, hash, ns, errChan)
	}
}

func reflectSecret(
	ctx context.Context,
	logger zerolog.Logger,
	wg *sync.WaitGroup,
	client corev1.CoreV1Interface,
	sec *v1.Secret,
	hash string,
	ns string,
	errChan chan error,
) {
	// spin off a goroutine for every level of concurrency
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := instrumentedReflect(
			ctx,
			logger,
			client.Secrets(ns),
			sec,
			hash,
			ns,
		); err != nil {
			logger.Error().Err(err).Msg("unable to reflect")
			errChan <- errors.Wrap(
				err,
				"error while reflecting secret to namespace")
		}
	}()
}

func instrumentedReflect(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.SecretInterface,
	og *v1.Secret,
	hash string,
	namespace string,
) error {
	start := time.Now()
	defer func() {
		reflectorSecretLatency.
			WithLabelValues(og.Name, og.Namespace).
			Observe(start.Sub(time.Now()).Seconds())
	}()
	return reflect(ctx, logger, client, og, hash, namespace)
}

func reflect(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.SecretInterface,
	og *v1.Secret,
	hash string,
	namespace string,
) error {
	// reflect to the new namespace
	// if it exists, then pull the resource and check if we own it
	reflected, err := client.Get(ctx, og.Name, metav1.GetOptions{})
	exists := !apierrors.IsNotFound(err)
	if err != nil && exists {
		logger.Error().Err(err).Msg("error while fetching secret from reflection namespace")
		return errors.Wrap(err, "error while getting reflected secret")
	}

	// if it does exist, check the hash to see if we need to update
	if exists && !secretNeedsUpdate(logger, reflected, hash) {
		return nil
	}

	logger.Debug().
		Bool("create", !exists).
		Bool("update", exists).
		Str("secret", og.Name).
		Str("namespace", namespace).
		Msg("performing action for reflected secret")
	return createOrUpdateSecret(
		ctx,
		client,
		createNewSecret(og, hash, namespace),
		exists)
}

func secretNeedsUpdate(
	logger zerolog.Logger,
	secret *v1.Secret,
	hash string,
) bool {
	// if there is no hash then we know we don't really own it and can short circuit
	reflectHash, ok := secret.Annotations[annotations.ReflectionHashAnnotation]
	if !ok {
		logger.Info().Msg("We don't own this secret: not updating")
		return false
	}

	if reflectHash == hash {
		logger.Debug().Str("hash", hash).Msg("No changes to secret, not updating")
		return false
	}

	// ownership is explicit -- if there is no ownership annotation then skip
	return annotations.CanOperate(secret.Annotations)
}

func createNewSecret(
	secret *v1.Secret,
	hash string,
	namespace string,
) *v1.Secret {

	// DeepCopy and fix the annotations
	toReflect := secret.DeepCopy()
	toReflect.Namespace = namespace

	// remove the reflection annotations so we don't get recursive reflection somewhere
	delete(toReflect.Annotations, annotations.ReflectAnnotation)
	delete(toReflect.Annotations, annotations.NamespaceAnnotation)

	toReflect.Annotations[annotations.ReflectedFromAnnotation] = secret.Namespace
	toReflect.Annotations[annotations.ReflectedAtAnnotation] = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	toReflect.Annotations[annotations.ReflectionHashAnnotation] = hash
	toReflect.Annotations[annotations.ReflectionOwnerAnnotation] = annotations.ReflectionOwned
	return toReflect
}

func createOrUpdateSecret(
	ctx context.Context,
	client corev1.SecretInterface,
	sec *v1.Secret,
	exists bool,
) (err error) {
	labels := []string{"create", sec.Name, "true", sec.Namespace}
	defer func() {
		if err != nil {
			labels[2] = "false"
		}
		reflectorReflections.WithLabelValues(labels...).Inc()
	}()

	// update if it does exist, create if it does not
	if exists {
		labels[0] = "update"
		_, err = client.Update(ctx, sec, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "error while updating secret")
		}
		return nil
	}

	_, err = client.Create(ctx, sec, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error while creating secret")
	}
	return nil
}

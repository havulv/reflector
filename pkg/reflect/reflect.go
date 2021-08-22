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
	start := time.Now()
	defer func() {
		reflectorReflectionLatency.
			WithLabelValues(sec.Name).
			Observe(start.Sub(time.Now()).Seconds())
	}()

	// shortcircuit if we have the best case of `do nothing`
	if len(namespaces) == 0 {
		logger.Info().
			Msg("no namespaces in annotation, skipping")
		return nil
	}

	// hash the og -- TODO is crc64 good enough here?
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(sec.String())))

	// if reflections are synchronous, do the easy thing
	if concurrency <= 1 {
		for _, namespace := range namespaces {
			if err := instrumentedReflect(
				ctx,
				logger.With().Str("reflectionNamespace", namespace).Logger(),
				client.Secrets(namespace),
				sec,
				hash,
				reflect,
			); err != nil {
				return err
			}
		}
		return nil
	}
	return reflectToNamespacesBatching(
		ctx, logger, client, sec, namespaces, concurrency, hash)
}

func reflectToNamespacesBatching(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	sec *v1.Secret,
	namespaces []string,
	concurrency int,
	hash string,
) error {
	counter := 0
	limit := len(namespaces)
	wg := sync.WaitGroup{}

	// make the errChan big enough to fit all the errors we ened
	errChan := make(chan error, concurrency)
	for ind, namespace := range namespaces {
		counter++
		batchInd := counter
		ns := namespace

		// spin off a goroutine for every level of concurrency
		wg.Add(1)
		go func(batchIndex int, singleNs string) {
			defer wg.Done()
			if err := instrumentedReflect(
				ctx,
				logger.With().
					Str("reflectionNamespace", namespace).
					Int("batchIndex", batchIndex).
					Logger(),
				client.Secrets(singleNs),
				sec,
				hash,
				reflect,
			); err != nil {
				errChan <- err
			}
		}(batchInd, ns)

		// wait for each batch to finish and then check if we have errors
		if counter >= concurrency || ind == limit {
			logger.Info().
				Int("concurrency", concurrency).
				Msg("beginning wait for batch to finish")
			wg.Wait()

			// don't block on errors if there are none on the channel
			select {
			case err := <-errChan:
				return errors.Wrap(err, "received first error in concurrency group")
			default:
			}
			counter = 0
		}
	}
	return nil
}

func instrumentedReflect(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.SecretInterface,
	og *v1.Secret,
	hash string,
	reflectFn func(
		context.Context, zerolog.Logger,
		corev1.SecretInterface, *v1.Secret,
		string,
	) error,
) error {
	start := time.Now()
	defer func() {
		reflectorSecretLatency.
			WithLabelValues(og.Name, og.Namespace).
			Observe(start.Sub(time.Now()).Seconds())
	}()
	return reflectFn(ctx, logger, client, og, hash)
}

func reflect(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.SecretInterface,
	og *v1.Secret,
	hash string,
	// namespace string,
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
	if exists {
		// if we can't find the hash annotation, then we don't own it
		reflectHash, ok := reflected.Annotations[annotations.ReflectionHashAnnotation]
		if !ok {
			logger.Info().Msg("We don't own this secret: not updating")
			return nil
		}

		if reflectHash == hash {
			logger.Debug().Str("hash", hash).Msg("No changes to secret, not updating")
			return nil
		}
	}

	// DeepCopy and fix the annotations
	toReflect := og.DeepCopy()

	// remove the reflection annotations so we don't get recursive reflection somewhere
	delete(toReflect.Annotations, annotations.ReflectAnnotation)
	delete(toReflect.Annotations, annotations.NamespaceAnnotation)

	// TODO: is this necessary?
	// toReflect.Namespace = namespace
	toReflect.Annotations[annotations.ReflectedFromAnnotation] = og.Namespace
	toReflect.Annotations[annotations.ReflectedAtAnnotation] = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	toReflect.Annotations[annotations.ReflectionHashAnnotation] = hash

	action := "create"
	if exists {
		action = "update"
	}
	logger.Debug().
		Str("action", action).
		Str("secret", toReflect.Name).
		Str("namespace", toReflect.Namespace).
		Msg("performing action for reflected secret")

	return createOrUpdateSecret(ctx, client, toReflect, exists)
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

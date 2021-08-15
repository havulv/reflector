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

	// if reflections are synchronous, do the easy thing
	if concurrency <= 1 {
		for _, namespace := range namespaces {
			if err := instrumentedReflect(
				ctx,
				logger,
				client.Secrets(namespace),
				sec,
				reflect,
			); err != nil {
				return err
			}
		}
		return nil
	}
	return reflectToNamespacesBatching(
		ctx, logger, client, sec, namespaces, concurrency)
}

func reflectToNamespacesBatching(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	sec *v1.Secret,
	namespaces []string,
	concurrency int,
) error {
	counter := 0
	limit := len(namespaces)
	wg := sync.WaitGroup{}

	// make the errChan big enough to fit all the errors we ened
	errChan := make(chan error, concurrency)
	for ind, namespace := range namespaces {
		counter++

		// spin off a goroutine for every level of concurrency
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := instrumentedReflect(
				ctx,
				logger,
				client.Secrets(namespace),
				sec,
				reflect,
			); err != nil {
				errChan <- err
			}
		}()

		// wait for each batch to finish and then check if we have errors
		if counter >= concurrency || ind == limit {
			logger.Info().Int("concurrency", concurrency).Msg("beginning wait for batch to finish")
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
	reflectFn func(
		context.Context, zerolog.Logger,
		corev1.SecretInterface, *v1.Secret,
	) error,
) error {
	start := time.Now()
	defer func() {
		reflectorSecretLatency.
			WithLabelValues(og.Name, og.Namespace).
			Observe(start.Sub(time.Now()).Seconds())
	}()
	return reflectFn(ctx, logger, client, og)
}

func reflect(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.SecretInterface,
	og *v1.Secret,
	// namespace string,
) error {
	// reflect to the new namespace

	// if it exists, then pull the resource and check if we own it
	reflected, err := client.Get(ctx, og.Name, metav1.GetOptions{})
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
			logger.Info().Msg("We don't own this secret: not updating")
			return nil
		}

		if reflectHash == ogHash {
			logger.Info().Str("hash", ogHash).Msg("No changes to secret, not updating")
			return nil
		}
	}

	// DeepCopy and fix the annotations
	toReflect := og.DeepCopy()

	// remove the reflection annotation so we don't get recursive reflection somewhere
	delete(toReflect.Annotations, annotations.ReflectAnnotation)
	// TODO: is this necessary?
	// toReflect.Namespace = namespace
	toReflect.Annotations[annotations.ReflectedFromAnnotation] = og.Namespace
	toReflect.Annotations[annotations.ReflectedAtAnnotation] = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	toReflect.Annotations[annotations.ReflectionHashAnnotation] = ogHash

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

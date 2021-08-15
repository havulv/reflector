package reflect

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func cascadeDeletion(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	secret string,
	namespaces []string,
) error {
	for _, namespace := range namespaces {
		secretClient := client.Secrets(namespace)
		if err := secretClient.Delete(
			ctx, secret, metav1.DeleteOptions{}); err != nil {
			logger.Error().
				Str("reflectionNamespace", namespace).
				Err(err).
				Msg("unable to delete secret")
			return errors.Wrap(
				err,
				"error while removing secret from the namspace")
		}
	}
	return nil
}

func cascadeDeleteConcurrent(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	secret string,
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
		batchInd := counter
		ns := namespace

		// spin off a goroutine for every level of concurrency
		wg.Add(1)
		go func(batchIndex int, singleNs string) {
			defer wg.Done()
			secretClient := client.Secrets(singleNs)
			if err := secretClient.Delete(
				ctx, secret, metav1.DeleteOptions{}); err != nil {
				logger.Error().
					Int("batchIndex", batchIndex).
					Str("reflectionNamespace", singleNs).
					Err(err).
					Msg("unable to delete secret")
				errChan <- errors.Wrap(
					err,
					"error while removing secret from the namspace")
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

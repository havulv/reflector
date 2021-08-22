package reflect

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func cascadeDelete(
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
		ns := namespace

		// spin off a goroutine for every level of concurrency
		deleteSecret(
			ctx, logger.With().
				Int("batchIndex", counter).
				Str("reflectionNamespace", ns).Logger(),
			&wg,
			client,
			secret,
			ns,
			errChan)

		// wait for each batch to finish and then check if we have errors
		if counter >= concurrency || ind == limit {
			counter = 0
			logger.Info().Int("concurrency", concurrency).
				Msg("beginning wait for batch to finish")

			if err := waitUntilError(&wg, errChan); err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteSecret(
	ctx context.Context,
	logger zerolog.Logger,
	wg *sync.WaitGroup,
	client corev1.CoreV1Interface,
	secret string,
	ns string,
	errChan chan error,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		secretClient := client.Secrets(ns)
		if err := secretClient.Delete(
			ctx, secret, metav1.DeleteOptions{}); err != nil {
			logger.Error().Err(err).Msg("unable to delete secret")
			errChan <- errors.Wrap(
				err,
				"error while removing secret from the namspace")
		}
	}()
}

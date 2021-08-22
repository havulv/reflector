package reflect

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	// shortcircuit if we have the best case of `do nothing`
	if len(namespaces) == 0 {
		logger.Info().Msg("no namespaces, skipping")
		return nil
	}

	return batchOverNamespaces(
		concurrency,
		namespaces,
		deleteLambda(ctx, logger, client, secret))
}

func deleteLambda(
	ctx context.Context,
	logger zerolog.Logger,
	client corev1.CoreV1Interface,
	secret string,
) func(wg *sync.WaitGroup, ns string, errChan chan error) {
	return func(wg *sync.WaitGroup, ns string, errChan chan error) {
		// spin off a goroutine for every level of concurrency
		deleteSecret(
			ctx, logger.With().
				Str("reflectionNamespace", ns).Logger(),
			wg, client, secret, ns, errChan)
	}
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
			ctx, secret, metav1.DeleteOptions{},
		); err != nil && !apierrors.IsNotFound(err) {
			logger.Error().Err(err).Msg("unable to delete secret")
			errChan <- errors.Wrap(
				err,
				"error while removing secret from the namspace")
		}
	}()
}

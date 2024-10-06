package reflect

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/sync/semaphore"
)

// batchOverNamespaces batches a lambda over namespaces, based on
// some predetermined concurrency. Importantly, errors are non fatal
// and only logged.
func batchOverNamespaces(
	ctx context.Context,
	logger zerolog.Logger,
	concurrency int,
	namespaces []string,
	lambda func(context.Context, string) error,
) error {
	if concurrency < 1 {
		return errors.New("concurrency value of less than 1")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := semaphore.NewWeighted(int64(concurrency))
	for _, namespace := range namespaces {
		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("failed to acquire semaphore: %w", err)
		}

		ns := namespace

		go func() {
			defer sem.Release(1)
			if err := lambda(ctx, ns); err != nil {
				logger.Error().Err(err).
					Str("namespace", ns).
					Msg("failed to run lambda")
			}
		}()
	}

	// acquire all semaphores to block
	if err := sem.Acquire(ctx, int64(concurrency)); err != nil {
		return fmt.Errorf("failed to acquire all semaphores: %w", err)
	}

	return nil
}

package reflect

import (
	"sync"

	"github.com/pkg/errors"
)

func batchOverNamespaces(
	concurrency int,
	namespaces []string,
	lambda func(*sync.WaitGroup, string, chan error),
) error {
	counter := 0
	limit := len(namespaces) - 1
	wg := &sync.WaitGroup{}
	errChan := make(chan error, concurrency)
	for ind, namespace := range namespaces {
		counter++
		ns := namespace

		lambda(wg, ns, errChan)

		if counter >= concurrency || ind == limit {
			counter = 0
			if err := waitUntilError(wg, errChan); err != nil {
				return err
			}
		}
	}
	return nil
}

func waitUntilError(
	wg *sync.WaitGroup,
	errChan chan error,
) error {
	// wait for every goroutine to finish so that we don't cancel deletions
	// that may have succeeded.
	wg.Wait()

	// don't block on errors if there are none on the channel
	select {
	case err := <-errChan:
		return errors.Wrap(err, "received error in concurrency batch")
	default:
	}
	return nil
}

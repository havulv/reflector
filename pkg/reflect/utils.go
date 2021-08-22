package reflect

import (
	"sync"

	"github.com/pkg/errors"
)

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

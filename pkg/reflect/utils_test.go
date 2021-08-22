package reflect

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWaitUntilError(t *testing.T) {
	tests := []struct {
		d   string
		err error
	}{
		{
			"waits until batch is finished and reports no errors",
			nil,
		},
		{
			"waits until batch is finished and reports errors",
			errors.New("some error"),
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			wg := sync.WaitGroup{}
			errChan := make(chan error, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if test.err != nil {
					errChan <- test.err
				}
			}()
			if test.err != nil {
				assert.NotNil(t, waitUntilError(&wg, errChan))
				return
			}
			assert.Nil(t, waitUntilError(&wg, errChan))
		})
	}
}

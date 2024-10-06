package reflect

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchOverNamespaces(t *testing.T) {
	tests := []struct {
		descrip      string
		concurrency  int
		namespaces   []string
		shouldCancel bool
		err          string
	}{
		{
			"batches over a single concurrency",
			1,
			[]string{
				"kube-system",
				"cert-manager",
			},
			false,
			"",
		},
		{
			"batches over a 10 concurrency",
			10,
			[]string{
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
			},
			false,
			"",
		},
		{
			"batches with an error",
			2,
			[]string{
				"kube-system",
				"cert-manager",
				"kube-system",
				"cert-manager",
			},
			true,
			"failed to acquire semaphore: context canceled",
		},
	}

	for _, l := range tests {
		test := l
		t.Run(test.descrip, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if test.shouldCancel {
				cancel()
			}
			err := batchOverNamespaces(
				ctx,
				zerolog.New(zerolog.NewTestWriter(t)),
				test.concurrency,
				test.namespaces,
				func(_ context.Context, _ string) error {
					return nil
				})
			if test.err != "" {
				require.Error(t, err)
				assert.EqualError(t, err, test.err)
				return
			}
			assert.Nil(t, err)
		})
	}
}

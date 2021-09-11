package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthcheck(t *testing.T) {
	tests := []struct {
		d          string
		val        int32
		statusCode int
	}{
		{
			"returns ok if health int set",
			0,
			503,
		},
		{
			"returns unavailable if health int is not set",
			1,
			200,
		},
	}
	for _, l := range tests {
		test := l
		t.Run(test.d, func(t *testing.T) {
			t.Parallel()
			f := healthcheck(&test.val)
			w := httptest.NewRecorder()
			f(w, nil)
			res := w.Result()
			err := res.Body.Close()
			assert.Nil(t, err)
			assert.Equal(t, test.statusCode, res.StatusCode)
		})
	}
}

func TestNewMetricsServer(t *testing.T) {
	t.Run("creates a new metrics server", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		l := zerolog.New(buf)
		addr := "localhost:8083"
		s := NewMetricsServer(l, addr)
		require.NotNil(t, s)
		assert.Equal(t, s.(*server).Addr, addr)
		assert.Equal(t, s.(*server).logger, l)
	})
}

func TestRun(t *testing.T) {
	t.Run("runs the metrics server and shuts it down", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		l := zerolog.New(buf)
		ctx, cancel := context.WithCancel(context.Background())

		s := &server{
			Server: http.Server{
				Addr: "localhost:8085",
			},
			logger: l,
		}
		cancel()
		assert.Nil(t, s.Run(ctx))
		assert.Equal(
			t,
			`{"level":"error","error":"context canceled","message":"Context finished, shutting down"}
{"level":"info","message":"Waiting on goroutines to finish after running server shutdown..."}
{"level":"info","address":"localhost:8085","message":"Started server"}
{"level":"info","message":"Server shut down"}
`, buf.String())
	})
}

func TestServe(t *testing.T) {
	t.Run("serves metrics and shuts down", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		l := zerolog.New(buf)
		ctx, cancel := context.WithCancel(context.Background())

		s := &server{
			Server: http.Server{
				Addr: "localhost:8090",
			},
		}
		go func() {
			time.Sleep(time.Millisecond)
			err := s.Shutdown(ctx)
			if errors.Is(err, http.ErrServerClosed) || errors.Is(err, context.Canceled) {
				return
			}
			assert.Nil(t, err)
		}()
		serve(s, l, cancel)
		assert.Equal(
			t,
			`{"level":"info","address":"localhost:8090","message":"Started server"}
{"level":"info","message":"Server shut down"}
`,
			buf.String())
	})
}

func TestShutdown(t *testing.T) {
	t.Run("shuts down a server", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		l := zerolog.New(buf)
		ctx, cancel := context.WithCancel(context.Background())
		// cancel early so we just auto shutdown
		cancel()

		s := &server{
			ready: 1,
			alive: 1,
		}
		wg := &sync.WaitGroup{}
		assert.Nil(t, shutdown(ctx, s, l, wg))
		assert.Equal(t, int32(0), s.alive)
		assert.Equal(t, int32(0), s.ready)
	})
}

package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

const readTimeout = 1 * time.Second
const writeTimeout = 1 * time.Second

// MetricsServer is a server interface which allows for prometheus
// to scrape metrics from the reflector.
type MetricsServer interface {
	Run(context.Context) error
}

type server struct {
	http.Server
	logger zerolog.Logger
}

// NewMetricsServer creates a new server for serving metrics
func NewMetricsServer(logger zerolog.Logger, address string) MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	s := &server{
		logger: logger,
	}

	s.Addr = address
	s.ReadTimeout = readTimeout
	s.WriteTimeout = writeTimeout
	s.Handler = mux
	return s
}

func (s *server) Run(ctx context.Context) error {
	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		serve(s, s.logger, cancel)
	}()
	return shutdown(
		serverCtx, s, s.logger, &wg)
}

func serve(s *server, logger zerolog.Logger, cancel func()) {
	// we cancel the context here as well as in the outer scope
	// because we want to ensure that a failed server doesn't stick around
	defer cancel()
	logger.Info().
		Str("address", s.Addr).
		Msg("Started server")
	if srvErr := s.ListenAndServe(); !errors.Is(srvErr, http.ErrServerClosed) {
		logger.Error().
			Err(srvErr).
			Msg("Error while listening and serving")
		return
	}
	logger.Info().Msg("Server shut down")
}

func shutdown(
	ctx context.Context,
	s *server,
	logger zerolog.Logger,
	wg *sync.WaitGroup,
) error {
	select {
	case <-ctx.Done():
		logger.Error().Err(ctx.Err()).Msg("Context finished, shutting down")
	}
	err := s.Shutdown(ctx)
	wg.Wait()

	// we closed the server or the original context was canceled so we don't care
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

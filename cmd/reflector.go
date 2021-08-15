package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/havulv/reflector/cmd/version"
	"github.com/havulv/reflector/pkg/reflect"
	"github.com/havulv/reflector/pkg/server"
)

func missed(dropped int) {
	fmt.Printf("Logger Dropped %d messages", dropped)
}

func setLogLevel(logger zerolog.Logger, verbose bool) zerolog.Logger {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	logger.Debug().
		Int8("verbosity", int8(zerolog.GlobalLevel())).
		Msg("Created logger")
	return logger
}

func reflector() *cobra.Command {
	var verbose *bool
	diodeWriter := diode.NewWriter(
		os.Stdout,
		100,
		0,
		missed)

	logger := zerolog.New(diodeWriter).With().
		Caller().
		Timestamp().
		Logger()
	// ovewrite the global logger to our new fancy one
	log.Logger = logger

	var cmdVersion *bool
	var metrics *bool
	var namespace *string
	var metricsAddr *string
	var workerCon *int
	var reflectCon *int
	var retries *int
	var cascadeDelete *bool

	cmd := &cobra.Command{
		Use:   "reflector",
		Short: "A kubernetes secret syncer",
		Long: strings.Trim(`
A utility kubernetes server for syncing secrets from one namespace
to others.  `, " "),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if *cmdVersion {
				return version.DumpVersion()
			}
			logger = setLogLevel(logger, *verbose)

			if *namespace == "" {
				*namespace = os.Getenv("POD_NAMESPACE")
			}

			// Ensure that, if either component goes through
			// a catastrophic error, then the context will
			// be cancelled and all components will begin shutdown
			signalCtx, death := signal.NotifyContext(
				ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
			defer death()

			cancellableCtx, cancel := context.WithCancel(signalCtx)
			// call cancel even though we know this will be a duplicate call
			defer cancel()
			wg := sync.WaitGroup{}

			if *metrics {
				metrics := server.NewMetricsServer(
					logger.With().Str("component", "metrics").Logger(),
					*metricsAddr)
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer cancel()
					if err := metrics.Run(cancellableCtx); err != nil {
						logger.Error().Err(err).Msg("Error while running metrics server")
					}
				}()
			}

			reflector, err := reflect.NewReflector(
				logger.With().Str("component", "reflector").Logger(),
				*reflectCon,
				*workerCon,
				*retries,
				*cascadeDelete,
				*namespace)
			if err != nil {
				return errors.Wrap(err, "unable to start reflector")
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				if err := reflector.Start(cancellableCtx); err != nil {
					logger.Error().Err(err).Msg("Error while running reflector")
				}
			}()
			wg.Wait()

			return nil
		},
	}

	namespace = cmd.Flags().StringP(
		"namespace", "n", "",
		`The namespace to sync secrets from`)
	retries = cmd.Flags().IntP(
		"retries", "r", 5,
		`The number of times to retry reflecting a
secret on error`)
	metrics = cmd.Flags().BoolP(
		"metrics", "m", true,
		`Enables Prometheus metrics for the reflector`)
	metricsAddr = cmd.Flags().String(
		"metrics-addr", "localhost:8080",
		`The address to expose metrics on`)
	workerCon = cmd.Flags().Int(
		"worker-concurrency", 10,
		`The number of workers who can pick work of
the work queue concurrently`)
	reflectCon = cmd.Flags().Int(
		"reflect-concurrency", 1,
		`The number of reflections that can happen
concurrently to different namespaces.`)
	cascadeDelete = cmd.Flags().Bool(
		"cascade-delete", false,
		`If enabled, secrets that were reflected into
other namespaces will be deleted when the
original secret is deleted.

***WARNING***
This can be very dangerous to set, and is
not recommended unless you are
_absolutely certain_ it fits your use case
***WARNING***`)
	cmdVersion = cmd.Flags().Bool(
		"version", false, "Output version information")
	verbose = cmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	return cmd
}

func main() {
	cmd := reflector()
	if err := cmd.Execute(); err != nil {
		return
	}
}

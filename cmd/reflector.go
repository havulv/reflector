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

	"github.com/havulv/reflector/pkg/reflect"
	"github.com/havulv/reflector/pkg/server"
)

var (
	commitHash string
	commitDate string
	semVer     string
)

func missed(dropped int) {
	fmt.Printf("Logger Dropped %d messages", dropped)
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

	// versioning is attached to the root command
	// so checking it is as simple as reflector --version
	var version *bool
	var namespace *string
	var metricsAddr *string
	var workerCon *int
	var reflectCon *int

	cmd := &cobra.Command{
		Use:   "reflector",
		Short: "A kubernetes secret syncer",
		Long: strings.Trim(`
A utility kubernetes server for syncing secrets from one namespace
to others.  `, " "),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if *version {
				if semVer == "" && commitHash == "" && commitDate == "" {
					return errors.New(
						"version information not linked at comile time")
				}
				fmt.Printf(
					"Version: %s\nCommit: %s \nDate: %s\n",
					semVer, commitHash, commitDate)
				return nil
			}
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
			if *verbose {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			}

			logger.Debug().
				Int8("verbosity", int8(zerolog.GlobalLevel())).
				Msg("Created logger")

			// Ensure that, if either component goes through
			// a catastrophic error, then the context will
			// be cancelled and all components will begin shutdown
			signalCtx, death := signal.NotifyContext(
				ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
			defer death()

			cancellableCtx, cancel := context.WithCancel(signalCtx)
			wg := sync.WaitGroup{}

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

			reflector, err := reflect.NewReflector(
				logger.With().Str("component", "reflector").Logger(),
				*reflectCon,
				*workerCon,
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
		"namespace", "n", "kube-system", "The namespace to sync secrets from")
	metricsAddr = cmd.Flags().String(
		"metrics-addr", "localhost:8080", "The address to expose metrics on")
	workerCon = cmd.Flags().Int(
		"worker-concurrency", 10, "The number of workers who can pick work of the work queue concurrently")
	reflectCon = cmd.Flags().Int(
		"reflect-concurrency", 1, "The number of reflections that can happen concurrently to different namespaces.")
	version = cmd.Flags().Bool(
		"version", false, "Output version information")
	verbose = cmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	return cmd
}

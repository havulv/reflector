package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/havulv/reflector/cmd/k8s"
	"github.com/havulv/reflector/cmd/version"
	"github.com/havulv/reflector/pkg/reflect"
	"github.com/havulv/reflector/pkg/server"
)

const (
	defaultRetries            = 5
	defaultWorkerConcurrency  = 10
	defaultReflectConcurrency = 1
)

// ReflectorArgs is a struct of the arguments to the reflector
// command.
type ReflectorArgs struct {
	Verbose       *bool
	CmdVersion    *bool
	Metrics       *bool
	Namespace     *string
	MetricsAddr   *string
	WorkerCon     *int
	ReflectCon    *int
	Retries       *int
	CascadeDelete *bool
	KubeConfig    *string
}

func startReflector(
	logger zerolog.Logger,
	newMetricsServer func(
		zerolog.Logger,
		string,
	) server.MetricsServer,
	newReflector func(
		zerolog.Logger,
		kubernetes.Interface,
		int, int, int,
		bool,
		string,
	) (reflect.Reflector, error),
	clientClosure func(*string) (kubernetes.Interface, error),
	rArgs *ReflectorArgs,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if rArgs.CmdVersion != nil && *rArgs.CmdVersion {
			return version.DumpVersion()
		}
		logger = setLogLevel(logger, *rArgs.Verbose)

		if *rArgs.Namespace == "" {
			*rArgs.Namespace = os.Getenv("POD_NAMESPACE")
		}

		client, err := clientClosure(rArgs.KubeConfig)
		if err != nil {
			return errors.Wrap(err, "unable to create k8s client")
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

		if rArgs.Metrics != nil && *rArgs.Metrics {
			metrics := newMetricsServer(
				logger.With().Str("component", "metrics").Logger(),
				*rArgs.MetricsAddr)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				if err := metrics.Run(cancellableCtx); err != nil {
					logger.Error().
						Err(err).
						Str("component", "metrics").
						Msg("Error while running metrics server")
				}
			}()
		}

		reflector, err := newReflector(
			logger.With().Str("component", "reflector").Logger(),
			client,
			*rArgs.ReflectCon,
			*rArgs.WorkerCon,
			*rArgs.Retries,
			*rArgs.CascadeDelete,
			*rArgs.Namespace)
		if err != nil {
			return errors.Wrap(err, "unable to start reflector")
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			if err := reflector.Start(cancellableCtx); err != nil {
				logger.Error().
					Err(err).
					Str("component", "reflector").
					Msg("Error while running reflector")
			}
		}()
		wg.Wait()

		return nil
	}
}

func reflectorCmd() *cobra.Command {
	args := ReflectorArgs{}
	cmd := &cobra.Command{
		Use:   "reflector",
		Short: "A kubernetes secret syncer",
		Long: strings.Trim(`
A utility kubernetes server for syncing secrets from one namespace
to others.  `, " "),
		SilenceUsage: true,
		RunE: startReflector(
			setupLogger(),
			server.NewMetricsServer,
			reflect.NewReflector,
			k8s.CreateK8sClient,
			&args),
	}

	args.KubeConfig = cmd.Flags().String(
		"kube-config", "",
		"The path to a kubernetes configuration if running outside a cluster")
	args.Namespace = cmd.Flags().StringP(
		"namespace", "n", "",
		`The namespace to sync secrets from`)
	args.Retries = cmd.Flags().IntP(
		"retries", "r", defaultRetries,
		`The number of times to retry reflecting a
secret on error`)
	args.Metrics = cmd.Flags().BoolP(
		"metrics", "m", true,
		`Enables Prometheus metrics for the reflector`)
	args.MetricsAddr = cmd.Flags().String(
		"metrics-addr", "localhost:8080",
		`The address to expose metrics on`)
	args.WorkerCon = cmd.Flags().Int(
		"worker-concurrency", defaultWorkerConcurrency,
		`The number of workers who can pick work of
the work queue concurrently`)
	args.ReflectCon = cmd.Flags().Int(
		"reflect-concurrency", defaultReflectConcurrency,
		`The number of reflections that can happen
concurrently to different namespaces.`)
	args.CascadeDelete = cmd.Flags().Bool(
		"cascade-delete", false,
		`If enabled, secrets that were reflected into
other namespaces will be deleted when the
original secret is deleted.

***WARNING***
This can be very dangerous to set, and is
not recommended unless you are
_absolutely certain_ it fits your use case
***WARNING***`)
	args.CmdVersion = cmd.Flags().Bool(
		"version", false, "Output version information")
	args.Verbose = cmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	return cmd
}

var startCmd = reflectorCmd

func main() {
	cmd := startCmd()
	if err := cmd.Execute(); err != nil {
		return
	}
}

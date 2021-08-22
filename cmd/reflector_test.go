package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/havulv/reflector/cmd/version"
	"github.com/havulv/reflector/pkg/mocks"
	"github.com/havulv/reflector/pkg/reflect"
	"github.com/havulv/reflector/pkg/server"
)

func createMocks(
	metricsArgsAssert func(string),
	reflectArgsAssert func(int, int, int, bool, string),
) (
	*mocks.MetricsServer,
	*mocks.Reflector,
	func(zerolog.Logger, string) server.MetricsServer,
	func(zerolog.Logger, int, int, int, bool, string) (reflect.Reflector, error),
) {
	mockServer := &mocks.MetricsServer{}
	metricsServer := func(l zerolog.Logger, a string) server.MetricsServer {
		metricsArgsAssert(a)
		return mockServer
	}
	reflector := &mocks.Reflector{}
	newReflector := func(l zerolog.Logger, a int, b int, c int, d bool, e string) (reflect.Reflector, error) {
		reflectArgsAssert(a, b, c, d, e)
		return reflector, nil
	}
	return mockServer, reflector, metricsServer, newReflector
}

func TestStartReflector(t *testing.T) {
	t.Run("tests that version dumps the version", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		_, _, metricsServer, newReflector := createMocks(
			func(s string) {}, func(a int, b int, c int, d bool, e string) {})

		cmdVersion := true
		version.CommitHash = "thing"
		version.OutputFunc = func(f string, a ...interface{}) (int, error) {
			return fmt.Fprintf(buf, f, a)
		}
		defer func() {
			version.CommitHash = ""
			version.OutputFunc = fmt.Printf
		}()
		startFunc := startReflector(
			logger,
			metricsServer,
			newReflector,
			ReflectorArgs{
				CmdVersion: &cmdVersion,
			})
		assert.Nil(t, startFunc(&cobra.Command{}, []string{}))
	})

	t.Run("tests that namespace is not grabbed from the environment", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		verbose := false
		namespace := "default"
		conn := 0
		require.Nil(t, os.Setenv("POD_NAMESPACE", "kube-system"))

		_, r, metricsServer, newReflector := createMocks(
			func(s string) {}, func(a int, b int, c int, d bool, n string) {
				assert.Equal(t, n, namespace)
			})
		r.On("Start", mock.Anything).Return(nil)

		startFunc := startReflector(
			logger,
			metricsServer,
			newReflector,
			ReflectorArgs{
				Verbose:       &verbose,
				Namespace:     &namespace,
				ReflectCon:    &conn,
				WorkerCon:     &conn,
				Retries:       &conn,
				CascadeDelete: &verbose,
			})
		cmd := &cobra.Command{}
		cmd.Execute()
		assert.Nil(t, startFunc(cmd, []string{}))
	})

	t.Run("tests that metrics are run when set", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		ns := "default"
		addr := "localhost:8080"
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, newReflector := createMocks(
			func(s string) {
				assert.Equal(t, s, addr)
			}, func(a int, b int, c int, d bool, n string) {})
		m.On("Run", mock.Anything).Return(nil)
		r.On("Start", mock.Anything).Return(nil)

		startFunc := startReflector(
			logger,
			metricsServer,
			newReflector,
			ReflectorArgs{
				Namespace:     &ns,
				Metrics:       &metrics,
				MetricsAddr:   &addr,
				Verbose:       &verbose,
				ReflectCon:    &conn,
				WorkerCon:     &conn,
				Retries:       &conn,
				CascadeDelete: &verbose,
			})
		cmd := &cobra.Command{}
		cmd.Execute()
		assert.Nil(t, startFunc(cmd, []string{}))
	})

	t.Run("tests that metrics errors are logged", func(t *testing.T) {
		t.Parallel()
		ns := "default"
		addr := "localhost:8080"
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, newReflector := createMocks(
			func(s string) {
				assert.Equal(t, s, addr)
			}, func(a int, b int, c int, d bool, n string) {})
		m.On("Run", mock.Anything).Return(errors.New("some error"))
		r.On("Start", mock.Anything).Return(nil)

		altBuf := bytes.NewBuffer([]byte{})
		tLogger := zerolog.New(altBuf)
		startFunc := startReflector(
			tLogger,
			metricsServer,
			newReflector,
			ReflectorArgs{
				Namespace:     &ns,
				Metrics:       &metrics,
				MetricsAddr:   &addr,
				Verbose:       &verbose,
				ReflectCon:    &conn,
				WorkerCon:     &conn,
				Retries:       &conn,
				CascadeDelete: &verbose,
			})
		cmd := &cobra.Command{}
		cmd.Execute()
		assert.Nil(t, startFunc(cmd, []string{}))
		assert.Equal(
			t,
			"{\"level\":\"error\",\"error\":\"some error\",\"component\":\"metrics\",\"message\":\"Error while running metrics server\"}\n",
			altBuf.String())
	})

	t.Run("tests that reflector errors are caught", func(t *testing.T) {
		t.Parallel()
		ns := ""
		addr := "localhost:8080"
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, newReflector := createMocks(
			func(s string) {
				assert.Equal(t, s, addr)
			}, func(a int, b int, c int, d bool, n string) {})
		m.On("Run", mock.Anything).Return(nil)
		r.On("Start", mock.Anything).Return(errors.New("some error"))

		altBuf := bytes.NewBuffer([]byte{})
		tLogger := zerolog.New(altBuf)
		startFunc := startReflector(
			tLogger,
			metricsServer,
			newReflector,
			ReflectorArgs{
				Namespace:     &ns,
				Metrics:       &metrics,
				MetricsAddr:   &addr,
				Verbose:       &verbose,
				ReflectCon:    &conn,
				WorkerCon:     &conn,
				Retries:       &conn,
				CascadeDelete: &verbose,
			})
		cmd := &cobra.Command{}
		cmd.Execute()
		assert.Nil(t, startFunc(cmd, []string{}))
		assert.Equal(
			t,
			"{\"level\":\"error\",\"error\":\"some error\",\"component\":\"reflector\",\"message\":\"Error while running reflector\"}\n",
			altBuf.String())

	})

	t.Run("tests that starting reflector errors are caught", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		ns := "default"
		addr := "localhost:8080"
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, _ := createMocks(
			func(s string) {}, func(a int, b int, c int, d bool, n string) {})
		m.On("Run", mock.Anything).Return(nil)
		r.On("Start", mock.Anything).Return(errors.New("some error"))

		startFunc := startReflector(
			logger,
			metricsServer,
			func(l zerolog.Logger, a int, b int, c int, d bool, e string) (reflect.Reflector, error) {
				return r, errors.New("can't start")
			},
			ReflectorArgs{
				Namespace:     &ns,
				Metrics:       &metrics,
				MetricsAddr:   &addr,
				Verbose:       &verbose,
				ReflectCon:    &conn,
				WorkerCon:     &conn,
				Retries:       &conn,
				CascadeDelete: &verbose,
			})
		cmd := &cobra.Command{}
		cmd.Execute()
		assert.NotNil(t, startFunc(cmd, []string{}))
	})
}

func TestReflectorCmd(t *testing.T) {
	t.Run("tests that the command has sane defaults set", func(t *testing.T) {
		t.Parallel()
		rCmd := reflectorCmd()
		assert.Equal(t, rCmd.Use, "reflector")
		assert.Greater(t, len(rCmd.Short), 0)
		assert.Greater(t, len(rCmd.Long), 0)
		assert.NotNil(t, rCmd.RunE)
	})
}

func TestMain(t *testing.T) {
	t.Run("runs with no error", func(t *testing.T) {
		t.Parallel()
		altCmd := startCmd
		startCmd = func() *cobra.Command {
			return &cobra.Command{}
		}
		defer func() {
			startCmd = altCmd
		}()
		main()
	})
	t.Run("runs with error", func(t *testing.T) {
		t.Parallel()
		altCmd := startCmd
		startCmd = func() *cobra.Command {
			return &cobra.Command{
				RunE: func(cmd *cobra.Command, args []string) error {
					return errors.New("error")
				},
				SilenceUsage:  true,
				SilenceErrors: true,
			}
		}
		defer func() {
			startCmd = altCmd
		}()
		main()
	})
}

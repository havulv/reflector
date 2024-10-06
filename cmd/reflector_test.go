package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/havulv/reflector/cmd/version"
	"github.com/havulv/reflector/pkg/mocks"
	"github.com/havulv/reflector/pkg/reflect"
	"github.com/havulv/reflector/pkg/server"
)

const defaultNamespace = "default"
const defaultAddr = "localhost:8080"

func createMocks(
	metricsArgsAssert func(string),
	reflectArgsAssert func(int, int, int, bool, string),
) (
	*mocks.MetricsServer,
	*mocks.Reflector,
	func(zerolog.Logger, string) server.MetricsServer,
	func(zerolog.Logger, kubernetes.Interface, int, int, int, bool, string) (reflect.Reflector, error),
) {
	mockServer := &mocks.MetricsServer{}
	metricsServer := func(_ zerolog.Logger, a string) server.MetricsServer {
		metricsArgsAssert(a)
		return mockServer
	}
	reflector := &mocks.Reflector{}
	newReflector := func(_ zerolog.Logger, _ kubernetes.Interface, a int, b int, c int, d bool, e string) (reflect.Reflector, error) {
		reflectArgsAssert(a, b, c, d, e)
		return reflector, nil
	}
	return mockServer, reflector, metricsServer, newReflector
}

func TestStartReflector(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "kube-system")
	t.Run("tests that version dumps the version", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		_, _, metricsServer, newReflector := createMocks(
			func(_ string) {}, func(_ int, _ int, _ int, _ bool, _ string) {})

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
			func(_ *string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil },
			&ReflectorArgs{
				CmdVersion: &cmdVersion,
			})
		assert.Nil(t, startFunc(&cobra.Command{}, []string{}))
	})

	t.Run("tests that namespace is not grabbed from the environment", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		verbose := false
		namespace := defaultNamespace
		conn := 0

		_, r, metricsServer, newReflector := createMocks(
			func(_ string) {}, func(_ int, _ int, _ int, _ bool, n string) {
				assert.Equal(t, n, namespace)
			})
		r.On("Start", mock.Anything).Return(nil)

		startFunc := startReflector(
			logger,
			metricsServer,
			newReflector,
			func(_ *string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil },
			&ReflectorArgs{
				Verbose:       &verbose,
				Namespace:     &namespace,
				ReflectCon:    &conn,
				WorkerCon:     &conn,
				Retries:       &conn,
				CascadeDelete: &verbose,
			})
		cmd := &cobra.Command{}
		assert.Nil(t, cmd.Execute())
		assert.Nil(t, startFunc(cmd, []string{}))
	})

	t.Run("tests that failures to create the client are caught", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		verbose := false
		namespace := defaultNamespace

		_, _, metricsServer, newReflector := createMocks(
			func(_ string) {}, func(_ int, _ int, _ int, _ bool, _ string) {})

		startFunc := startReflector(
			logger,
			metricsServer,
			newReflector,
			func(_ *string) (kubernetes.Interface, error) { return nil, errors.New("err") },
			&ReflectorArgs{
				Verbose:   &verbose,
				Namespace: &namespace,
			})
		cmd := &cobra.Command{}
		assert.Nil(t, cmd.Execute())
		assert.NotNil(t, startFunc(cmd, []string{}))
	})

	t.Run("tests that metrics are run when set", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		logger := zerolog.New(buf)
		t.Parallel()
		ns := defaultNamespace
		addr := defaultAddr
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, newReflector := createMocks(
			func(s string) {
				assert.Equal(t, s, addr)
			}, func(_ int, _ int, _ int, _ bool, _ string) {})
		m.On("Run", mock.Anything).Return(nil)
		r.On("Start", mock.Anything).Return(nil)

		startFunc := startReflector(
			logger,
			metricsServer,
			newReflector,
			func(_ *string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil },
			&ReflectorArgs{
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
		assert.Nil(t, cmd.Execute())
		assert.Nil(t, startFunc(cmd, []string{}))
	})

	t.Run("tests that metrics errors are logged", func(t *testing.T) {
		t.Parallel()
		ns := defaultNamespace
		addr := defaultAddr
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, newReflector := createMocks(
			func(s string) {
				assert.Equal(t, s, addr)
			}, func(_ int, _ int, _ int, _ bool, _ string) {})
		m.On("Run", mock.Anything).Return(errors.New("some error"))
		r.On("Start", mock.Anything).Return(nil)

		altBuf := bytes.NewBuffer([]byte{})
		tLogger := zerolog.New(altBuf)
		startFunc := startReflector(
			tLogger,
			metricsServer,
			newReflector,
			func(_ *string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil },
			&ReflectorArgs{
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
		assert.Nil(t, cmd.Execute())
		assert.Nil(t, startFunc(cmd, []string{}))
		assert.Equal(
			t,
			"{\"level\":\"error\",\"error\":\"some error\",\"component\":\"metrics\",\"message\":\"Error while running metrics server\"}\n",
			altBuf.String())
	})

	t.Run("tests that reflector errors are caught", func(t *testing.T) {
		t.Parallel()
		ns := ""
		addr := defaultAddr
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, newReflector := createMocks(
			func(s string) {
				assert.Equal(t, s, addr)
			}, func(_ int, _ int, _ int, _ bool, _ string) {})
		m.On("Run", mock.Anything).Return(nil)
		r.On("Start", mock.Anything).Return(errors.New("some error"))

		altBuf := bytes.NewBuffer([]byte{})
		tLogger := zerolog.New(altBuf)
		startFunc := startReflector(
			tLogger,
			metricsServer,
			newReflector,
			func(_ *string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil },
			&ReflectorArgs{
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
		assert.Nil(t, cmd.Execute())
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
		ns := defaultNamespace
		addr := defaultAddr
		metrics := true
		verbose := false
		conn := 0

		m, r, metricsServer, _ := createMocks(
			func(_ string) {}, func(_ int, _ int, _ int, _ bool, _ string) {})
		m.On("Run", mock.Anything).Return(nil)
		r.On("Start", mock.Anything).Return(errors.New("some error"))

		startFunc := startReflector(
			logger,
			metricsServer,
			func(_ zerolog.Logger, _ kubernetes.Interface, _ int, _ int, _ int, _ bool, _ string) (reflect.Reflector, error) {
				return r, errors.New("can't start")
			},
			func(_ *string) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil },
			&ReflectorArgs{
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
		assert.Nil(t, cmd.Execute())
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
		altCmd := startCmd
		startCmd = func() *cobra.Command {
			return &cobra.Command{
				RunE: func(_ *cobra.Command, _ []string) error {
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

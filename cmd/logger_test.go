package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestMissed(t *testing.T) {
	// assert that it will run
	t.Run("tests that missed will dump missed messages", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte{})
		outputFunc = func(f string, a ...interface{}) (int, error) {
			return fmt.Fprintf(buf, f, a...)
		}
		defer func() {
			outputFunc = fmt.Printf
		}()

		missed(20)
		assert.Equal(t, "Logger Dropped 20 messages", buf.String())
	})

	t.Run("erroring output func runs through branch", func(t *testing.T) {
		outputFunc = func(f string, a ...interface{}) (int, error) {
			return 0, errors.New("some error")
		}
		defer func() {
			outputFunc = fmt.Printf
		}()
		missed(20)
	})
}

func TestSetLogLevel(t *testing.T) {
	t.Run("tests that a normal logger is set correctly", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		logger := setLogLevel(zerolog.New(buf), false)
		assert.NotNil(t, logger)
		assert.Equal(t, "", buf.String())
	})
	t.Run("tests that a verbose logger is set correctly", func(t *testing.T) {
		t.Parallel()
		buf := bytes.NewBuffer([]byte{})
		logger := setLogLevel(zerolog.New(buf), true)
		assert.NotNil(t, logger)
		assert.Equal(
			t,
			"{\"level\":\"debug\",\"verbosity\":0,\"message\":\"Created logger\"}\n",
			buf.String())
	})
}

func TestSetupLogger(t *testing.T) {
	t.Run("tests that the logger is set everywhere", func(t *testing.T) {
		t.Parallel()
		logger := setupLogger()
		assert.Equal(t, log.Logger.GetLevel(), logger.GetLevel())
	})
}

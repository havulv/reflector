package main

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"github.com/rs/zerolog/log"
)

// outputFunc is a variable for the printer that will dump
// the missed messages out (to avoid cluttering test logs).
var outputFunc = fmt.Printf

func missed(dropped int) {
	_, err := outputFunc("Logger Dropped %d messages", dropped)
	if err != nil {
		// TODO: do something?
	}
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

func setupLogger() zerolog.Logger {
	diodeWriter := diode.NewWriter(os.Stdout, 100, 0, missed)

	logger := zerolog.New(diodeWriter).With().
		Caller().
		Timestamp().
		Logger()
	// ovewrite the global logger to our new fancy one
	log.Logger = logger
	return logger
}

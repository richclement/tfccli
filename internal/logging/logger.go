// Package logging provides a logr-based logger factory with settings-driven log levels.
package logging

import (
	"io"
	"log"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
)

// LogLevel represents the configured logging verbosity.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// verbosityForLevel maps log level strings to stdr verbosity levels.
// stdr uses V-levels where higher numbers mean more verbose.
// At verbosity V, messages with V(n) where n <= V are shown.
func verbosityForLevel(level LogLevel) int {
	switch level {
	case LevelDebug:
		return 2
	case LevelInfo:
		return 1
	case LevelWarn:
		return 0
	case LevelError:
		return -1
	default:
		// Default to info if unknown level
		return 1
	}
}

// NewLogger creates a logr.Logger using stdr with the specified log level.
// If debug is true, it overrides the level to debug.
// The logLevel parameter should be one of "debug", "info", "warn", "error".
func NewLogger(logLevel string, debug bool) logr.Logger {
	return NewLoggerWithOutput(logLevel, debug, os.Stderr)
}

// NewLoggerWithOutput creates a logr.Logger with a custom output writer.
// This is useful for testing or redirecting log output.
func NewLoggerWithOutput(logLevel string, debug bool, output io.Writer) logr.Logger {
	level := LogLevel(logLevel)

	// Override to debug if --debug flag is set
	if debug {
		level = LevelDebug
	}

	verbosity := verbosityForLevel(level)

	// Create a standard library logger that writes to the specified output
	stdLogger := log.New(output, "", log.LstdFlags)

	// Configure stdr with the verbosity level
	stdr.SetVerbosity(verbosity)

	return stdr.New(stdLogger)
}

// Discard returns a logger that discards all output.
// Useful for testing or when logging is not desired.
func Discard() logr.Logger {
	return logr.Discard()
}

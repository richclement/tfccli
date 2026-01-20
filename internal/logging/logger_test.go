package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewLogger_DebugOverridesSettings(t *testing.T) {
	// Given settings log_level = "info"
	logLevel := "info"
	debug := true

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// When we log at debug level (V(2))
	logger.V(2).Info("debug message")

	// Then the message should be logged (debug enabled by --debug flag)
	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("expected debug message to be logged when --debug is set, got: %q", output)
	}
}

func TestNewLogger_SettingsLogLevelRespected(t *testing.T) {
	// Given settings log_level = "error"
	logLevel := "error"
	debug := false

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// When we log at info level (V(1))
	logger.V(1).Info("info message")

	// Then logs do not include info-level messages
	output := buf.String()
	if strings.Contains(output, "info message") {
		t.Errorf("expected info message NOT to be logged at error level, got: %q", output)
	}
}

func TestNewLogger_InfoLevelLogsInfo(t *testing.T) {
	logLevel := "info"
	debug := false

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// V(1) messages should appear at info level
	logger.V(1).Info("info message")

	output := buf.String()
	if !strings.Contains(output, "info message") {
		t.Errorf("expected info message to be logged at info level, got: %q", output)
	}
}

func TestNewLogger_WarnLevelDoesNotLogInfo(t *testing.T) {
	logLevel := "warn"
	debug := false

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// V(1) (info) messages should NOT appear at warn level
	logger.V(1).Info("info message")

	output := buf.String()
	if strings.Contains(output, "info message") {
		t.Errorf("expected info message NOT to be logged at warn level, got: %q", output)
	}
}

func TestNewLogger_WarnLevelLogsWarn(t *testing.T) {
	logLevel := "warn"
	debug := false

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// V(0) messages should appear at warn level
	logger.V(0).Info("warn message")

	output := buf.String()
	if !strings.Contains(output, "warn message") {
		t.Errorf("expected warn message to be logged at warn level, got: %q", output)
	}
}

func TestNewLogger_DebugLevelLogsAll(t *testing.T) {
	logLevel := "debug"
	debug := false

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// All levels should be logged
	logger.V(2).Info("debug message")
	logger.V(1).Info("info message")
	logger.V(0).Info("warn message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("expected debug message at debug level, got: %q", output)
	}
	if !strings.Contains(output, "info message") {
		t.Errorf("expected info message at debug level, got: %q", output)
	}
	if !strings.Contains(output, "warn message") {
		t.Errorf("expected warn message at debug level, got: %q", output)
	}
}

func TestNewLogger_UnknownLevelDefaultsToInfo(t *testing.T) {
	logLevel := "unknown"
	debug := false

	var buf bytes.Buffer
	logger := NewLoggerWithOutput(logLevel, debug, &buf)

	// V(1) (info) should be logged
	logger.V(1).Info("info message")

	output := buf.String()
	if !strings.Contains(output, "info message") {
		t.Errorf("expected info message with unknown level (defaults to info), got: %q", output)
	}

	// V(2) (debug) should NOT be logged
	buf.Reset()
	logger.V(2).Info("debug message")

	output = buf.String()
	if strings.Contains(output, "debug message") {
		t.Errorf("expected debug message NOT logged with unknown level (defaults to info), got: %q", output)
	}
}

func TestVerbosityForLevel(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected int
	}{
		{LevelDebug, 2},
		{LevelInfo, 1},
		{LevelWarn, 0},
		{LevelError, -1},
		{"unknown", 1}, // defaults to info
	}

	for _, tc := range tests {
		got := verbosityForLevel(tc.level)
		if got != tc.expected {
			t.Errorf("verbosityForLevel(%q) = %d, want %d", tc.level, got, tc.expected)
		}
	}
}

func TestDiscard(t *testing.T) {
	logger := Discard()

	// Should not panic when logging
	logger.Info("this is discarded")
	logger.V(1).Info("also discarded")
	logger.Error(nil, "error also discarded")
}

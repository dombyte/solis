package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Level
	}{
		{"debug uppercase", "DEBUG", LevelDebug},
		{"debug lowercase", "debug", LevelDebug},
		{"debug mixed", "Debug", LevelDebug},
		{"info uppercase", "INFO", LevelInfo},
		{"info lowercase", "info", LevelInfo},
		{"warn uppercase", "WARN", LevelWarn},
		{"warn lowercase", "warn", LevelWarn},
		{"error uppercase", "ERROR", LevelError},
		{"error lowercase", "error", LevelError},
		{"fatal uppercase", "FATAL", LevelFatal},
		{"fatal lowercase", "fatal", LevelFatal},
		{"unknown returns info", "UNKNOWN", LevelInfo},
		{"empty returns info", "", LevelInfo},
		{"random returns info", "random", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevel_ToZerologLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    Level
		expected string
	}{
		{"debug", LevelDebug, "debug"},
		{"info", LevelInfo, "info"},
		{"warn", LevelWarn, "warn"},
		{"error", LevelError, "error"},
		{"fatal", LevelFatal, "fatal"},
		{"default (unknown)", Level(99), "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.level.toZerologLevel()
			resultStr := result.String()
			// Convert to lowercase for comparison
			if !strings.EqualFold(resultStr, tt.expected) {
				t.Errorf("Level(%d).toZerologLevel() = %s, want %s", tt.level, resultStr, tt.expected)
			}
		})
	}
}

func TestNewComponentLogger(t *testing.T) {
	// Initialize the logger first
	var buf bytes.Buffer
	Init(false, &buf, false, "INFO")

	// Test that NewComponentLogger doesn't panic
	logger := NewComponentLogger("test")
	// Check that logger is not nil by trying to use it
	if logger.GetLevel() == zerolog.NoLevel {
		t.Error("NewComponentLogger returned zero-value logger")
	}

	// Test with empty component name
	emptyLogger := NewComponentLogger("")
	if emptyLogger.GetLevel() == zerolog.NoLevel {
		t.Error("NewComponentLogger with empty string returned zero-value logger")
	}
}

func TestInit(t *testing.T) {
	// Test that Init doesn't panic with various configurations
	tests := []struct {
		name     string
		debug    bool
		pretty   bool
		logLevel string
	}{
		{"debug true, pretty false, no level", true, false, ""},
		{"debug false, pretty true, info", false, true, "INFO"},
		{"debug false, pretty false, debug", false, false, "DEBUG"},
		{"debug false, pretty false, warn", false, false, "WARN"},
		{"debug false, pretty false, error", false, false, "ERROR"},
		{"debug false, pretty false, fatal", false, false, "FATAL"},
		{"all defaults", false, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			// Init should not panic
			Init(tt.debug, &buf, tt.pretty, tt.logLevel)
		})
	}
}

// Package logging builds the application's zerolog logger. There is no global logger:
// internal/app constructs one root logger with New and hands every component a child
// scoped with Str("component", ...).
package logging

import (
	"io"
	"strings"

	"github.com/rs/zerolog"
)

// consoleTimeFormat is the timestamp layout of human-readable output.
const consoleTimeFormat = "15:04:05.000"

// ParseLevel maps a config level (DEBUG, INFO, WARN, ERROR, FATAL; case-insensitive)
// to a zerolog level. Unknown values fall back to info.
func ParseLevel(level string) zerolog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return zerolog.DebugLevel
	case "WARN":
		return zerolog.WarnLevel
	case "ERROR":
		return zerolog.ErrorLevel
	case "FATAL":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// New returns a root logger writing to w at the given level. pretty selects the
// human-readable console format instead of JSON.
func New(w io.Writer, level string, pretty bool) zerolog.Logger {
	out := w
	if pretty {
		out = zerolog.ConsoleWriter{Out: w, TimeFormat: consoleTimeFormat}
	}
	return zerolog.New(out).Level(ParseLevel(level)).With().Timestamp().Logger()
}

// Component returns a child of root scoped to a component name.
func Component(root zerolog.Logger, name string) zerolog.Logger {
	return root.With().Str("component", name).Logger()
}

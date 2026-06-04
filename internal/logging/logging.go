// Package logging provides a centralized logging setup for the Solis monitor application.
// It uses zerolog for structured, JSON-formatted logging with configurable debug mode.
package logging

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger is the application's main logger instance.
// It should be initialized once at startup and used throughout the application.
var globalLogger zerolog.Logger
var globalLoggerMu sync.RWMutex
var globalLoggerLevel zerolog.Level

// init sets up a default logger to prevent zero-value (no-op) logger issues
// before Init() is explicitly called.
func init() {
	// Initialize with a basic stderr logger
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()
	globalLogger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05.000"}).With().Timestamp().Caller().Logger()
	globalLoggerLevel = zerolog.InfoLevel
	zerolog.SetGlobalLevel(globalLoggerLevel)
	log.Logger = globalLogger
}

// Level represents the logging level.
type Level int

const (
	// LevelDebug enables debug logging (verbose).
	LevelDebug Level = iota
	// LevelInfo is the default logging level.
	LevelInfo
	// LevelWarn logs warnings.
	LevelWarn
	// LevelError logs errors.
	LevelError
	// LevelFatal logs fatal errors (application will exit).
	LevelFatal
)

// String converts Level to zerolog.Level.
func (l Level) toZerologLevel() zerolog.Level {
	switch l {
	case LevelDebug:
		return zerolog.DebugLevel
	case LevelInfo:
		return zerolog.InfoLevel
	case LevelWarn:
		return zerolog.WarnLevel
	case LevelError:
		return zerolog.ErrorLevel
	case LevelFatal:
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// ParseLogLevel parses a string log level and returns the corresponding Level.
// Returns LevelInfo if the level is not recognized.
func ParseLogLevel(levelStr string) Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// Init initializes the global logger with the specified configuration.
// It should be called once at application startup.
// Parameters:
//   - debug: if true, enables debug-level logging (deprecated, use logLevel)
//   - output: the io.Writer to write logs to (defaults to os.Stderr)
//   - pretty: if true, uses human-readable console output (for development)
//   - logLevel: the logging level to use (DEBUG, INFO, WARN, ERROR, FATAL)
func Init(debug bool, output io.Writer, pretty bool, logLevel ...string) {
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()

	// Determine the log level
	level := LevelInfo
	if debug {
		level = LevelDebug
	}
	if len(logLevel) > 0 && logLevel[0] != "" {
		level = ParseLogLevel(logLevel[0])
	}
	globalLoggerLevel = level.toZerologLevel()

	if pretty {
		// Human-readable console output for development
		globalLogger = zerolog.New(zerolog.ConsoleWriter{Out: output, TimeFormat: "15:04:05.000"}).With().Timestamp().Caller().Logger()
	} else {
		// JSON output for production
		globalLogger = zerolog.New(output).With().Timestamp().Caller().Logger()
	}

	zerolog.SetGlobalLevel(globalLoggerLevel)
	log.Logger = globalLogger
}

// NewComponentLogger creates a logger for a specific component.
// This is the recommended way to get a logger for a package or component.
//
// Example:
//
//	var logger = logging.NewComponentLogger("poller")
//	func (p *Poller) Start() {
//	    logger.Info().Msg("Starting poller")
//	}
func NewComponentLogger(component string) zerolog.Logger {
	globalLoggerMu.RLock()
	defer globalLoggerMu.RUnlock()
	return globalLogger.With().Str("component", component).Logger()
}

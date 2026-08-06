package logging

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Config holds logging configuration
type Config struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// Logger wraps zerolog.Logger for easier use
type Logger = zerolog.Logger

// shortCallerMarshalFunc formats caller to show only filename and line number
// This avoids long paths like vendor/git.futuregamestudio.net/...
func shortCallerMarshalFunc(pc uintptr, file string, line int) string {
	// Extract just the filename from the full path
	filename := filepath.Base(file)
	
	// If file is in vendor, try to extract package name
	if strings.Contains(file, "/vendor/") {
		// Extract package path after vendor
		parts := strings.Split(file, "/vendor/")
		if len(parts) > 1 {
			// Get the last 2 parts of the path (package/name.go)
			vendorPath := parts[1]
			pathParts := strings.Split(vendorPath, "/")
			if len(pathParts) >= 2 {
				// Use last 2 parts: package/name.go
				filename = strings.Join(pathParts[len(pathParts)-2:], "/")
			} else {
				filename = pathParts[len(pathParts)-1]
			}
		}
	}
	
	return filename + ":" + strconv.Itoa(line)
}

// New creates a new logger with the given configuration
func New(config Config) zerolog.Logger {
	level := parseLogLevel(config.Level)
	zerolog.SetGlobalLevel(level)

	// Set custom caller marshal function to shorten paths
	zerolog.CallerMarshalFunc = shortCallerMarshalFunc

	var output io.Writer = os.Stdout
	if config.Output == "stderr" {
		output = os.Stderr
	}

	if config.Format == "pretty" || config.Format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
		}
	}

	logger := zerolog.New(output).
		With().
		Timestamp().
		Caller().
		Logger()

	log.Logger = logger

	return logger
}

// NewDefault creates a logger with default settings
func NewDefault() zerolog.Logger {
	return New(Config{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	})
}

// parseLogLevel converts string log level to zerolog.Level
func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// WithTraceID adds trace_id to logger context
func WithTraceID(logger zerolog.Logger, traceID string) zerolog.Logger {
	return logger.With().Str("trace_id", traceID).Logger()
}

// WithUserID adds user_id to logger context
func WithUserID(logger zerolog.Logger, userID string) zerolog.Logger {
	return logger.With().Str("user_id", userID).Logger()
}

// WithGameCode adds game_code to logger context
func WithGameCode(logger zerolog.Logger, gameCode string) zerolog.Logger {
	return logger.With().Str("game_code", gameCode).Logger()
}

// WithComponent adds component name to logger context
func WithComponent(logger zerolog.Logger, component string) zerolog.Logger {
	return logger.With().Str("component", component).Logger()
}

// WithFields adds multiple fields to logger context
func WithFields(logger zerolog.Logger, fields map[string]interface{}) zerolog.Logger {
	ctx := logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return ctx.Logger()
}



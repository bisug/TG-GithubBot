// Package logging centralizes logger configuration so that every part of the
// application (and any code still using the stdlib "log" package) emits
// structured slog records to stdout.
package logging

import (
	"log"
	"log/slog"
	"os"
	"strings"
)

// stdlibWriter adapts the stdlib log package's Writer interface onto slog,
// so any log.Printf call that was missed (including in third-party libs)
// still lands in the structured log stream instead of unstructured stderr.
type stdlibWriter struct {
	logger *slog.Logger
}

func (w *stdlibWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg != "" {
		w.logger.Warn(msg, "source", "stdlib_log")
	}
	return len(p), nil
}

// Setup configures the global slog JSON logger (level configurable via the
// LOG_LEVEL env var: debug, info, warn, error — default info), redirects the
// stdlib log package into slog, and returns the new logger.
func Setup() *slog.Logger {
	var level slog.Level
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// Catch-all: anything still written through the stdlib log package
	// (ours or a dependency's) is routed into slog as a warning.
	log.SetFlags(0)
	log.SetOutput(&stdlibWriter{logger: logger})

	return logger
}

// Package logger provides a structured logger built on log/slog.
// Use New to create a logger and inject it via constructor arguments — do not use a global.
package logger

import (
	"log/slog"
	"os"
)

// New returns a *slog.Logger writing JSON to stdout.
// level is "debug", "info", "warn", or "error"; defaults to "info" on unknown values.
func New(env, level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	// Use text handler in development for readability; JSON elsewhere.
	var handler slog.Handler
	if env == "development" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

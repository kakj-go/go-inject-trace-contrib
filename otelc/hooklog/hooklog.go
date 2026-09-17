// Package hooklog provides the structured hook logger. It lives outside
// hooksupport because it imports log/slog: code injected into the stdlib
// "log" package must not transitively import log/slog (log/slog imports log,
// closing a cycle), while hooksupport must stay importable from there.
package hooklog

import (
	"log/slog"
	"os"
	"strings"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	Level: logLevel(),
}))

// Logger returns the hook logger. The level is fixed at process start from
// OTEL_LOG_LEVEL so hook packages observe it without init ordering.
func Logger() *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger
}

// logLevel returns the log level from environment variable.
func logLevel() slog.Level {
	levelStr := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_LOG_LEVEL")))
	switch levelStr {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

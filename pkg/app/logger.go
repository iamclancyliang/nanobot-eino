package app

import (
	"log/slog"
	"os"
)

// InitLogger configures the global slog default logger.
//
// Environment variables:
//   - LOG_LEVEL: debug, info, warn, error (default: info)
//   - LOG_FORMAT: json or text (default: text)
func InitLogger() {
	var level slog.Level
	if err := level.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}

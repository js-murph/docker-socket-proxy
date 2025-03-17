package logging

import (
	"log/slog"
	"os"
)

var (
	// Default logger instance
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
)

// SetLevel changes the logging level
func SetLevel(level slog.Level) {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

// GetLogger returns the configured logger
func GetLogger() *slog.Logger {
	return logger
}

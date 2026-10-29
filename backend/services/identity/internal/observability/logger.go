package observability

import (
	"log/slog"
	"os"
)

// NewLogger returns a structured JSON logger suitable for local containers and
// centralized log aggregation. Secrets and request bodies are never logged here.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
package logger

import (
	"log/slog"
	"os"
)

// Setup creates and returns a structured JSON logger configured
// for production use. It sets standard attributes available globally.
func Setup() *slog.Logger {
	// A robust production system would pull the minimum severity level from ENV.
	// We default to Info here but allow debugging if trace is needed.
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)

	// We append standard tags so all logs out of this service 
	// can be grouped easily in Datadog/Kibana/etc.
	attr := slog.Group("system",
		slog.String("component", "task-queue"),
		slog.Int("pid", os.Getpid()),
	)

	return slog.New(handler).With(attr)
}

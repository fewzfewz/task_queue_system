package logger

import (
	"log/slog"
	"os"
	"strings"
)

var sensitiveKeys = map[string]struct{}{
	"password":       {},
	"passwd":         {},
	"pass":           {},
	"secret":         {},
	"client_secret":  {},
	"token":          {},
	"auth_token":     {},
	"access_token":   {},
	"refresh_token":  {},
	"api_key":        {},
	"apikey":         {},
	"authorization":  {},
	"auth":           {},
	"cookie":         {},
	"private_key":    {},
	"session_token":  {},
}

// maskSensitiveAttr redacts sensitive fields from structured logs.
func maskSensitiveAttr(groups []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(strings.TrimSpace(a.Key))
	if _, ok := sensitiveKeys[key]; ok {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

// Setup creates and returns a structured JSON logger configured
// for production use. It sets standard attributes available globally
// and masks sensitive credentials.
func Setup() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: maskSensitiveAttr,
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

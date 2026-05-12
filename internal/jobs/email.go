package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/worker/plugin"
)

// EmailJob holds the typed fields expected inside an "email" job's Payload.
type EmailJob struct {
	To      string
	Subject string
	Body    string
}

// EmailPlugin implements plugin.JobPlugin for jobs of type "email".
type EmailPlugin struct {
	logger *slog.Logger
}

// NewEmailPlugin creates an EmailPlugin with the provided logger.
func NewEmailPlugin(logger *slog.Logger) *EmailPlugin {
	return &EmailPlugin{logger: logger}
}

func init() {
	// Note: In a real production system, you might want to pass a global logger
	// or use a factory. For this demo, we use a default slog logger for auto-registration.
	p := NewEmailPlugin(slog.Default())
	plugin.RegisterGlobal(p)
}

func (p *EmailPlugin) Type() string {
	return "email"
}

// Execute extracts email fields from the payload and simulates sending.
func (p *EmailPlugin) Execute(ctx context.Context, payload map[string]interface{}) (interface{}, error) {
	to, _ := payload["to"].(string)
	subject, _ := payload["subject"].(string)

	if to == "" {
		return nil, fmt.Errorf("email plugin: missing required field 'to'")
	}

	p.logger.Info("sending email", "to", to, "subject", subject)

	// --- Simulated work ---
	time.Sleep(50 * time.Millisecond) // simulate SMTP round-trip
	// ----------------------

	p.logger.Info("email sent successfully", "to", to)
	return fmt.Sprintf("email sent to %s", to), nil
}

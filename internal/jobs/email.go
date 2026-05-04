package jobs

import (
	"fmt"
	"log/slog"
	"time"
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

func (p *EmailPlugin) Type() string {
	return "email"
}

// Execute extracts email fields from the payload and simulates sending.
func (p *EmailPlugin) Execute(payload map[string]interface{}) error {
	to, _ := payload["to"].(string)
	subject, _ := payload["subject"].(string)

	if to == "" {
		return fmt.Errorf("email plugin: missing required field 'to'")
	}

	p.logger.Info("sending email", "to", to, "subject", subject)

	// --- Simulated work ---
	time.Sleep(50 * time.Millisecond) // simulate SMTP round-trip
	// ----------------------

	p.logger.Info("email sent successfully", "to", to)
	return nil
}

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

// EmailHandler implements plugin.JobPlugin for jobs of type "email".
type EmailHandler struct {
	logger *slog.Logger
}

// NewEmailHandler creates an EmailHandler with the provided logger.
func NewEmailHandler(logger *slog.Logger) *EmailHandler {
	return &EmailHandler{logger: logger}
}

func (h *EmailHandler) Type() string {
	return "email"
}

// Execute extracts email fields from the payload and simulates sending.
func (h *EmailHandler) Execute(payload map[string]interface{}) error {
	to, _ := payload["to"].(string)
	subject, _ := payload["subject"].(string)

	if to == "" {
		return fmt.Errorf("email handler: missing required field 'to'")
	}

	h.logger.Info("sending email", "to", to, "subject", subject)

	// --- Simulated work ---
	time.Sleep(50 * time.Millisecond) // simulate SMTP round-trip
	// ----------------------

	h.logger.Info("email sent successfully", "to", to)
	return nil
}

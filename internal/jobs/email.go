package jobs

import (
	"context"
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

// EmailHandler handles jobs of type "email".
type EmailHandler struct {
	logger *slog.Logger
}

// NewEmailHandler creates an EmailHandler with the provided logger.
func NewEmailHandler(logger *slog.Logger) *EmailHandler {
	return &EmailHandler{logger: logger}
}

// Handle extracts email fields from the job payload and simulates sending.
func (h *EmailHandler) Handle(ctx context.Context, job *Job) error {
	to, _ := job.Payload["to"].(string)
	subject, _ := job.Payload["subject"].(string)

	if to == "" {
		return fmt.Errorf("email handler: missing required field 'to' in job %s", job.ID)
	}

	h.logger.Info("sending email",
		"job_id", job.ID,
		"to", to,
		"subject", subject,
	)

	// --- Simulated work ---
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond): // simulate SMTP round-trip
	}
	// ----------------------

	h.logger.Info("email sent successfully", "job_id", job.ID, "to", to)
	return nil
}

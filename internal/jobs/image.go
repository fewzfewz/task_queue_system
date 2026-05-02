package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ImageHandler handles jobs of type "image".
type ImageHandler struct {
	logger *slog.Logger
}

// NewImageHandler creates an ImageHandler with the provided logger.
func NewImageHandler(logger *slog.Logger) *ImageHandler {
	return &ImageHandler{logger: logger}
}

// Handle extracts image processing fields from the job payload and simulates processing.
func (h *ImageHandler) Handle(ctx context.Context, job *Job) error {
	sourceURL, _ := job.Payload["source_url"].(string)
	operation, _ := job.Payload["operation"].(string) // e.g. "resize", "compress", "watermark"

	if sourceURL == "" {
		return fmt.Errorf("image handler: missing required field 'source_url' in job %s", job.ID)
	}
	if operation == "" {
		operation = "process"
	}

	h.logger.Info("processing image",
		"job_id", job.ID,
		"source_url", sourceURL,
		"operation", operation,
	)

	// --- Simulated work ---
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond): // simulate image processing latency
	}
	// ----------------------

	h.logger.Info("image processed successfully",
		"job_id", job.ID,
		"source_url", sourceURL,
		"operation", operation,
	)
	return nil
}

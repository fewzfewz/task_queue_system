package jobs

import (
	"fmt"
	"log/slog"
	"time"
)

// ImageHandler implements plugin.JobPlugin for jobs of type "image".
type ImageHandler struct {
	logger *slog.Logger
}

// NewImageHandler creates an ImageHandler with the provided logger.
func NewImageHandler(logger *slog.Logger) *ImageHandler {
	return &ImageHandler{logger: logger}
}

func (h *ImageHandler) Type() string {
	return "image"
}

// Execute extracts image processing fields from the payload and simulates processing.
func (h *ImageHandler) Execute(payload map[string]interface{}) error {
	sourceURL, _ := payload["source_url"].(string)
	operation, _ := payload["operation"].(string) // e.g. "resize", "compress", "watermark"

	if sourceURL == "" {
		return fmt.Errorf("image handler: missing required field 'source_url'")
	}
	if operation == "" {
		operation = "process"
	}

	h.logger.Info("processing image", "source_url", sourceURL, "operation", operation)

	// --- Simulated work ---
	time.Sleep(100 * time.Millisecond) // simulate image processing latency
	// ----------------------

	h.logger.Info("image processed successfully", "source_url", sourceURL, "operation", operation)
	return nil
}

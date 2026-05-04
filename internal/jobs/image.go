package jobs

import (
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/worker/plugin"
)

// ImagePlugin implements plugin.JobPlugin for jobs of type "image".
type ImagePlugin struct {
	logger *slog.Logger
}

// NewImagePlugin creates an ImagePlugin with the provided logger.
func NewImagePlugin(logger *slog.Logger) *ImagePlugin {
	return &ImagePlugin{logger: logger}
}

func init() {
	p := NewImagePlugin(slog.Default())
	plugin.RegisterGlobal(p)
}

func (p *ImagePlugin) Type() string {
	return "image"
}

// Execute extracts image processing fields from the payload and simulates processing.
func (p *ImagePlugin) Execute(payload map[string]interface{}) (interface{}, error) {
	sourceURL, _ := payload["source_url"].(string)
	operation, _ := payload["operation"].(string) // e.g. "resize", "compress", "watermark"

	if sourceURL == "" {
		return nil, fmt.Errorf("image plugin: missing required field 'source_url'")
	}
	if operation == "" {
		operation = "process"
	}

	p.logger.Info("processing image", "source_url", sourceURL, "operation", operation)

	// --- Simulated work ---
	time.Sleep(100 * time.Millisecond) // simulate image processing latency
	// ----------------------

	p.logger.Info("image processed successfully", "source_url", sourceURL, "operation", operation)
	return map[string]string{
		"status":    "processed",
		"operation": operation,
		"url":       sourceURL,
	}, nil
}

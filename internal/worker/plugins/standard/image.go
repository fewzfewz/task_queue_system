package standard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
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
func (p *ImagePlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	if job.Version > 1 {
		p.logger.Warn("unsupported job version, falling back to v1 logic", "version", job.Version, "job_id", job.ID)
	}

	payload := job.Payload
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
	if sleepMs, ok := payload["sleep_ms"]; ok {
		if ms, ok := sleepMs.(float64); ok && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		time.Sleep(100 * time.Millisecond)
	}
	// ----------------------

	p.logger.Info("image processed successfully", "source_url", sourceURL, "operation", operation)
	return map[string]string{
		"status":    "processed",
		"operation": operation,
		"url":       sourceURL,
	}, nil
}

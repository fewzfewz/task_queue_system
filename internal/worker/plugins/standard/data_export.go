package standard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// DataExportPlugin implements plugin.JobPlugin for jobs of type "data_export".
// Simulates a long-running data export task and uses the Progress callback.
type DataExportPlugin struct {
	logger *slog.Logger
}

func NewDataExportPlugin(logger *slog.Logger) *DataExportPlugin {
	return &DataExportPlugin{logger: logger}
}

func init() {
	plugin.RegisterGlobal(NewDataExportPlugin(slog.Default()))
}

func (p *DataExportPlugin) Type() string {
	return "data_export"
}

func (p *DataExportPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	format, _ := job.Payload["format"].(string)
	if format == "" {
		format = "csv"
	}
	
	p.logger.Info("starting data export", "format", format, "job_id", job.ID)

	// Simulate a long-running task with progress updates
	totalRows := 1000
	batchSize := 200

	for i := 0; i < totalRows; i += batchSize {
		// Check for cancellation
		select {
		case <-ctx.Done():
			p.logger.Warn("data export cancelled", "job_id", job.ID)
			return nil, ctx.Err()
		default:
		}

		// Simulate work
		time.Sleep(500 * time.Millisecond)

		// Calculate and report progress
		progress := float64(i+batchSize) / float64(totalRows) * 100.0
		plugin.ReportProgress(ctx, progress)
		p.logger.Debug("data export progress", "progress", progress)
	}

	url := fmt.Sprintf("https://storage.example.com/exports/export_%s.%s", job.ID, format)
	p.logger.Info("data export completed", "url", url)

	return map[string]string{
		"download_url": url,
		"format":       format,
		"status":       "ready",
	}, nil
}

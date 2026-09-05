package standard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// PDFPlugin implements plugin.JobPlugin for jobs of type "pdf".
// Simulates rendering HTML or a URL into a PDF document.
type PDFPlugin struct {
	logger *slog.Logger
}

func NewPDFPlugin(logger *slog.Logger) *PDFPlugin {
	return &PDFPlugin{logger: logger}
}

func init() {
	plugin.RegisterGlobal(NewPDFPlugin(slog.Default()))
}

func (p *PDFPlugin) Type() string {
	return "pdf"
}

func (p *PDFPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	html, _ := job.Payload["html"].(string)
	url, _ := job.Payload["url"].(string)

	if html == "" && url == "" {
		return nil, fmt.Errorf("pdf plugin: must provide either 'html' or 'url' in payload")
	}

	source := "html content"
	if url != "" {
		source = url
	}

	p.logger.Info("starting pdf generation", "source", source, "job_id", job.ID)

	// Simulate rendering time
	select {
	case <-ctx.Done():
		p.logger.Warn("pdf generation cancelled", "job_id", job.ID)
		return nil, ctx.Err()
	case <-time.After(800 * time.Millisecond):
	}

	pdfURL := fmt.Sprintf("https://storage.example.com/pdfs/rendered_%s.pdf", job.ID)
	p.logger.Info("pdf generation completed", "pdf_url", pdfURL)

	return map[string]string{
		"pdf_url": pdfURL,
		"status":  "success",
	}, nil
}

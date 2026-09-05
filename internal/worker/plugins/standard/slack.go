package standard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// SlackPlugin implements plugin.JobPlugin for jobs of type "slack".
// Sends a message to a Slack webhook.
type SlackPlugin struct {
	logger *slog.Logger
}

func NewSlackPlugin(logger *slog.Logger) *SlackPlugin {
	return &SlackPlugin{logger: logger}
}

func init() {
	plugin.RegisterGlobal(NewSlackPlugin(slog.Default()))
}

func (p *SlackPlugin) Type() string {
	return "slack"
}

func (p *SlackPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	webhookURL, _ := job.Payload["webhook_url"].(string)
	text, _ := job.Payload["text"].(string)

	if text == "" {
		return nil, fmt.Errorf("slack plugin: missing required field 'text'")
	}

	// Use environment variable if not provided in payload
	if webhookURL == "" {
		webhookURL = os.Getenv("SLACK_WEBHOOK_URL")
	}

	if webhookURL == "" {
		p.logger.Info("slack notification simulated (no webhook_url provided)", "text", text)
		time.Sleep(50 * time.Millisecond) // Simulate network delay
		return "slack message simulated", nil
	}

	// Real delivery
	payload := map[string]interface{}{
		"text": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("slack plugin: failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("slack plugin: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack plugin: http post failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("slack plugin: received error status %d", resp.StatusCode)
	}

	p.logger.Info("slack notification sent", "status_code", resp.StatusCode)
	return "slack message delivered", nil
}

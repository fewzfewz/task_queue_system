package standard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// HTTPPlugin executes jobs by making an HTTP request defined in the payload.
// Payload fields: url (required), method (default POST), headers (map), body (any).
type HTTPPlugin struct {
	logger *slog.Logger
	client *http.Client
}

func NewHTTPPlugin(logger *slog.Logger) *HTTPPlugin {
	return &HTTPPlugin{
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func init() {
	p := NewHTTPPlugin(slog.Default())
	plugin.RegisterGlobal(p)
}

func (p *HTTPPlugin) Type() string { return "http" }

func (p *HTTPPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	payload := job.Payload
	url, _ := payload["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("http plugin: missing required field 'url'")
	}

	method, _ := payload["method"].(string)
	if method == "" {
		method = http.MethodPost
	}
	method = strings.ToUpper(method)

	var bodyReader io.Reader
	if body, ok := payload["body"]; ok && body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("http plugin: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http plugin: build request: %w", err)
	}

	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if headers, ok := payload["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}
	req.Header.Set("X-Task-Queue-Job-ID", job.ID)
	req.Header.Set("X-Task-Queue-Tenant-ID", job.TenantID)

	p.logger.Info("executing http job", "url", url, "method", method, "job_id", job.ID)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http plugin: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http plugin: upstream returned %d: %s", resp.StatusCode, string(respBody))
	}

	return map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(respBody),
	}, nil
}

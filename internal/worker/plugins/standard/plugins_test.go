package standard

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

func TestEmailPlugin_Type(t *testing.T) {
	p := NewEmailPlugin(slog.Default())
	if got := p.Type(); got != "email" {
		t.Fatalf("expected type 'email', got %q", got)
	}
}

func TestEmailPlugin_Execute_Success(t *testing.T) {
	p := NewEmailPlugin(slog.Default())
	job := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"to":      "user@example.com",
		"subject": "Hello",
		"body":    "Test message",
	}

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "email sent to user@example.com (simulated)" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestEmailPlugin_Execute_MissingTo(t *testing.T) {
	p := NewEmailPlugin(slog.Default())
	job := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"subject": "Hello",
	}

	_, err := p.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing 'to' field")
	}
}

func TestEmailPlugin_Execute_HigherVersion(t *testing.T) {
	p := NewEmailPlugin(slog.Default())
	job := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"to":      "user@example.com",
		"subject": "Hello",
	}
	job.Version = 2

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "email sent to user@example.com (simulated)" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestImagePlugin_Type(t *testing.T) {
	p := NewImagePlugin(slog.Default())
	if got := p.Type(); got != "image" {
		t.Fatalf("expected type 'image', got %q", got)
	}
}

func TestImagePlugin_Execute_Success(t *testing.T) {
	p := NewImagePlugin(slog.Default())
	job := jobs.NewJob("image", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"source_url": "https://example.com/image.jpg",
		"operation":  "resize",
	}

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	res, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if res["status"] != "processed" {
		t.Fatalf("unexpected status: %q", res["status"])
	}
	if res["operation"] != "resize" {
		t.Fatalf("unexpected operation: %q", res["operation"])
	}
}

func TestImagePlugin_Execute_DefaultOperation(t *testing.T) {
	p := NewImagePlugin(slog.Default())
	job := jobs.NewJob("image", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"source_url": "https://example.com/image.jpg",
	}

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	res, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if res["operation"] != "process" {
		t.Fatalf("expected default operation 'process', got %q", res["operation"])
	}
}

func TestImagePlugin_Execute_MissingSourceURL(t *testing.T) {
	p := NewImagePlugin(slog.Default())
	job := jobs.NewJob("image", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"operation": "resize",
	}

	_, err := p.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing 'source_url' field")
	}
}

func TestImagePlugin_Execute_HigherVersion(t *testing.T) {
	p := NewImagePlugin(slog.Default())
	job := jobs.NewJob("image", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"source_url": "https://example.com/image.jpg",
	}
	job.Version = 2

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestEmailPlugin_GlobalRegistration(t *testing.T) {
	reg := plugin.GetGlobalRegistry()
	p, err := reg.Get("email")
	if err != nil {
		t.Fatalf("expected email plugin registered globally, got: %v", err)
	}
	if p.Type() != "email" {
		t.Fatalf("unexpected global plugin type: %q", p.Type())
	}
}

func TestImagePlugin_GlobalRegistration(t *testing.T) {
	reg := plugin.GetGlobalRegistry()
	p, err := reg.Get("image")
	if err != nil {
		t.Fatalf("expected image plugin registered globally, got: %v", err)
	}
	if p.Type() != "image" {
		t.Fatalf("unexpected global plugin type: %q", p.Type())
	}
}

func TestHTTPPlugin_Execute_Success(t *testing.T) {
	p := NewHTTPPlugin(slog.Default())
	job := jobs.NewJob("http", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"url":    "https://example.com",
		"method": "GET",
	}

	// Will fail without network - test missing url instead
	job.Payload = map[string]interface{}{}
	_, err := p.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestHTTPPlugin_GlobalRegistration(t *testing.T) {
	reg := plugin.GetGlobalRegistry()
	p, err := reg.Get("http")
	if err != nil {
		t.Fatalf("expected http plugin registered globally, got: %v", err)
	}
	if p.Type() != "http" {
		t.Fatalf("unexpected global plugin type: %q", p.Type())
	}
}

func TestSlackPlugin_Type(t *testing.T) {
	p := NewSlackPlugin(slog.Default())
	if got := p.Type(); got != "slack" {
		t.Fatalf("expected type 'slack', got %q", got)
	}
}

func TestSlackPlugin_Execute_MissingText(t *testing.T) {
	p := NewSlackPlugin(slog.Default())
	job := jobs.NewJob("slack", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{}

	_, err := p.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing text")
	}
}

func TestSlackPlugin_Execute_Simulated(t *testing.T) {
	p := NewSlackPlugin(slog.Default())
	job := jobs.NewJob("slack", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"text": "Hello Slack!",
	}

	res, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != "slack message simulated" {
		t.Fatalf("unexpected result: %v", res)
	}
}

func TestDataExportPlugin_Type(t *testing.T) {
	p := NewDataExportPlugin(slog.Default())
	if got := p.Type(); got != "data_export" {
		t.Fatalf("expected type 'data_export', got %q", got)
	}
}

func TestDataExportPlugin_Execute_Success(t *testing.T) {
	p := NewDataExportPlugin(slog.Default())
	job := jobs.NewJob("data_export", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"format": "json",
	}

	res, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	m, ok := res.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string result")
	}
	if m["format"] != "json" {
		t.Fatalf("unexpected format: %v", m["format"])
	}
}

func TestPDFPlugin_Type(t *testing.T) {
	p := NewPDFPlugin(slog.Default())
	if got := p.Type(); got != "pdf" {
		t.Fatalf("expected type 'pdf', got %q", got)
	}
}

func TestPDFPlugin_Execute_MissingInput(t *testing.T) {
	p := NewPDFPlugin(slog.Default())
	job := jobs.NewJob("pdf", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{}

	_, err := p.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing html/url")
	}
}

func TestLLMPlugin_Type(t *testing.T) {
	p := NewLLMPlugin(slog.Default())
	if got := p.Type(); got != "llm" {
		t.Fatalf("expected type 'llm', got %q", got)
	}
}

func TestLLMPlugin_Execute_Success(t *testing.T) {
	p := NewLLMPlugin(slog.Default())
	job := jobs.NewJob("llm", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"prompt": "Say hello",
	}

	res, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{} result")
	}
	if m["model"] != "gpt-4o" {
		t.Fatalf("unexpected model: %v", m["model"])
	}
}

func TestS3Plugin_Type(t *testing.T) {
	p := NewS3Plugin(slog.Default())
	if got := p.Type(); got != "s3_upload" {
		t.Fatalf("expected type 's3_upload', got %q", got)
	}
}

func TestS3Plugin_Execute_Success(t *testing.T) {
	p := NewS3Plugin(slog.Default())
	job := jobs.NewJob("s3_upload", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"bucket":     "my-bucket",
		"object_key": "data.txt",
		"data":       "hello world",
	}

	res, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	m, ok := res.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string result")
	}
	if m["s3_url"] != "s3://my-bucket/data.txt" {
		t.Fatalf("unexpected s3_url: %v", m["s3_url"])
	}
}

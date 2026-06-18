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
	job := jobs.NewJob("email", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"to":      "user@example.com",
		"subject": "Hello",
		"body":    "Test message",
	}

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "email sent to user@example.com" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestEmailPlugin_Execute_MissingTo(t *testing.T) {
	p := NewEmailPlugin(slog.Default())
	job := jobs.NewJob("email", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
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
	job := jobs.NewJob("email", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	job.Payload = map[string]interface{}{
		"to":      "user@example.com",
		"subject": "Hello",
	}
	job.Version = 2

	result, err := p.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "email sent to user@example.com" {
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
	job := jobs.NewJob("image", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
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
	job := jobs.NewJob("image", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
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
	job := jobs.NewJob("image", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
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
	job := jobs.NewJob("image", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
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

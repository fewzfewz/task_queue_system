package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSetup(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	logger := Setup()
	logger.Info("hello", "key", "value")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected log output, got empty")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("expected valid JSON: %v\nraw: %s", err, line)
	}

	if payload["msg"] != "hello" {
		t.Fatalf("expected msg='hello', got %v", payload["msg"])
	}
	if payload["key"] != "value" {
		t.Fatalf("expected key='value', got %v", payload["key"])
	}

	sys, ok := payload["system"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'system' group attribute")
	}
	if sys["component"] != "task-queue" {
		t.Fatalf("expected component=task-queue, got %v", sys["component"])
	}
	pid, ok := sys["pid"].(float64)
	if !ok || pid <= 0 {
		t.Fatalf("expected positive pid, got %v", sys["pid"])
	}
}

func TestSetupReturnsLogger(t *testing.T) {
	logger := Setup()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

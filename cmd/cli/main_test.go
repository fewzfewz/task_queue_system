package main

import (
	"os"
	"strings"
	"testing"
)

func captureOutput(fn func()) string {
	// Helper to capture stdout/stderr by running the function with modified args
	// In practice we test the run function logic
	return ""
}

func TestRunInvalidConfig(t *testing.T) {
	os.Setenv("STORE_BACKEND", "invalid")
	defer os.Unsetenv("STORE_BACKEND")

	err := run()
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
	if !strings.Contains(err.Error(), "invalid configuration") {
		t.Fatalf("expected config validation error, got: %v", err)
	}
}

func TestRunNoArgs(t *testing.T) {
	os.Unsetenv("STORE_BACKEND")
	// Reset to valid config
	os.Setenv("STORE_BACKEND", "redis")
	defer os.Unsetenv("STORE_BACKEND")

	os.Args = []string{"cli"}
	err := run()
	if err == nil {
		t.Fatal("expected error for no subcommand, got nil")
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	os.Setenv("STORE_BACKEND", "redis")
	defer os.Unsetenv("STORE_BACKEND")

	os.Args = []string{"cli", "unknown"}
	err := run()
	if err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}
}

func TestMainCompiles(t *testing.T) {
	var _ func() error = run
}

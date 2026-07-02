package main

import (
	"os"
	"testing"
)

func TestRunInvalidConfig(t *testing.T) {
	// Set an invalid config to trigger validation failure
	os.Setenv("STORE_BACKEND", "invalid")
	defer os.Unsetenv("STORE_BACKEND")

	err := run()
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
}

func TestMainCompiles(t *testing.T) {
	// Verify the package builds by checking a known symbol exists
	var _ func() error = run
}

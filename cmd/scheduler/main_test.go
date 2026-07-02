package main

import (
	"os"
	"testing"
)

func TestRunInvalidConfig(t *testing.T) {
	os.Setenv("STORE_BACKEND", "invalid")
	defer os.Unsetenv("STORE_BACKEND")

	err := run()
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
}

func TestMainCompiles(t *testing.T) {
	var _ func() error = run
}

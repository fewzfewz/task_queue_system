package errors

import (
	"errors"
	"testing"
)

func TestAppError(t *testing.T) {
	t.Run("NewNotFound", func(t *testing.T) {
		err := NewNotFound("job", "123")
		if err.Code != CodeNotFound {
			t.Fatalf("expected code NOT_FOUND, got %s", err.Code)
		}
		if err.Error() != "[NOT_FOUND] job not found: 123" {
			t.Fatalf("unexpected message: %s", err.Error())
		}
	})

	t.Run("NewInvalidArgument", func(t *testing.T) {
		err := NewInvalidArgument("bad input")
		if err.Code != CodeInvalidArgument {
			t.Fatalf("expected code INVALID_ARGUMENT, got %s", err.Code)
		}
	})

	t.Run("NewInternal with cause", func(t *testing.T) {
		cause := errors.New("db failure")
		err := NewInternal("query failed", cause)
		if err.Code != CodeInternal {
			t.Fatalf("expected code INTERNAL_ERROR, got %s", err.Code)
		}
		if !errors.Is(err.Err, cause) {
			t.Fatal("expected cause to be preserved")
		}
		if err.Error() != "[INTERNAL_ERROR] query failed: db failure" {
			t.Fatalf("unexpected message: %s", err.Error())
		}
	})

	t.Run("NewInternal without cause", func(t *testing.T) {
		err := NewInternal("oops", nil)
		if err.Error() != "[INTERNAL_ERROR] oops" {
			t.Fatalf("unexpected message: %s", err.Error())
		}
	})

	t.Run("NewQueueFull", func(t *testing.T) {
		err := NewQueueFull()
		if err.Code != CodeQueueFull {
			t.Fatalf("expected code QUEUE_FULL, got %s", err.Code)
		}
	})

	t.Run("NewTooManyRequests", func(t *testing.T) {
		err := NewTooManyRequests("slow down")
		if err.Code != CodeTooManyRequests {
			t.Fatalf("expected code TOO_MANY_REQUESTS, got %s", err.Code)
		}
	})

	t.Run("NewConflict", func(t *testing.T) {
		err := NewConflict("duplicate")
		if err.Code != CodeConflict {
			t.Fatalf("expected code CONFLICT, got %s", err.Code)
		}
	})

	t.Run("NewForbidden", func(t *testing.T) {
		err := NewForbidden("no access")
		if err.Code != CodePermissionDenied {
			t.Fatalf("expected code PERMISSION_DENIED, got %s", err.Code)
		}
	})
}

func TestIsCode(t *testing.T) {
	err := NewNotFound("job", "1")
	if !IsCode(err, CodeNotFound) {
		t.Fatal("IsCode should match NOT_FOUND")
	}
	if IsCode(err, CodeInternal) {
		t.Fatal("IsCode should not match INTERNAL_ERROR")
	}

	if IsCode(errors.New("plain error"), CodeNotFound) {
		t.Fatal("IsCode should return false for plain error")
	}
}

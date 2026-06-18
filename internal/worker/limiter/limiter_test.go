package limiter

import (
	"context"
	"testing"
	"time"
)

func TestTokenBucketLimiter_New(t *testing.T) {
	l := NewTokenBucketLimiter(10)
	if l == nil {
		t.Fatal("expected non-nil limiter")
	}
}

func TestTokenBucketLimiter_NewZero(t *testing.T) {
	l := NewTokenBucketLimiter(0)
	if l != nil {
		t.Fatal("expected nil limiter for rate <= 0")
	}
}

func TestTokenBucketLimiter_Wait(t *testing.T) {
	l := NewTokenBucketLimiter(100)
	ctx := context.Background()

	// Should be able to consume tokens immediately (burst)
	for i := 0; i < 10; i++ {
		err := l.Wait(ctx)
		if err != nil {
			t.Fatalf("unexpected error at attempt %d: %v", i, err)
		}
	}
}

func TestTokenBucketLimiter_CancelledContext(t *testing.T) {
	l := NewTokenBucketLimiter(100)

	ctx, cancel := context.WithCancel(context.Background())

	// Spawn a goroutine that blocks on Wait, will be unblocked by cancel
	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Wait(ctx)
	}()

	// Cancel while it's waiting
	cancel()

	err := <-errCh
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestTokenBucketLimiter_Timeout(t *testing.T) {
	l := NewTokenBucketLimiter(100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	// Consume tokens as fast as possible until context times out
	for {
		if err := l.Wait(ctx); err != nil {
			return // timeout expected
		}
	}
}

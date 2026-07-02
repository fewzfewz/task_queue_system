package tracing

import (
	"context"
	"testing"
)

func TestStartAndFromContext(t *testing.T) {
	ctx := context.Background()

	ctx2, id := Start(ctx)
	if id == "" {
		t.Fatal("expected non-empty trace ID")
	}

	got, ok := FromContext(ctx2)
	if !ok {
		t.Fatal("expected trace ID in context")
	}
	if got != id {
		t.Fatalf("expected %s, got %s", id, got)
	}

	_, ok = FromContext(ctx)
	if ok {
		t.Fatal("expected no trace ID in original context")
	}
}

func TestStartTwiceReturnsSame(t *testing.T) {
	ctx := context.Background()
	ctx, id1 := Start(ctx)

	_, id2 := Start(ctx)
	if id2 != id1 {
		t.Fatalf("expected same trace ID %s, got %s", id1, id2)
	}
}

func TestInject(t *testing.T) {
	ctx := context.Background()

	id := Inject(ctx)
	if id != "" {
		t.Fatal("expected empty trace ID from empty context")
	}

	ctx, traceID := Start(ctx)
	id = Inject(ctx)
	if id != traceID {
		t.Fatalf("expected %s, got %s", traceID, id)
	}
}

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()

	ctx = WithTraceID(ctx, "test-trace-123")
	id, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected trace ID in context")
	}
	if id != "test-trace-123" {
		t.Fatalf("expected test-trace-123, got %s", id)
	}

	ctx2 := WithTraceID(ctx, "")
	if ctx2 != ctx {
		t.Fatal("expected unchanged context for empty trace ID")
	}
}

func TestInitNoEndpoint(t *testing.T) {
	ctx := context.Background()
	shutdown, err := Init(ctx, "")
	if err != nil {
		t.Fatalf("expected no error with empty endpoint, got: %v", err)
	}
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestForceFlushNoop(t *testing.T) {
	ForceFlush(context.Background())
}

func TestNewID(t *testing.T) {
	id := newID()
	if len(id) != 32 {
		t.Fatalf("expected 32-char hex string, got %d: %s", len(id), id)
	}
}

func TestNewIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newID()
		if ids[id] {
			t.Fatal("duplicate trace ID generated")
		}
		ids[id] = true
	}
}

func TestNewIDOverride(t *testing.T) {
	orig := newID
	t.Cleanup(func() { newID = orig })

	newID = func() string { return "fixed-id" }

	ctx, id := Start(context.Background())
	if id != "fixed-id" {
		t.Fatalf("expected fixed-id, got %s", id)
	}

	got, _ := FromContext(ctx)
	if got != "fixed-id" {
		t.Fatalf("expected fixed-id from context, got %s", got)
	}
}

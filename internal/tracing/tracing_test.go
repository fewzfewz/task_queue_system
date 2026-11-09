package tracing

import (
	"context"
	"sync"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestStartAndFromContext(t *testing.T) {
	ctx := context.Background()

	ctx2, span := Start(ctx)
	defer span.End()
	id := Inject(ctx2)
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
	ctx, span1 := Start(ctx)
	defer span1.End()
	id1 := Inject(ctx)

	_, span2 := Start(ctx)
	defer span2.End()
	id2 := Inject(ctx)
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

	ctx, span := Start(ctx)
	defer span.End()
	traceID := Inject(ctx)
	id = Inject(ctx)
	if id != traceID {
		t.Fatalf("expected %s, got %s", traceID, id)
	}
}

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()

	ctx = WithTraceID(ctx, "0123456789abcdef0123456789abcdef")
	id, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected trace ID in context")
	}
	if id != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("expected 0123456789abcdef0123456789abcdef, got %s", id)
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

	ctx, span := Start(context.Background())
	defer span.End()
	id := Inject(ctx)
	if id != "fixed-id" {
		t.Fatalf("expected fixed-id, got %s", id)
	}

	got, _ := FromContext(ctx)
	if got != "fixed-id" {
		t.Fatalf("expected fixed-id from context, got %s", got)
	}
}

type testSpanExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *testSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *testSpanExporter) Shutdown(_ context.Context) error {
	return nil
}

func TestSpanDurationNonZero(t *testing.T) {
	exporter := &testSpanExporter{}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	SetTracerProvider(provider)
	defer func() {
		_ = provider.Shutdown(context.Background())
		SetTracerProvider(nil)
	}()

	_, span := Start(context.Background(), "test-job-operation")
	time.Sleep(15 * time.Millisecond)
	span.End()

	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	if len(exporter.spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(exporter.spans))
	}
	s := exporter.spans[0]
	duration := s.EndTime().Sub(s.StartTime())
	if duration < 10*time.Millisecond {
		t.Fatalf("expected span duration >= 10ms, got %v", duration)
	}
	if s.Name() != "test-job-operation" {
		t.Fatalf("expected span name 'test-job-operation', got %s", s.Name())
	}
}

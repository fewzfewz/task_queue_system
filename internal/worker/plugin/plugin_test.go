package plugin

import (
	"context"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
)

// testPlugin implements JobPlugin for testing.
type testPlugin struct {
	executeErr error
}

func (p *testPlugin) Type() string { return "test" }

func (p *testPlugin) Execute(_ context.Context, _ *jobs.Job) (interface{}, error) {
	if p.executeErr != nil {
		return nil, p.executeErr
	}
	return "done", nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := &testPlugin{}

	if err := r.Register(p); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := r.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Type() != "test" {
		t.Fatalf("expected type test, got %s", got.Type())
	}
}

func TestRegistryDuplicateRegistration(t *testing.T) {
	r := NewRegistry()
	p := &testPlugin{}

	if err := r.Register(p); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := r.Register(p); err == nil {
		t.Fatal("expected error on duplicate registration")
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin type")
	}
}

func TestRegistryThreadSafe(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			_ = r.Register(&testPlugin{})
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = r.Get("test")
		}
		done <- true
	}()

	<-done
	<-done
}

func TestGlobalRegistry(t *testing.T) {
	reg := GetGlobalRegistry()
	if reg == nil {
		t.Fatal("expected non-nil global registry")
	}
	// Reset for test isolation
	globalRegistry = NewRegistry()
}

func TestRegisterGlobal(t *testing.T) {
	// Reset
	globalRegistry = NewRegistry()
	p := &testPlugin{}
	RegisterGlobal(p)

	got, err := GetGlobalRegistry().Get("test")
	if err != nil {
		t.Fatalf("Get failed after RegisterGlobal: %v", err)
	}
	if got.Type() != "test" {
		t.Fatalf("expected type test, got %s", got.Type())
	}
}

func TestPluginInterfaceImplementation(t *testing.T) {
	var _ JobPlugin = (*testPlugin)(nil)
}

func TestProgressCallback(t *testing.T) {
	ctx := context.Background()

	var reported float64
	fn := func(pct float64) {
		reported = pct
	}

	ctx = WithProgressCallback(ctx, fn)
	ReportProgress(ctx, 42.5)

	if reported != 42.5 {
		t.Fatalf("expected reported 42.5, got %f", reported)
	}
}

func TestReportProgressNoCallback(t *testing.T) {
	ctx := context.Background()
	// Should not panic
	ReportProgress(ctx, 50.0)
}

func TestCircuitBreakerDefaults(t *testing.T) {
	cb := NewCircuitBreaker(0, 0)
	if cb.threshold != 5 {
		t.Fatalf("expected default threshold 5, got %d", cb.threshold)
	}
	if cb.cooldown != 30*time.Second {
		t.Fatalf("expected default cooldown 30s, got %v", cb.cooldown)
	}
}

func TestCircuitBreakerIsAllowedInitially(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second)
	if !cb.IsAllowed("email") {
		t.Fatal("expected email to be allowed initially")
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)

	cb.RecordFailure("email", nil)
	cb.RecordFailure("email", nil)

	if cb.IsAllowed("email") {
		t.Fatal("expected email to be blocked after 2 failures")
	}

	if state := cb.State("email"); state != StateOpen {
		t.Fatalf("expected StateOpen, got %v", state)
	}
}

func TestCircuitBreakerClosesAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(1, 10*time.Millisecond)

	cb.RecordFailure("email", nil)
	if cb.IsAllowed("email") {
		t.Fatal("expected email to be blocked immediately")
	}

	time.Sleep(20 * time.Millisecond)

	if !cb.IsAllowed("email") {
		t.Fatal("expected email to be allowed after cooldown")
	}
	if state := cb.State("email"); state != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen after cooldown, got %v", state)
	}
}

func TestCircuitBreakerRecordSuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)

	cb.RecordFailure("email", nil)
	cb.RecordSuccess("email")

	// Should still be allowed because RecordSuccess reset failures
	if !cb.IsAllowed("email") {
		t.Fatal("expected email to be allowed after RecordSuccess reset")
	}

	if state := cb.State("email"); state != StateClosed {
		t.Fatalf("expected StateClosed after RecordSuccess, got %v", state)
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Minute)

	cb.RecordFailure("email", nil)
	cb.Reset("email")

	if !cb.IsAllowed("email") {
		t.Fatal("expected email allowed after Reset")
	}
	if state := cb.State("email"); state != StateClosed {
		t.Fatalf("expected StateClosed after Reset, got %v", state)
	}
}

func TestCircuitBreakerMultipleTypes(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Minute)

	cb.RecordFailure("email", nil)
	cb.RecordSuccess("image")

	if cb.IsAllowed("email") {
		t.Fatal("expected email blocked (single failure)")
	}
	if !cb.IsAllowed("image") {
		t.Fatal("expected image allowed (success reset)")
	}
}

func TestCircuitBreakerStatus(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)

	status := cb.Status()
	if len(status) != 0 {
		t.Fatalf("expected empty status, got %v", status)
	}

	cb.RecordFailure("email", nil)
	cb.RecordFailure("email", nil)

	status = cb.Status()
	label, ok := status["email"]
	if !ok {
		t.Fatal("expected email in status")
	}
	if label == "" {
		t.Fatal("expected non-empty status label")
	}
}

func TestCircuitBreakerImplementsInterface(t *testing.T) {
	cb := NewCircuitBreaker(5, 30*time.Second)
	// Just ensure methods exist
	_ = cb.IsAllowed("test")
	cb.RecordSuccess("test")
	cb.RecordFailure("test", nil)
	_ = cb.State("test")
	cb.Reset("test")
	_ = cb.Status()
}

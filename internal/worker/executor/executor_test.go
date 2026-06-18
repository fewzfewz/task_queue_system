package executor

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

var zeroTime time.Time

type mockPlugin struct {
	jobType   string
	executeFn func(ctx context.Context, job *jobs.Job) (interface{}, error)
}

func (m *mockPlugin) Type() string { return m.jobType }

func (m *mockPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, job)
	}
	return "done", nil
}

func TestJobExecutor_Execute_Success(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.Register(&mockPlugin{jobType: "test-plugin"})
	je := &JobExecutor{registry: reg, circuitBreaker: plugin.NewCircuitBreaker(5, 30*time.Second), logger: slog.Default()}

	job := jobs.NewJob("test-plugin", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	result, err := je.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "done" {
		t.Fatalf("expected 'done', got %v", result)
	}
}

func TestJobExecutor_Execute_UnknownType(t *testing.T) {
	je := NewJobExecutor(slog.Default())

	job := jobs.NewJob("nonexistent", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	_, err := je.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for unknown job type")
	}
}

func TestJobExecutor_Execute_NilJob(t *testing.T) {
	je := NewJobExecutor(slog.Default())

	_, err := je.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil job")
	}
}

func TestJobExecutor_Execute_PluginError(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.Register(&mockPlugin{
		jobType: "failing",
		executeFn: func(ctx context.Context, job *jobs.Job) (interface{}, error) {
			return nil, errors.New("plugin error")
		},
	})
	je := &JobExecutor{registry: reg, circuitBreaker: plugin.NewCircuitBreaker(5, 30*time.Second), logger: slog.Default()}

	job := jobs.NewJob("failing", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	_, err := je.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error from plugin")
	}
}

func TestJobExecutor_Execute_PanicRecovery(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.Register(&mockPlugin{
		jobType: "panicking",
		executeFn: func(ctx context.Context, job *jobs.Job) (interface{}, error) {
			panic("something went wrong")
		},
	})
	je := &JobExecutor{registry: reg, circuitBreaker: plugin.NewCircuitBreaker(5, 30*time.Second), logger: slog.Default()}

	job := jobs.NewJob("panicking", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	_, err := je.Execute(context.Background(), job)
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
}

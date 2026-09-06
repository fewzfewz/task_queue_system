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

type MockPlugin struct {
	TypeStr   string
	Result    interface{}
	Err       error
	ExecCount int
}

func (m *MockPlugin) Type() string { return m.TypeStr }
func (m *MockPlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	m.ExecCount++
	return m.Result, m.Err
}

func TestJobExecutor_MiddlewareChain(t *testing.T) {
	logger := slog.Default()
	executor := NewJobExecutor(logger)

	// Create and register a mock plugin
	mockP := &MockPlugin{TypeStr: "test_job", Result: "success"}
	executor.RegisterPlugin(mockP)

	// Variables to track middleware execution
	var order []string

	// Middleware 1: Appends "A1" before, "A2" after
	m1 := func(ctx context.Context, job *jobs.Job, next plugin.NextFunc) (interface{}, error) {
		order = append(order, "A1")
		res, err := next(ctx, job)
		order = append(order, "A2")
		return res, err
	}

	// Middleware 2: Appends "B1" before, "B2" after
	m2 := func(ctx context.Context, job *jobs.Job, next plugin.NextFunc) (interface{}, error) {
		order = append(order, "B1")
		res, err := next(ctx, job)
		order = append(order, "B2")
		return res, err
	}

	executor.Use(m1)
	executor.Use(m2)

	job := jobs.NewJob("test_job", nil, nil, "medium", 1, time.Now(), "", 0, 1, "t1")
	res, err := executor.Execute(context.Background(), job)
	
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != "success" {
		t.Fatalf("expected result 'success', got %v", res)
	}

	// The order should be A1 -> B1 -> (plugin execution) -> B2 -> A2
	expectedOrder := []string{"A1", "B1", "B2", "A2"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("expected length %d, got %d", len(expectedOrder), len(order))
	}
	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("at index %d, expected %s, got %s", i, v, order[i])
		}
	}

	if mockP.ExecCount != 1 {
		t.Errorf("expected plugin to execute 1 time, got %d", mockP.ExecCount)
	}
}

func TestJobExecutor_MiddlewareAbort(t *testing.T) {
	logger := slog.Default()
	executor := NewJobExecutor(logger)
	mockP := &MockPlugin{TypeStr: "test_job", Result: "success"}
	executor.RegisterPlugin(mockP)

	// Middleware that aborts execution
	abortM := func(ctx context.Context, job *jobs.Job, next plugin.NextFunc) (interface{}, error) {
		return nil, errors.New("aborted by middleware")
	}

	executor.Use(abortM)

	job := jobs.NewJob("test_job", nil, nil, "medium", 1, time.Now(), "", 0, 1, "t1")
	_, err := executor.Execute(context.Background(), job)
	
	if err == nil || err.Error() != "aborted by middleware" {
		t.Fatalf("expected abort error, got %v", err)
	}

	if mockP.ExecCount != 0 {
		t.Errorf("expected plugin to NOT execute, but it ran %d times", mockP.ExecCount)
	}
}

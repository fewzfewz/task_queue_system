package queue

import (
	"context"
	"encoding/json"

	"task-queue-system/internal/jobs"
)

type MockQueue struct {
	jobs      chan *jobs.Job
	failed    chan *jobs.Job
	processed map[string]bool
	workers   map[string]string
}

func NewMockQueue() *MockQueue {
	return &MockQueue{
		jobs:      make(chan *jobs.Job, 100),
		failed:    make(chan *jobs.Job, 100),
		processed: make(map[string]bool),
		workers:   make(map[string]string),
	}
}

func (m *MockQueue) Enqueue(_ context.Context, job *jobs.Job) error {
	select {
	case m.jobs <- job:
	default:
	}
	return nil
}

func (m *MockQueue) Size(_ context.Context) (int64, error) {
	return int64(len(m.jobs)), nil
}

func (m *MockQueue) Dequeue(ctx context.Context) (*jobs.Job, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case job, ok := <-m.jobs:
		if !ok {
			return nil, ctx.Err()
		}
		return job, nil
	}
}

func (m *MockQueue) Ack(_ context.Context, jobID string) error {
	return nil
}

func (m *MockQueue) Fail(_ context.Context, jobID string, err error) error {
	select {
	case m.failed <- &jobs.Job{ID: jobID}:
	default:
	}
	return nil
}

func (m *MockQueue) GetFailedJobs(_ context.Context) ([]*jobs.Job, error) {
	var result []*jobs.Job
	for {
		select {
		case j := <-m.failed:
			result = append(result, j)
		default:
			return result, nil
		}
	}
}

func (m *MockQueue) GetMetrics(_ context.Context) (QueueMetrics, error) {
	return QueueMetrics{}, nil
}

func (m *MockQueue) RegisterHeartbeat(_ context.Context, workerID string) error {
	m.workers[workerID] = "alive"
	return nil
}

func (m *MockQueue) GetActiveWorkers(_ context.Context) ([]WorkerInfo, error) {
	var result []WorkerInfo
	for id := range m.workers {
		result = append(result, WorkerInfo{ID: id, LastHeartbeat: "now"})
	}
	return result, nil
}

func (m *MockQueue) IsProcessed(_ context.Context, jobID string) (bool, error) {
	return m.processed[jobID], nil
}

func (m *MockQueue) MarkProcessed(_ context.Context, jobID string) error {
	m.processed[jobID] = true
	return nil
}

func (m *MockQueue) PromoteScheduledJobs(_ context.Context) (int, error) {
	return 0, nil
}

func (m *MockQueue) ReclaimTimedOutJobs(_ context.Context) (int, error) {

	return 0, nil
}

func (m *MockQueue) IsAllowed(_ context.Context, tenantID string) (bool, error) {
	return true, nil
}

func (m *MockQueue) RateLimitStatus(_ context.Context) ([]TenantRateStatus, error) {
	return []TenantRateStatus{}, nil
}

func (m *MockQueue) PriorityPartitionDepths(ctx context.Context) (PriorityDepthReport, error) {
	size, _ := m.Size(ctx)
	return PriorityDepthReport{
		DequeueWeights:        map[string]int{"high": 70, "medium": 20, "low": 10},
		PartitionsPerPriority: 3,
		ByPriority: map[string]PriorityTierDepth{
			"high":   {Total: 0, Partitions: map[string]int64{"1": 0, "2": 0, "3": 0}},
			"medium": {Total: size, Partitions: map[string]int64{"1": size, "2": 0, "3": 0}},
			"low":    {Total: 0, Partitions: map[string]int64{"1": 0, "2": 0, "3": 0}},
		},
	}, nil
}

func (m *MockQueue) PublishWebhookEvent(_ context.Context, event interface{}) error {
	_, err := json.Marshal(event)
	return err
}

func (m *MockQueue) ReconcileDeferredJobs(ctx context.Context) (int, error) { return 0, nil }

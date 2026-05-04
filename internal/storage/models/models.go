// Package models defines the storage abstraction (Store interface) and a
// thread-safe in-memory implementation suitable for development and testing.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
)

// ─── Store interface ──────────────────────────────────────────────────────────

// Store is the persistence contract for job records.
// Any concrete backend (in-memory, Postgres, Redis, etc.) must implement it.
type Store interface {
	// Save persists a job. Overwrites any existing record with the same ID.
	Save(ctx context.Context, job *jobs.Job) error

	// GetByID returns the job with the given ID.
	// Returns ErrJobNotFound if no record exists.
	GetByID(ctx context.Context, id string) (*jobs.Job, error)

	// UpdateStatus changes the status (and UpdatedAt/ProcessedBy) of an existing job.
	// Returns ErrJobNotFound if no record exists.
	UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error

	// UpdateResult updates status, processor and the final result of the job.
	UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error
}

// ErrJobNotFound is returned when a job ID does not exist in the store.
var ErrJobNotFound = fmt.Errorf("job not found")

// ─── In-Memory Store ──────────────────────────────────────────────────────────

// InMemoryStore is a goroutine-safe, map-backed Store.
// Use it for local development, integration tests, and as a drop-in mock.
type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string]*jobs.Job
}

// NewInMemoryStore returns an initialised InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string]*jobs.Job)}
}

// Save adds or replaces the job record. Stores a shallow copy to prevent
// accidental mutation of the caller's pointer.
func (s *InMemoryStore) Save(_ context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("store: cannot save nil job")
	}
	copy := *job // shallow copy — Payload map is shared but that's acceptable here
	s.mu.Lock()
	s.data[job.ID] = &copy
	s.mu.Unlock()
	return nil
}

// GetByID returns a copy of the stored job so callers cannot mutate store state.
func (s *InMemoryStore) GetByID(_ context.Context, id string) (*jobs.Job, error) {
	s.mu.RLock()
	stored, ok := s.data[id]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	copy := *stored
	return &copy, nil
}

// UpdateStatus mutates only the Status and ProcessedBy field of an existing job record.
func (s *InMemoryStore) UpdateStatus(_ context.Context, id string, status jobs.JobStatus, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.data[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	job.Status = status
	job.ProcessedBy = workerID
	return nil
}

// UpdateResult mutates Status, ProcessedBy and Result fields.
func (s *InMemoryStore) UpdateResult(_ context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.data[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	job.Status = status
	job.ProcessedBy = workerID
	job.Result = result
	job.UpdatedAt = time.Now().UTC()
	return nil
}

// ─── Redis Store ──────────────────────────────────────────────────────────────

const jobStoreKey = "task_queue:store:jobs"

// RedisStore persists job records as JSON in a Redis Hash.
// This allows multiple distributed instances to share a consistent view of job states.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a RedisStore.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Save(ctx context.Context, job *jobs.Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("redis_store: failed to marshal job: %w", err)
	}

	return s.client.HSet(ctx, jobStoreKey, job.ID, payload).Err()
}

func (s *RedisStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	val, err := s.client.HGet(ctx, jobStoreKey, id).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
		}
		return nil, fmt.Errorf("redis_store: HGET failed: %w", err)
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return nil, fmt.Errorf("redis_store: failed to unmarshal job: %w", err)
	}

	return &job, nil
}

func (s *RedisStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	// We need to fetch, mutate, and save because we store the whole object as JSON.
	// For high scale, we'd store fields individually or use a Lua script.
	val, err := s.client.HGet(ctx, jobStoreKey, id).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("%w: %s", ErrJobNotFound, id)
		}
		return fmt.Errorf("redis_store: HGET failed: %w", err)
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return fmt.Errorf("redis_store: failed to unmarshal job: %w", err)
	}

	job.Status = status
	job.ProcessedBy = workerID
	job.UpdatedAt = time.Now().UTC()

	updated, err := json.Marshal(&job)
	if err != nil {
		return fmt.Errorf("redis_store: failed to marshal updated job: %w", err)
	}

	return s.client.HSet(ctx, jobStoreKey, id, updated).Err()
}

func (s *RedisStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	val, err := s.client.HGet(ctx, jobStoreKey, id).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("%w: %s", ErrJobNotFound, id)
		}
		return fmt.Errorf("redis_store: HGET failed: %w", err)
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(val), &job); err != nil {
		return fmt.Errorf("redis_store: failed to unmarshal job: %w", err)
	}

	job.Status = status
	job.ProcessedBy = workerID
	job.Result = result
	job.UpdatedAt = time.Now().UTC()

	updated, err := json.Marshal(&job)
	if err != nil {
		return fmt.Errorf("redis_store: failed to marshal updated job: %w", err)
	}

	return s.client.HSet(ctx, jobStoreKey, id, updated).Err()
}

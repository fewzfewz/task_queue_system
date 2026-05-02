// Package models defines the storage abstraction (Store interface) and a
// thread-safe in-memory implementation suitable for development and testing.
package models

import (
	"context"
	"fmt"
	"sync"

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

	// UpdateStatus changes the status (and UpdatedAt) of an existing job.
	// Returns ErrJobNotFound if no record exists.
	UpdateStatus(ctx context.Context, id string, status jobs.JobStatus) error
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

// UpdateStatus mutates only the Status field of an existing job record.
func (s *InMemoryStore) UpdateStatus(_ context.Context, id string, status jobs.JobStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.data[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	job.Status = status
	return nil
}

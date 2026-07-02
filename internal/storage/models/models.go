// Package models defines the storage abstraction (Store interface) and a
// thread-safe in-memory implementation suitable for development and testing.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
)

const dedupKeyTTL = 24 * time.Hour

// JobFilter holds search parameters for querying jobs.
type JobFilter struct {
	TenantID     string
	Status       string
	Type         string
	LabelKey     string
	LabelValue   string
	CreatedAfter time.Time
	CreatedBefore time.Time
	Limit        int
	Offset       int
}

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

	// UpdateProgress sets the progress percentage (0.0 – 100.0) for an in-flight job.
	UpdateProgress(ctx context.Context, id string, progress float64) error

	// UpdateResult updates status, processor and the final result of the job.
	UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error

	// GetByWorkerAndStatus retrieves all jobs currently marked as being processed by a specific worker.
	GetByWorkerAndStatus(ctx context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error)

	// Enqueue adds a job for future processing.
	Enqueue(ctx context.Context, job *jobs.Job) error

	// Dequeue atomically moves a job to 'processing'.
	Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error)

	// Heartbeat updates the liveness of an active job.
	Heartbeat(ctx context.Context, jobID string) error

	// Complete marks a job as successfully finished.
	Complete(ctx context.Context, jobID string, result interface{}) error

	// Fail handles job failures with optional requeue.
	Fail(ctx context.Context, jobID string, err error, requeue bool) error

	// ListJobs returns a paginated list of jobs for a tenant.
	ListJobs(ctx context.Context, tenantID string, status string, typeStr string, limit, offset int) ([]*jobs.Job, error)

	// SearchJobs returns a filtered, paginated list of jobs.
	SearchJobs(ctx context.Context, filter JobFilter) ([]*jobs.Job, error)

	// RecoverOrphans resets jobs from crashed workers back to 'pending'.
	RecoverOrphans(ctx context.Context, timeout time.Duration) (int64, error)

	// DLQ Management
	DeleteJob(ctx context.Context, jobID string) error
	DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error)

	// IsDedupKeyTaken returns true if the given dedup_key already exists (exactly-once guard).
	IsDedupKeyTaken(ctx context.Context, dedupKey string, tenantID string) (bool, error)

	// GetByIDs returns multiple jobs by their IDs (for DAG dependency checks).
	GetByIDs(ctx context.Context, ids []string) ([]*jobs.Job, error)

	// GetQueueLengths returns pending job counts segmented by queue and tenant.
	GetQueueLengths(ctx context.Context) (map[string]map[string]int64, error)
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

// UpdateProgress sets the progress percentage for an in-flight job.
func (s *InMemoryStore) UpdateProgress(_ context.Context, id string, progress float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.data[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	job.Progress = progress
	job.UpdatedAt = time.Now().UTC()
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

func (s *InMemoryStore) GetByWorkerAndStatus(_ context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*jobs.Job
	for _, j := range s.data {
		if j.ProcessedBy == workerID && j.Status == status {
			copy := *j
			results = append(results, &copy)
		}
	}
	return results, nil
}


func (s *InMemoryStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	return s.Save(ctx, job)
}

func (s *InMemoryStore) Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error) {
	return nil, fmt.Errorf("InMemoryStore.Dequeue is not implemented; use RedisQueue which owns the dequeue/priority LUA script logic")
}

func (s *InMemoryStore) Heartbeat(ctx context.Context, jobID string) error {
	return nil
}

func (s *InMemoryStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	return s.UpdateResult(ctx, jobID, jobs.StatusCompleted, "", result)
}

func (s *InMemoryStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	status := jobs.StatusFailed
	if requeue {
		status = jobs.StatusPending
	}
	return s.UpdateResult(ctx, jobID, status, "", err.Error())
}

func (s *InMemoryStore) ListJobs(ctx context.Context, tenantID string, status string, typeStr string, limit, offset int) ([]*jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*jobs.Job
	for _, j := range s.data {
		if (tenantID == "" || j.TenantID == tenantID) &&
			(status == "" || string(j.Status) == status) &&
			(typeStr == "" || j.Type == typeStr) {
			copy := *j
			results = append(results, &copy)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(results)
	}
	if offset >= len(results) {
		return []*jobs.Job{}, nil
	}

	end := offset + limit
	if end > len(results) {
		end = len(results)
	}

	return results[offset:end], nil
}

func (s *InMemoryStore) SearchJobs(ctx context.Context, filter JobFilter) ([]*jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*jobs.Job
	for _, j := range s.data {
		if filter.TenantID != "" && j.TenantID != filter.TenantID {
			continue
		}
		if filter.Status != "" && string(j.Status) != filter.Status {
			continue
		}
		if filter.Type != "" && j.Type != filter.Type {
			continue
		}
		if filter.LabelKey != "" {
			v, ok := j.Labels[filter.LabelKey]
			if !ok || (filter.LabelValue != "" && v != filter.LabelValue) {
				continue
			}
		}
		if !filter.CreatedAfter.IsZero() && j.CreatedAt.Before(filter.CreatedAfter) {
			continue
		}
		if !filter.CreatedBefore.IsZero() && j.CreatedAt.After(filter.CreatedBefore) {
			continue
		}
		copy := *j
		results = append(results, &copy)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Offset >= len(results) {
		return []*jobs.Job{}, nil
	}
	end := filter.Offset + filter.Limit
	if end > len(results) {
		end = len(results)
	}
	return results[filter.Offset:end], nil
}

func (s *InMemoryStore) RecoverOrphans(ctx context.Context, timeout time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-timeout)
	var count int64
	for _, j := range s.data {
		if j.Status == jobs.StatusProcessing && j.UpdatedAt.Before(cutoff) {
			j.Status = jobs.StatusPending
			j.ProcessedBy = ""
			count++
		}
	}
	return count, nil
}

func (s *InMemoryStore) DeleteJob(_ context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, jobID)
	return nil
}

func (s *InMemoryStore) DeleteJobsBefore(_ context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for id, j := range s.data {
		if (tenantID == "" || j.TenantID == tenantID) &&
			(status == "" || string(j.Status) == status) &&
			(jobType == "" || j.Type == jobType) &&
			j.CreatedAt.Before(before) {
			delete(s.data, id)
			count++
		}
	}
	return count, nil
}

func (s *InMemoryStore) IsDedupKeyTaken(_ context.Context, dedupKey, tenantID string) (bool, error) {
	if dedupKey == "" {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, j := range s.data {
		if j.DedupKey == dedupKey && j.TenantID == tenantID {
			return true, nil
		}
	}
	return false, nil
}

func (s *InMemoryStore) GetByIDs(_ context.Context, ids []string) ([]*jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*jobs.Job, 0, len(ids))
	for _, id := range ids {
		if j, ok := s.data[id]; ok {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *InMemoryStore) GetQueueLengths(_ context.Context) (map[string]map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make(map[string]map[string]int64)
	for _, j := range s.data {
		if j.Status == jobs.StatusPending {
			if results[j.Type] == nil {
				results[j.Type] = make(map[string]int64)
			}
			results[j.Type][j.TenantID]++
		}
	}
	return results, nil
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

func (s *RedisStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
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

	job.Progress = progress
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

func (s *RedisStore) GetByWorkerAndStatus(ctx context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error) {
	vals, err := s.client.HVals(ctx, jobStoreKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis_store: HVALS failed: %w", err)
	}

	var results []*jobs.Job
	for _, v := range vals {
		var j jobs.Job
		if err := json.Unmarshal([]byte(v), &j); err != nil {
			continue
		}
		if j.ProcessedBy == workerID && j.Status == status {
			results = append(results, &j)
		}
	}
	return results, nil
}


func (s *RedisStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	return s.Save(ctx, job)
}

func (s *RedisStore) Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error) {
	return nil, fmt.Errorf("redis store dequeue not implemented; use RedisQueue")
}

func (s *RedisStore) Heartbeat(ctx context.Context, jobID string) error {
	return nil
}

func (s *RedisStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	return s.UpdateResult(ctx, jobID, jobs.StatusCompleted, "", result)
}

func (s *RedisStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	status := jobs.StatusFailed
	if requeue {
		status = jobs.StatusPending
	}
	return s.UpdateResult(ctx, jobID, status, "", err.Error())
}

func (s *RedisStore) ListJobs(ctx context.Context, tenantID string, status string, typeStr string, limit, offset int) ([]*jobs.Job, error) {
	var results []*jobs.Job
	need := limit
	skip := offset
	if need <= 0 {
		need = -1 // no limit
	}

	var cursor uint64
	for {
		keys, nextCursor, err := s.client.HScan(ctx, jobStoreKey, cursor, "*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("redis_store: HSCAN failed: %w", err)
		}
		for i := 0; i+1 < len(keys); i += 2 {
			var j jobs.Job
			if err := json.Unmarshal([]byte(keys[i+1]), &j); err != nil {
				continue
			}
			if (tenantID == "" || j.TenantID == tenantID) &&
				(status == "" || string(j.Status) == status) &&
				(typeStr == "" || j.Type == typeStr) {
				if skip > 0 {
					skip--
					continue
				}
				copy := j
				results = append(results, &copy)
				if need > 0 && len(results) >= need {
					break
				}
			}
		}
		cursor = nextCursor
		if cursor == 0 || (need > 0 && len(results) >= need) {
			break
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return results, nil
}

func (s *RedisStore) SearchJobs(ctx context.Context, filter JobFilter) ([]*jobs.Job, error) {
	return s.ListJobs(ctx, filter.TenantID, filter.Status, filter.Type, filter.Limit, filter.Offset)
}

func (s *RedisStore) RecoverOrphans(ctx context.Context, timeout time.Duration) (int64, error) {
	vals, err := s.client.HVals(ctx, jobStoreKey).Result()
	if err != nil {
		return 0, fmt.Errorf("redis_store: HVALS failed: %w", err)
	}

	cutoff := time.Now().Add(-timeout)
	var count int64
	for _, v := range vals {
		var j jobs.Job
		if err := json.Unmarshal([]byte(v), &j); err != nil {
			continue
		}
		if j.Status == jobs.StatusProcessing && j.UpdatedAt.Before(cutoff) {
			j.Status = jobs.StatusPending
			j.ProcessedBy = ""
			payload, err := json.Marshal(j)
			if err != nil {
				continue
			}
			if err := s.client.HSet(ctx, jobStoreKey, j.ID, payload).Err(); err == nil {
				count++
			}
		}
	}
	return count, nil
}

func (s *RedisStore) DeleteJob(ctx context.Context, jobID string) error {
	return s.client.HDel(ctx, jobStoreKey, jobID).Err()
}

func (s *RedisStore) DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	var cursor uint64
	var count int64
	for {
		keys, nextCursor, err := s.client.HScan(ctx, jobStoreKey, cursor, "*", 100).Result()
		if err != nil {
			return count, err
		}
		for i := 0; i+1 < len(keys); i += 2 {
			id := keys[i]
			if id == "" {
				continue
			}
			var j jobs.Job
			if err := json.Unmarshal([]byte(keys[i+1]), &j); err != nil {
				continue
			}
			if (tenantID == "" || j.TenantID == tenantID) &&
				(status == "" || string(j.Status) == status) &&
				(jobType == "" || j.Type == jobType) &&
				j.CreatedAt.Before(before) {
				s.client.HDel(ctx, jobStoreKey, id)
				count++
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return count, nil
}

func (s *RedisStore) IsDedupKeyTaken(ctx context.Context, dedupKey, tenantID string) (bool, error) {
	if dedupKey == "" {
		return false, nil
	}
	// Use a Redis SET with TTL for dedup keys.
	key := fmt.Sprintf("dedup:%s:%s", tenantID, dedupKey)
	// SETNX returns true if key was set (didn't exist).
	return s.client.SetNX(ctx, key, "1", 24*time.Hour).Result()
}

func (s *RedisStore) GetByIDs(ctx context.Context, ids []string) ([]*jobs.Job, error) {
	vals, err := s.client.HMGet(ctx, jobStoreKey, ids...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis_store: HMGET failed: %w", err)
	}
	out := make([]*jobs.Job, 0, len(ids))
	for _, v := range vals {
		if v == nil {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var j jobs.Job
		if err := json.Unmarshal([]byte(str), &j); err != nil {
			continue
		}
		out = append(out, &j)
	}
	return out, nil
}

func (s *RedisStore) GetQueueLengths(ctx context.Context) (map[string]map[string]int64, error) {
	vals, err := s.client.HVals(ctx, jobStoreKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis_store: HVALS failed: %w", err)
	}

	results := make(map[string]map[string]int64)
	for _, v := range vals {
		var j jobs.Job
		if err := json.Unmarshal([]byte(v), &j); err != nil {
			continue
		}
		if j.Status != jobs.StatusPending {
			continue
		}
		if results[j.Type] == nil {
			results[j.Type] = make(map[string]int64)
		}
		results[j.Type][j.TenantID]++
	}

	return results, nil
}


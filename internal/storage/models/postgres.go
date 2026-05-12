package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"task-queue-system/internal/jobs"
)

// PostgresStore implements the Store interface using a PostgreSQL backend.
// It provides robust, durable persistence for jobs and execution history.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore and verifies the connection.
func NewPostgresStore(ctx context.Context, connStr string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres_store: failed to parse config: %w", err)
	}

	// Optimise pool for high concurrency
	config.MaxConns = 25
	config.MinConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres_store: failed to create pool: %w", err)
	}

	// Verify connectivity immediately
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres_store: health check failed: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

// Close gracefully shuts down the connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Save(ctx context.Context, job *jobs.Job) error {
	payload, _ := json.Marshal(job.Payload)
	
	query := `
		INSERT INTO jobs (
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			attempts = EXCLUDED.attempts,
			updated_at = EXCLUDED.updated_at,
			processed_by = EXCLUDED.processed_by,
			result = EXCLUDED.result,
			error = EXCLUDED.error
	`
	_, err := s.pool.Exec(ctx, query,
		job.ID, job.TenantID, job.Type, payload, string(job.Status), string(job.Priority),
		job.Retries, job.MaxRetries, job.CorrelationID, job.Timeout,
		job.Version, job.RunAt, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres_store: Save failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	var j jobs.Job
	var payload []byte
	var res []byte
	var priority string
	var status string
	var processedBy *string
	var errStr *string

	query := `
		SELECT 
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, created_at, updated_at, processed_by, result, error
		FROM jobs WHERE id = $1
	`
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&j.ID, &j.TenantID, &j.Type, &payload, &status, &priority,
		&j.Retries, &j.MaxRetries, &j.CorrelationID, &j.Timeout,
		&j.Version, &j.RunAt, &j.CreatedAt, &j.UpdatedAt, &processedBy, &res, &errStr,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrJobNotFound, id)
		}
		return nil, fmt.Errorf("postgres_store: GetByID failed: %w", err)
	}

	j.Status = jobs.JobStatus(status)
	j.Priority = jobs.JobPriority(priority)
	if payload != nil {
		_ = json.Unmarshal(payload, &j.Payload)
	}
	if res != nil {
		_ = json.Unmarshal(res, &j.Result)
	}
	if processedBy != nil {
		j.ProcessedBy = *processedBy
	}
	if errStr != nil && *errStr != "" {
		j.Result = *errStr // In failed state, we use result to store error string
	}

	return &j, nil
}

func (s *PostgresStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	query := `UPDATE jobs SET status = $1, processed_by = $2, updated_at = $3 WHERE id = $4`
	ct, err := s.pool.Exec(ctx, query, string(status), workerID, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("postgres_store: UpdateStatus failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	return nil
}

func (s *PostgresStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	var resJSON []byte
	var errorStr *string

	if status == jobs.StatusFailed {
		if s, ok := result.(string); ok {
			errorStr = &s
		} else {
			s := fmt.Sprintf("%v", result)
			errorStr = &s
		}
	} else {
		resJSON, _ = json.Marshal(result)
	}

	completedAt := time.Now().UTC()
	query := `
		UPDATE jobs 
		SET status = $1, processed_by = $2, result = $3, error = $4, updated_at = $5, completed_at = $6
		WHERE id = $7
	`
	ct, err := s.pool.Exec(ctx, query, string(status), workerID, resJSON, errorStr, completedAt, completedAt, id)
	if err != nil {
		return fmt.Errorf("postgres_store: UpdateResult failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	return nil
}

func (s *PostgresStore) GetByWorkerAndStatus(ctx context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error) {
	query := `
		SELECT 
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, created_at, updated_at, processed_by
		FROM jobs WHERE processed_by = $1 AND status = $2
	`
	rows, err := s.pool.Query(ctx, query, workerID, string(status))
	if err != nil {
		return nil, fmt.Errorf("postgres_store: GetByWorkerAndStatus query failed: %w", err)
	}
	defer rows.Close()

	var results []*jobs.Job
	for rows.Next() {
		var j jobs.Job
		var payload []byte
		var priority string
		var stat string
		var processedBy *string

		err := rows.Scan(
			&j.ID, &j.TenantID, &j.Type, &payload, &stat, &priority,
			&j.Retries, &j.MaxRetries, &j.CorrelationID, &j.Timeout,
			&j.Version, &j.RunAt, &j.CreatedAt, &j.UpdatedAt, &processedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres_store: scan failed: %w", err)
		}

		j.Status = jobs.JobStatus(stat)
		j.Priority = jobs.JobPriority(priority)
		if payload != nil {
			_ = json.Unmarshal(payload, &j.Payload)
		}
		if processedBy != nil {
			j.ProcessedBy = *processedBy
		}
		results = append(results, &j)
	}

	return results, nil
}

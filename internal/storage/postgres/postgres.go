package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/models"
)

// PostgresStore implements the models.Store interface using a PostgreSQL backend.
// It provides robust, durable persistence and efficient task polling via SKIP LOCKED.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// Ensure PostgresStore satisfies the Store interface.
var _ models.Store = (*PostgresStore)(nil)

// New creates a new PostgresStore and verifies connectivity.
func New(ctx context.Context, connStr string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to parse config: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres: health check failed: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

// Close shut downs the pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Save(ctx context.Context, job *jobs.Job) error {
	payload, _ := json.Marshal(job.Payload)
	var url, secret *string
	var events []string
	var lastStatus, attempts int
	if job.Webhook != nil {
		url = &job.Webhook.URL
		secret = &job.Webhook.Secret
		events = job.Webhook.Events
		lastStatus = job.Webhook.LastStatus
		attempts = job.Webhook.Attempts
	}

	query := `
		INSERT INTO jobs (
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, updated_at,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
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
		url, secret, events, lastStatus, attempts,
	)
	return err
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	var j jobs.Job
	var payload []byte
	var res []byte
	var priority string
	var status string
	var processedBy *string
	var errStr *string
	var webhookURL, webhookSecret *string
	var webhookEvents []string
	var webhookLastStatus, webhookAttempts *int
	var errHistoryJSON []byte

	query := `
		SELECT 
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, created_at, updated_at, processed_by, result, error,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,
			error_history
		FROM jobs WHERE id = $1
	`
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&j.ID, &j.TenantID, &j.Type, &payload, &status, &priority,
		&j.Retries, &j.MaxRetries, &j.CorrelationID, &j.Timeout,
		&j.Version, &j.RunAt, &j.CreatedAt, &j.UpdatedAt, &processedBy, &res, &errStr,
		&webhookURL, &webhookSecret, &webhookEvents, &webhookLastStatus, &webhookAttempts,
		&errHistoryJSON,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", models.ErrJobNotFound, id)
		}
		return nil, err
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
		j.Result = *errStr
	}

	if webhookURL != nil {
		j.Webhook = &jobs.WebhookConfig{
			URL:        *webhookURL,
			Secret:     *webhookSecret,
			Events:     webhookEvents,
			LastStatus: *webhookLastStatus,
			Attempts:   *webhookAttempts,
		}
	}

	if errHistoryJSON != nil {
		_ = json.Unmarshal(errHistoryJSON, &j.ErrorHistory)
	}

	return &j, nil
}

func (s *PostgresStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	query := `UPDATE jobs SET status = $1, processed_by = $2, updated_at = $3 WHERE id = $4`
	ct, err := s.pool.Exec(ctx, query, string(status), workerID, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", models.ErrJobNotFound, id)
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

	now := time.Now().UTC()
	var completedAt *time.Time
	if status == jobs.StatusCompleted || status == jobs.StatusFailed {
		completedAt = &now
	}

	// Atomically append to error_history using JSONB concatenation
	var errHistParam interface{}

	if status == jobs.StatusFailed || (status == jobs.StatusPending && result != nil) {
		entry := []jobs.AttemptError{{
			Error:     fmt.Sprintf("%v", result),
			Timestamp: now,
		}}
		marshalled, _ := json.Marshal(entry)
		errHistParam = marshalled
	}

	query := `
		UPDATE jobs 
		SET status = $1, processed_by = $2, result = $3, error = $4, 
		    updated_at = $5, completed_at = $6, 
		    error_history = CASE WHEN $7::jsonb IS NOT NULL 
		        THEN COALESCE(error_history, '[]'::jsonb) || $7::jsonb 
		        ELSE error_history 
		    END
		WHERE id = $8
	`
	ct, err := s.pool.Exec(ctx, query, string(status), workerID, resJSON, errorStr, now, completedAt, errHistParam, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", models.ErrJobNotFound, id)
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
		return nil, err
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

func (s *PostgresStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	payload, _ := json.Marshal(job.Payload)
	var url, secret *string
	var events []string
	var lastStatus, attempts int
	if job.Webhook != nil {
		url = &job.Webhook.URL
		secret = &job.Webhook.Secret
		events = job.Webhook.Events
		lastStatus = job.Webhook.LastStatus
		attempts = job.Webhook.Attempts
	}

	query := `
		INSERT INTO jobs (
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, updated_at,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
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
		url, secret, events, lastStatus, attempts,
	)
	return err
}

func (s *PostgresStore) Dequeue(ctx context.Context, tenantID string) (*jobs.Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT id FROM jobs 
		WHERE status = 'pending' AND scheduled_at <= $1 AND ($2 = '' OR tenant_id = $2)
		ORDER BY priority = 'high' DESC, priority = 'medium' DESC, scheduled_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`
	var id string
	err = tx.QueryRow(ctx, query, time.Now().UTC(), tenantID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil 
		}
		return nil, err
	}

	updateQuery := `
		UPDATE jobs SET status = 'processing', updated_at = $1, last_heartbeat_at = $1 
		WHERE id = $2 RETURNING id
	`
	_ = tx.QueryRow(ctx, updateQuery, time.Now().UTC(), id).Scan(&id)

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.GetByID(ctx, id)
}

func (s *PostgresStore) Heartbeat(ctx context.Context, jobID string) error {
	query := `UPDATE jobs SET last_heartbeat_at = $1, updated_at = $1 WHERE id = $2`
	_, err := s.pool.Exec(ctx, query, time.Now().UTC(), jobID)
	return err
}

func (s *PostgresStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	return s.UpdateResult(ctx, jobID, jobs.StatusCompleted, "", result)
}

func (s *PostgresStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	status := jobs.StatusFailed
	if requeue {
		status = jobs.StatusPending
	}
	return s.UpdateResult(ctx, jobID, status, "", err.Error())
}

func (s *PostgresStore) ListJobs(ctx context.Context, tenantID string, status string, typeStr string, limit, offset int) ([]*jobs.Job, error) {
	query := `
		SELECT 
			id, tenant_id, type, payload, status, priority, 
			attempts, max_attempts, correlation_id, timeout_seconds, 
			version, scheduled_at, created_at, updated_at, processed_by,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts
		FROM jobs 
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR type = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`
	rows, err := s.pool.Query(ctx, query, tenantID, status, typeStr, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

func (s *PostgresStore) RecoverOrphans(ctx context.Context, timeout time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-timeout)
	query := `
		UPDATE jobs SET status = 'pending', processed_by = NULL, updated_at = $1
		WHERE status = 'processing' AND last_heartbeat_at < $2
	`
	ct, err := s.pool.Exec(ctx, query, time.Now().UTC(), cutoff)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (s *PostgresStore) DeleteJob(ctx context.Context, jobID string) error {
	query := `DELETE FROM jobs WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, jobID)
	return err
}

func (s *PostgresStore) DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	query := `
		DELETE FROM jobs 
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR type = $3)
		  AND created_at < $4
	`
	ct, err := s.pool.Exec(ctx, query, tenantID, status, jobType, before)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (s *PostgresStore) GetQueueLengths(ctx context.Context) (map[string]map[string]int64, error) {
	query := `
		SELECT type, tenant_id, COUNT(*) 
		FROM jobs 
		WHERE status = 'pending'
		GROUP BY type, tenant_id
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]map[string]int64)
	for rows.Next() {
		var jobType, tenantID string
		var count int64
		if err := rows.Scan(&jobType, &tenantID, &count); err != nil {
			return nil, err
		}
		if results[jobType] == nil {
			results[jobType] = make(map[string]int64)
		}
		results[jobType][tenantID] = count
	}
	return results, nil
}


func (s *PostgresStore) scanJobs(rows pgx.Rows) ([]*jobs.Job, error) {
	var results []*jobs.Job
	for rows.Next() {
		var j jobs.Job
		var payload []byte
		var priority string
		var stat string
		var processedBy *string
		var webhookURL, webhookSecret *string
		var webhookEvents []string
		var webhookLastStatus, webhookAttempts *int
		var errHistoryJSON []byte

		err := rows.Scan(
			&j.ID, &j.TenantID, &j.Type, &payload, &stat, &priority,
			&j.Retries, &j.MaxRetries, &j.CorrelationID, &j.Timeout,
			&j.Version, &j.RunAt, &j.CreatedAt, &j.UpdatedAt, &processedBy,
			&webhookURL, &webhookSecret, &webhookEvents, &webhookLastStatus, &webhookAttempts,
			&errHistoryJSON,
		)
		if err != nil {
			return nil, err
		}

		j.Status = jobs.JobStatus(stat)
		j.Priority = jobs.JobPriority(priority)
		if payload != nil {
			_ = json.Unmarshal(payload, &j.Payload)
		}
		if processedBy != nil {
			j.ProcessedBy = *processedBy
		}
		if webhookURL != nil {
			j.Webhook = &jobs.WebhookConfig{
				URL:        *webhookURL,
				Secret:     *webhookSecret,
				Events:     webhookEvents,
				LastStatus: *webhookLastStatus,
				Attempts:   *webhookAttempts,
			}
		}
		if errHistoryJSON != nil {
			_ = json.Unmarshal(errHistoryJSON, &j.ErrorHistory)
		}
		results = append(results, &j)
	}
	return results, nil
}

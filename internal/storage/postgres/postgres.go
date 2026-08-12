package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

// CreatePool creates a pgxpool.Pool from a connection string without running migrations.
func CreatePool(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
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
		pool.Close()
		return nil, fmt.Errorf("postgres: health check failed: %w", err)
	}
	return pool, nil
}

// New creates a new PostgresStore with connection retry and backoff.
// It retries up to maxRetries times with exponential backoff (1s, 2s, 4s, ...)
// before giving up. A zero or negative maxRetries disables retry.
func New(ctx context.Context, connStr string) (*PostgresStore, error) {
	return NewWithRetry(ctx, connStr, 5)
}

// NewWithRetry creates a PostgresStore, retrying the initial connection
// with exponential backoff. maxRetries=0 means no retry, just one attempt.
func NewWithRetry(ctx context.Context, connStr string, maxRetries int) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to parse config: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5

	var pool *pgxpool.Pool
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("postgres: context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		pool, err = pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			lastErr = fmt.Errorf("postgres: failed to create pool (attempt %d/%d): %w", attempt+1, maxRetries+1, err)
			continue
		}

		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			lastErr = fmt.Errorf("postgres: health check failed (attempt %d/%d): %w", attempt+1, maxRetries+1, err)
			continue
		}

		migrationsDir := os.Getenv("POSTGRES_MIGRATIONS_DIR")
		if migrationsDir == "" {
			migrationsDir = "db/migrations"
		}
		if err := RunMigrations(ctx, pool, migrationsDir, slog.Default()); err != nil {
			pool.Close()
			lastErr = fmt.Errorf("postgres: auto-migration failed (attempt %d/%d): %w", attempt+1, maxRetries+1, err)
			continue
		}

		return &PostgresStore{pool: pool}, nil
	}

	return nil, lastErr
}

// Close shut downs the pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Save(ctx context.Context, job *jobs.Job) error {
	payload, _ := json.Marshal(job.Payload)
	depsJSON, _ := json.Marshal(job.Dependencies)
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
			version, scheduled_at, updated_at, progress,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,
			dedup_key, dependencies, shard_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			attempts = EXCLUDED.attempts,
			updated_at = EXCLUDED.updated_at,
			processed_by = EXCLUDED.processed_by,
			result = EXCLUDED.result,
			error = EXCLUDED.error
	`
	dedup := nullIfEmpty(job.DedupKey)
	shard := nullIfEmpty(job.ShardKey)
	_, err := s.pool.Exec(ctx, query,
		job.ID, job.TenantID, job.Type, payload, string(job.Status), string(job.Priority),
		job.Retries, job.MaxRetries, job.CorrelationID, job.Timeout,
		job.Version, job.RunAt, time.Now().UTC(),
		job.Progress,
		url, secret, events, lastStatus, attempts,
		dedup, string(depsJSON), shard,
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
	var dedupKey, shardKey *string
	var depsJSON []byte

	query := `
		SELECT 
			id, tenant_id, type, payload, status, priority,
			attempts, max_attempts, correlation_id, timeout_seconds,
			version, scheduled_at, created_at, updated_at, processed_by, result, error, progress,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,
			error_history, dedup_key, dependencies, shard_key
		FROM jobs WHERE id = $1
	`
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&j.ID, &j.TenantID, &j.Type, &payload, &status, &priority,
		&j.Retries, &j.MaxRetries, &j.CorrelationID, &j.Timeout,
		&j.Version, &j.RunAt, &j.CreatedAt, &j.UpdatedAt, &processedBy, &res, &errStr, &j.Progress,
		&webhookURL, &webhookSecret, &webhookEvents, &webhookLastStatus, &webhookAttempts,
		&errHistoryJSON, &dedupKey, &depsJSON, &shardKey,
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

	if dedupKey != nil {
		j.DedupKey = *dedupKey
	}
	if len(depsJSON) > 0 {
		_ = json.Unmarshal(depsJSON, &j.Dependencies)
	}
	if shardKey != nil {
		j.ShardKey = *shardKey
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

func (s *PostgresStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE jobs SET progress = $1, updated_at = $2 WHERE id = $3`,
		progress, time.Now().UTC(), id,
	)
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
			version, scheduled_at, created_at, updated_at, processed_by,
			progress,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,
			error_history,
			dedup_key, dependencies, shard_key
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
			version, scheduled_at, updated_at, progress,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
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
		job.Progress,
		url, secret, events, lastStatus, attempts,
	)
	return err
}

func (s *PostgresStore) Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT id FROM jobs j
		WHERE j.status = 'pending'
		  AND j.scheduled_at <= $1
		  AND ($2 = '' OR j.tenant_id = $2)
		  AND ($3 = '' OR j.shard_key = $3)
		  AND (
		    j.dependencies IS NULL
		    OR j.dependencies = '[]'::jsonb
		    OR j.dependencies = 'null'::jsonb
		    OR NOT EXISTS (
		      SELECT 1
		      FROM jsonb_array_elements_text(j.dependencies) AS dep_id
		      LEFT JOIN jobs d ON d.id = dep_id AND d.status = 'completed'
		      WHERE d.id IS NULL
		    )
		  )
		ORDER BY j.priority = 'high' DESC, j.priority = 'medium' DESC, j.scheduled_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`
	var id string
	err = tx.QueryRow(ctx, query, time.Now().UTC(), tenantID, shardKey).Scan(&id)
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
			progress,
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,
			error_history,
			dedup_key, dependencies, shard_key
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

func (s *PostgresStore) SearchJobs(ctx context.Context, filter models.JobFilter) ([]*jobs.Job, error) {
	return s.ListJobs(ctx, filter.TenantID, filter.Status, filter.Type, filter.Limit, filter.Offset)
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

func (s *PostgresStore) IsDedupKeyTaken(ctx context.Context, dedupKey, tenantID string) (bool, error) {
	if dedupKey == "" {
		return false, nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM jobs WHERE dedup_key = $1 AND tenant_id = $2)`,
		dedupKey, tenantID,
	).Scan(&exists)
	return exists, err
}

func (s *PostgresStore) GetByIDs(ctx context.Context, ids []string) ([]*jobs.Job, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, type, payload, status, priority,
		        attempts, max_attempts, correlation_id, timeout_seconds,
		        version, scheduled_at, created_at, updated_at, processed_by,
		        progress,
		        webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,
		        error_history,
		        dedup_key, dependencies, shard_key
		 FROM jobs WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanJobs(rows)
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

func (s *PostgresStore) RegisterClient(ctx context.Context, tenantID, apiKeyHash string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO clients (tenant_id, api_key_hash) VALUES ($1, $2)`,
		tenantID, apiKeyHash)
	return err
}

func (s *PostgresStore) VerifyClient(ctx context.Context, apiKeyHash string) (string, error) {
	var tenantID string
	err := s.pool.QueryRow(ctx,
		`SELECT tenant_id FROM clients WHERE api_key_hash = $1`,
		apiKeyHash).Scan(&tenantID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", models.ErrClientNotFound
		}
		return "", err
	}
	return tenantID, nil
}

func (s *PostgresStore) ListClients(ctx context.Context) ([]*models.ClientRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT tenant_id, created_at FROM clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("postgres_store: failed to list clients: %w", err)
	}
	defer rows.Close()

	var out []*models.ClientRecord
	seen := make(map[string]bool)

	for rows.Next() {
		var r models.ClientRecord
		if err := rows.Scan(&r.TenantID, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres_store: failed to scan client record: %w", err)
		}
		if !seen[r.TenantID] {
			seen[r.TenantID] = true
			out = append(out, &r)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres_store: error iterating clients: %w", err)
	}

	return out, nil
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
		var dedupKey, shardKey *string
		var depsJSON []byte

		err := rows.Scan(
			&j.ID, &j.TenantID, &j.Type, &payload, &stat, &priority,
			&j.Retries, &j.MaxRetries, &j.CorrelationID, &j.Timeout,
			&j.Version, &j.RunAt, &j.CreatedAt, &j.UpdatedAt, &processedBy,
			&j.Progress,
			&webhookURL, &webhookSecret, &webhookEvents, &webhookLastStatus, &webhookAttempts,
			&errHistoryJSON,
			&dedupKey, &depsJSON, &shardKey,
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
		if dedupKey != nil {
			j.DedupKey = *dedupKey
		}
		if len(depsJSON) > 0 {
			_ = json.Unmarshal(depsJSON, &j.Dependencies)
		}
		if shardKey != nil {
			j.ShardKey = *shardKey
		}
		if errHistoryJSON != nil {
			_ = json.Unmarshal(errHistoryJSON, &j.ErrorHistory)
		}
		results = append(results, &j)
	}
	return results, nil
}

// nullIfEmpty returns nil if s is empty, otherwise a pointer to s.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

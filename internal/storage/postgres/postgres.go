// Package postgres provides a PostgreSQL-backed implementation of models.Store.
// SQL queries are stubbed with TODO markers — wire up a *sql.DB or *pgxpool.Pool
// and replace each stub with the real query when ready.
package postgres

import (
	"context"
	"fmt"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/models"
)

// PostgresStore implements models.Store against a PostgreSQL database.
type PostgresStore struct {
	// db *pgxpool.Pool  ← inject your preferred driver here
}

// New creates a PostgresStore.
// Replace the parameter type with your actual DB connection pool.
func New( /* db *pgxpool.Pool */ ) *PostgresStore {
	return &PostgresStore{}
}

// Verify at compile time that PostgresStore satisfies the Store interface.
var _ models.Store = (*PostgresStore)(nil)

// Save inserts or upserts the job record.
func (s *PostgresStore) Save(_ context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("postgres: cannot save nil job")
	}
	// TODO: execute an INSERT ... ON CONFLICT (id) DO UPDATE query.
	// Example (pgx):
	//   _, err := s.db.Exec(ctx,
	//       `INSERT INTO jobs (id, type, payload, status, retries, max_retries, created_at, updated_at)
	//        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	//        ON CONFLICT (id) DO UPDATE SET status=$4, retries=$5, updated_at=$8`,
	//       job.ID, job.Type, job.Payload, job.Status,
	//       job.Retries, job.MaxRetries, job.CreatedAt, job.UpdatedAt,
	//   )
	return fmt.Errorf("postgres: Save not implemented")
}

// GetByID fetches a single job row by primary key.
func (s *PostgresStore) GetByID(_ context.Context, id string) (*jobs.Job, error) {
	// TODO: SELECT * FROM jobs WHERE id = $1
	_ = id
	return nil, fmt.Errorf("postgres: GetByID not implemented")
}

// UpdateStatus sets a job's status column and bumps updated_at.
func (s *PostgresStore) UpdateStatus(_ context.Context, id string, status jobs.JobStatus, workerID string) error {
	// TODO: UPDATE jobs SET status=$2, processed_by=$3, updated_at=NOW() WHERE id=$1
	_, _, _ = id, status, workerID
	return fmt.Errorf("postgres: UpdateStatus not implemented")
}

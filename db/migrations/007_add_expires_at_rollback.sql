DROP INDEX IF EXISTS idx_jobs_expires_at;
ALTER TABLE jobs DROP COLUMN expires_at;

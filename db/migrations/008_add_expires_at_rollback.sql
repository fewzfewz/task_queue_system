DROP INDEX IF EXISTS idx_jobs_expires_at;
ALTER TABLE jobs DROP COLUMN IF EXISTS expires_at;

ALTER TABLE jobs ADD COLUMN expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_jobs_expires_at ON jobs (expires_at) WHERE expires_at IS NOT NULL;

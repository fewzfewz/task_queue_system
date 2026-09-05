-- Drop the non-unique index
DROP INDEX IF EXISTS idx_jobs_dedup_key;

-- Create a unique index for tenant_id and dedup_key
CREATE UNIQUE INDEX idx_jobs_dedup_key_unique ON jobs(tenant_id, dedup_key) WHERE dedup_key IS NOT NULL AND dedup_key != '';

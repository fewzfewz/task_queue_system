-- 002_add_dedup_deps_partition.sql
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dedup_key TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dependencies JSONB DEFAULT '[]'::jsonb;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS shard_key TEXT;

-- Index for dedup lookup
CREATE INDEX IF NOT EXISTS idx_jobs_dedup_key ON jobs(tenant_id, dedup_key) WHERE dedup_key IS NOT NULL;
-- Index for shard key distribution
CREATE INDEX IF NOT EXISTS idx_jobs_shard_key ON jobs(shard_key) WHERE shard_key IS NOT NULL;

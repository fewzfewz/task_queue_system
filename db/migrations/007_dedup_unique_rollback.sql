-- 007_dedup_unique_rollback.sql
DROP INDEX IF EXISTS idx_jobs_dedup_key_unique;
CREATE INDEX IF NOT EXISTS idx_jobs_dedup_key ON jobs(tenant_id, dedup_key) WHERE dedup_key IS NOT NULL;

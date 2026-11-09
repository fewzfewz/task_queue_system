-- 002_rollback.sql
DROP INDEX IF EXISTS idx_jobs_dedup_key;
DROP INDEX IF EXISTS idx_jobs_shard_key;
ALTER TABLE jobs DROP COLUMN IF EXISTS dedup_key;
ALTER TABLE jobs DROP COLUMN IF EXISTS dependencies;
ALTER TABLE jobs DROP COLUMN IF EXISTS shard_key;

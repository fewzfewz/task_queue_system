-- 004_add_progress_rollback.sql
ALTER TABLE jobs DROP COLUMN IF EXISTS progress;

-- 005_add_cron_expr_rollback.sql
ALTER TABLE jobs DROP COLUMN IF EXISTS cron_expr;

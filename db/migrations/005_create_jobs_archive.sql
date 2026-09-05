-- 001_create_jobs.sql
CREATE TABLE IF NOT EXISTS jobs_archive (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL,
    priority TEXT NOT NULL,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    error TEXT,
    result JSONB,
    correlation_id TEXT,
    timeout_seconds INTEGER DEFAULT 60,
    version INTEGER DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    scheduled_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    processed_by TEXT,
    last_heartbeat_at TIMESTAMP WITH TIME ZONE,
    webhook_url TEXT,
    webhook_secret TEXT,
    webhook_events TEXT[],
    webhook_last_status INTEGER,
    webhook_attempts INTEGER DEFAULT 0,
    error_history JSONB
    ,cron_expr TEXT
);

-- Indexes for performance and multi-tenancy isolation
CREATE INDEX idx_jobs_archive_tenant_status ON jobs_archive(tenant_id, status);
CREATE INDEX idx_jobs_archive_type_status_scheduled ON jobs_archive(type, status, scheduled_at);
CREATE INDEX idx_jobs_archive_correlation_id ON jobs_archive(correlation_id);
CREATE INDEX idx_jobs_archive_worker_status ON jobs_archive(processed_by, status);

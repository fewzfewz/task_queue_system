-- 001_create_jobs.sql
CREATE TABLE IF NOT EXISTS jobs (
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
    processed_by TEXT
);

-- Indexes for performance and multi-tenancy isolation
CREATE INDEX idx_jobs_tenant_status ON jobs(tenant_id, status);
CREATE INDEX idx_jobs_type_status_scheduled ON jobs(type, status, scheduled_at);
CREATE INDEX idx_jobs_correlation_id ON jobs(correlation_id);
CREATE INDEX idx_jobs_worker_status ON jobs(processed_by, status);

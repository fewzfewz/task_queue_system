CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor VARCHAR(255) NOT NULL,
    action VARCHAR(255) NOT NULL,
    target_type VARCHAR(255) NOT NULL,
    target_id VARCHAR(255) NOT NULL,
    details JSONB
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor);

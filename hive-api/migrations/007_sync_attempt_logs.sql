CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS sync_attempt_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id TEXT NOT NULL,
    source_dev_id TEXT NOT NULL,
    project TEXT NOT NULL,
    client TEXT,
    daemon_id TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
    http_status INTEGER,
    error_code TEXT,
    error_message TEXT,
    request_id TEXT,
    sync_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_dev_id, attempt_id)
);

CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_started_at ON sync_attempt_logs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_source_dev_started ON sync_attempt_logs (source_dev_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_project_started ON sync_attempt_logs (project, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_client_daemon_started ON sync_attempt_logs (client, daemon_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_outcome_started ON sync_attempt_logs (outcome, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_error_code_started ON sync_attempt_logs (error_code, started_at DESC);

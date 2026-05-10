-- Migración 004: audit_logs para trazabilidad admin y sync
-- Idempotente y compatible con PostgreSQL 15+

CREATE TABLE IF NOT EXISTS audit_logs (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    actor_user_id UUID         NULL,
    project       VARCHAR(100) NULL,
    action        VARCHAR(50)  NOT NULL,
    outcome       VARCHAR(30)  NOT NULL,
    entry_count   INT          NOT NULL DEFAULT 0,
    reason_code   TEXT         NULL,
    metadata      JSONB        NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at ON audit_logs(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id ON audit_logs(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_project ON audit_logs(project);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_outcome ON audit_logs(outcome);
CREATE INDEX IF NOT EXISTS idx_audit_logs_project_occurred_at ON audit_logs(project, occurred_at DESC);

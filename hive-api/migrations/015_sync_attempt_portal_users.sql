ALTER TABLE sync_attempt_logs
    ADD COLUMN IF NOT EXISTS portal_user_id UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS portal_user_source TEXT;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'sync_attempt_logs_portal_user_source_check') THEN
        ALTER TABLE sync_attempt_logs ADD CONSTRAINT sync_attempt_logs_portal_user_source_check
            CHECK (portal_user_source IS NULL OR portal_user_source IN ('auth_subject', 'admin_dev_id', 'legacy_email'));
    END IF;
END $$;

UPDATE sync_attempt_logs logs
SET portal_user_id = users.id, portal_user_source = 'legacy_email'
FROM users WHERE logs.portal_user_id IS NULL AND logs.source_dev_id = users.email;

CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_project_portal_user_latest
    ON sync_attempt_logs (project, portal_user_id, ended_at DESC, started_at DESC, ingested_at DESC, attempt_id DESC, id DESC);

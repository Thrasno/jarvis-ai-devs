CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_portal_user_completed_latest
    ON sync_attempt_logs (portal_user_id, ended_at DESC, ingested_at DESC, attempt_id DESC, id DESC)
    INCLUDE (outcome)
    WHERE portal_user_id IS NOT NULL AND ended_at IS NOT NULL;

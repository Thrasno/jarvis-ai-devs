-- Composite indexes supporting keyset pagination for bounded legacy pull (PR 2a).
-- Pull pagination orders by (synced_at ASC, sync_id ASC) and filters by project,
-- so these indexes let the postgres repository implementations satisfy
-- `WHERE (synced_at, sync_id) > (?, ?) [AND project = ?] ORDER BY synced_at, sync_id`
-- without a sequential scan as the memories/sessions tables grow.
CREATE INDEX IF NOT EXISTS idx_memories_synced_at_sync_id
    ON memories (synced_at, sync_id);

CREATE INDEX IF NOT EXISTS idx_sessions_synced_at_sync_id
    ON sessions (synced_at, sync_id);

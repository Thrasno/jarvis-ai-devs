-- Project-scoped indexes for bounded legacy pull keyset pagination.
-- Pull queries always filter by project before walking (synced_at, sync_id), so
-- leading with project avoids scanning a global synced_at/sync_id index across
-- tenants as the memories/sessions tables grow.
CREATE INDEX IF NOT EXISTS idx_memories_project_synced_at_sync_id
    ON memories (project, synced_at, sync_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_project_synced_at_sync_id
    ON sessions (project, synced_at, sync_id);

-- Remove indexes that are no longer the best match for their original purpose.
-- The single-column memories(synced_at) index is covered by the remaining
-- global (synced_at, sync_id) index for global live-activity reads.
DROP INDEX IF EXISTS idx_memories_synced_at;

-- Session pull is project-scoped and there are no global sessions synced_at
-- readers, so the migration 010 global sessions keyset index is duplicate
-- write-path overhead after the project-scoped replacement exists.
DROP INDEX IF EXISTS idx_sessions_synced_at_sync_id;

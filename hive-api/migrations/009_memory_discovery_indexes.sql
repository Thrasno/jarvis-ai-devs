CREATE INDEX IF NOT EXISTS idx_memories_active_created_at
    ON memories (created_at DESC, synced_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_memories_active_project_category_created_at
    ON memories (project, category, created_at DESC, synced_at DESC)
    WHERE deleted_at IS NULL;

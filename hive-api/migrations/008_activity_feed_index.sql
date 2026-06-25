CREATE INDEX IF NOT EXISTS idx_memory_mutations_activity_feed_recency
    ON memory_mutations (occurred_at DESC, sequence DESC, event_id DESC)
    WHERE entity_type = 'memory' AND op IN ('create', 'update', 'delete');

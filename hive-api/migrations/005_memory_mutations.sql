-- Additive soft-delete + domain mutation journal schema.
-- audit_logs intentionally remains operational/admin only.

ALTER TABLE memories ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS deleted_by VARCHAR(100);
ALTER TABLE memories ADD COLUMN IF NOT EXISTS delete_reason TEXT;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS restored_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS memory_mutations (
    sequence       BIGSERIAL PRIMARY KEY,
    event_id       UUID         NOT NULL UNIQUE,
    entity_type    VARCHAR(50)  NOT NULL,
    entity_sync_id UUID         NOT NULL,
    project        VARCHAR(100) NOT NULL,
    op             VARCHAR(20)  NOT NULL,
    occurred_at    TIMESTAMPTZ  NOT NULL,
    actor_id       VARCHAR(100),
    base_updated_at TIMESTAMPTZ,
    memory         JSONB,
    tombstone      JSONB,
    synced_at      TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_memory_mutations_entity CHECK (entity_type = 'memory'),
    CONSTRAINT chk_memory_mutations_op CHECK (op IN ('create', 'update', 'delete', 'restore'))
);

CREATE TABLE IF NOT EXISTS mutation_cursors (
    consumer_id VARCHAR(200) NOT NULL,
    project     VARCHAR(100) NOT NULL,
    sequence    BIGINT       NOT NULL DEFAULT 0,
    event_id    UUID,
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_id, project)
);

CREATE INDEX IF NOT EXISTS idx_memories_active_project
    ON memories(project, synced_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memory_mutations_project_sequence_event
    ON memory_mutations(project, sequence ASC, event_id ASC);

CREATE INDEX IF NOT EXISTS idx_memory_mutations_entity
    ON memory_mutations(entity_type, entity_sync_id, sequence DESC);

CREATE INDEX IF NOT EXISTS idx_mutation_cursors_project
    ON mutation_cursors(project, sequence DESC);

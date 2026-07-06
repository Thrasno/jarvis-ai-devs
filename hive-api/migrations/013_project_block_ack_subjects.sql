ALTER TABLE project_block_acks
    ADD COLUMN IF NOT EXISTS ack_auth_subject text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ack_daemon_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ack_client text NOT NULL DEFAULT '';

ALTER TABLE project_block_acks
    DROP CONSTRAINT IF EXISTS project_block_acks_command_id_canonical_project_key_key;

DELETE FROM project_block_acks a
USING project_block_acks b
WHERE a.command_id = b.command_id
  AND a.canonical_project_key = b.canonical_project_key
  AND a.ack_auth_subject = b.ack_auth_subject
  AND (a.updated_at, a.created_at, a.id::text) < (b.updated_at, b.created_at, b.id::text);

DO $$
DECLARE
    c record;
BEGIN
    FOR c IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'project_block_acks'::regclass
          AND contype = 'u'
          AND pg_get_constraintdef(oid) = 'UNIQUE (command_id, canonical_project_key, ack_auth_subject, ack_daemon_id, ack_client)'
    LOOP
        EXECUTE format('ALTER TABLE project_block_acks DROP CONSTRAINT %I', c.conname);
    END LOOP;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'project_block_acks_subject_key'
    ) THEN
        ALTER TABLE project_block_acks
            ADD CONSTRAINT project_block_acks_subject_key UNIQUE (command_id, canonical_project_key, ack_auth_subject);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS project_block_ack_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    command_id uuid NOT NULL,
    canonical_project_key text NOT NULL REFERENCES project_blocks(canonical_project_key) ON DELETE CASCADE,
    ack_token text NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    ack_auth_subject text NOT NULL,
    ack_daemon_id text NOT NULL,
    ack_client text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (command_id, canonical_project_key, ack_auth_subject)
);

DELETE FROM project_block_ack_deliveries a
USING project_block_ack_deliveries b
WHERE a.command_id = b.command_id
  AND a.canonical_project_key = b.canonical_project_key
  AND a.ack_auth_subject = b.ack_auth_subject
  AND (a.updated_at, a.created_at, a.id::text) < (b.updated_at, b.created_at, b.id::text);

DO $$
DECLARE
    c record;
BEGIN
    FOR c IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'project_block_ack_deliveries'::regclass
          AND contype = 'u'
          AND pg_get_constraintdef(oid) = 'UNIQUE (command_id, canonical_project_key, ack_auth_subject, ack_daemon_id, ack_client)'
    LOOP
        EXECUTE format('ALTER TABLE project_block_ack_deliveries DROP CONSTRAINT %I', c.conname);
    END LOOP;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'project_block_ack_deliveries_subject_key'
    ) THEN
        ALTER TABLE project_block_ack_deliveries
            ADD CONSTRAINT project_block_ack_deliveries_subject_key UNIQUE (command_id, canonical_project_key, ack_auth_subject);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_project_block_ack_deliveries_subject
    ON project_block_ack_deliveries(canonical_project_key, command_id, ack_auth_subject);

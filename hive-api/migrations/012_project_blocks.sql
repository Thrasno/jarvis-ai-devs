CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- This file used to CREATE OR REPLACE FUNCTION canonical_project_key(text), an
-- in-SQL fold of project spellings. It is gone: the daemon is the sole authority
-- on project identity and the API matches the literal it stores.
--
-- It is removed here rather than left for migration 021 to drop, because there
-- is no migration ledger — every boot replays every file in order, so this CREATE
-- re-created on each startup what 021 drops, and the function existed for the
-- whole window in between. 021 still drops it, for databases that already have it.
--
-- canonical_project_key below is a plain text column holding a stored literal.
-- It shares the name and nothing else.

CREATE TABLE IF NOT EXISTS project_blocks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    command_id uuid NOT NULL DEFAULT gen_random_uuid(),
    ack_token text NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    project text NOT NULL,
    canonical_project_key text NOT NULL UNIQUE,
    action text NOT NULL,
    reason text NOT NULL,
    confirmation text NOT NULL,
    export_marker text NOT NULL,
    actor_user_id text,
    blocked boolean NOT NULL DEFAULT true,
    blocked_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_block_acks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    command_id uuid NOT NULL,
    canonical_project_key text NOT NULL REFERENCES project_blocks(canonical_project_key) ON DELETE CASCADE,
    ack_token text NOT NULL,
	ack_auth_subject text NOT NULL DEFAULT '',
	ack_daemon_id text NOT NULL DEFAULT '',
	ack_client text NOT NULL DEFAULT '',
    status text NOT NULL,
    warning text NOT NULL DEFAULT '',
    applied_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (command_id, canonical_project_key, ack_auth_subject)
);

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

CREATE INDEX IF NOT EXISTS idx_project_blocks_canonical_blocked ON project_blocks(canonical_project_key) WHERE blocked = true;
CREATE INDEX IF NOT EXISTS idx_project_blocks_blocked_at ON project_blocks(blocked_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_block_acks_canonical ON project_block_acks(canonical_project_key, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_block_ack_deliveries_subject ON project_block_ack_deliveries(canonical_project_key, command_id, ack_auth_subject);

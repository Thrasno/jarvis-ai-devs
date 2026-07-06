CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION canonical_project_key(input text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT trim(both '-' from regexp_replace(lower(btrim(coalesce(input, ''))), '[^a-z0-9.]+', '-', 'g'))
$$;

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

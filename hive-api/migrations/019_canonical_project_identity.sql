-- The registry of project literals the API has observed. project_key holds the
-- literal itself: the API never derives identity, so this table records names,
-- it does not group them. Additive, so older API clients keep using project.
CREATE TABLE IF NOT EXISTS project_identities (
    project_key text PRIMARY KEY,
    first_spelling text NOT NULL,
    first_seen_at timestamptz NOT NULL,
    remote_spelling text,
    remote_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (btrim(project_key) <> ''),
    CHECK (remote_spelling IS NULL OR btrim(remote_spelling) <> ''),
    CHECK ((remote_spelling IS NULL) = (remote_seen_at IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_project_identities_first_seen
    ON project_identities (first_seen_at ASC, project_key ASC);

-- A separate spelling->key table used to live here. It is gone: the registry is
-- keyed by the project literal, so a spelling map would be redundant with
-- project_identities and is the join that leaked one project's rows to another.
-- Migration 021 drops it from databases that already have it.

DO $$
DECLARE
    child_table text;
    constraint_name text;
BEGIN
    FOREACH child_table IN ARRAY ARRAY['project_block_acks', 'project_block_ack_deliveries']
    LOOP
        SELECT conname INTO constraint_name
        FROM pg_constraint
        WHERE conrelid = child_table::regclass
          AND contype = 'f'
          AND confrelid = 'project_blocks'::regclass;

        IF constraint_name IS NOT NULL THEN
            EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', child_table, constraint_name);
        END IF;
        EXECUTE format(
            'ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (canonical_project_key) REFERENCES project_blocks(canonical_project_key) ON UPDATE CASCADE ON DELETE CASCADE',
            child_table, child_table || '_canonical_project_key_fkey');
        constraint_name := NULL;
    END LOOP;
END $$;

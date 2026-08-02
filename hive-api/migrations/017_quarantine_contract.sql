-- Keep historical project-block rows readable while allowing current clients
-- to omit the legacy export marker. Application validation rejects unsupported
-- actions before a transition can mutate audit or generation state.
ALTER TABLE project_blocks
    ALTER COLUMN export_marker DROP NOT NULL;

ALTER TABLE project_blocks
    ADD COLUMN IF NOT EXISTS generation bigint;

UPDATE project_blocks SET generation = 1 WHERE generation IS NULL;

ALTER TABLE project_blocks
    ALTER COLUMN generation SET NOT NULL;

ALTER TABLE project_blocks
    ALTER COLUMN generation SET DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_project_blocks_generation
    ON project_blocks(canonical_project_key, generation DESC);

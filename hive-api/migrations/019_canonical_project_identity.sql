-- Canonical keys are derived only by the shared Go projectidentity contract.
-- This registry is additive so older API clients can continue using project.
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

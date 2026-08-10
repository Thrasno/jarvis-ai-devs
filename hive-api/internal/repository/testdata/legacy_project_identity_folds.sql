-- The three project-identity constructs that shipped to production, verbatim
-- from the migrations that created them (012 and 019 as they were released).
--
-- This file is a FIXTURE, not a migration. It exists so a test can put a
-- database into the state an upgraded production database is actually in — the
-- fold present, the spelling registry present, the identity registry present —
-- and then prove migrations 021 and 022 remove all three. Seeding this by hand
-- is the only way to reach that state, since 019 still creates the identity
-- registry on every boot and 022 is what makes its removal final.
--
-- It lives under testdata/ so the Go toolchain ignores it and so the identity
-- guard skips it; nothing here is ever applied to a real database.

CREATE OR REPLACE FUNCTION canonical_project_key(input text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT trim(both '-' from regexp_replace(lower(btrim(coalesce(input, ''))), '[^a-z0-9.]+', '-', 'g'))
$$;

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

CREATE TABLE IF NOT EXISTS project_identity_spellings (
    spelling text PRIMARY KEY,
    project_key text NOT NULL REFERENCES project_identities(project_key) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_project_identity_spellings_key
    ON project_identity_spellings (project_key);

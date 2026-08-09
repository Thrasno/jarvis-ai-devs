-- Remove the two SQL constructs that could derive project identity.
--
-- The API stores the literal it receives and matches by exact equality; the
-- daemon is the sole authority on identity. Neither of these had a legitimate
-- reader left, and each was the mechanism of a real defect:
--
--   * project_identity_spellings mapped every observed spelling to a canonical
--     key. The dashboard's memory list joined it, so ?project=foo-bar returned
--     rows stored as "Foo.Bar" — a cross-tenant read. project_identities is now
--     keyed by the literal, which makes this table exactly redundant with it.
--
--   * canonical_project_key(text) folded spellings inside SQL, diverging from
--     the Go contract it shadowed. The quarantine predicate used it and
--     quarantined unrelated projects.
--
-- Dropping them means a future predicate that tries to fold identity in SQL
-- fails loudly instead of silently widening a project scope.
DROP TABLE IF EXISTS project_identity_spellings;

DROP FUNCTION IF EXISTS canonical_project_key(text);

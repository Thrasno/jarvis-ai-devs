-- Issue #119: topic_key changes from upsert/identity key to grouping/context key.
-- Drop the partial UNIQUE index so multiple rows may share (project, topic_key);
-- sync_id remains the idempotency key. Replace with a non-unique index so
-- grouping/most-recent-per-topic_key retrieval stays fast.
--
-- IMPORTANT — applicability note:
-- This migration is meaningful ONLY for databases deployed with the pre-Issue-#119
-- schema, which created idx_memories_project_topic_key as a UNIQUE partial index
-- inside 001_initial.sql. On fresh installs from the updated 001_initial.sql,
-- that UNIQUE index never existed, so the DROP below is a safe no-op (IF EXISTS
-- guarantees no error). Similarly, the CREATE INDEX below is also a no-op on
-- fresh installs because 001_initial.sql already creates idx_memories_topic_key
-- as a non-unique index.

DROP INDEX IF EXISTS idx_memories_project_topic_key;

CREATE INDEX IF NOT EXISTS idx_memories_topic_key
    ON memories(project, topic_key)
    WHERE topic_key IS NOT NULL;

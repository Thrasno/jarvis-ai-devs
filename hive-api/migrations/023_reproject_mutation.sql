-- Migration 023: the daemon can tell the server that a memory's project literal
-- changed, as a first-class mutation op.
--
-- The daemon is the sole authority on project identity. Its local identity
-- migration rewrites its own rows ("Foo.Bar" -> "foo-bar"), and until now that
-- decision could not reach the server: the memory mutation protocol has no op
-- that changes `project`, and `update` uses project to FIND the row.
--
-- Idempotent, like every migration in this directory.

ALTER TABLE memory_mutations ADD COLUMN IF NOT EXISTS reproject JSONB;

ALTER TABLE memory_mutations DROP CONSTRAINT IF EXISTS chk_memory_mutations_op;
ALTER TABLE memory_mutations ADD CONSTRAINT chk_memory_mutations_op
    CHECK (op IN ('create', 'update', 'delete', 'restore', 'reproject'));

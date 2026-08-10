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

-- Widening the op check is guarded on the CONSTRAINT'S DEFINITION, not on its
-- name, and the distinction is the whole point.
--
-- There is no migration ledger here: migrations.Ordered() replays this file on
-- every boot. An unguarded DROP + ADD therefore took ACCESS EXCLUSIVE on
-- memory_mutations and full-scanned the journal to validate the new check every
-- single restart, at a cost that grows with the journal — plain downtime on a
-- single-instance deploy.
--
-- Guarding on the name alone (the 015 pattern) would have been worse than doing
-- nothing: a database that predates this migration already carries a constraint
-- with this exact name listing only four ops, so the migration would skip and
-- the server would reject every reproject it is here to journal. Matching on the
-- definition upgrades that database and leaves an already-correct one untouched.
DO $$
DECLARE
    current_definition TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO current_definition
    FROM pg_constraint
    WHERE conrelid = 'memory_mutations'::regclass
      AND conname = 'chk_memory_mutations_op';

    IF current_definition IS NULL OR position('''reproject''' IN current_definition) = 0 THEN
        IF current_definition IS NOT NULL THEN
            ALTER TABLE memory_mutations DROP CONSTRAINT chk_memory_mutations_op;
        END IF;
        ALTER TABLE memory_mutations ADD CONSTRAINT chk_memory_mutations_op
            CHECK (op IN ('create', 'update', 'delete', 'restore', 'reproject'));
    END IF;
END $$;

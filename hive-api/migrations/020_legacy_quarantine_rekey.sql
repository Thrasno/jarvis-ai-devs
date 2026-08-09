-- Repoint every pre-contract quarantine at the literal its admin named.
--
-- Blocks written before the exact-equality contract carry the admin's literal
-- in `project` and a canonicalized key in `canonical_project_key`. The read
-- predicate and the write-side check now both compare a stored row literal
-- against `canonical_project_key`, so those blocks match no rows at all — while
-- Status and ListQuarantines still report them as active. This is a one-time
-- correction, NOT a startup rekey: rekeying on every boot would repoint a live
-- block written under the contract.
--
-- Row-by-row, so each UPDATE sees the ones before it and the collision check
-- stays accurate within the transaction.
DO $$
DECLARE
    legacy RECORD;
BEGIN
    FOR legacy IN
        SELECT id, project, canonical_project_key AS legacy_key
        FROM project_blocks
        WHERE canonical_project_key <> project
        -- Active quarantines claim their literal first, then oldest wins.
        ORDER BY blocked DESC, created_at ASC, id ASC
    LOOP
        -- canonical_project_key is UNIQUE. A block written under the contract
        -- already keys on this literal and is authoritative for that project, so
        -- the legacy row is left exactly as it is rather than failing the
        -- deployment. It keeps quarantining precisely what it quarantined
        -- before, so skipping never widens or narrows any block.
        IF EXISTS (
            SELECT 1 FROM project_blocks other
            WHERE other.id <> legacy.id
              AND other.canonical_project_key = legacy.project
        ) THEN
            CONTINUE;
        END IF;

        -- project_quarantine_commands has no foreign key, so its history must be
        -- moved explicitly or QuarantineProgress loses the block's generations.
        -- UNIQUE (canonical_project_key, generation) can already be taken by the
        -- colliding block's own history; those rows stay where they are.
        UPDATE project_quarantine_commands command
        SET canonical_project_key = legacy.project
        WHERE command.canonical_project_key = legacy.legacy_key
          AND command.project = legacy.project
          AND NOT EXISTS (
              SELECT 1 FROM project_quarantine_commands other
              WHERE other.canonical_project_key = legacy.project
                AND other.generation = command.generation
          );

        -- project_block_acks and project_block_ack_deliveries follow through
        -- ON UPDATE CASCADE (migration 019).
        UPDATE project_blocks
        SET canonical_project_key = legacy.project, updated_at = now()
        WHERE id = legacy.id;
    END LOOP;
END $$;

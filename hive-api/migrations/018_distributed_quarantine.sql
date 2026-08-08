-- Keep an immutable, generation-addressable command history while
-- project_blocks remains the current cloud quarantine head.
CREATE TABLE IF NOT EXISTS project_quarantine_commands (
    command_id uuid PRIMARY KEY,
    canonical_project_key text NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    project text NOT NULL,
    action text NOT NULL CHECK (action IN ('block', 'unblock')),
    reason text NOT NULL,
    actor_user_id text,
    blocked_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (canonical_project_key, generation)
);

INSERT INTO project_quarantine_commands (command_id, canonical_project_key, generation, project, action, reason, actor_user_id, blocked_at)
SELECT command_id, canonical_project_key, generation, project, action, reason, actor_user_id, blocked_at
FROM project_blocks
WHERE action IN ('block', 'unblock')
ON CONFLICT (command_id) DO NOTHING;

CREATE OR REPLACE FUNCTION record_project_quarantine_command() RETURNS trigger AS $$
BEGIN
    IF NEW.action IN ('block', 'unblock') THEN
        INSERT INTO project_quarantine_commands (command_id, canonical_project_key, generation, project, action, reason, actor_user_id, blocked_at)
        VALUES (NEW.command_id, NEW.canonical_project_key, NEW.generation, NEW.project, NEW.action, NEW.reason, NEW.actor_user_id, NEW.blocked_at)
        ON CONFLICT (command_id) DO UPDATE SET generation = EXCLUDED.generation;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS project_blocks_record_quarantine_command ON project_blocks;
CREATE TRIGGER project_blocks_record_quarantine_command
AFTER INSERT OR UPDATE OF command_id, generation ON project_blocks
FOR EACH ROW EXECUTE FUNCTION record_project_quarantine_command();

CREATE INDEX IF NOT EXISTS idx_project_quarantine_commands_delivery
    ON project_quarantine_commands (canonical_project_key, generation ASC);

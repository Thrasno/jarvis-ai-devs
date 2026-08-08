package migrations

import (
	"strings"
	"testing"
)

func TestQuarantineContractMigrationPreservesLegacyFieldsForMixedVersions(t *testing.T) {
	for _, statement := range []string{
		"ALTER COLUMN export_marker DROP NOT NULL",
		"ADD COLUMN IF NOT EXISTS generation bigint",
		"UPDATE project_blocks SET generation = 1",
	} {
		if !strings.Contains(QuarantineContractSQL, statement) {
			t.Fatalf("contract migration must contain %q", statement)
		}
	}
}

func TestDistributedQuarantineMigrationRetainsImmutableCommands(t *testing.T) {
	for _, statement := range []string{
		"CREATE TABLE IF NOT EXISTS project_quarantine_commands",
		"UNIQUE (canonical_project_key, generation)",
		"CREATE TRIGGER project_blocks_record_quarantine_command",
		"AFTER INSERT OR UPDATE OF command_id, generation ON project_blocks",
	} {
		if !strings.Contains(DistributedQuarantineSQL, statement) {
			t.Fatalf("distributed lifecycle migration must contain %q", statement)
		}
	}
}

func TestCanonicalProjectRegistryMigrationUsesSharedKeysAndPreservesLegacyRows(t *testing.T) {
	for _, statement := range []string{
		"CREATE TABLE IF NOT EXISTS project_identities",
		"project_key text PRIMARY KEY",
		"first_spelling text NOT NULL",
		"remote_spelling text",
		"first_seen_at timestamptz NOT NULL",
		"remote_seen_at timestamptz",
		"CHECK (btrim(project_key) <> '')",
		"idx_project_identities_first_seen",
	} {
		if !strings.Contains(CanonicalProjectRegistrySQL, statement) {
			t.Fatalf("canonical project registry migration must contain %q", statement)
		}
	}
}

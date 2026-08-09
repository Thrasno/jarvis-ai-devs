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

// TestDropProjectIdentityRegistryMigrationDropsTheTableWithoutCascade pins the
// shape of the drop. CASCADE would take unnamed dependents with it; a plain
// DROP fails loudly instead, which is what we want if anything still points
// here. TestFullMigrationSetRemovesTheLegacyProjectIdentitySchema proves the
// effect on a database that already has the table.
func TestDropProjectIdentityRegistryMigrationDropsTheTableWithoutCascade(t *testing.T) {
	if !strings.Contains(DropProjectIdentityRegistrySQL, "DROP TABLE IF EXISTS project_identities;") {
		t.Fatal("registry drop migration must drop project_identities")
	}
	if strings.Contains(DropProjectIdentityRegistrySQL, "CASCADE") {
		t.Fatal("registry drop migration must not CASCADE; an unexpected dependent has to fail loudly")
	}
}

// TestOrderedRunsTheRegistryDropAfterTheSpellingDrop pins the one ordering this
// module cannot get wrong. project_identity_spellings carries a foreign key to
// project_identities on every upgraded database, so dropping the registry first
// would fail. There is no migration ledger here — order IS the contract.
func TestOrderedRunsTheRegistryDropAfterTheSpellingDrop(t *testing.T) {
	ordered := Ordered()
	folds, registry := -1, -1
	for index, sql := range ordered {
		switch sql {
		case DropProjectIdentityFoldsSQL:
			folds = index
		case DropProjectIdentityRegistrySQL:
			registry = index
		}
	}
	if folds < 0 || registry < 0 {
		t.Fatalf("both drop migrations must be in Ordered(); folds=%d registry=%d", folds, registry)
	}
	if registry < folds {
		t.Fatalf("the registry drop must run after the spelling drop; folds=%d registry=%d", folds, registry)
	}
}

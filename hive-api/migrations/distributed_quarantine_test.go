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

// TestCanonicalProjectRegistryMigrationKeepsOnlyItsSurvivingEffect pins what is
// left of migration 019 once the ordered set has finished running.
//
// 019 is unchanged on purpose: it is the record of a schema that shipped. But
// the identity registry it creates is no longer part of this module's schema —
// the registry drop removes it in the same boot pass, before the server accepts
// a request — so pinning that table's columns and index here would assert a
// registry that does not outlive startup. What survives 019 is the child
// foreign keys it re-points at project_blocks, and the ordering that guarantees
// the registry is gone by the end of the pass.
func TestCanonicalProjectRegistryMigrationKeepsOnlyItsSurvivingEffect(t *testing.T) {
	for _, statement := range []string{
		"'project_block_acks', 'project_block_ack_deliveries'",
		"REFERENCES project_blocks(canonical_project_key)",
	} {
		if !strings.Contains(CanonicalProjectRegistrySQL, statement) {
			t.Fatalf("canonical project registry migration must contain %q", statement)
		}
	}

	ordered := Ordered()
	registry, drop := -1, -1
	for index, sql := range ordered {
		switch sql {
		case CanonicalProjectRegistrySQL:
			registry = index
		case DropProjectIdentityRegistrySQL:
			drop = index
		}
	}
	if registry < 0 || drop < 0 {
		t.Fatalf("both migrations must be in Ordered(); registry=%d drop=%d", registry, drop)
	}
	if drop < registry {
		t.Fatalf("the drop must run after the migration that creates the registry; registry=%d drop=%d", registry, drop)
	}
}

// TestDropProjectIdentityRegistryMigrationDropsTheTableWithoutCascade pins the
// shape of the drop. CASCADE would take unnamed dependents with it; a plain
// DROP fails loudly instead, which is what we want if anything still points
// here. TestFullMigrationSetRemovesTheLegacyProjectIdentitySchema proves the
// effect on a database that already has the table.
func TestDropProjectIdentityRegistryMigrationDropsTheTableWithoutCascade(t *testing.T) {
	if !strings.Contains(DropProjectIdentityRegistrySQL, "DROP TABLE IF EXISTS project_identities;") {
		t.Fatal("registry drop migration must drop the identity registry table")
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

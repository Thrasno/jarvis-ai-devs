package sddstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchivedAuthorityRejectsSymlinkedEvidenceAncestor(t *testing.T) {
	base := t.TempDir()
	actual := filepath.Join(base, "actual", "apply-evidence")
	if err := os.MkdirAll(actual, 0o755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "linked")
	if err := os.Symlink(filepath.Join(base, "actual"), linked); err != nil {
		t.Fatal(err)
	}
	if validArchivedEvidenceTopology(filepath.Join(linked, "apply-evidence")) {
		t.Fatal("validArchivedEvidenceTopology() accepted a symlinked authoritative ancestor")
	}
}

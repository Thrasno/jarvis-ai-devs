package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

// `hive project identity rollback` printed "The managed daemon will restart,
// restore it before reopening SQLite" unconditionally. That is only true on the
// BLOCKED path, where the daemon schedules the restore itself and stops.
//
// When the migration gate is ready the daemon takes the other branch entirely:
// RestoreBackup is PlanRestore, which only validates the archive and answers
// coordination_required (202). Nothing is scheduled, nothing is restored, and
// the operator must stop the daemon themselves — while the CLI told them it was
// already handled.
func TestRollbackReportsCoordinationRequiredInsteadOfAPromisedRestart(t *testing.T) {
	client := &fakeHiveClient{restoreResult: hiveclient.RestoreResult{
		BackupID:              "migration-backup-1",
		Status:                "coordination_required",
		RequiresDaemonRestart: true,
		Message:               "restore archive validated; stop/restart daemon coordination is required before replacing the live database",
	}}

	out, err := executeHiveCommand(t, NewRootCommand(client), "project", "identity", "rollback",
		"--backup-id", "migration-backup-1", "--confirmation", "RESTORE migration-backup-1")
	if err != nil {
		t.Fatalf("identity rollback: %v", err)
	}

	if strings.Contains(out, "was scheduled") || strings.Contains(out, "will restart") {
		t.Fatalf("output promises a restart the daemon never scheduled: %q", out)
	}
	for _, want := range []string{"coordination_required", "not applied", "Stop"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "stop/restart daemon coordination is required") {
		t.Errorf("the daemon's own explanation must reach the operator: %q", out)
	}
}

// The blocked path really does schedule the restore, and must keep saying so.
func TestRollbackStillReportsTheScheduledRestartOnTheBlockedPath(t *testing.T) {
	client := &fakeHiveClient{restoreResult: hiveclient.RestoreResult{
		BackupID:              "migration-backup-1",
		Status:                "restart-requested",
		RequiresDaemonRestart: true,
		Message:               "restore scheduled; the daemon will restart and restore the backup before reopening SQLite",
	}}

	out, err := executeHiveCommand(t, NewRootCommand(client), "project", "identity", "rollback",
		"--backup-id", "migration-backup-1", "--confirmation", "RESTORE migration-backup-1")
	if err != nil {
		t.Fatalf("identity rollback: %v", err)
	}
	if !strings.Contains(out, "was scheduled") {
		t.Fatalf("output = %q, want the managed restore continuation", out)
	}
	if strings.Contains(out, "Run: hive project identity retry") {
		t.Fatalf("output = %q, want no manual retry on the managed path", out)
	}
}

// A migration backup is retained for 24h and then reclaimed. It is the ONLY
// rollback artifact, so an operator who cannot see its deadline discovers it by
// finding the backup gone.
func TestBackupsListingSurfacesTheRetentionDeadline(t *testing.T) {
	retainUntil := time.Date(2026, 8, 10, 9, 30, 0, 0, time.UTC)
	client := &fakeHiveClient{backups: []hiveclient.Backup{{
		ID:          "migration-backup-1",
		SizeBytes:   4096,
		ArchivePath: "/backups/migration-backup-1.tar",
		CreatedAt:   retainUntil.Add(-24 * time.Hour),
		RetainUntil: retainUntil,
	}}}

	out, err := executeHiveCommand(t, NewRootCommand(client), "backups")
	if err != nil {
		t.Fatalf("backups: %v", err)
	}
	if !strings.Contains(out, "retain_until=") {
		t.Fatalf("backups listing does not surface retention: %q", out)
	}
	if !strings.Contains(out, formatTime(retainUntil)) {
		t.Fatalf("backups listing = %q, want the retention deadline %s", out, formatTime(retainUntil))
	}
}

// Not every backup expires — an operator-created one has no deadline, and must
// not be rendered as if it were about to vanish.
func TestBackupsListingMarksAnUnboundedRetentionAsSuch(t *testing.T) {
	client := &fakeHiveClient{backups: []hiveclient.Backup{{
		ID: "manual-backup-1", SizeBytes: 4096, ArchivePath: "/backups/manual-backup-1.tar",
	}}}

	out, err := executeHiveCommand(t, NewRootCommand(client), "backups")
	if err != nil {
		t.Fatalf("backups: %v", err)
	}
	if !strings.Contains(out, "retain_until=-") {
		t.Fatalf("backups listing = %q, want an explicit dash for no deadline", out)
	}
}

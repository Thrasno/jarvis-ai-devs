package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddspecsync"
)

const (
	specSyncChange  = "issue-784"
	specSyncProject = "jarvis-dev"

	authMainSpec = "# Auth Specification\n\n## Requirements\n\n" +
		"### Requirement: Login\n\nThe system MUST log users in.\n\n" +
		"### Requirement: Legacy Token\n\nThe system MUST accept legacy tokens.\n"
	logoutRequirement = "### Requirement: Logout\n\nThe system MUST end sessions.\n"
	authAddedDelta    = "# Delta for Auth\n\n## Notes\n\nContext only.\n\n## ADDED Requirements\n\n" + logoutRequirement
	authRemovedDelta  = "# Delta for Auth\n\n## REMOVED Requirements\n\n### Requirement: Legacy Token\n\n" +
		"(Reason: Legacy tokens were retired.)\n(Migration: Clients use OAuth tokens.)\n"
	authInvalidDelta = "# Delta for Auth\n\n## MODIFIED Requirements\n\n### Requirement: Ghost\n\nText.\n"

	authMainPath     = "openspec/specs/auth/spec.md"
	deliveryMainPath = "openspec/specs/delivery/spec.md"
)

type specSyncArchiveFixture struct {
	workspace   string
	root        string
	destination string
}

// newSpecSyncArchiveFixture prepares an archive-ready OpenSpec-bound change
// whose delta specs are the fixture's new "delivery" capability plus an auth
// delta against an existing auth main spec.
func newSpecSyncArchiveFixture(t *testing.T, authDelta string) specSyncArchiveFixture {
	t.Helper()
	workspace := canonicalSddTestWorkspace(t)
	root, _ := writeBoundArchiveReadyOpenSpec(t, workspace, specSyncChange, "# Archive report\n")
	writeOpenSpecBinding(t, workspace, specSyncChange, "openspec", "persisted:test", nil)
	for path, content := range map[string]string{
		filepath.Join(workspace, filepath.FromSlash(authMainPath)): authMainSpec,
		filepath.Join(root, "specs", "auth", "spec.md"):            authDelta,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/"+specSyncChange+"/store-binding", "project="+specSyncProject, "")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	return specSyncArchiveFixture{
		workspace:   workspace,
		root:        root,
		destination: filepath.Join(workspace, "openspec", "changes", "archive", "2026-10-10-"+specSyncChange),
	}
}

func (f specSyncArchiveFixture) run(t *testing.T, flags ...string) (archiveOutput, error) {
	t.Helper()
	command := newBoundSddArchiveCommand()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(append([]string{"--root", f.root, "--destination", f.destination, "--project", specSyncProject}, flags...))
	err := command.Execute()
	var output archiveOutput
	if decodeErr := json.Unmarshal(stdout.Bytes(), &output); decodeErr != nil {
		t.Fatalf("archive stdout is not JSON: %v\n%s", decodeErr, stdout.String())
	}
	return output, err
}

func (f specSyncArchiveFixture) read(t *testing.T, slashPath string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.workspace, filepath.FromSlash(slashPath)))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func (f specSyncArchiveFixture) requireNotMoved(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(f.root, "apply-progress.md")); err != nil {
		t.Fatalf("change root moved: %v", err)
	}
	if _, err := os.Stat(f.destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive destination exists: %v", err)
	}
}

func (f specSyncArchiveFixture) requireSpecsUntouched(t *testing.T) {
	t.Helper()
	if got, _ := f.read(t, authMainPath); got != authMainSpec {
		t.Fatalf("auth main spec changed: %q", got)
	}
	if _, exists := f.read(t, deliveryMainPath); exists {
		t.Fatal("delivery main spec was created")
	}
}

func sha256Digest(data string) string {
	sum := sha256.Sum256([]byte(data))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func findSpecSyncTarget(t *testing.T, output archiveOutput, capability string) specSyncTargetOutput {
	t.Helper()
	if output.SpecSync == nil {
		t.Fatalf("output has no spec_sync: %+v", output)
	}
	for _, target := range output.SpecSync.Targets {
		if target.Capability == capability {
			return target
		}
	}
	t.Fatalf("spec_sync has no %q target: %+v", capability, output.SpecSync.Targets)
	return specSyncTargetOutput{}
}

func TestBoundSddArchiveSyncsSpecsThenMoves(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authAddedDelta)

	output, err := f.run(t)
	if err != nil {
		t.Fatalf("archive error = %v", err)
	}
	wantAuth := authMainSpec + "\n" + logoutRequirement
	if got, _ := f.read(t, authMainPath); got != wantAuth {
		t.Fatalf("auth main spec = %q, want %q", got, wantAuth)
	}
	delivery, exists := f.read(t, deliveryMainPath)
	if !exists || delivery != boundArchiveDeliveryDelta {
		t.Fatalf("delivery main spec = %q (exists=%t), want the new capability copied", delivery, exists)
	}
	if _, err := os.Stat(filepath.Join(f.destination, "apply-progress.md")); err != nil {
		t.Fatalf("change not moved after sync: %v", err)
	}
	if _, err := os.Stat(f.root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active change root still exists: %v", err)
	}

	if output.Outcome != "archived" || output.Code != "" || output.Destination != f.destination || !output.SpecSync.Synced {
		t.Fatalf("output = %+v", output)
	}
	if _, err := os.Stat(filepath.Join(f.destination, specSyncJournalName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spec sync journal left in the archived change: %v", err)
	}
	auth := findSpecSyncTarget(t, output, "auth")
	if auth.Path != authMainPath || auth.Action != "updated" || auth.BeforeDigest != sha256Digest(authMainSpec) ||
		auth.AfterDigest != sha256Digest(wantAuth) || strings.Join(auth.Added, ",") != "Logout" ||
		strings.Join(auth.IgnoredSections, ",") != "Notes" {
		t.Fatalf("auth target = %+v", auth)
	}
	created := findSpecSyncTarget(t, output, "delivery")
	if created.Path != deliveryMainPath || created.Action != "created" || created.BeforeDigest != sddspecsync.AbsentDigest || created.AfterDigest != sha256Digest(delivery) {
		t.Fatalf("delivery target = %+v", created)
	}
}

func TestBoundSddArchiveDestructiveSyncRequiresConfirmation(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authRemovedDelta)

	output, err := f.run(t)
	if err == nil || !strings.Contains(err.Error(), sddspecsync.CodeConfirmationRequired) {
		t.Fatalf("archive error = %v, want %s", err, sddspecsync.CodeConfirmationRequired)
	}
	if output.Outcome != "blocked" || output.Code != sddspecsync.CodeConfirmationRequired || output.Recovery == "" ||
		output.SpecSync == nil || !output.SpecSync.Destructive || output.SpecSync.Synced {
		t.Fatalf("output = %+v", output)
	}
	if len(output.SpecSync.Removals) != 1 || output.SpecSync.Removals[0].Capability != "auth" ||
		strings.Join(output.SpecSync.Removals[0].Requirements, ",") != "Legacy Token" {
		t.Fatalf("removals = %+v", output.SpecSync.Removals)
	}
	f.requireSpecsUntouched(t)
	f.requireNotMoved(t)

	output, err = f.run(t, "--confirm-destructive")
	if err != nil {
		t.Fatalf("confirmed archive error = %v", err)
	}
	want := "# Auth Specification\n\n## Requirements\n\n### Requirement: Login\n\nThe system MUST log users in.\n"
	if got, _ := f.read(t, authMainPath); got != want || output.Outcome != "archived" {
		t.Fatalf("confirmed archive auth = %q outcome = %q", got, output.Outcome)
	}
	if auth := findSpecSyncTarget(t, output, "auth"); strings.Join(auth.Removed, ",") != "Legacy Token" {
		t.Fatalf("auth target = %+v", auth)
	}
}

func TestBoundSddArchivePlanWritesNothing(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authRemovedDelta)
	specsBefore := snapshotBoundArchiveFilesystem(t, filepath.Join(f.workspace, "openspec", "specs"))
	changeBefore := snapshotBoundArchiveFilesystem(t, f.root)

	output, err := f.run(t, "--plan")
	if err != nil {
		t.Fatalf("archive --plan error = %v", err)
	}
	if got := snapshotBoundArchiveFilesystem(t, filepath.Join(f.workspace, "openspec", "specs")); got != specsBefore {
		t.Fatalf("--plan changed main specs:\nwant %q\n got %q", specsBefore, got)
	}
	if got := snapshotBoundArchiveFilesystem(t, f.root); got != changeBefore {
		t.Fatalf("--plan changed the change root:\nwant %q\n got %q", changeBefore, got)
	}
	f.requireNotMoved(t)

	if output.Outcome != "planned" || output.Code != "" || output.SpecSync == nil || output.SpecSync.Synced || !output.SpecSync.Destructive {
		t.Fatalf("output = %+v", output)
	}
	auth := findSpecSyncTarget(t, output, "auth")
	want := "# Auth Specification\n\n## Requirements\n\n### Requirement: Login\n\nThe system MUST log users in.\n"
	if auth.Action != "updated" || strings.Join(auth.Removed, ",") != "Legacy Token" ||
		auth.BeforeDigest != sha256Digest(authMainSpec) || auth.AfterDigest != sha256Digest(want) {
		t.Fatalf("auth target = %+v", auth)
	}
	if created := findSpecSyncTarget(t, output, "delivery"); created.Action != "created" || created.BeforeDigest != sddspecsync.AbsentDigest {
		t.Fatalf("delivery target = %+v", created)
	}
}

func TestBoundSddArchiveMergeErrorBlocksWithoutWritesOrMove(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authInvalidDelta)

	output, err := f.run(t)
	if err == nil || !strings.Contains(err.Error(), sddspecsync.CodeMergeBlocked) {
		t.Fatalf("archive error = %v, want %s", err, sddspecsync.CodeMergeBlocked)
	}
	if output.Outcome != "blocked" || output.Code != sddspecsync.CodeMergeBlocked || !strings.Contains(output.Detail, "Ghost") || output.Recovery == "" {
		t.Fatalf("output = %+v", output)
	}
	f.requireSpecsUntouched(t)
	f.requireNotMoved(t)
}

func TestBoundSddArchiveRevertsSpecSyncWhenMoveFails(t *testing.T) {
	moveErr := errors.New("injected archive move failure")
	for _, tt := range []struct {
		name         string
		concurrent   string
		wantCode     string
		wantReverted bool
	}{
		{name: "revert restores every spec", wantCode: "archive_recovery_required", wantReverted: true},
		{name: "revert never overwrites a concurrent edit", concurrent: "# Someone else\n", wantCode: sddspecsync.CodeRecoveryRequired},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newSpecSyncArchiveFixture(t, authAddedDelta)
			previous := openArchiveStore
			t.Cleanup(func() { openArchiveStore = previous })
			openArchiveStore = func(root string) sddprogress.OpenSpec {
				return sddprogress.OpenSpec{Root: root, ArchiveRename: func(string, string) error {
					if got, _ := f.read(t, authMainPath); got != authMainSpec+"\n"+logoutRequirement {
						t.Fatalf("move attempted before sync: auth = %q", got)
					}
					if _, err := os.Stat(filepath.Join(f.root, specSyncJournalName)); err != nil {
						t.Fatalf("spec sync journal missing before the move: %v", err)
					}
					if tt.concurrent != "" {
						if err := os.WriteFile(filepath.Join(f.workspace, filepath.FromSlash(authMainPath)), []byte(tt.concurrent), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					return moveErr
				}}
			}

			output, err := f.run(t)
			if !errors.Is(err, moveErr) || !strings.Contains(err.Error(), tt.wantCode) {
				t.Fatalf("archive error = %v, want move failure with %s", err, tt.wantCode)
			}
			if output.Outcome != "blocked" || output.Code != tt.wantCode || output.SpecSync == nil ||
				output.SpecSync.Reverted != tt.wantReverted || !strings.Contains(output.Detail, moveErr.Error()) || output.Recovery == "" {
				t.Fatalf("output = %+v", output)
			}
			f.requireNotMoved(t)
			if _, exists := f.read(t, deliveryMainPath); exists {
				t.Fatal("created delivery spec was not reverted")
			}
			_, journalErr := os.Stat(filepath.Join(f.root, specSyncJournalName))
			if tt.wantReverted != errors.Is(journalErr, os.ErrNotExist) {
				t.Fatalf("journal stat after revert = %v, want removed only after a clean revert", journalErr)
			}
			got, _ := f.read(t, authMainPath)
			if tt.concurrent == "" {
				if got != authMainSpec {
					t.Fatalf("auth main spec not reverted: %q", got)
				}
				return
			}
			if got != tt.concurrent {
				t.Fatalf("revert overwrote a concurrent edit: %q", got)
			}
			if len(output.SpecSync.RecoveryTargets) != 1 || output.SpecSync.RecoveryTargets[0].Path != authMainPath ||
				output.SpecSync.RecoveryTargets[0].ExpectedDigest != sha256Digest(authMainSpec) {
				t.Fatalf("recovery targets = %+v", output.SpecSync.RecoveryTargets)
			}
		})
	}
}

const specSyncJournalName = ".spec-sync-journal.json"

// crashDuringMove runs archive with a move that panics after the spec sync
// wrote the main specs, modeling a process that dies before the rename and
// before any revert.
func (f specSyncArchiveFixture) crashDuringMove(t *testing.T) {
	t.Helper()
	previous := openArchiveStore
	t.Cleanup(func() { openArchiveStore = previous })
	openArchiveStore = func(root string) sddprogress.OpenSpec {
		return sddprogress.OpenSpec{Root: root, ArchiveRename: func(string, string) error { panic("simulated crash") }}
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "simulated crash" {
				t.Fatalf("recovered = %v, want the simulated crash", recovered)
			}
		}()
		command := newBoundSddArchiveCommand()
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"--root", f.root, "--destination", f.destination, "--project", specSyncProject})
		_ = command.Execute()
	}()
	openArchiveStore = previous
	if _, err := os.Stat(filepath.Join(f.root, specSyncJournalName)); err != nil {
		t.Fatalf("spec sync journal missing after crash: %v", err)
	}
	if got, _ := f.read(t, authMainPath); got != authMainSpec+"\n"+logoutRequirement {
		t.Fatalf("auth main spec after crash = %q, want synced", got)
	}
	f.requireNotMoved(t)
}

func (f specSyncArchiveFixture) requireArchivedWithoutJournal(t *testing.T, output archiveOutput, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("archive error = %v (output %+v)", err, output)
	}
	if output.Outcome != "archived" || output.Code != "" || output.SpecSync == nil || !output.SpecSync.Synced {
		t.Fatalf("output = %+v", output)
	}
	if got, _ := f.read(t, authMainPath); got != authMainSpec+"\n"+logoutRequirement {
		t.Fatalf("auth main spec = %q, want synced exactly once", got)
	}
	if got, _ := f.read(t, deliveryMainPath); got != boundArchiveDeliveryDelta {
		t.Fatalf("delivery main spec = %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.destination, "apply-progress.md")); err != nil {
		t.Fatalf("change not moved: %v", err)
	}
	for _, dir := range []string{f.root, f.destination} {
		if _, err := os.Stat(filepath.Join(dir, specSyncJournalName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("spec sync journal left behind in %s: %v", dir, err)
		}
	}
}

func TestBoundSddArchiveResumesFromJournalAfterCrash(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authAddedDelta)
	f.crashDuringMove(t)

	specsBefore := snapshotBoundArchiveFilesystem(t, filepath.Join(f.workspace, "openspec", "specs"))
	planned, err := f.run(t, "--plan")
	if err != nil || planned.Outcome != "planned" || planned.SpecSync == nil || !planned.SpecSync.Synced || len(planned.SpecSync.Targets) != 2 {
		t.Fatalf("--plan after crash = %+v, %v; want a planned resume", planned, err)
	}
	if got := snapshotBoundArchiveFilesystem(t, filepath.Join(f.workspace, "openspec", "specs")); got != specsBefore {
		t.Fatal("--plan after crash changed main specs")
	}
	if _, err := os.Stat(filepath.Join(f.root, specSyncJournalName)); err != nil {
		t.Fatalf("--plan removed the journal: %v", err)
	}
	f.requireNotMoved(t)

	output, err := f.run(t)
	f.requireArchivedWithoutJournal(t, output, err)
	if !output.SpecSync.Resumed {
		t.Fatalf("output = %+v, want a resumed spec sync", output.SpecSync)
	}
	if auth := findSpecSyncTarget(t, output, "auth"); auth.BeforeDigest != sha256Digest(authMainSpec) || auth.AfterDigest != sha256Digest(authMainSpec+"\n"+logoutRequirement) {
		t.Fatalf("auth target = %+v", auth)
	}
}

func TestBoundSddArchiveJournalAtBeforeRunsNormalSync(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authAddedDelta)
	f.crashDuringMove(t)
	if err := os.WriteFile(filepath.Join(f.workspace, filepath.FromSlash(authMainPath)), []byte(authMainSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.workspace, filepath.FromSlash(deliveryMainPath))); err != nil {
		t.Fatal(err)
	}

	output, err := f.run(t)
	f.requireArchivedWithoutJournal(t, output, err)
	if output.SpecSync.Resumed {
		t.Fatalf("output = %+v, want a fresh spec sync", output.SpecSync)
	}
}

func TestBoundSddArchiveMixedJournalRequiresRecovery(t *testing.T) {
	f := newSpecSyncArchiveFixture(t, authAddedDelta)
	f.crashDuringMove(t)
	if err := os.WriteFile(filepath.Join(f.workspace, filepath.FromSlash(authMainPath)), []byte(authMainSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	specsBefore := snapshotBoundArchiveFilesystem(t, filepath.Join(f.workspace, "openspec", "specs"))

	output, err := f.run(t)
	if err == nil || !strings.Contains(err.Error(), sddspecsync.CodeRecoveryRequired) {
		t.Fatalf("archive error = %v, want %s", err, sddspecsync.CodeRecoveryRequired)
	}
	if output.Outcome != "blocked" || output.Code != sddspecsync.CodeRecoveryRequired || output.Recovery == "" || output.SpecSync == nil {
		t.Fatalf("output = %+v", output)
	}
	if len(output.SpecSync.RecoveryTargets) != 1 || output.SpecSync.RecoveryTargets[0].Path != deliveryMainPath ||
		output.SpecSync.RecoveryTargets[0].ExpectedDigest != sddspecsync.AbsentDigest {
		t.Fatalf("recovery targets = %+v", output.SpecSync.RecoveryTargets)
	}
	if got := snapshotBoundArchiveFilesystem(t, filepath.Join(f.workspace, "openspec", "specs")); got != specsBefore {
		t.Fatal("mixed journal recovery wrote main specs")
	}
	if _, err := os.Stat(filepath.Join(f.root, specSyncJournalName)); err != nil {
		t.Fatalf("mixed journal recovery removed the journal: %v", err)
	}
	f.requireNotMoved(t)
}

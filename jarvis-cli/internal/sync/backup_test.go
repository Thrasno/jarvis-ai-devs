package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
)

// mutatingRunner writes to a real path on its first component, so a backup taken
// after it would archive the post-mutation bytes and be caught.
type mutatingRunner struct {
	*recordingRunner
	path    string
	content []byte
}

func (r *mutatingRunner) ApplyModels(target AgentTarget) error {
	if err := os.WriteFile(r.path, r.content, 0o644); err != nil {
		return err
	}
	return r.recordingRunner.ApplyModels(target)
}

func planWithTrackedFile(t *testing.T, home, content string) (Plan, string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tracked := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(tracked, []byte(content), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	return Plan{Tracked: []TrackedPath{{Path: tracked, Mode: ManagedFileMode}}}, tracked
}

// The backup is only a recovery path if it holds what was on disk BEFORE replay
// touched it. PR 4b discards a managed CLAUDE.md carrying no Jarvis sentinels
// and renders it fresh, so the archived pre-mutation bytes are the whole answer
// to "where did my file go".
func TestRun_ArchivesTrackedPathsAsTheyWereBeforeTheFirstMutation(t *testing.T) {
	home := t.TempDir()
	plan, tracked := planWithTrackedFile(t, home, "hand-written notes")
	store := lifecycle.NewBackupStore(home)
	runner := &mutatingRunner{recordingRunner: &recordingRunner{}, path: tracked, content: []byte("rendered fresh")}

	result, err := Run(RunInput{
		Plan:   plan,
		Apply:  ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}},
		Backup: store.CreateSnapshotOfTargets,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got, err := os.ReadFile(tracked); err != nil || string(got) != "rendered fresh" {
		t.Fatalf("replay must still mutate the tracked path: got %q err %v", got, err)
	}
	if !result.Report.Converged() {
		t.Fatalf("report should converge: %+v", result.Report)
	}
	if result.Backup.SourceOperation != BackupSourceOperation {
		t.Fatalf("source operation = %q, want %q", result.Backup.SourceOperation, BackupSourceOperation)
	}
	if len(result.Backup.Entries) != 1 || result.Backup.Entries[0].Path != tracked {
		t.Fatalf("manifest must cover the tracked path: %#v", result.Backup.Entries)
	}
	before := sha256.Sum256([]byte("hand-written notes"))
	if got := result.Backup.Entries[0].Checksum; got != hex.EncodeToString(before[:]) {
		t.Fatalf("manifest recorded post-mutation content: checksum %q", got)
	}
	// The archive itself, not just the manifest, must hold the original bytes.
	if err := store.ValidateSnapshot(result.Backup); err != nil {
		t.Fatalf("archive does not match the pre-mutation checksums: %v", err)
	}
}

func TestRun_BackupFailureBlocksEveryMutation(t *testing.T) {
	home := t.TempDir()
	plan, tracked := planWithTrackedFile(t, home, "hand-written notes")
	boom := errors.New("backup archive is not writable")
	runner := &recordingRunner{}

	result, err := Run(RunInput{
		Plan:  plan,
		Apply: ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}, {ID: "opencode"}}},
		Backup: func(string, []lifecycle.BackupTarget) (lifecycle.BackupManifest, error) {
			return lifecycle.BackupManifest{}, boom
		},
	})

	if err == nil {
		t.Fatal("a failed backup must be reported, not swallowed")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("reported failure = %v, want it to wrap %v", err, boom)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no component may run without a backup, got calls %v", runner.calls)
	}
	if len(result.Report.Agents) != 0 {
		t.Fatalf("a blocked run reports no agent outcome: %+v", result.Report.Agents)
	}
	if got, err := os.ReadFile(tracked); err != nil || string(got) != "hand-written notes" {
		t.Fatalf("tracked path was mutated after a failed backup: got %q err %v", got, err)
	}
}

// D1, over the combination that actually exercises it: the backup succeeded and
// an agent then failed. Run still returns no error, because a per-agent failure
// is the report's own content and the command derives its exit status from
// there. Raising it here would claim the replay pass itself broke, which the
// recovery point and the sibling agent's outcome both disprove.
func TestRun_ReportsAPerAgentFailureWithoutRaisingItWhenTheBackupSucceeded(t *testing.T) {
	home := t.TempDir()
	plan, _ := planWithTrackedFile(t, home, "hand-written notes")
	boom := errors.New("native MCP replacement failed")
	runner := &recordingRunner{failAt: map[string]error{"claude/" + ComponentMCPs: boom}}

	result, err := runReportingFailures(home, plan, runner, "claude")

	if err != nil {
		t.Fatalf("a per-agent failure must be reported, not raised: %v", err)
	}
	if result.Backup.SnapshotID == "" {
		t.Fatal("the backup succeeded, so the report must still name its recovery point")
	}
	if result.Report.Converged() {
		t.Fatalf("a failed agent must not converge the run: %+v", result.Report)
	}
	if len(result.Report.Agents) != 1 || result.Report.Agents[0].FailedAt != ComponentMCPs {
		t.Fatalf("the report must name the component that failed: %+v", result.Report.Agents)
	}
	if !errors.Is(result.Report.Agents[0].Err, boom) {
		t.Fatalf("agent error = %v, want it to wrap %v", result.Report.Agents[0].Err, boom)
	}
	if result.Report.ExitCode() == 0 {
		t.Fatal("the exit status is derived from the report, so it must be non-zero")
	}
}

// A missing backup seam must read as "cannot proceed", never as "no backup
// needed".
func TestRun_RefusesToMutateWithoutABackupSeam(t *testing.T) {
	runner := &recordingRunner{}

	_, err := Run(RunInput{Apply: ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}}})

	if !errors.Is(err, ErrNoBackup) {
		t.Fatalf("err = %v, want %v", err, ErrNoBackup)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no component may run, got calls %v", runner.calls)
	}
}

// The single-list rule: backup coverage and the idempotency diff read the same
// Plan.Tracked, so neither can quietly stop covering what the other measures.
func TestBackupTargets_ProjectsThePlansOwnTrackedList(t *testing.T) {
	plan := Plan{Tracked: []TrackedPath{
		{Path: "/home/dev/.claude/CLAUDE.md", Mode: ManagedFileMode},
		{Path: "/home/dev/.claude/statusline-command.sh", Mode: ManagedExecutableMode},
	}}

	want := []lifecycle.BackupTarget{
		{Path: "/home/dev/.claude/CLAUDE.md"},
		{Path: "/home/dev/.claude/statusline-command.sh"},
	}
	if got := BackupTargets(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("BackupTargets = %#v, want %#v", got, want)
	}
	if got := BackupTargets(Plan{}); len(got) != 0 {
		t.Fatalf("an empty plan tracks nothing, got %#v", got)
	}
}

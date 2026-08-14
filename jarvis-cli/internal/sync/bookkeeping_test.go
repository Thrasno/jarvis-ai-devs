package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// seedManifest isolates HOME and seeds a manifest carrying the given digest.
func seedManifest(t *testing.T, home, digest string) string {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seeded := state.New()
	seeded.ManagedAssetDigest = digest
	if err := state.Save(seeded); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	return filepath.Join(home, ".jarvis", "state.yaml")
}

func manifestBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func bookkeptRun(t *testing.T, home string, plan Plan, runner ComponentRunner, book *Bookkeeping) (RunResult, error) {
	t.Helper()
	return Run(RunInput{
		Plan:        plan,
		Apply:       ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}},
		Backup:      lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
		Bookkeeping: book,
	})
}

// Both halves of the rule, over the same machine. The first run changes a
// target and writes the record through the lock, rebuilt from a manifest
// re-read inside it: the concurrent writer landing while the lock is taken
// proves the re-read, because its skill survives where a stale in-memory copy
// would have clobbered it. The second changes nothing, never takes the lock,
// leaves the record byte-identical.
func TestRun_WritesBookkeepingUnderLockOnlyWhenATargetChanged(t *testing.T) {
	home := t.TempDir()
	manifest := seedManifest(t, home, "sha256:previous")

	const body = "claude instructions\n"
	tracked := filepath.Join(home, ".claude", "CLAUDE.md")
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(body))},
	}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: tracked, body: body, mode: 0o644}},
	}}

	locked := 0
	book := &Bookkeeping{
		ManagedAssetDigest: "sha256:current",
		Lock: func(critical func() error) error {
			locked++
			concurrent := state.New()
			concurrent.ManagedAssetDigest = "sha256:previous"
			concurrent.Skills = []string{"late-arrival"}
			if err := state.Save(concurrent); err != nil {
				return err
			}
			return critical()
		},
	}

	changed, err := bookkeptRun(t, home, plan, runner, book)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if want := []string{tracked}; !reflect.DeepEqual(changed.Report.Changed, want) {
		t.Fatalf("Report.Changed = %v, want %v", changed.Report.Changed, want)
	}
	if locked != 1 {
		t.Fatalf("critical section entered %d times, want exactly one", locked)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if loaded.ManagedAssetDigest != "sha256:current" {
		t.Fatalf("managed_asset_digest = %q, want the digest this run replayed", loaded.ManagedAssetDigest)
	}
	if want := []string{"late-arrival"}; !reflect.DeepEqual(loaded.Skills, want) {
		t.Fatalf("skills = %v, want %v; the write clobbered a concurrent writer instead of re-reading", loaded.Skills, want)
	}
	recorded := manifestBytes(t, manifest)

	converged, err := bookkeptRun(t, home, plan, runner, book)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(converged.Report.Changed) != 0 {
		t.Fatalf("converged run reported %v as changed", converged.Report.Changed)
	}
	if locked != 1 {
		t.Fatalf("a run that changed nothing took the lock: entered %d times", locked)
	}
	if got := manifestBytes(t, manifest); got != recorded {
		t.Fatalf("a run that changed nothing rewrote the bookkeeping record:\n%s", got)
	}
}

// A failure that leaves the diff unmeasured is not evidence that nothing
// changed: the applier already ran, so the record is written rather than
// skipped. EnforceModes fails here because a tracked path sits under what the
// applier just made a regular file.
func TestRun_WritesBookkeepingWhenTheDiffCouldNotBeMeasured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this forces the failure through ENOTDIR on a path under a regular file, which Windows does not report the same way")
	}

	home := t.TempDir()
	seedManifest(t, home, "sha256:previous")

	blocker := filepath.Join(home, ".claude", "skills")
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: blocker, Mode: ManagedFileMode},
		{Agent: "claude", Path: filepath.Join(blocker, "SKILL.md"), Mode: ManagedFileMode},
	}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: blocker, body: "not a directory\n", mode: 0o644}},
	}}

	result, err := bookkeptRun(t, home, plan, runner, &Bookkeeping{ManagedAssetDigest: "sha256:current"})
	if err == nil {
		t.Fatal("a tracked path under a regular file must fail the mode assertion")
	}
	if len(result.Report.Agents) != 1 || !result.Report.Agents[0].Converged {
		t.Fatalf("the applier ran and its outcome must still be reported: %+v", result.Report)
	}
	if loaded, loadErr := state.Load(); loadErr != nil || loaded.ManagedAssetDigest != "sha256:current" {
		t.Fatalf("manifest after an unmeasured diff = %+v (%v); that is not a no-op run", loaded, loadErr)
	}
}

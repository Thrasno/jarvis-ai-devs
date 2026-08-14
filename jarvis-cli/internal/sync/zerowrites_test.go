package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Zero changed files is not zero writes. desiredStateRunner removes and
// rewrites every path it owns on every run, exactly the way InstallStatusline
// does (agent/claude.go:882-885), so an implementation that applies first and
// diffs afterwards still touches every managed file on an unchanged machine.
// The second run must not reach the applier at all; the third, over drift, must.
func TestRun_SecondRunOverMatchingDesiredStatePerformsZeroWrites(t *testing.T) {
	home := t.TempDir()
	instructions := filepath.Join(home, ".claude", "CLAUDE.md")
	script := filepath.Join(home, ".claude", statuslineScriptName)
	const instructionsBody = "<!-- jarvis -->\n"
	const scriptBody = "#!/bin/sh\necho jarvis\n"
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: instructions, Mode: ManagedFileMode, Desired: digestOf([]byte(instructionsBody))},
		{Agent: "claude", Path: script, Mode: ManagedExecutableMode, Desired: digestOf([]byte(scriptBody))},
	}}
	writes := map[string][]plannedWrite{"claude": {
		{path: instructions, body: instructionsBody, mode: 0o644},
		{path: script, body: scriptBody, mode: 0o755},
	}}
	replay := func() (*recordingRunner, RunResult) {
		runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: writes}
		return runner.recordingRunner, measuredRun(t, home, plan, runner, "claude")
	}

	_, first := replay()
	if want := []string{instructions, script}; !reflect.DeepEqual(first.Report.Changed, want) {
		t.Fatalf("first run Changed = %v, want %v", first.Report.Changed, want)
	}
	untouched := modTimes(t, instructions, script)

	runner, second := replay()

	if len(runner.calls) != 0 {
		t.Fatalf("second run invoked the applier: %v", runner.calls)
	}
	if !reflect.DeepEqual(modTimes(t, instructions, script), untouched) {
		t.Fatal("second run rewrote a tracked path; an unchanged machine must be written to zero times")
	}
	if len(second.Report.Changed) != 0 || !second.Report.Converged() || second.Report.ExitCode() != 0 {
		t.Fatalf("a machine already in its desired state has converged with no change: %+v", second.Report)
	}
	// A run that mutates nothing has no prior state to preserve, so it takes no
	// recovery point either.
	if second.Backup.SnapshotID != "" {
		t.Fatalf("second run took backup %q, want none", second.Backup.SnapshotID)
	}

	// The short-circuit is all-or-nothing: one drifted path and the run applies.
	if err := os.WriteFile(script, []byte("tampered\n"), 0o755); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	runner, third := replay()
	if len(runner.calls) == 0 {
		t.Fatal("a drifted tracked path must reach the applier")
	}
	if want := []string{script}; !reflect.DeepEqual(third.Report.Changed, want) {
		t.Fatalf("third run Changed = %v, want %v", third.Report.Changed, want)
	}
	if third.Backup.SnapshotID == "" {
		t.Fatal("a mutating run must still take its recovery point first")
	}
}

func modTimes(t *testing.T, paths ...string) []string {
	t.Helper()
	stamps := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat %s: %v", path, err)
		}
		stamps = append(stamps, info.ModTime().String())
	}
	return stamps
}

package sync

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
)

// Verification asks whether the machine holds the desired content, not whether
// a component said so: a missing output and a stale one both fail. The passing
// case is covered by TestRun_SecondRunOverMatchingDesiredStatePerformsZeroWrites.
// D3 decides the recovery command: sync has no per-agent retry, so a failed run
// points at `jarvis sync`, and an agent-less manifest at a `jarvis` reinstall.
func TestRun_PostApplyVerificationDetectsInvalidOutputsAndNamesTheRecovery(t *testing.T) {
	const desired = "claude instructions\n"
	for _, tt := range []struct {
		name    string
		written string
		targets []AgentTarget
		want    string
	}{
		{name: "stale output", written: "stale\n", targets: []AgentTarget{{ID: "claude"}}, want: "run `jarvis sync` to repair"},
		{name: "missing output, agent-less manifest", targets: nil, want: "run `jarvis` to repair"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tracked := filepath.Join(home, ".claude", "CLAUDE.md")
			plan := Plan{Tracked: []TrackedPath{
				{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(desired))},
			}}
			runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{}}
			if tt.written != "" {
				runner.writes["claude"] = []plannedWrite{{path: tracked, body: tt.written, mode: 0o644}}
			}

			_, err := Run(RunInput{
				Plan:   plan,
				Apply:  ApplyInput{Runner: runner, Targets: tt.targets},
				Backup: lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
			})

			if err == nil {
				t.Fatal("an invalid managed output must fail verification")
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), tracked) {
				t.Fatalf("error = %q, want it to name %s and %q", err, tracked, tt.want)
			}
		})
	}
}

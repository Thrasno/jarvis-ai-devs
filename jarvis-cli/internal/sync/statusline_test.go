package sync

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// statuslineRunner performs the real statusline component and no-ops every
// other one, so drift can be asserted after a full pass of the locked order.
type statuslineRunner struct{ component StatuslineComponent }

func (r statuslineRunner) ApplyModels(AgentTarget) error              { return nil }
func (r statuslineRunner) ApplySkills(AgentTarget) error              { return nil }
func (r statuslineRunner) ApplyRuntimeAssets(AgentTarget) error       { return nil }
func (r statuslineRunner) ApplyMCPs(AgentTarget) error                { return nil }
func (r statuslineRunner) ApplyPersonaInstructions(AgentTarget) error { return nil }
func (r statuslineRunner) ApplyStatusline(t AgentTarget) error        { return r.component.Apply(t) }

// persistManifest writes a valid manifest recording the given statusline
// tri-state and returns its path, so a test can compare bytes across replay.
func persistManifest(t *testing.T, instructionsPath string, statusline state.StatuslineState) string {
	t.Helper()
	st := state.New()
	st.InstalledAgents = []state.Agent{{
		ID:               "claude",
		InstructionsPath: instructionsPath,
		ConfigPath:       filepath.Dir(instructionsPath),
	}}
	st.Persona = "gentleman"
	st.ManagedAssetDigest = "sha256:replay-fixture"
	st.Statusline = statusline
	if err := state.Save(st); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	path, err := state.Path()
	if err != nil {
		t.Fatalf("resolve manifest path: %v", err)
	}
	return path
}

// runStatuslineReplay walks the locked order with the real statusline component.
func runStatuslineReplay(t *testing.T, a agent.Agent, consent state.StatuslineState, target AgentTarget) Report {
	t.Helper()
	return Apply(ApplyInput{
		Runner: statuslineRunner{component: StatuslineComponent{
			Resolve: func(id string) (agent.Agent, bool) { return a, id == a.Name() },
			HooksFS: jarvis.HooksFS,
			Consent: consent,
		}},
		Targets: []AgentTarget{target},
	})
}

// D2: the manifest is the sole authority for statusline intent. A recorded,
// enabled decision whose script is missing from disk is drift, not revocation,
// so replay reinstalls it rather than inferring intent from disk state.
func TestStatuslineComponent_ReinstallsADeletedScriptWithoutTouchingTheManifest(t *testing.T) {
	claude, instructions := claudeReplayFixture(t)
	consent := state.StatuslineState{Decided: true, Enabled: true}
	manifestPath := persistManifest(t, instructions, consent)
	before := readFileString(t, manifestPath)

	scriptPath := filepath.Join(filepath.Dir(instructions), "statusline-command.sh")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("the fixture must start with the statusline script absent, stat error = %v", err)
	}

	report := runStatuslineReplay(t, claude, consent, AgentTarget{ID: "claude", InstructionsPath: instructions})

	if !report.Converged() {
		t.Fatalf("replay must converge, got %+v", report.Agents)
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("a decided-enabled manifest must reinstall the drifted script: %v", err)
	}
	// Windows has no POSIX permission bits: Perm always reads 0666 or 0444.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("statusline script mode = %04o, want 0755", info.Mode().Perm())
	}
	// Rule 9: replay writes bookkeeping only. Reinstalling a drifted artifact is
	// not a reason to rewrite the stored decision.
	if got := readFileString(t, manifestPath); got != before {
		t.Fatalf("replay rewrote the manifest.\n got: %q\nwant: %q", got, before)
	}
}

// The reinstall is authority-driven, not unconditional: without a recorded,
// enabled decision the absent script stays absent.
func TestStatuslineComponent_LeavesTheScriptAbsentWithoutARecordedEnabledDecision(t *testing.T) {
	tests := []struct {
		name    string
		consent state.StatuslineState
	}{
		{"never asked", state.StatuslineState{}},
		{"decided against", state.StatuslineState{Decided: true}},
		{"enabled but never decided", state.StatuslineState{Enabled: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claude, instructions := claudeReplayFixture(t)
			manifestPath := persistManifest(t, instructions, tt.consent)
			before := readFileString(t, manifestPath)

			report := runStatuslineReplay(t, claude, tt.consent, AgentTarget{ID: "claude", InstructionsPath: instructions})

			if !report.Converged() {
				t.Fatalf("replay must converge, got %+v", report.Agents)
			}
			scriptPath := filepath.Join(filepath.Dir(instructions), "statusline-command.sh")
			if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
				t.Fatalf("statusline script must stay absent, stat error = %v", err)
			}
			if got := readFileString(t, manifestPath); got != before {
				t.Fatal("replay rewrote the manifest for an unauthorized statusline decision")
			}
		})
	}
}

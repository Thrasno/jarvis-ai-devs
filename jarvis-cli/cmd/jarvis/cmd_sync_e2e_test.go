package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// The tests in this file drive runSync itself, past the preflight and through
// every stage it composes: the migration, the manifest load, the replay input,
// the plan, the applier and the report. Everything else about `jarvis sync` is
// tested a part at a time -- the renderer against a hand-built RunResult, the
// exit mapping against a hand-built Report, the planner and the applier against
// fake runners in internal/sync -- and none of that says whether the parts are
// wired to each other. The claim here is composition: the digests the planner
// computes must describe the bytes the installer actually writes, or a machine
// that just replayed would report drift on its very next run.
//
// They stay on OpenCode alone, and that is a load-bearing choice rather than a
// convenience. The managed-MCP component hands Claude to the native `claude`
// CLI (agent/native_mcp_recovery.go:187-196), so a Claude target would shell out
// to whatever binary the machine running the suite happens to have. With no
// Claude among the selected agents the executor is handed no native MCP
// definitions and drops the native boundary entirely
// (agent/production_bridge.go:624-626), so the whole replay stays inside the
// fixture home.

// newSyncFixtureHome isolates a home this suite may replay into, with OpenCode
// detectable in it.
//
// isolateTestHome pins USERPROFILE and XDG_CONFIG_HOME alongside HOME. Neither
// is optional: os.UserHomeDir reads USERPROFILE on Windows and this suite runs a
// windows-latest job, and OpenCode is detected under XDG_CONFIG_HOME. A test
// that pinned only HOME would replay against the CI runner's real home, which is
// a defect rather than a test.
func newSyncFixtureHome(t *testing.T) string {
	t.Helper()
	home := isolateTestHome(t)
	for _, dir := range []string{filepath.Join(home, ".jarvis"), filepath.Join(home, ".config", "opencode")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	return home
}

// seedReplayManifest records the installation runSync replays.
func seedReplayManifest(t *testing.T, agents ...state.Agent) {
	t.Helper()
	manifest := state.New()
	manifest.SelectionConfigured = true
	manifest.InstalledAgents = agents
	// A skill is recorded on purpose: it is the one component whose desired
	// content the planner renders itself (sync/plan.go:186-206) while the
	// installer writes it through its own pass, so it is where the two halves
	// would disagree first.
	manifest.Skills = []string{"go-testing"}
	manifest.Persona = "neutra"
	manifest.Scope = state.ScopeLocalOnly
	if err := state.Save(manifest); err != nil {
		t.Fatalf("seed the manifest: %v", err)
	}
}

// openCodeAgent is the manifest entry a real installation records for OpenCode:
// absolute paths, because that is what the installer writes and what replay has
// to be able to read back.
func openCodeAgent(home string) state.Agent {
	return state.Agent{
		ID:               "opencode",
		InstructionsPath: filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		ConfigPath:       filepath.Join(home, ".config", "opencode", "opencode.json"),
	}
}

// The whole composition on a seeded machine: runSync must reach the applier,
// write the instruction file the manifest records, name it as changed, and exit
// zero. Every stage between the preflight and the report participates, so a
// stage wired to the wrong value fails here rather than in any of the isolated
// unit tests around the parts.
func TestRunSync_ReplaysASeededMachineAndReportsWhatItChanged(t *testing.T) {
	home := newSyncFixtureHome(t)
	seedReplayManifest(t, openCodeAgent(home))
	instructions := filepath.Join(home, ".config", "opencode", "AGENTS.md")

	var err error
	out := captureStdout(t, func() { err = runSync() })

	if err != nil {
		t.Fatalf("runSync on a seeded machine returned %v\n%s", err, out)
	}
	if _, statErr := os.Stat(instructions); statErr != nil {
		t.Fatalf("the replay did not write the recorded instruction file: %v\n%s", statErr, out)
	}
	for _, want := range []string{
		"opencode: converged",
		"verification: passed",
		instructions,
		"backup snapshot: ",
		hiveNotSynchronizedNotice,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "changed paths: 0") {
		t.Errorf("a replay onto an empty home changed nothing, which cannot be true:\n%s", out)
	}
}

// The convergence claim, which is the one assertion that can only be made end to
// end: replay a machine, then replay it again. The second run reaches the
// planner, measures the machine against the digests the planner computed, finds
// them already satisfied and skips the applier entirely -- no backup, no writes,
// nothing changed. It holds only if every path the installer wrote carries
// exactly the bytes and mode the planner predicted for it, which is precisely
// the wiring no per-part test can check.
func TestRunSync_SecondRunOverAnAlreadyCurrentMachineConvergesWithoutApplying(t *testing.T) {
	home := newSyncFixtureHome(t)
	seedReplayManifest(t, openCodeAgent(home))

	var first error
	firstOut := captureStdout(t, func() { first = runSync() })
	if first != nil {
		t.Fatalf("the first replay returned %v\n%s", first, firstOut)
	}

	var second error
	out := captureStdout(t, func() { second = runSync() })

	if second != nil {
		t.Fatalf("the second replay returned %v\n%s", second, out)
	}
	for _, want := range []string{"changed paths: 0", "opencode: converged", "already current"} {
		if !strings.Contains(out, want) {
			t.Errorf("the second run is missing %q:\n%s", want, out)
		}
	}
	// A short-circuited run takes no backup, because it mutates nothing
	// (sync/backup.go:98-100). The absence of the line is therefore evidence that
	// the run really did skip the applier rather than rewrite identical bytes.
	if strings.Contains(out, "backup snapshot: ") {
		t.Errorf("an already-current machine must not be backed up, so the applier was not skipped:\n%s", out)
	}
}

// A failure that reaches the report instead of an early return. The manifest
// records an agent this machine does not have installed, which is what a
// user-removed agent leaves behind: the plan still covers it, the backup still
// protects it, and the run still replays its sibling. What must not happen is
// an abort -- the agent that can converge does, the one that cannot is named
// with its cause and its component, verification fails because the recorded
// paths were never written, and the command exits non-zero.
func TestRunSync_ReportsAnUninstalledAgentWithoutAbortingTheRun(t *testing.T) {
	home := newSyncFixtureHome(t)
	seedReplayManifest(t, openCodeAgent(home), state.Agent{
		ID:               "claude",
		InstructionsPath: filepath.Join(home, ".claude", "CLAUDE.md"),
		ConfigPath:       filepath.Join(home, ".claude", "settings.json"),
	})

	var err error
	out := captureStdout(t, func() { err = runSync() })

	if err == nil {
		t.Fatalf("a run that could not converge every agent must exit non-zero:\n%s", out)
	}
	for _, want := range []string{
		"opencode: converged",
		"claude: failed at models",
		"not installed on this machine",
		"verification: failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "already current") {
		t.Errorf("a failed run must never claim the machine is current:\n%s", out)
	}
}

// appendToFile adds content at the end of an existing file, the way a user
// editing their own instruction file does.
func appendToFile(t *testing.T, path, content string) {
	t.Helper()
	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(existing, []byte(content)...), 0o644); err != nil {
		t.Fatalf("append to %s: %v", path, err)
	}
}

// readFileOrFail reads a file the run is expected to have written.
func readFileOrFail(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// The user's own prose. An instruction file is a document the user shares with
// Jarvis: the writer deliberately preserves everything outside the markers, so
// a run that compared the whole file measured a file it had just written
// correctly as invalid and told the user to repair it -- forever, because the
// prose is still there on the next run.
//
// This is the case that can only be made end to end. The planner composes with
// nothing from disk and the writer composes with the user's file, and only a
// run that drives both against a real home shows the two answers being
// compared.
func TestRunSync_ConvergesWhenTheUserWritesTheirOwnProseOutsideTheManagedRegions(t *testing.T) {
	home := newSyncFixtureHome(t)
	seedReplayManifest(t, openCodeAgent(home))
	instructions := filepath.Join(home, ".config", "opencode", "AGENTS.md")

	var first error
	firstOut := captureStdout(t, func() { first = runSync() })
	if first != nil {
		t.Fatalf("the first replay returned %v\n%s", first, firstOut)
	}
	const prose = "\n## My own conventions\n\nAlways review the migration plan first.\n"
	appendToFile(t, instructions, prose)

	var second error
	out := captureStdout(t, func() { second = runSync() })

	if second != nil {
		t.Fatalf("a user's own prose outside the markers must not fail sync: %v\n%s", second, out)
	}
	for _, want := range []string{"opencode: converged", "verification: passed"} {
		if !strings.Contains(out, want) {
			t.Errorf("the second run is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "missing or invalid") {
		t.Errorf("the run reported a managed output as invalid:\n%s", out)
	}
	if !strings.Contains(readFileOrFail(t, instructions), "Always review the migration plan first.") {
		t.Error("sync dropped the user's own prose, which is the promise this fix must not break")
	}
}

// The other direction, end to end: an edit inside the Jarvis markers is Jarvis's
// own content being tampered with, and the run must still notice it and put it
// back. A fix that bought convergence by ignoring the managed regions would be
// worse than the bug it removes.
func TestRunSync_StillRepairsAnEditInsideTheManagedRegions(t *testing.T) {
	home := newSyncFixtureHome(t)
	seedReplayManifest(t, openCodeAgent(home))
	instructions := filepath.Join(home, ".config", "opencode", "AGENTS.md")

	var first error
	firstOut := captureStdout(t, func() { first = runSync() })
	if first != nil {
		t.Fatalf("the first replay returned %v\n%s", first, firstOut)
	}
	composed := readFileOrFail(t, instructions)
	start := strings.Index(composed, agent.HiveProtocolStart)
	end := strings.Index(composed, agent.HiveProtocolEnd)
	if start == -1 || end == -1 {
		t.Fatalf("the replayed file carries no Hive protocol block:\n%s", composed)
	}
	tampered := composed[:start+len(agent.HiveProtocolStart)] + "\nthe user deleted the protocol\n" + composed[end:]
	if err := os.WriteFile(instructions, []byte(tampered), 0o644); err != nil {
		t.Fatalf("tamper with %s: %v", instructions, err)
	}

	var second error
	out := captureStdout(t, func() { second = runSync() })

	if second != nil {
		t.Fatalf("the repairing run returned %v\n%s", second, out)
	}
	repaired := readFileOrFail(t, instructions)
	if strings.Contains(repaired, "the user deleted the protocol") {
		t.Errorf("sync left a tampered managed region in place:\n%s", out)
	}
	if repaired != composed {
		t.Errorf("the repaired file does not match what sync composes:\n%s", out)
	}
	if strings.Contains(out, "changed paths: 0") {
		t.Errorf("a run that repaired a tampered file must report it as changed:\n%s", out)
	}
}

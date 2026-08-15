package main

import (
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sync"
)

// TestSyncCommand_RejectsEveryFlagWithoutRunning locks the CLI boundary of this
// version: `jarvis sync` takes no flags at all, --dry-run included, and a
// rejected invocation must never reach the run seam, so it can mutate nothing.
//
// The inherited-flag case is the one that matters: cobra rejects an unknown
// flag by itself, but a persistent flag declared on the root command parses
// happily on every subcommand, so an assumed rejection would be wrong.
func TestSyncCommand_RejectsEveryFlagWithoutRunning(t *testing.T) {
	for _, tc := range []struct{ name, flag string }{
		{"dry run", "--dry-run"},
		{"unknown long flag", "--force"},
		{"unknown short flag", "-f"},
		{"inherited global flag", "--no-tui"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := 0
			root := newSyncTestRoot(func() error { runs++; return nil })
			root.SetArgs([]string{"sync", tc.flag})

			if err := root.Execute(); err == nil {
				t.Fatalf("expected %q to be a usage error", tc.flag)
			}
			if runs != 0 {
				t.Fatalf("expected zero runs after a usage error, got %d", runs)
			}
		})
	}
}

// TestSyncCommand_RunsWhenInvokedWithNoFlags is the other half of the boundary:
// the guard must refuse flags, not the command.
func TestSyncCommand_RunsWhenInvokedWithNoFlags(t *testing.T) {
	runs := 0
	root := newSyncTestRoot(func() error { runs++; return nil })
	root.SetArgs([]string{"sync"})

	if err := root.Execute(); err != nil {
		t.Fatalf("expected a flagless sync to run, got %v", err)
	}
	if runs != 1 {
		t.Fatalf("expected exactly one run, got %d", runs)
	}
}

// The guard must belong to the run, not to cobra's dispatch. A PreRunE-only
// check holds solely for callers that go through Execute, and this binary is
// driven directly through RunE elsewhere in its own test suite, so a supplied
// flag would reach the replay seam with nothing left to stop it.
func TestSyncCommand_RejectsASuppliedFlagOnADirectRunECall(t *testing.T) {
	runs := 0
	root := newSyncTestRoot(func() error { runs++; return nil })
	// Cobra's flag values outlive the invocation that parsed them, which is how a
	// supplied flag reaches a later direct call at all.
	root.SetArgs([]string{"sync", "--no-tui"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected the dispatched invocation to be refused")
	}
	cmd, _, err := root.Find([]string{"sync"})
	if err != nil {
		t.Fatalf("find sync: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("a supplied flag must be refused by RunE itself, not only by cobra's dispatch")
	}
	if runs != 0 {
		t.Fatalf("the run seam must not be reached, got %d runs", runs)
	}
}

// newSyncTestRoot mirrors the production wiring: a root command carrying the
// same persistent flag, with sync mounted underneath it.
func newSyncTestRoot(run func() error) *cobra.Command {
	root := &cobra.Command{Use: "jarvis", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("no-tui", false, "disable TUI, use readline prompts")
	root.AddCommand(newSyncCommand(run))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

// TestSyncReport_IsTheWholeObservabilityContract locks what a run must make
// inspectable: which version replayed which desired state, the recovery point,
// every path that moved, each agent's outcome with its cause, the verification
// result with its recovery command, the manual step the cloud portion needs,
// and the plain statement that Hive memory data was left alone.
func TestSyncReport_IsTheWholeObservabilityContract(t *testing.T) {
	const changedPath = "/home/u/.claude/CLAUDE.md"
	manifest := &state.State{
		SchemaVersion:   1,
		InstalledAgents: []state.Agent{{ID: "claude"}, {ID: "opencode"}},
		Skills:          []string{"go-testing", "work-unit-commits"},
		Persona:         "neutral",
		Statusline:      state.StatuslineState{Decided: true, Enabled: true},
		PhaseModels:     state.PhaseModels{Claude: map[string]state.ClaudeModelAssignment{"apply": {Model: "opus"}}},
		Scope:           state.ScopeLocalCloud,
	}
	partial := sync.RunResult{
		Backup: lifecycle.BackupManifest{SnapshotID: "snap-42"},
		Report: sync.Report{
			Agents: []sync.AgentResult{
				{Agent: "claude", Converged: true},
				{Agent: "opencode", FailedAt: "mcps", Err: errors.New("native MCP replacement failed")},
			},
			Changed: []string{changedPath},
		},
	}
	converged := sync.RunResult{Report: sync.Report{
		Agents:  []sync.AgentResult{{Agent: "claude", Converged: true}},
		Changed: []string{},
	}}
	// What sync.Run returns when it fails after the applier already wrote: the
	// agent outcomes are populated and Changed is nil, because the closing
	// measurement never ran. Nil is what distinguishes it from a measured zero.
	unmeasured := sync.RunResult{
		Backup: lifecycle.BackupManifest{SnapshotID: "snap-7"},
		Report: sync.Report{Agents: []sync.AgentResult{
			{Agent: "claude", Converged: true, Completed: []string{"models", "skills"}},
			{Agent: "opencode", FailedAt: "mcps", Err: errors.New("boom"), Completed: []string{"models"}},
		}},
	}

	for _, tc := range []struct {
		name           string
		result         sync.RunResult
		cloud          string
		runErr         error
		want, unwanted []string
	}{
		{
			// The cloud half of the contract rides along here on purpose: the
			// manual step must be named without hiding the local replay.
			name:   "a partial run names every fact it produced",
			result: partial,
			cloud:  sync.CloudManualActionMessage,
			runErr: errors.New("1 managed output missing or invalid; run `jarvis sync` to repair"),
			want: []string{
				"schema 1", "claude, opencode", "skills 2", "persona neutral",
				"claude 1", "statusline enabled", "snap-42", "jarvis login",
				"changed paths: 1", changedPath, "claude: converged",
				"opencode: failed at mcps", "native MCP replacement failed",
				"jarvis sync", hiveNotSynchronizedNotice,
			},
			// A measured run names the paths that moved, so repeating the component
			// list would add noise to the one report that does not need it.
			unwanted: []string{"already current", "components completed"},
		},
		{
			name:     "a converged machine is already current and lists nothing",
			result:   converged,
			want:     []string{"already current", "changed paths: 0", "verification: passed"},
			unwanted: []string{"snapshot", changedPath, "components completed"},
		},
		{
			// A failure after the applier ran must never be reported as a
			// measured zero: the diff was not taken, which is not evidence that
			// nothing changed, and the operator needs that distinction to decide
			// whether to restore the snapshot named above it. Since no path can
			// honestly be named, the report falls back to what the run does know:
			// which components each agent actually completed before it stopped.
			name:   "a run that failed after mutating says the diff was not measured",
			result: unmeasured,
			runErr: errors.New("assert mode on /home/u/.claude/CLAUDE.md: permission denied"),
			// The component lines are matched whole, label included: an assertion
			// on the tail alone ("completed: models") passes for any label that
			// happens to end in it, and so verifies less than the full-line
			// assertions beside it appear to promise.
			want: []string{
				"not measured", "not evidence that nothing changed", "snap-7", "verification: failed",
				"claude: converged", "    components completed: models, skills\n",
				"opencode: failed at mcps", "    components completed: models\n",
			},
			unwanted: []string{"changed paths: 0", "already current"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := renderSyncReport(manifest, tc.result, tc.cloud, tc.runErr)
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("report is missing %q:\n%s", want, out)
				}
			}
			for _, unwanted := range tc.unwanted {
				if strings.Contains(out, unwanted) {
					t.Errorf("report must not contain %q:\n%s", unwanted, out)
				}
			}
		})
	}
}

// TestSyncExit_IsNonZeroForApplyAndVerificationFailures locks the exit
// contract. syncExit deliberately takes no cloud argument: an unusable cloud
// portion is reported, never raised, so there is no shape here through which it
// could abort a local replay that otherwise converged.
func TestSyncExit_IsNonZeroForApplyAndVerificationFailures(t *testing.T) {
	converged := sync.Report{Agents: []sync.AgentResult{{Agent: "claude", Converged: true}}}
	failed := sync.Report{Agents: []sync.AgentResult{{Agent: "claude", FailedAt: "mcps", Err: errors.New("boom")}}}

	for _, tc := range []struct {
		name    string
		report  sync.Report
		runErr  error
		wantErr bool
	}{
		{name: "every agent converged", report: converged},
		{name: "an agent failed", report: failed, wantErr: true},
		{name: "verification failed", report: converged, runErr: errors.New("invalid output"), wantErr: true},
		{name: "no agent at all", report: sync.Report{}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := syncExit(tc.report, tc.runErr); (err != nil) != tc.wantErr {
				t.Fatalf("syncExit() error = %v, want error = %v", err, tc.wantErr)
			}
		})
	}
}

// TestAgentsSubFS_ResolvesOnlyTheTreesThisBinaryEmbeds locks the mapping the
// runner needs: Claude installs file-based agent definitions from
// embed/agents/claude, and the JSON-config platforms have no such tree at all,
// which is a nil FS rather than an error.
func TestAgentsSubFS_ResolvesOnlyTheTreesThisBinaryEmbeds(t *testing.T) {
	claude, err := agentsSubFS("claude")
	if err != nil {
		t.Fatalf("agentsSubFS(claude) error = %v", err)
	}
	if entries, err := fs.ReadDir(claude, "."); err != nil || len(entries) == 0 {
		t.Fatalf("expected embedded Claude agent definitions, got %d entries, err = %v", len(entries), err)
	}
	if openCode, err := agentsSubFS("opencode"); err != nil || openCode != nil {
		t.Fatalf("agentsSubFS(opencode) = (%v, %v), want (nil, nil)", openCode, err)
	}
}

// TestReplayInput_ResolvesEveryDetectedAgentThroughTheSharedIdentifierRule
// covers the seam between detection and resolution. The map that indexes the
// detected agents and the closure that looks an identifier up in it must apply
// the same normalisation, and nothing in the compiler or the type system says
// so: a rule applied twice desynchronises the moment one copy changes, and an
// agent recorded in the manifest then resolves to nothing and is refused as
// "not installed on this machine". The test therefore drives both halves
// through normalizeAgentID, so a rule that stops being shared stops passing.
func TestReplayInput_ResolvesEveryDetectedAgentThroughTheSharedIdentifierRule(t *testing.T) {
	home := t.TempDir()
	// Detection reads the agent config directories under the home dir, so the
	// set of detected agents is fixed here rather than inherited from whatever
	// the machine running the suite happens to have installed.
	t.Setenv("HOME", home)
	// os.UserHomeDir reads USERPROFILE on Windows, so HOME alone leaves the real
	// home in play and the fixture below detects nothing. This suite runs on
	// windows-latest in CI, so pinning only HOME would make the test assert
	// against the developer's own machine there. Same reason as mcpReplayFixture.
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("create the Claude config dir: %v", err)
	}

	input, err := replayInput(home, &state.State{SchemaVersion: 1, Persona: "neutra", Skills: []string{"go-testing"}})
	if err != nil {
		t.Fatalf("replayInput: %v", err)
	}

	for _, spelling := range []string{"claude", "CLAUDE", "  Claude\t"} {
		if got := normalizeAgentID(spelling); got != "claude" {
			t.Fatalf("normalizeAgentID(%q) = %q, want %q", spelling, got, "claude")
		}
		found, ok := input.Resolve(spelling)
		if !ok {
			t.Fatalf("the detected Claude agent is unreachable through %q, so detection and resolution disagree", spelling)
		}
		if found.Name() != "claude" {
			t.Fatalf("Resolve(%q) returned the %q agent", spelling, found.Name())
		}
	}
	// The other half: resolution must stay a lookup, not a fallback. An agent
	// this machine does not have is a miss, which is what makes the runner
	// refuse it instead of replaying into nothing.
	if found, ok := input.Resolve("no-such-agent"); ok {
		t.Fatalf("Resolve(no-such-agent) = (%v, true), want a miss", found.Name())
	}
}

// TestSyncImportClosure_NeverReachesHiveMemorySync proves the domain boundary
// structurally rather than by reading the command body. The closure is seeded
// per file, not per package: cmd/jarvis as a whole reaches Hive through sibling
// commands such as `jarvis login`, and the question here is what sync reaches.
// The toolchain resolves the transitive half, so build tags and module
// replacements are honoured rather than re-implemented.
//
// The vacuity guard is the important half: a closure seeded from a file that
// imports no Jarvis package is empty, and an empty input set satisfies every
// "contains no X" assertion without proving anything at all.
func TestSyncImportClosure_NeverReachesHiveMemorySync(t *testing.T) {
	const module = "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"

	parsed, err := parser.ParseFile(token.NewFileSet(), "cmd_sync.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse cmd_sync.go: %v", err)
	}
	seed := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", spec.Path.Value, err)
		}
		if strings.HasPrefix(imported, module) {
			seed = append(seed, imported)
		}
	}
	if len(seed) == 0 {
		t.Fatal("cmd_sync.go imports no Jarvis package, so this closure proves nothing")
	}

	listed, err := exec.Command("go", append([]string{"list", "-deps"}, seed...)...).Output()
	if err != nil {
		// This test has two failure classes that must be told apart: a genuine
		// forbidden dependency, and a cold module cache, a restricted resolve or
		// a missing toolchain. Only the child's stderr distinguishes them, and
		// Output() captured it into ExitError.Stderr precisely because Cmd.Stderr
		// is nil here; %v alone would render "exit status 1" and discard it.
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go list -deps %s: %v: %s", strings.Join(seed, " "), err, exit.Stderr)
		}
		t.Fatalf("go list -deps %s: %v", strings.Join(seed, " "), err)
	}
	closure := string(listed)
	if !strings.Contains(closure, module+"/internal/sync\n") {
		t.Fatal("the replay package is absent from the closure, so this proves nothing about replay")
	}
	// The inclusion criterion, so the list stays extensible by reading rather
	// than by archaeology: a package belongs here when it carries Hive memory
	// content, the transport that moves it, or the credentials that reach it.
	// `hivederive` fails that criterion despite its name -- it derives a project
	// name from a git remote and carries none of the three -- and listing it
	// would fail this test on a word rather than on a dependency.
	//
	// Each entry is matched exactly as strictly as the positive anchor above:
	// module-qualified and newline-terminated, so a package of another module
	// whose path merely contains one of these names cannot trip the guard.
	for _, forbidden := range []string{"internal/hiveclient", "internal/hiveui", "internal/importui", "internal/apiclient"} {
		if strings.Contains(closure, module+"/"+forbidden+"\n") {
			t.Errorf("jarvis sync reaches Hive memory synchronization through %q", forbidden)
		}
	}
}

package main

import (
	"errors"
	"io"
	"io/fs"
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
		Report: sync.Report{Agents: []sync.AgentResult{{Agent: "claude", Converged: true}}},
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
			unwanted: []string{"already current"},
		},
		{
			name:     "a converged machine is already current and lists nothing",
			result:   converged,
			want:     []string{"already current", "changed paths: 0", "verification: passed"},
			unwanted: []string{"snapshot", changedPath},
		},
		{
			// A failure after the applier ran must never be reported as a
			// measured zero: the diff was not taken, which is not evidence that
			// nothing changed, and the operator needs that distinction to decide
			// whether to restore the snapshot named above it.
			name:     "a run that failed after mutating says the diff was not measured",
			result:   unmeasured,
			runErr:   errors.New("assert mode on /home/u/.claude/CLAUDE.md: permission denied"),
			want:     []string{"not measured", "not evidence that nothing changed", "snap-7", "verification: failed"},
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

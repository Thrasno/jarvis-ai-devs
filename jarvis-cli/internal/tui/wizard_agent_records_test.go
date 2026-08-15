package tui

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// agentIDs lists the recorded agent IDs in order, so a failure message shows
// what the manifest actually holds.
func agentIDs(agents []state.Agent) []string {
	ids := make([]string, 0, len(agents))
	for _, agent := range agents {
		ids = append(ids, agent.ID)
	}
	return ids
}

// agent.Detect is presence-based: a user who moves an agent's config directory
// away is not telling the installer to forget that agent. The record a run did
// not observe is the only ownership proof that authorizes cleaning that agent
// up later, so it has to survive the run that did not see it.
func TestRecordWizardAgents_KeepsAgentsThisRunDidNotObserve(t *testing.T) {
	desired := state.New()
	desired.InstalledAgents = []state.Agent{
		{ID: "claude", InstructionsPath: "~/.claude/CLAUDE.md"},
		{ID: "opencode", ConfigPath: "~/.config/opencode/opencode.json"},
	}

	recordWizardAgents(desired, []AgentApplyResult{{
		AgentName: "claude",
		State: state.AgentRecord{
			Configured:       true,
			InstructionsPath: "~/.claude/CLAUDE.md",
		},
	}})

	got := agentIDs(desired.InstalledAgents)
	if len(got) != 2 || got[0] != "claude" || got[1] != "opencode" {
		t.Fatalf("installed agents = %v, want [claude opencode]; a run that only detected claude erased opencode's record", got)
	}
}

// The same invariant at its worst: a run that detects nothing must not empty
// the record. An empty installed_agents list makes `jarvis sync` hard-fail and
// flips the config status to recover.
func TestRecordWizardAgents_KeepsEveryRecordWhenTheRunObservedNone(t *testing.T) {
	desired := state.New()
	desired.InstalledAgents = []state.Agent{{ID: "claude"}, {ID: "opencode"}}

	recordWizardAgents(desired, nil)

	if got := agentIDs(desired.InstalledAgents); len(got) != 2 {
		t.Fatalf("installed agents = %v, want both records preserved", got)
	}
}

// Merging must not freeze stale paths: an agent this run reconfigured is
// recorded with the paths this run wrote.
func TestRecordWizardAgents_RefreshesThePathsOfAnAgentThisRunConfigured(t *testing.T) {
	desired := state.New()
	desired.InstalledAgents = []state.Agent{{ID: "claude", InstructionsPath: "~/old/CLAUDE.md"}}

	recordWizardAgents(desired, []AgentApplyResult{{
		AgentName: "claude",
		State:     state.AgentRecord{Configured: true, InstructionsPath: "~/.claude/CLAUDE.md"},
	}})

	if len(desired.InstalledAgents) != 1 {
		t.Fatalf("installed agents = %v, want a single claude record", agentIDs(desired.InstalledAgents))
	}
	if desired.InstalledAgents[0].InstructionsPath != "~/.claude/CLAUDE.md" {
		t.Fatalf("instructions path = %q, want the path this run wrote", desired.InstalledAgents[0].InstructionsPath)
	}
}

// The commit is the second half of the same invariant: the manifest on disk may
// already record an agent this wizard run never loaded, and the write must not
// replace the recorded list with the run's view of it.
func TestRecordWizardDesiredState_DoesNotEraseAgentsAlreadyOnDisk(t *testing.T) {
	isolateTestHome(t)

	if err := state.Update(func(st *state.State) {
		st.InstalledAgents = []state.Agent{{ID: "claude"}, {ID: "opencode"}}
	}); err != nil {
		t.Fatalf("seed the manifest: %v", err)
	}

	desired := state.New()
	desired.InstalledAgents = []state.Agent{{ID: "claude"}}
	if err := recordWizardDesiredState(desired); err != nil {
		t.Fatalf("recordWizardDesiredState: %v", err)
	}

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load the manifest: %v", err)
	}
	if got := agentIDs(manifest.InstalledAgents); len(got) != 2 {
		t.Fatalf("installed agents = %v, want opencode's record preserved", got)
	}
}

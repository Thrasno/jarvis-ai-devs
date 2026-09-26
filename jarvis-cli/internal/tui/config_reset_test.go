package tui

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// claudeAgentsDirDisplayLine is the exact contract Display text for the
// claude.agents.directory reset surface (internal/sddruntime/reset_inventory.go).
// The step must show this text verbatim, never a hand-written paraphrase.
const claudeAgentsDirDisplayLine = "agents/: entire directory (SDD phase agents, Judgment Day agents, retired review-* agents, and any user-added agent files)"

func TestInitializeConfigResetStep_NoAgents_HidesStep(t *testing.T) {
	m := Model{}
	m = initializeConfigResetStep(m)
	if m.resetInventories != nil {
		t.Fatalf("resetInventories = %+v, want nil with no detected agents", m.resetInventories)
	}
	if initialWizardStep(m) != StepScope {
		t.Fatalf("initialWizardStep = %v, want StepScope with no detected agents", initialWizardStep(m))
	}
}

func TestInitializeConfigResetStep_ShownForOneDetectedAgent(t *testing.T) {
	tests := []struct {
		name      string
		configure func(home string) error
		wantName  string
	}{
		{
			name: "claude only",
			configure: func(home string) error {
				return os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
			},
			wantName: "claude",
		},
		{
			name: "opencode only",
			configure: func(home string) error {
				return os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)
			},
			wantName: "opencode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateTestHome(t)
			t.Setenv("PATH", "")
			if err := tt.configure(home); err != nil {
				t.Fatalf("configure: %v", err)
			}

			m := Model{Agents: agent.Detect(embed.FS{})}
			if len(m.Agents) != 1 {
				t.Fatalf("agent.Detect() = %+v, want exactly one detected agent", m.Agents)
			}
			m = initializeConfigResetStep(m)
			if initialWizardStep(m) != StepConfigReset {
				t.Fatalf("initialWizardStep = %v, want StepConfigReset", initialWizardStep(m))
			}
			if len(m.resetInventories) != 1 {
				t.Fatalf("resetInventories = %+v, want exactly one entry", m.resetInventories)
			}
			inv := m.resetInventories[0]
			if inv.AgentName != tt.wantName {
				t.Fatalf("AgentName = %q, want %q", inv.AgentName, tt.wantName)
			}
			if inv.PlanErr != nil {
				t.Fatalf("PlanErr = %v, want nil for a fresh config dir", inv.PlanErr)
			}
			if len(inv.DisplayLines) == 0 {
				t.Fatalf("DisplayLines is empty, want the contract's reset inventory")
			}
		})
	}
}

func TestInitializeConfigResetStep_ShownForBothDetectedAgents(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatalf("mkdir opencode: %v", err)
	}

	m := Model{Agents: agent.Detect(embed.FS{})}
	if len(m.Agents) != 2 {
		t.Fatalf("agent.Detect() = %+v, want two detected agents", m.Agents)
	}
	m = initializeConfigResetStep(m)
	if len(m.resetInventories) != 2 {
		t.Fatalf("resetInventories = %+v, want two entries", m.resetInventories)
	}
	if initialWizardStep(m) != StepConfigReset {
		t.Fatalf("initialWizardStep = %v, want StepConfigReset", initialWizardStep(m))
	}
}

func TestInitializeConfigResetStep_DisplayLinesComeFromContract(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	m := Model{Agents: agent.Detect(embed.FS{})}
	m = initializeConfigResetStep(m)
	if len(m.resetInventories) != 1 {
		t.Fatalf("resetInventories = %+v", m.resetInventories)
	}

	want := sddruntime.ResetInventory(sddruntime.PlatformClaude)
	got := m.resetInventories[0].DisplayLines
	if len(got) != len(want) {
		t.Fatalf("DisplayLines = %v, want %d lines matching sddruntime.ResetInventory(claude)", got, len(want))
	}
	found := false
	for _, line := range got {
		if line == claudeAgentsDirDisplayLine {
			found = true
		}
	}
	if !found {
		t.Fatalf("DisplayLines = %v, want the exact contract Display line for claude.agents.directory", got)
	}
}

func TestInitializeConfigResetStep_PlanError_ForcesNoAndBlocksNavigation(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("seed unparseable settings.json: %v", err)
	}

	m := Model{Agents: agent.Detect(embed.FS{})}
	m = initializeConfigResetStep(m)
	if !m.resetPlanFailed() {
		t.Fatalf("resetPlanFailed() = false, want true for an unparseable settings.json")
	}
	if m.resetInventories[0].PlanErr == nil {
		t.Fatalf("PlanErr = nil, want an error naming the unparseable settings.json")
	}
	if !strings.Contains(m.resetInventories[0].PlanErr.Error(), "settings.json") {
		t.Fatalf("PlanErr = %v, want it to name settings.json", m.resetInventories[0].PlanErr)
	}

	view := viewConfigReset(m)
	if !strings.Contains(view, "Cannot plan a reset") {
		t.Fatalf("view does not surface the plan error:\n%s", view)
	}

	// Even navigating to Yes must not survive: confirming is forced back to No.
	updated, _ := updateConfigReset(m, tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.resetChoice != 0 {
		t.Fatalf("resetChoice = %d after Down with a failed plan, want 0 (navigation locked)", m.resetChoice)
	}
	updated, _ = updateConfigReset(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.resetConsented {
		t.Fatalf("resetConsented = true, want false when the plan failed")
	}
	if m.Step != StepScope {
		t.Fatalf("Step = %v, want StepScope after confirming", m.Step)
	}
}

func TestUpdateConfigReset_NavigationTogglesAndDefaultNo(t *testing.T) {
	fresh := func() Model {
		m := Model{Agents: []agent.Agent{&setupAgentStub{name: "claude", configDir: t.TempDir()}}}
		return initializeConfigResetStep(m)
	}

	m := fresh()
	if m.resetChoice != 0 {
		t.Fatalf("resetChoice = %d, want 0 (No) by default", m.resetChoice)
	}

	// Enter immediately: defaults to No.
	updated, cmd := updateConfigReset(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected a nil cmd, got %T", cmd)
	}
	immediate := updated.(Model)
	if immediate.resetConsented {
		t.Fatalf("resetConsented = true, want false when Enter is pressed immediately")
	}
	if immediate.Step != StepScope {
		t.Fatalf("Step = %v, want StepScope after confirming", immediate.Step)
	}

	// Toggle to Yes with Down, back to No with Up, then to Yes again with 'j'.
	m = fresh()
	updated, _ = updateConfigReset(m, tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.resetChoice != 1 {
		t.Fatalf("resetChoice = %d after Down, want 1 (Yes)", m.resetChoice)
	}
	updated, _ = updateConfigReset(m, tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.resetChoice != 0 {
		t.Fatalf("resetChoice = %d after Up, want 0 (No) again", m.resetChoice)
	}
	updated, _ = updateConfigReset(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	if m.resetChoice != 1 {
		t.Fatalf("resetChoice = %d after 'j', want 1 (Yes)", m.resetChoice)
	}
	updated, _ = updateConfigReset(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.resetConsented {
		t.Fatalf("resetConsented = false, want true after confirming Yes")
	}
	if m.Step != StepScope {
		t.Fatalf("Step = %v, want StepScope after confirming", m.Step)
	}
}

func TestUpdateConfigReset_NoAgents_SkipsToScope(t *testing.T) {
	m := Model{}
	updated, cmd := updateConfigReset(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected a nil cmd, got %T", cmd)
	}
	m = updated.(Model)
	if m.Step != StepScope {
		t.Fatalf("Step = %v, want StepScope when no agents detected", m.Step)
	}
}

func TestViewConfigReset_ShowsContractLinesConcreteResidueAndDefaultNo(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(filepath.Join(claudeDir, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "agents", "my-custom-agent.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed user agent: %v", err)
	}

	m := Model{Agents: agent.Detect(embed.FS{})}
	m = initializeConfigResetStep(m)
	view := viewConfigReset(m)

	// A short Display line that the bordered panel renders on one line, so
	// wrapping does not interfere with the substring check. The long
	// agents/-directory line is covered exactly (unwrapped) by
	// TestInitializeConfigResetStep_DisplayLinesComeFromContract.
	const shortDisplayLine = "CLAUDE.md: Jarvis instructions marker block content"
	if !strings.Contains(view, shortDisplayLine) {
		t.Fatalf("view does not show the contract Display line %q:\n%s", shortDisplayLine, view)
	}
	if !strings.Contains(view, "my-custom-agent.md") || !strings.Contains(view, "user-added") {
		t.Fatalf("view does not surface the concrete user-added residue found on this machine:\n%s", view)
	}
	if !strings.Contains(view, "No, keep my current configuration") {
		t.Fatalf("view is missing the No option:\n%s", view)
	}
	if !strings.Contains(view, "Yes, reset it before continuing") {
		t.Fatalf("view is missing the Yes option:\n%s", view)
	}
}

func TestConfigureWizardAgents_DeclinedReset_LeavesResidueUntouched(t *testing.T) {
	configDir := t.TempDir()
	residuePath := filepath.Join(configDir, "agents", "review-risk.md")
	if err := os.MkdirAll(filepath.Dir(residuePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(residuePath, []byte("legacy 4R agent"), 0o644); err != nil {
		t.Fatalf("seed residue: %v", err)
	}

	assignments, err := sddruntime.DefaultAssignmentsForPlatform(sddruntime.PlatformClaude)
	if err != nil {
		t.Fatalf("resolve default assignments: %v", err)
	}
	a := &setupAgentStub{name: "claude", configDir: configDir, observeRuntime: passingRuntimeObservation(t, "claude", assignments, nil)}

	results := configureWizardAgents([]agent.Agent{a}, state.PhaseModels{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil, nil, func() bool { return true }, false, "")
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if results[0].ResetApplied {
		t.Fatalf("ResetApplied = true, want false when the reset step was declined")
	}
	if results[0].ResetSnapshotID != "" {
		t.Fatalf("ResetSnapshotID = %q, want empty when declined", results[0].ResetSnapshotID)
	}
	if _, err := os.Stat(residuePath); err != nil {
		t.Fatalf("residue file was touched despite a declined reset: %v", err)
	}
	if lines := configResetSummaryLines(results); len(lines) != 0 {
		t.Fatalf("configResetSummaryLines = %v, want empty for a declined reset", lines)
	}
}

func TestConfigureWizardAgents_AcceptedReset_AppliesBeforeInstallAndSummarizes(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	residuePath := filepath.Join(configDir, "agents", "review-risk.md")
	if err := os.MkdirAll(filepath.Dir(residuePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(residuePath, []byte("legacy 4R agent"), 0o644); err != nil {
		t.Fatalf("seed residue: %v", err)
	}

	assignments, err := sddruntime.DefaultAssignmentsForPlatform(sddruntime.PlatformClaude)
	if err != nil {
		t.Fatalf("resolve default assignments: %v", err)
	}
	a := &setupAgentStub{name: "claude", configDir: configDir, observeRuntime: passingRuntimeObservation(t, "claude", assignments, nil)}

	results := configureWizardAgents([]agent.Agent{a}, state.PhaseModels{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil, nil, func() bool { return true }, true, home)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if !results[0].ResetApplied || results[0].ResetSnapshotID == "" {
		t.Fatalf("results[0] = %+v, want ResetApplied=true and a non-empty snapshot ID", results[0])
	}
	if _, err := os.Stat(residuePath); !os.IsNotExist(err) {
		t.Fatalf("residue file still present after an accepted reset, stat err = %v", err)
	}

	lines := configResetSummaryLines(results)
	if len(lines) != 1 {
		t.Fatalf("configResetSummaryLines = %v, want exactly one line", lines)
	}
	if !strings.Contains(lines[0], "claude") || !strings.Contains(lines[0], results[0].ResetSnapshotID) {
		t.Fatalf("configResetSummaryLines[0] = %q, want it to name claude and the snapshot ID %q", lines[0], results[0].ResetSnapshotID)
	}
}

func TestConfigResetSummaryLines_EmptyWhenNothingWasReset(t *testing.T) {
	results := []AgentApplyResult{{AgentName: "claude"}, {AgentName: "opencode"}}
	if lines := configResetSummaryLines(results); len(lines) != 0 {
		t.Fatalf("configResetSummaryLines = %v, want empty when nothing was reset", lines)
	}
}

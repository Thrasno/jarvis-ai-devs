package tui

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
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

	results := configureWizardAgents([]agent.Agent{a}, WizardAgentApplyOptions{PhaseModels: state.PhaseModels{}, HiveEntry: agent.MCPEntry{Name: "hive"}, Context7Entry: agent.MCPEntry{Name: "context7"}, PresetCtx: wizardPresetApplyContext{}, SkillsSubFS: testSkillsFS, StatuslineConfirm: func() bool { return true }})
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

	results := configureWizardAgents([]agent.Agent{a}, WizardAgentApplyOptions{PhaseModels: state.PhaseModels{}, HiveEntry: agent.MCPEntry{Name: "hive"}, Context7Entry: agent.MCPEntry{Name: "context7"}, PresetCtx: wizardPresetApplyContext{}, SkillsSubFS: testSkillsFS, StatuslineConfirm: func() bool { return true }, ResetConsented: true, Home: home})
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

// --- Hardening R2-002/R2-003: the "entirely replaced" wording is derived
// per platform from the contract, not a hand-written Claude-only paragraph ---

func TestViewConfigReset_OpenCodeOnly_WarningNamesOpenCodeSurfacesNotClaudeAgentsDir(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatalf("mkdir opencode: %v", err)
	}

	m := Model{Agents: agent.Detect(embed.FS{}), width: 120}
	m = initializeConfigResetStep(m)
	view := viewConfigReset(m)

	if strings.Contains(view, "~/.claude/agents/") {
		t.Fatalf("view names Claude's agents/ directory for an OpenCode-only machine:\n%s", view)
	}
	const openCodeCoreKeysDisplay = "opencode.json: default_agent, permission, agent (entirely Jarvis-owned keys, removed whole)"
	if !strings.Contains(view, openCodeCoreKeysDisplay) {
		t.Fatalf("view does not derive the warning from opencode.settings.core_keys' contract Display text:\n%s", view)
	}
}

func TestConfigResetSummaryLines_OpenCode_NamesOpenCodeSurfacesNotAgentsDirectory(t *testing.T) {
	results := []AgentApplyResult{{AgentName: "opencode", ResetApplied: true, ResetSnapshotID: "snap-123"}}
	lines := configResetSummaryLines(results)
	if len(lines) != 1 {
		t.Fatalf("configResetSummaryLines = %v, want exactly one line", lines)
	}
	if strings.Contains(lines[0], "agents/ directory") {
		t.Fatalf("configResetSummaryLines[0] = %q, want no mention of Claude's agents/ directory for opencode", lines[0])
	}
	if !strings.Contains(lines[0], "default_agent") {
		t.Fatalf("configResetSummaryLines[0] = %q, want it to name opencode's ReplacedEntirely surfaces", lines[0])
	}
}

func TestConfigResetSummaryLines_EmptyWhenNothingWasReset(t *testing.T) {
	results := []AgentApplyResult{{AgentName: "claude"}, {AgentName: "opencode"}}
	if lines := configResetSummaryLines(results); len(lines) != 0 {
		t.Fatalf("configResetSummaryLines = %v, want empty when nothing was reset", lines)
	}
}

// --- Hardening R4-002: a reset applied for an agent whose install then
// fails is rolled back, not left deleted with nothing reinstalled ---

func TestConfigureWizardAgents_ResetAppliedThenInstallFails_RollsBackThatAgent(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	residuePath := filepath.Join(configDir, "agents", "review-risk.md")
	if err := os.MkdirAll(filepath.Dir(residuePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const residueContent = "legacy 4R agent"
	if err := os.WriteFile(residuePath, []byte(residueContent), 0o644); err != nil {
		t.Fatalf("seed residue: %v", err)
	}

	failing := &setupAgentStub{name: "claude", configDir: configDir, installOrchErr: errors.New("install orchestrator fail")}

	results := configureWizardAgents([]agent.Agent{failing}, WizardAgentApplyOptions{
		PhaseModels:       state.PhaseModels{},
		HiveEntry:         agent.MCPEntry{Name: "hive"},
		Context7Entry:     agent.MCPEntry{Name: "context7"},
		PresetCtx:         wizardPresetApplyContext{},
		SkillsSubFS:       testSkillsFS,
		StatuslineConfirm: func() bool { return true },
		ResetConsented:    true,
		Home:              home,
	})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one result", results)
	}
	res := results[0]
	if res.Err == nil {
		t.Fatalf("expected the install failure to surface as an error")
	}
	if !strings.Contains(res.Err.Error(), "install orchestrator fail") {
		t.Fatalf("res.Err = %v, want it to still name the install failure", res.Err)
	}
	if !res.ResetApplied {
		t.Fatalf("res.ResetApplied = false, want true: the reset ran before the failing install step")
	}
	if !res.ResetRolledBack {
		t.Fatalf("res.ResetRolledBack = false, want true: rollback should have fully restored the residue")
	}
	if res.ResetRollbackErr != nil {
		t.Fatalf("res.ResetRollbackErr = %v, want nil on a successful rollback", res.ResetRollbackErr)
	}
	got, err := os.ReadFile(residuePath)
	if err != nil {
		t.Fatalf("residue file missing after rollback, want it restored: %v", err)
	}
	if string(got) != residueContent {
		t.Fatalf("residue content = %q, want restored %q", got, residueContent)
	}
}

func TestConfigureWizardAgents_TwoAgents_EarlierResetAgentStaysAppliedWhenLaterAgentFails(t *testing.T) {
	home := t.TempDir()
	claudeConfigDir := filepath.Join(home, ".claude")
	residuePath := filepath.Join(claudeConfigDir, "agents", "review-risk.md")
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
	succeeding := &setupAgentStub{name: "claude", configDir: claudeConfigDir, observeRuntime: passingRuntimeObservation(t, "claude", assignments, nil)}

	openCodeConfigDir := filepath.Join(home, ".config", "opencode")
	openCodeResidue := filepath.Join(openCodeConfigDir, "sdd-orchestrator.md")
	if err := os.MkdirAll(openCodeConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir opencode config dir: %v", err)
	}
	const openCodeResidueContent = "opencode orchestrator content"
	if err := os.WriteFile(openCodeResidue, []byte(openCodeResidueContent), 0o644); err != nil {
		t.Fatalf("seed opencode residue: %v", err)
	}
	failing := &setupAgentStub{name: "opencode", configDir: openCodeConfigDir, installOrchErr: errors.New("opencode install fail")}

	results := configureWizardAgents([]agent.Agent{succeeding, failing}, WizardAgentApplyOptions{
		PhaseModels:       state.PhaseModels{},
		HiveEntry:         agent.MCPEntry{Name: "hive"},
		Context7Entry:     agent.MCPEntry{Name: "context7"},
		PresetCtx:         wizardPresetApplyContext{},
		SkillsSubFS:       testSkillsFS,
		StatuslineConfirm: func() bool { return true },
		ResetConsented:    true,
		Home:              home,
	})
	if len(results) != 2 {
		t.Fatalf("results = %+v, want exactly two results (one per agent)", results)
	}
	if results[0].Err != nil {
		t.Fatalf("first (claude) result = %+v, want no error: it installed successfully after its own reset", results[0])
	}
	if !results[0].ResetApplied || results[0].ResetRolledBack {
		t.Fatalf("first (claude) result = %+v, want ResetApplied=true and ResetRolledBack=false: it must stay as-is", results[0])
	}
	if _, err := os.Stat(residuePath); !os.IsNotExist(err) {
		t.Fatalf("claude residue still present, want it removed by its own successful reset+reinstall")
	}
	if results[1].Err == nil || !results[1].ResetRolledBack {
		t.Fatalf("second (opencode) result = %+v, want an error and ResetRolledBack=true", results[1])
	}
	got, err := os.ReadFile(openCodeResidue)
	if err != nil {
		t.Fatalf("opencode residue missing after rollback, want it restored: %v", err)
	}
	if string(got) != openCodeResidueContent {
		t.Fatalf("opencode residue content = %q, want restored %q", got, openCodeResidueContent)
	}
}

// --- Hardening R4-001/R2-002/R2-003: a shared post-install failure (persona
// profile apply) rolls back every agent's already-applied reset; a
// runtime-verification failure after a complete reinstall does not roll
// back, and the reset line is never dropped from the summary either way ---

func TestConfigureWizardAgents_ProfileApplyFailure_RollsBackSucceededAgentsReset(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	residuePath := filepath.Join(configDir, "agents", "review-risk.md")
	if err := os.MkdirAll(filepath.Dir(residuePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const residueContent = "legacy 4R agent"
	if err := os.WriteFile(residuePath, []byte(residueContent), 0o644); err != nil {
		t.Fatalf("seed residue: %v", err)
	}

	assignments, err := sddruntime.DefaultAssignmentsForPlatform(sddruntime.PlatformClaude)
	if err != nil {
		t.Fatalf("resolve default assignments: %v", err)
	}
	// setupAgentStub does not implement persona.ProfileAgent, so a non-nil
	// Resolved makes applyWizardProfile fail for it -- every agent's own
	// install/merge step already succeeded by the time this runs.
	a := &setupAgentStub{name: "claude", configDir: configDir, observeRuntime: passingRuntimeObservation(t, "claude", assignments, nil)}

	results := configureWizardAgents([]agent.Agent{a}, WizardAgentApplyOptions{
		PhaseModels:       state.PhaseModels{},
		HiveEntry:         agent.MCPEntry{Name: "hive"},
		Context7Entry:     agent.MCPEntry{Name: "context7"},
		Resolved:          &persona.ResolvedProfile{},
		PresetCtx:         wizardPresetApplyContext{},
		SkillsSubFS:       testSkillsFS,
		StatuslineConfirm: func() bool { return true },
		ResetConsented:    true,
		Home:              home,
	})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one result", results)
	}
	res := results[0]
	if res.Err == nil || !strings.Contains(res.Err.Error(), "apply preset pipeline") {
		t.Fatalf("res.Err = %v, want it to name the preset pipeline failure", res.Err)
	}
	if !res.ResetApplied {
		t.Fatalf("res.ResetApplied = false, want true")
	}
	if !res.ResetRolledBack {
		t.Fatalf("res.ResetRolledBack = false, want true: a shared post-install failure must roll back this agent's reset")
	}
	if res.ResetRollbackErr != nil {
		t.Fatalf("res.ResetRollbackErr = %v, want nil on a successful rollback", res.ResetRollbackErr)
	}
	got, err := os.ReadFile(residuePath)
	if err != nil {
		t.Fatalf("residue file missing after rollback, want it restored: %v", err)
	}
	if string(got) != residueContent {
		t.Fatalf("residue content = %q, want restored %q", got, residueContent)
	}

	lines := configResetSummaryLines(results)
	if len(lines) != 1 {
		t.Fatalf("configResetSummaryLines = %v, want exactly one line even though this agent's overall setup failed", lines)
	}
	if !strings.Contains(lines[0], res.ResetSnapshotID) {
		t.Fatalf("configResetSummaryLines[0] = %q, want it to still name the snapshot ID %q", lines[0], res.ResetSnapshotID)
	}
	if !strings.Contains(lines[0], "rolled it back") {
		t.Fatalf("configResetSummaryLines[0] = %q, want it to report the rollback", lines[0])
	}
}

func TestConfigureWizardAgents_RuntimeVerificationFailure_DoesNotRollBackReset(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	residuePath := filepath.Join(configDir, "agents", "review-risk.md")
	if err := os.MkdirAll(filepath.Dir(residuePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(residuePath, []byte("legacy 4R agent"), 0o644); err != nil {
		t.Fatalf("seed residue: %v", err)
	}

	a := &setupAgentStub{name: "claude", configDir: configDir, observeRuntimeErr: errors.New("injected runtime observe failure")}

	results := configureWizardAgents([]agent.Agent{a}, WizardAgentApplyOptions{
		PhaseModels:       state.PhaseModels{},
		HiveEntry:         agent.MCPEntry{Name: "hive"},
		Context7Entry:     agent.MCPEntry{Name: "context7"},
		PresetCtx:         wizardPresetApplyContext{},
		SkillsSubFS:       testSkillsFS,
		StatuslineConfirm: func() bool { return true },
		ResetConsented:    true,
		Home:              home,
	})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one result", results)
	}
	res := results[0]
	if res.Err == nil || !strings.Contains(res.Err.Error(), "runtime verification") {
		t.Fatalf("res.Err = %v, want it to name the runtime verification failure", res.Err)
	}
	if !res.ResetApplied {
		t.Fatalf("res.ResetApplied = false, want true")
	}
	if res.ResetRolledBack {
		t.Fatalf("res.ResetRolledBack = true, want false: a runtime-verification failure after a complete reinstall must not roll back")
	}
	if res.ResetRollbackErr != nil {
		t.Fatalf("res.ResetRollbackErr = %v, want nil: rollback was never attempted", res.ResetRollbackErr)
	}
	if _, err := os.Stat(residuePath); !os.IsNotExist(err) {
		t.Fatalf("residue file still present, want it removed by the reset that ran and was kept")
	}

	lines := configResetSummaryLines(results)
	if len(lines) != 1 {
		t.Fatalf("configResetSummaryLines = %v, want exactly one line: the reset ran and must still be reported", lines)
	}
	if !strings.Contains(lines[0], res.ResetSnapshotID) {
		t.Fatalf("configResetSummaryLines[0] = %q, want it to still name the snapshot ID %q", lines[0], res.ResetSnapshotID)
	}
	if !strings.Contains(lines[0], "kept") {
		t.Fatalf("configResetSummaryLines[0] = %q, want it to report the reset was kept despite the later failure", lines[0])
	}
}

// --- Hardening R1-001/R4-002/R2-004: NotInDurableSnapshot is surfaced in the
// plan view before consent ---

func TestViewConfigReset_ShowsNotInDurableSnapshotWarning(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	configDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	realOrchestratorPath := filepath.Join(home, "dotfiles", "sdd-orchestrator.md")
	if err := os.MkdirAll(filepath.Dir(realOrchestratorPath), 0o755); err != nil {
		t.Fatalf("mkdir dotfiles: %v", err)
	}
	if err := os.WriteFile(realOrchestratorPath, []byte("shared orchestrator content"), 0o644); err != nil {
		t.Fatalf("seed orchestrator: %v", err)
	}
	orchestratorLink := filepath.Join(configDir, "sdd-orchestrator.md")
	if err := os.Symlink(realOrchestratorPath, orchestratorLink); err != nil {
		t.Fatalf("symlink orchestrator: %v", err)
	}

	m := Model{Agents: agent.Detect(embed.FS{}), width: 120}
	m = initializeConfigResetStep(m)
	view := viewConfigReset(m)

	if !strings.Contains(view, "only be recoverable through the in-process rollback") {
		t.Fatalf("view does not warn about durable-snapshot exclusions:\n%s", view)
	}
	if !strings.Contains(view, orchestratorLink) {
		t.Fatalf("view does not name the excluded path %s:\n%s", orchestratorLink, view)
	}
}

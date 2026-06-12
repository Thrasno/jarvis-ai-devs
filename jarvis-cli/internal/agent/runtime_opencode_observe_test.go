package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// validOpenCodeJSON produces a minimal but structurally valid opencode.json
// with the SDD orchestrator and a couple of subagents.
func validOpenCodeJSON(t *testing.T) []byte {
	t.Helper()
	agents := map[string]any{
		"sdd-orchestrator": map[string]any{
			"mode":   "primary",
			"model":  "opus",
			"prompt": "{file:./sdd-orchestrator.md}",
			"permission": map[string]any{
				"task": map[string]any{
					"*":           "deny",
					"sdd-explore": "allow",
				},
			},
		},
		"sdd-explore": map[string]any{
			"mode":   "subagent",
			"hidden": true,
			"model":  "sonnet",
			"prompt": "explore prompt",
		},
	}
	doc := map[string]any{
		"share":         "disabled",
		"default_agent": "sdd-orchestrator",
		"permission": map[string]any{
			"bash": map[string]any{"*": "allow"},
			"read": map[string]any{"*": "allow", ".env": "deny"},
		},
		"agent": agents,
		"mcp": map[string]any{
			"hive":     map[string]any{"type": "local", "command": []string{"hive-daemon"}},
			"context7": map[string]any{"type": "remote", "url": "https://mcp.context7.com/mcp"},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal test opencode.json: %v", err)
	}
	return data
}

// TestObserveRuntimeWithConfig_OpenCode_PopulatesOpenCodeConfig asserts that
// when an OpenCode agent observes a valid opencode.json, the resulting
// ObservedRuntime.OpenCode has ParseSucceeded==true and key fields set.
func TestObserveRuntimeWithConfig_OpenCode_PopulatesOpenCodeConfig(t *testing.T) {
	home := t.TempDir()
	a := &OpenCodeAgent{home: home, templatesFS: testTemplatesFS}

	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	// Write a valid opencode.json to the config dir.
	if err := os.WriteFile(a.settingsPath(), validOpenCodeJSON(t), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}
	// Satisfy required managed artifacts (instructions, orchestrator, skills).
	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}
	if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}
	skillsFS := fstest.MapFS{"_shared/SKILL.md": {Data: []byte("# shared")}}
	if err := a.InstallSkills(skillsFS, nil); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	observed, err := a.ObserveRuntime()
	if err != nil {
		t.Fatalf("ObserveRuntime: %v", err)
	}

	if !observed.OpenCode.ParseSucceeded {
		t.Fatal("OpenCode.ParseSucceeded must be true for valid opencode.json")
	}
	if observed.OpenCode.DefaultAgent != "sdd-orchestrator" {
		t.Errorf("OpenCode.DefaultAgent = %q, want %q", observed.OpenCode.DefaultAgent, "sdd-orchestrator")
	}
	if observed.OpenCode.ShareMode != "disabled" {
		t.Errorf("OpenCode.ShareMode = %q, want %q", observed.OpenCode.ShareMode, "disabled")
	}
	if observed.OpenCode.OrchestratorMode != "primary" {
		t.Errorf("OpenCode.OrchestratorMode = %q, want %q", observed.OpenCode.OrchestratorMode, "primary")
	}
}

// TestObserveRuntimeWithConfig_Claude_LeavesOpenCodeConfigAtZero asserts that
// the Claude adapter never populates ObservedRuntime.OpenCode — it must remain
// at its zero value (ParseSucceeded==false).
func TestObserveRuntimeWithConfig_Claude_LeavesOpenCodeConfigAtZero(t *testing.T) {
	home := t.TempDir()
	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}
	if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}
	skillsFS := fstest.MapFS{"_shared/SKILL.md": {Data: []byte("# shared")}}
	if err := a.InstallSkills(skillsFS, nil); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	observed, err := a.ObserveRuntime()
	if err != nil {
		t.Fatalf("ObserveRuntime: %v", err)
	}

	if observed.OpenCode.ParseSucceeded {
		t.Error("Claude adapter must leave OpenCode.ParseSucceeded == false")
	}
	if observed.OpenCode.ShareMode != "" {
		t.Errorf("Claude adapter must leave OpenCode.ShareMode empty, got %q", observed.OpenCode.ShareMode)
	}
}

// TestObserveRuntimeWithConfig_OpenCode_MissingSettingsFile_ZeroOpenCode asserts
// that when opencode.json is absent, OpenCode.ParseSucceeded remains false and
// observation does not error.
func TestObserveRuntimeWithConfig_OpenCode_MissingSettingsFile_ZeroOpenCode(t *testing.T) {
	plan, err := sddruntime.Build("opencode")
	if err != nil {
		t.Fatalf("Build opencode: %v", err)
	}
	configDir := t.TempDir()

	// Satisfy required artifacts (no settings file).
	if err := os.WriteFile(filepath.Join(configDir, "AGENTS.md"), []byte("<!-- jarvis:layer1:start -->\nX\n<!-- jarvis:layer1:end -->"), 0644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sdd-orchestrator.md"), []byte("# orchestrator"), 0644); err != nil {
		t.Fatalf("write orchestrator: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	observed, err := observeRuntime(configDir, plan)
	if err != nil {
		t.Fatalf("observeRuntime: %v", err)
	}

	if observed.OpenCode.ParseSucceeded {
		t.Error("OpenCode.ParseSucceeded must be false when opencode.json is absent")
	}
}

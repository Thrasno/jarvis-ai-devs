package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
)

// TestOpenCodeAgent_SupportsOutputStyles verifies OpenCodeAgent returns false (SPEC-001).
func TestOpenCodeAgent_SupportsOutputStyles(t *testing.T) {
	agent := &OpenCodeAgent{}
	if agent.SupportsOutputStyles() {
		t.Error("OpenCodeAgent.SupportsOutputStyles() = true, want false")
	}
}

// TestOpenCodeAgent_WriteOutputStyle_NoOp verifies WriteOutputStyle is a no-op
// and doesn't create any files (SPEC-001, SPEC-009).
func TestOpenCodeAgent_WriteOutputStyle_NoOp(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	preset := &persona.Preset{
		Name:        "argentino",
		DisplayName: "Argentino",
		Description: "Test",
		Notes:       "Test notes.",
	}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Errorf("WriteOutputStyle() returned error: %v, want nil", err)
	}

	// Verify no output-styles directory was created
	outputStylesDir := filepath.Join(tmpHome, ".config", "opencode", "output-styles")
	if _, err := os.Stat(outputStylesDir); !os.IsNotExist(err) {
		t.Errorf("output-styles directory should not exist, but found: %s", outputStylesDir)
	}

	// Verify no output-style file was created
	outputStyleFile := filepath.Join(outputStylesDir, "Argentino.md")
	if _, err := os.ReadFile(outputStyleFile); !os.IsNotExist(err) {
		t.Error("output-style file should not be created for OpenCodeAgent")
	}

	// Verify settings.json was not modified
	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings file should not be created by WriteOutputStyle for OpenCodeAgent")
	}
}

// TestOpenCodeAgent_MergeConfig_Context7 verifies Context7 MCP is added with correct remote format.
// Spec R3: OpenCode uses remote mode with specific URL.
func TestOpenCodeAgent_MergeConfig_Context7(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	// Create empty opencode.json first
	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	// Call MergeConfig with context7 entry
	entry := MCPEntry{Name: "context7"}
	err := agent.MergeConfig(entry)
	if err != nil {
		t.Fatalf("MergeConfig(context7) failed: %v", err)
	}

	// Read and verify opencode.json
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal opencode.json: %v", err)
	}

	mcp, ok := settings["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp missing from opencode.json")
	}

	context7, ok := mcp["context7"].(map[string]any)
	if !ok {
		t.Fatal("context7 entry missing from mcp")
	}

	// Verify remote format (Spec R3)
	if context7["type"] != "remote" {
		t.Errorf("expected context7.type=remote, got %v", context7["type"])
	}

	if context7["url"] != "https://mcp.context7.com/mcp" {
		t.Errorf("expected context7.url=https://mcp.context7.com/mcp, got %v", context7["url"])
	}

	if context7["enabled"] != true {
		t.Errorf("expected context7.enabled=true, got %v", context7["enabled"])
	}
}

// TestOpenCodeAgent_MergeConfig_Context7_PreservesExisting verifies Context7 doesn't clobber other MCPs.
// Spec R4: Deep merge behavior preserves existing servers.
func TestOpenCodeAgent_MergeConfig_Context7_PreservesExisting(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}

	// Create opencode.json with existing hive and engram entries
	existingSettings := `{
		"agents": ["primary"],
		"mcp": {
			"hive": {"command": ["/usr/local/bin/hive-daemon"], "type": "local"},
			"engram": {"command": ["/go/bin/engram", "mcp"], "type": "local"}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(existingSettings), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	// Add Context7
	entry := MCPEntry{Name: "context7"}
	err := agent.MergeConfig(entry)
	if err != nil {
		t.Fatalf("MergeConfig(context7) failed: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal opencode.json: %v", err)
	}

	// Verify agents array preserved
	agents, ok := settings["agents"].([]any)
	if !ok || len(agents) == 0 {
		t.Errorf("agents array was lost, expected [primary], got %v", settings["agents"])
	}

	mcp := settings["mcp"].(map[string]any)

	// Verify existing servers preserved
	if _, ok := mcp["hive"]; !ok {
		t.Error("hive entry was lost after adding context7")
	}
	if _, ok := mcp["engram"]; !ok {
		t.Error("engram entry was lost after adding context7")
	}

	// Verify context7 was added
	if _, ok := mcp["context7"]; !ok {
		t.Error("context7 entry was not added")
	}
}

// TestOpenCodeAgent_MergeConfig_Context7_Idempotent verifies running MergeConfig twice doesn't duplicate entries.
// Spec R5: Idempotency — no duplication on re-run.
func TestOpenCodeAgent_MergeConfig_Context7_Idempotent(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	entry := MCPEntry{Name: "context7"}

	// Run MergeConfig TWICE
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("first MergeConfig(context7) failed: %v", err)
	}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("second MergeConfig(context7) failed: %v", err)
	}

	// Verify only ONE context7 entry exists
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal opencode.json: %v", err)
	}

	mcp := settings["mcp"].(map[string]any)
	context7 := mcp["context7"].(map[string]any)

	// Verify still correct format after re-run
	if context7["type"] != "remote" {
		t.Errorf("expected context7.type=remote after re-run, got %v", context7["type"])
	}

	// Count mcp keys — should be exactly 1 (context7)
	if len(mcp) != 1 {
		t.Errorf("expected exactly 1 MCP server, got %d: %v", len(mcp), mcp)
	}
}

func TestOpenCodeAgent_MergeConfig_Hive_SkipsPartialEnv(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	err := agent.MergeConfig(MCPEntry{
		Name:       "hive",
		DaemonPath: "/usr/local/bin/hive-daemon",
		APIURL:     "https://hivemem.dev",
		Email:      "user@example.com",
		// Password intentionally missing.
	})
	if err != nil {
		t.Fatalf("MergeConfig(hive) failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal opencode.json: %v", err)
	}

	hive := settings["mcp"].(map[string]any)["hive"].(map[string]any)
	if _, ok := hive["environment"]; ok {
		t.Fatalf("expected no environment block for partial creds, got: %v", hive["environment"])
	}
	if _, ok := hive["env"]; ok {
		t.Fatalf("OpenCode schema requires environment, not env: %v", hive["env"])
	}
}

func TestOpenCodeAgent_MergeConfig_Hive_IncludesEnvWhenComplete(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	err := agent.MergeConfig(MCPEntry{
		Name:       "hive",
		DaemonPath: "/usr/local/bin/hive-daemon",
		APIURL:     "https://hivemem.dev",
		Email:      "user@example.com",
		Password:   "s3cr3t",
	})
	if err != nil {
		t.Fatalf("MergeConfig(hive) failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal opencode.json: %v", err)
	}

	hive := settings["mcp"].(map[string]any)["hive"].(map[string]any)
	env := hive["environment"].(map[string]any)
	if env["HIVE_API_PASSWORD"] != "s3cr3t" {
		t.Fatalf("expected complete environment, got: %v", env)
	}
	if _, ok := hive["env"]; ok {
		t.Fatalf("OpenCode schema requires environment, not env: %v", hive["env"])
	}
}

func TestOpenCodeAgent_ClearOutputStyle_NoOp(t *testing.T) {
	tmpHome := t.TempDir()
	a := &OpenCodeAgent{home: tmpHome}
	if err := a.ClearOutputStyle("anything"); err != nil {
		t.Fatalf("ClearOutputStyle should be no-op: %v", err)
	}
}

func TestOpenCodeAgent_InstallOrchestrator_WritesToConfigDir(t *testing.T) {
	tmpHome := t.TempDir()
	a := &OpenCodeAgent{home: tmpHome}
	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}

	if err := a.InstallOrchestrator([]byte("# opencode orchestrator\n")); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(a.ConfigDir(), "sdd-orchestrator.md"))
	if err != nil {
		t.Fatalf("read installed orchestrator: %v", err)
	}
	if string(content) == "" {
		t.Fatal("expected orchestrator content to be written")
	}
}

func TestOpenCodeAgent_MergeGeneratedConfig_RendersTopologyPermissionsAndPreservesUserKeys(t *testing.T) {
	tmpHome := t.TempDir()
	a := &OpenCodeAgent{home: tmpHome, templatesFS: testTemplatesFS}
	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	existing := `{
		"$schema": "https://example.com/custom-opencode-schema.json",
		"username": "alice",
		"mcp": {
			"hive": {"type": "local", "command": ["/opt/hive-daemon"]},
			"context7": {"type": "remote", "url": "https://mcp.context7.com/mcp", "enabled": true},
			"custom": {"type": "remote", "url": "https://example.com/mcp"}
		},
		"agent": {"user-reviewer": {"mode": "subagent", "model": "anthropic/claude-sonnet-4-5"}},
		"permission": {"bash": {"custom-tool *": "allow"}}
	}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	cfg := defaultRuntimeConfig()
	cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"orchestrator": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
		"sdd-apply":    {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "medium"},
	}

	if err := a.MergeGeneratedConfig(cfg); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	if err := a.MergeGeneratedConfig(cfg); err != nil {
		t.Fatalf("MergeGeneratedConfig rerun: %v", err)
	}

	settings := readJSONFile(t, settingsPath)
	if settings["$schema"] != "https://example.com/custom-opencode-schema.json" {
		t.Fatalf("existing $schema was not preserved: %v", settings["$schema"])
	}
	if settings["username"] != "alice" {
		t.Fatalf("user-owned top-level key was not preserved: %v", settings["username"])
	}
	if settings["share"] != "disabled" {
		t.Fatalf("share = %v, want disabled", settings["share"])
	}
	mcp := settings["mcp"].(map[string]any)
	for _, key := range []string{"hive", "context7", "custom"} {
		if _, ok := mcp[key]; !ok {
			t.Fatalf("mcp.%s missing after generated config merge", key)
		}
	}
	agents := settings["agent"].(map[string]any)
	if _, ok := agents["user-reviewer"]; !ok {
		t.Fatal("user-owned agent was not preserved")
	}
	orchestrator := agents["sdd-orchestrator"].(map[string]any)
	if orchestrator["mode"] != "primary" {
		t.Fatalf("sdd-orchestrator mode = %v, want primary", orchestrator["mode"])
	}
	if orchestrator["model"] != "openai/gpt-5.1-codex-max" {
		t.Fatalf("sdd-orchestrator model = %v, want provider-qualified runtime assignment", orchestrator["model"])
	}
	taskPerm := orchestrator["permission"].(map[string]any)["task"].(map[string]any)
	if taskPerm["*"] != "deny" || taskPerm["sdd-apply"] != "allow" || taskPerm["jd-judge-a"] != "allow" {
		t.Fatalf("unexpected orchestrator task permission: %#v", taskPerm)
	}
	if _, ok := taskPerm["sdd-orchestrator"]; ok {
		t.Fatal("sdd-orchestrator must not grant task permission to itself")
	}
	applyAgent := agents["sdd-apply"].(map[string]any)
	if applyAgent["mode"] != "subagent" || applyAgent["hidden"] != true {
		t.Fatalf("sdd-apply should be hidden subagent, got %#v", applyAgent)
	}
	if applyAgent["model"] != "openai/gpt-5.1-codex-max" || applyAgent["variant"] != "medium" {
		t.Fatalf("sdd-apply model/variant not resolved from runtime config: %#v", applyAgent)
	}
	if !strings.Contains(applyAgent["prompt"].(string), ".jarvis/skills/sdd-apply/SKILL.md") {
		t.Fatalf("sdd-apply prompt should point at Jarvis skill path, got %q", applyAgent["prompt"])
	}
	permission := settings["permission"].(map[string]any)
	bash := permission["bash"].(map[string]any)
	if bash["*"] != "allow" || bash["git push --force*"] != "ask" || bash["git reset --hard*"] != "ask" {
		t.Fatalf("unexpected bash permissions: %#v", bash)
	}
	read := permission["read"].(map[string]any)
	if read["*"] != "allow" || read["**/.env*"] != "deny" || read["**/*secret*"] != "deny" {
		t.Fatalf("unexpected read permissions: %#v", read)
	}

	missingSchemaHome := t.TempDir()
	missingSchemaAgent := &OpenCodeAgent{home: missingSchemaHome, templatesFS: testTemplatesFS}
	missingSchemaPath := filepath.Join(missingSchemaHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(missingSchemaPath), 0755); err != nil {
		t.Fatalf("create opencode dir without schema: %v", err)
	}
	if err := os.WriteFile(missingSchemaPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write opencode.json without schema: %v", err)
	}
	if err := missingSchemaAgent.MergeGeneratedConfig(defaultRuntimeConfig()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	missingSchemaSettings := readJSONFile(t, missingSchemaPath)
	if missingSchemaSettings["$schema"] != "https://opencode.ai/config.json" {
		t.Fatalf("$schema = %v, want opencode schema", missingSchemaSettings["$schema"])
	}
}

func TestOpenCodeAgent_MergeGeneratedConfig_DefaultModelsAreProviderQualified(t *testing.T) {
	tmpHome := t.TempDir()
	a := &OpenCodeAgent{home: tmpHome, templatesFS: testTemplatesFS}
	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	if err := a.MergeGeneratedConfig(defaultRuntimeConfig()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}

	settings := readJSONFile(t, settingsPath)
	agents := settings["agent"].(map[string]any)
	for name, raw := range agents {
		agent := raw.(map[string]any)
		model, ok := agent["model"].(string)
		if !ok || !strings.Contains(model, "/") {
			t.Fatalf("agent.%s.model = %#v, want provider-qualified provider/model", name, agent["model"])
		}
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSON file: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal JSON file: %v", err)
	}
	return out
}

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
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

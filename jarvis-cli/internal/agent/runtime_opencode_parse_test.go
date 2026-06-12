package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOpenCodeConfig_ValidFullJSON(t *testing.T) {
	// A realistic opencode.json with all observed fields populated.
	content := `{
  "share": "disabled",
  "default_agent": "sdd-orchestrator",
  "permission": {
    "bash": {"*": "allow", "git push --force*": "ask"},
    "read": {"*": "allow", ".env": "deny", "secrets": "deny", "tokens": "deny", "credentials": "deny"}
  },
  "agent": {
    "sdd-orchestrator": {
      "mode": "primary",
      "model": "opus",
      "prompt": "{file:./sdd-orchestrator.md}",
      "permission": {"task": {"*": "deny", "sdd-explore": "allow", "sdd-apply": "allow"}}
    },
    "sdd-explore": {"mode": "subagent", "hidden": true, "model": "sonnet", "prompt": "explore prompt"},
    "sdd-apply":   {"mode": "subagent", "hidden": true, "model": "sonnet", "prompt": "apply prompt"}
  },
  "mcp": {
    "hive":     {"type": "local",  "command": ["hive-daemon"]},
    "context7": {"type": "remote", "url": "https://mcp.context7.com/mcp"}
  }
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if !got.StructureValid {
		t.Fatal("StructureValid must be true for valid JSON")
	}
	if got.ShareMode != "disabled" {
		t.Errorf("ShareMode = %q, want %q", got.ShareMode, "disabled")
	}
	if got.DefaultAgent != "sdd-orchestrator" {
		t.Errorf("DefaultAgent = %q, want %q", got.DefaultAgent, "sdd-orchestrator")
	}
	if got.OrchestratorMode != "primary" {
		t.Errorf("OrchestratorMode = %q, want %q", got.OrchestratorMode, "primary")
	}
	if got.OrchestratorModel != "opus" {
		t.Errorf("OrchestratorModel = %q, want %q", got.OrchestratorModel, "opus")
	}
	if got.OrchestratorPrompt != "{file:./sdd-orchestrator.md}" {
		t.Errorf("OrchestratorPrompt = %q, want %q", got.OrchestratorPrompt, "{file:./sdd-orchestrator.md}")
	}
	if len(got.AgentNames) != 3 {
		t.Errorf("len(AgentNames) = %d, want 3, got %v", len(got.AgentNames), got.AgentNames)
	}
	if len(got.HiddenSubagents) != 2 {
		t.Errorf("len(HiddenSubagents) = %d, want 2, got %v", len(got.HiddenSubagents), got.HiddenSubagents)
	}
	if !got.TaskWildcardDeny {
		t.Error("TaskWildcardDeny must be true")
	}
	if len(got.TaskAllows) != 2 {
		t.Errorf("len(TaskAllows) = %d, want 2, got %v", len(got.TaskAllows), got.TaskAllows)
	}
	if !got.BashWildcardAllow {
		t.Error("BashWildcardAllow must be true when bash[\"*\"] == \"allow\"")
	}
	if !got.ReadSecretDenies {
		t.Error("ReadSecretDenies must be true when .env/secrets/tokens/credentials are denied")
	}
	if !got.MCPHivePresent {
		t.Error("MCPHivePresent must be true for local hive with non-empty command")
	}
	if !got.MCPContext7Present {
		t.Error("MCPContext7Present must be true for remote context7")
	}
}

func TestParseOpenCodeConfig_MissingFile(t *testing.T) {
	got := parseOpenCodeConfig(filepath.Join(t.TempDir(), "nonexistent.json"))

	if got.StructureValid {
		t.Error("StructureValid must be false for missing file")
	}
	// All other fields should be zero.
	if got.ShareMode != "" {
		t.Errorf("ShareMode = %q, want empty", got.ShareMode)
	}
}

func TestParseOpenCodeConfig_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if got.StructureValid {
		t.Error("StructureValid must be false for empty file")
	}
}

func TestParseOpenCodeConfig_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{ not valid json`), 0644); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if got.StructureValid {
		t.Error("StructureValid must be false for malformed JSON")
	}
}

func TestParseOpenCodeConfig_MissingOptionalFields(t *testing.T) {
	// Valid JSON but missing optional fields — StructureValid must be true,
	// missing fields at zero values.
	content := `{"share": "disabled"}`
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if !got.StructureValid {
		t.Error("StructureValid must be true for valid JSON even with missing optional fields")
	}
	if got.ShareMode != "disabled" {
		t.Errorf("ShareMode = %q, want %q", got.ShareMode, "disabled")
	}
	if got.DefaultAgent != "" {
		t.Errorf("DefaultAgent = %q, want empty (missing in JSON)", got.DefaultAgent)
	}
	if len(got.AgentNames) != 0 {
		t.Errorf("AgentNames = %v, want empty (no agents in JSON)", got.AgentNames)
	}
	if got.MCPHivePresent {
		t.Error("MCPHivePresent must be false when mcp is absent")
	}
}

func TestParseOpenCodeConfig_HiveMCPWithEmptyCommand(t *testing.T) {
	// Hive MCP entry present but command is empty — MCPHivePresent must be false.
	content := `{
  "mcp": {
    "hive": {"type": "local", "command": []}
  }
}`
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if !got.StructureValid {
		t.Error("StructureValid must be true for valid JSON")
	}
	if got.MCPHivePresent {
		t.Error("MCPHivePresent must be false when command array is empty")
	}
}

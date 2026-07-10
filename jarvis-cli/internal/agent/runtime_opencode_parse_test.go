package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
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

	if !got.ParseSucceeded {
		t.Fatal("ParseSucceeded must be true for valid JSON")
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

	if got.ParseSucceeded {
		t.Error("ParseSucceeded must be false for missing file")
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

	if got.ParseSucceeded {
		t.Error("ParseSucceeded must be false for empty file")
	}
}

func TestParseOpenCodeConfig_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{ not valid json`), 0644); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if got.ParseSucceeded {
		t.Error("ParseSucceeded must be false for malformed JSON")
	}
}

func TestParseOpenCodeConfig_MissingOptionalFields(t *testing.T) {
	// Valid JSON but missing optional fields — ParseSucceeded must be true,
	// missing fields at zero values.
	content := `{"share": "disabled"}`
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if !got.ParseSucceeded {
		t.Error("ParseSucceeded must be true for valid JSON even with missing optional fields")
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

func TestParseOpenCodeConfig_HiveMCPWithUnusableCommandArray(t *testing.T) {
	tests := []struct {
		name        string
		commandJSON string
	}{
		{name: "empty array", commandJSON: `[]`},
		{name: "empty command string in array", commandJSON: `[""]`},
		{name: "whitespace-only command string in array", commandJSON: `["   "]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `{"mcp": {"hive": {"type": "local", "command": ` + tt.commandJSON + `}}}`
			path := filepath.Join(t.TempDir(), "opencode.json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			got := parseOpenCodeConfig(path)

			if !got.ParseSucceeded {
				t.Error("ParseSucceeded must be true for valid JSON")
			}
			if got.MCPHivePresent {
				t.Errorf("MCPHivePresent must be false for unusable command array %s", tt.commandJSON)
			}
		})
	}
}

func TestParseOpenCodeConfig_RetainsSDDSubagentHiveGrantEvidence(t *testing.T) {
	content := `{
  "agent": {
    "sdd-apply": {
      "mode": "subagent",
      "hidden": true,
      "permission": {
        "task": "deny",
        "edit": "allow",
        "hive_mem_search": "allow",
        "hive_mem_get_observation": "allow",
        "hive_mem_save": "allow"
      }
    },
    "sdd-verify": {
      "mode": "subagent",
      "hidden": true,
      "permission": {
        "task": "deny",
        "edit": "deny",
        "hive_mem_*": "allow"
      }
    },
    "sdd-onboard": {
      "mode": "subagent",
      "hidden": true,
      "permission": {
        "task": "deny",
        "edit": "deny",
        "hive_*": "allow"
      }
    },
    "sdd-design": {
      "mode": "subagent",
      "hidden": true,
      "permission": {
        "task": "deny",
        "edit": "deny",
        "hive_mem_*": "allow",
        "hive_mem_save": "deny",
        "hive_mem_context": "ask"
      }
    }
  }
}`
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	got := parseOpenCodeConfig(path)

	if !got.ParseSucceeded {
		t.Fatal("ParseSucceeded must be true for valid JSON")
	}
	assertHiveGrantEvidence(t, got.SDDSubagentHiveGrantEvidence["sdd-apply"], "hive_mem_search", "hive_mem_get_observation", "hive_mem_save")
	assertHiveGrantEvidence(t, got.SDDSubagentHiveGrantEvidence["sdd-verify"], "hive_mem_*")
	assertHiveGrantEvidence(t, got.SDDSubagentHiveGrantEvidence["sdd-onboard"], "hive_*")
	assertHivePermissionEvidence(t, got.SDDSubagentHiveGrantEvidence["sdd-design"], "hive_mem_*", "allow")
	assertHivePermissionEvidence(t, got.SDDSubagentHiveGrantEvidence["sdd-design"], "hive_mem_save", "deny")
	assertHivePermissionEvidence(t, got.SDDSubagentHiveGrantEvidence["sdd-design"], "hive_mem_context", "ask")
}

func TestParseOpenCodeConfig_PreservesHiveGrantOrderForLastMatchingVerification(t *testing.T) {
	tests := []struct {
		name        string
		permission  string
		wantStatus  sddruntime.IntegrityStatus
		wantMessage string
	}{
		{
			name: "wildcard deny before exact Hive allows passes",
			permission: `
        "hive_mem_*": "deny",
        "hive_mem_search": "allow",
        "hive_mem_get_observation": "allow",
        "hive_mem_save": "allow",
        "hive_mem_context": "allow",
        "hive_mem_session_summary": "allow"`,
			wantStatus: sddruntime.StatusPass,
		},
		{
			name: "exact Hive allows before wildcard ask fails",
			permission: `
        "hive_mem_search": "allow",
        "hive_mem_get_observation": "allow",
        "hive_mem_save": "allow",
        "hive_mem_context": "allow",
        "hive_mem_session_summary": "allow",
        "hive_mem_*": "ask"`,
			wantStatus:  sddruntime.StatusFail,
			wantMessage: "manually adjust",
		},
		{
			name: "exact Hive allows before wildcard deny fails",
			permission: `
        "hive_mem_search": "allow",
        "hive_mem_get_observation": "allow",
        "hive_mem_save": "allow",
        "hive_mem_context": "allow",
        "hive_mem_session_summary": "allow",
        "hive_mem_*": "deny"`,
			wantStatus:  sddruntime.StatusFail,
			wantMessage: "manually adjust",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeOpenCodeConfigWithApplyPermission(t, tt.permission)
			parsed := parseOpenCodeConfig(path)
			if !parsed.ParseSucceeded {
				t.Fatal("ParseSucceeded must be true for valid JSON")
			}

			report := sddruntime.Verify("opencode", sddruntime.ObservedRuntime{
				StoreMode:     "hive",
				StoreReadFrom: []string{"hive"},
				StoreWriteTo:  []string{"hive"},
				OpenCode:      parsed,
			})
			check := findOpenCodeCheck(report.Checks, "invariant.opencode.sdd_hive_grants")
			if check == nil {
				t.Fatal("expected invariant.opencode.sdd_hive_grants check")
			}
			if check.Status != tt.wantStatus {
				t.Fatalf("expected %q, got %q (observed=%q message=%q)", tt.wantStatus, check.Status, check.Observed, check.Message)
			}
			if tt.wantMessage != "" && !strings.Contains(check.Message, tt.wantMessage) {
				t.Fatalf("expected message to contain %q, got %q", tt.wantMessage, check.Message)
			}
		})
	}
}

func TestOpenCodeMergeGeneratedConfig_PreservesOrderSensitiveHivePermissionGuardrail(t *testing.T) {
	tmpHome := t.TempDir()
	a := &OpenCodeAgent{home: tmpHome, templatesFS: testTemplatesFS}
	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	existing := `{
  "agent": {
    "sdd-apply": {
      "permission": {
        "task": "deny",
        "edit": "allow",
        "hive_mem_search": "allow",
        "hive_mem_get_observation": "allow",
        "hive_mem_save": "allow",
        "hive_mem_context": "allow",
        "hive_mem_session_summary": "allow",
        "hive_mem_*": "deny"
      }
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	if err := a.MergeGeneratedConfig(defaultRuntimeConfig()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	if err := a.MergeGeneratedConfig(defaultRuntimeConfig()); err != nil {
		t.Fatalf("MergeGeneratedConfig rerun: %v", err)
	}

	parsed := parseOpenCodeConfig(settingsPath)
	if !parsed.ParseSucceeded {
		t.Fatal("ParseSucceeded must be true for merged JSON")
	}
	evidence := parsed.SDDSubagentHiveGrantEvidence["sdd-apply"]
	assertHivePermissionEvidenceOrder(t, evidence, []string{
		"hive_mem_search",
		"hive_mem_get_observation",
		"hive_mem_save",
		"hive_mem_context",
		"hive_mem_session_summary",
		"hive_mem_*",
	})

	report := sddruntime.Verify("opencode", sddruntime.ObservedRuntime{
		StoreMode:     "hive",
		StoreReadFrom: []string{"hive"},
		StoreWriteTo:  []string{"hive"},
		OpenCode:      parsed,
	})
	check := findOpenCodeCheck(report.Checks, "invariant.opencode.sdd_hive_grants")
	if check == nil {
		t.Fatal("expected invariant.opencode.sdd_hive_grants check")
	}
	if check.Status != sddruntime.StatusFail || !strings.Contains(check.Message, "manually adjust") {
		t.Fatalf("expected trailing guardrail to remain effective, got status=%q message=%q", check.Status, check.Message)
	}
}

func writeOpenCodeConfigWithApplyPermission(t *testing.T, applyPermission string) string {
	t.Helper()

	var agents strings.Builder
	agents.WriteString(`
    "sdd-orchestrator": {
      "mode": "primary",
      "model": "opus",
      "prompt": "{file:./sdd-orchestrator.md}",
      "permission": {"task": {"*": "deny", "sdd-init": "allow", "sdd-explore": "allow", "sdd-propose": "allow", "sdd-spec": "allow", "sdd-design": "allow", "sdd-tasks": "allow", "sdd-apply": "allow", "sdd-verify": "allow", "sdd-archive": "allow", "sdd-onboard": "allow"}}
    }`)
	for _, name := range openCodeSDDSubagents() {
		agents.WriteString(",\n")
		agents.WriteString(`    "` + name + `": {"mode": "subagent", "hidden": true, "permission": {"task": "deny", `)
		if name == "sdd-apply" {
			agents.WriteString(applyPermission)
		} else {
			agents.WriteString(`"hive_mem_*": "allow"`)
		}
		agents.WriteString(`}}`)
	}

	content := `{
  "share": "disabled",
  "default_agent": "sdd-orchestrator",
  "permission": {
    "bash": {"*": "allow"},
    "read": {".env": "deny"}
  },
  "agent": {` + agents.String() + `
  },
  "mcp": {
    "hive": {"type": "local", "command": ["hive-daemon"]},
    "context7": {"type": "remote", "url": "https://mcp.context7.com/mcp"}
  }
}`

	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}
	return path
}

func findOpenCodeCheck(checks []sddruntime.CheckResult, key string) *sddruntime.CheckResult {
	for i := range checks {
		if checks[i].Key == key {
			return &checks[i]
		}
	}
	return nil
}

func assertHiveGrantEvidence(t *testing.T, got []sddruntime.OpenCodePermissionEvidence, want ...string) {
	t.Helper()
	for _, expected := range want {
		assertHivePermissionEvidence(t, got, expected, "allow")
	}
}

func assertHivePermissionEvidence(t *testing.T, got []sddruntime.OpenCodePermissionEvidence, wantKey, wantAction string) {
	t.Helper()
	for _, actual := range got {
		if actual.Key == wantKey && actual.Action == wantAction {
			return
		}
	}
	t.Fatalf("missing hive permission evidence %q=%q in %v", wantKey, wantAction, got)
}

func assertHivePermissionEvidenceOrder(t *testing.T, got []sddruntime.OpenCodePermissionEvidence, wantKeys []string) {
	t.Helper()
	if len(got) < len(wantKeys) {
		t.Fatalf("evidence length = %d, want at least %d: %v", len(got), len(wantKeys), got)
	}
	for i, wantKey := range wantKeys {
		if got[i].Key != wantKey {
			t.Fatalf("evidence[%d].Key = %q, want %q; evidence=%v", i, got[i].Key, wantKey, got)
		}
	}
}

func TestIsMCPCommandNonEmpty(t *testing.T) {
	tests := []struct {
		name        string
		commandJSON string // value for "command" key inside the hive MCP entry
		wantPresent bool
	}{
		{
			name:        "null command",
			commandJSON: `null`,
			wantPresent: false,
		},
		{
			name:        "empty string command",
			commandJSON: `""`,
			wantPresent: false,
		},
		{
			name:        "whitespace-only string command",
			commandJSON: `"   "`,
			wantPresent: false,
		},
		{
			name:        "non-empty string command",
			commandJSON: `"hive-daemon"`,
			wantPresent: true,
		},
		{
			name:        "empty array command",
			commandJSON: `[]`,
			wantPresent: false,
		},
		{
			name:        "non-empty array command",
			commandJSON: `["hive-daemon"]`,
			wantPresent: true,
		},
		{
			name:        "empty string array command",
			commandJSON: `[""]`,
			wantPresent: false,
		},
		{
			name:        "whitespace-only string array command",
			commandJSON: `["   "]`,
			wantPresent: false,
		},
		{
			name:        "object format command",
			commandJSON: `{"bin": "hive-daemon"}`,
			wantPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `{"mcp": {"hive": {"type": "local", "command": ` + tt.commandJSON + `}}}`
			path := filepath.Join(t.TempDir(), "opencode.json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			got := parseOpenCodeConfig(path)

			if got.MCPHivePresent != tt.wantPresent {
				t.Errorf("MCPHivePresent = %v, want %v (command: %s)", got.MCPHivePresent, tt.wantPresent, tt.commandJSON)
			}
		})
	}
}

func TestParseOpenCodeConfig_HiveMCPEnabledFlagControlsPresence(t *testing.T) {
	tests := []struct {
		name        string
		enabledJSON string
		wantPresent bool
	}{
		{name: "enabled omitted", enabledJSON: ``, wantPresent: true},
		{name: "enabled true", enabledJSON: `, "enabled": true`, wantPresent: true},
		{name: "enabled false", enabledJSON: `, "enabled": false`, wantPresent: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `{"mcp": {"hive": {"type": "local", "command": ["hive-daemon"]` + tt.enabledJSON + `}}}`
			path := filepath.Join(t.TempDir(), "opencode.json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			got := parseOpenCodeConfig(path)

			if got.MCPHivePresent != tt.wantPresent {
				t.Fatalf("MCPHivePresent = %v, want %v", got.MCPHivePresent, tt.wantPresent)
			}
		})
	}
}

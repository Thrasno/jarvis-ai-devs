package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionExecutorMigratesPreReleaseOpenCodeConfigs(t *testing.T) {
	tests := []struct {
		name     string
		selected []string
		config   string
		sidecar  string
		preserve string
	}{
		{name: "Claude and OpenCode without sidecar", selected: []string{"claude", "opencode"}, config: `{"theme":"dark","mcp":{"hive":{"old":true},"context7":{"old":true},"team":{"keep":true}}}`, preserve: `"theme":"dark"`},
		{name: "OpenCode only with stale v1", selected: []string{"opencode"}, config: `{"plugin":["keep"],"mcp":{"hive":{"old":true}}}`, sidecar: `{"version":"v1","digest":"stale"}`, preserve: `"plugin":["keep"]`},
		{name: "malformed v1 sidecar", selected: []string{"opencode"}, config: `{"mcp":{"context7":{"old":true},"other":{"keep":true}}}`, sidecar: `{broken`, preserve: `"other":{"keep":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, filepath.Join(root, openCodeGlobalConfigLocation), tt.config)
			if tt.sidecar != "" {
				writeFixture(t, filepath.Join(root, openCodeProvenanceManifestLocation), tt.sidecar)
			}
			native := &compositionNativeMCP{}
			executor := NewProductionExecutorWithNative(native)
			input := wiringWizardInput(root, tt.selected)
			if _, err := executor.ExecuteWizard(input); err != nil {
				t.Fatalf("ExecuteWizard() error = %v", err)
			}
			first, err := os.ReadFile(filepath.Join(root, openCodeGlobalConfigLocation))
			if err != nil {
				t.Fatal(err)
			}
			assertOpenCodePreservedAndManaged(t, first)
			if !strings.Contains(string(first), tt.preserve) {
				t.Fatalf("unrelated config lost: %s", first)
			}
			if _, err := executor.ExecuteWizard(input); err != nil {
				t.Fatalf("second ExecuteWizard() error = %v", err)
			}
			second, _ := os.ReadFile(filepath.Join(root, openCodeGlobalConfigLocation))
			if string(first) != string(second) {
				t.Fatalf("rerun changed config:\n%s\n%s", first, second)
			}
			sidecar, _ := os.ReadFile(filepath.Join(root, openCodeProvenanceManifestLocation))
			if !strings.Contains(string(sidecar), `"version":"v2"`) {
				t.Fatalf("sidecar = %s, want v2", sidecar)
			}
		})
	}
}

func TestProductionExecutorFailsClosedOnMalformedOpenCodeConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, openCodeGlobalConfigLocation)
	writeFixture(t, path, `{broken`)
	_, err := NewProductionExecutorWithNative(&compositionNativeMCP{}).ExecuteWizard(wiringWizardInput(root, []string{"opencode"}))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("ExecuteWizard() error = %v, want scoped malformed error", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != `{broken` {
		t.Fatalf("malformed config mutated to %q", got)
	}
}

func wiringWizardInput(root string, selected []string) WizardReconcileInput {
	input := WizardReconcileInput{SelectedAgents: selected, Root: root, EvidencePath: filepath.Join(root, ".jarvis/recovery.json"), OpenCodeMCPs: OpenCodeManagedMCPs{
		"hive": `{"type":"local","command":["hive-daemon"]}`, "context7": `{"type":"remote","url":"https://mcp.context7.com/mcp","enabled":true}`,
	}}
	if selectedAgent(selected, "claude") {
		input.ClaudeHive = nativeMCPDefinition("hive", "hive")
		input.ClaudeContext7 = nativeMCPDefinition("context7", "context7")
	}
	return input
}

func writeFixture(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertOpenCodePreservedAndManaged(t *testing.T, raw []byte) {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(doc["mcp"], &mcp); err != nil {
		t.Fatal(err)
	}
	if len(mcp["hive"]) == 0 || len(mcp["context7"]) == 0 {
		t.Fatalf("managed MCPs missing: %s", raw)
	}
	if strings.Contains(string(raw), `"old":true`) {
		t.Fatalf("managed stale values remain: %s", raw)
	}
	if strings.Contains(string(raw), `"team"`) && len(mcp["team"]) == 0 {
		t.Fatalf("unrelated MCP lost: %s", raw)
	}
}

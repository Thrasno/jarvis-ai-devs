package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

func TestOpenCodeGlobalAdapterRenderPreservesUnrelatedKeysWithoutAuthorizingMutation(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".config", "opencode", "opencode.json")
	writeOpenCodeFixture(t, configPath, `{"theme":"night","provider":{"custom":{"api":"keep"}},"mcp":{"other":{"type":"remote","url":"https://example.test"}}}`)

	output, err := NewOpenCodeGlobalAdapter(osFS{}, root).Render(OpenCodeManagedMCPs{
		"hive":     `{"type":"local","command":["hive-daemon"]}`,
		"context7": `{"type":"remote","url":"https://context7.test"}`,
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(string(output.Bytes), `"theme":"night"`) ||
		!strings.Contains(string(output.Bytes), `"custom":{"api":"keep"}`) ||
		!strings.Contains(string(output.Bytes), `"other":{"type":"remote","url":"https://example.test"}`) ||
		!strings.Contains(string(output.Bytes), `"hive":{"type":"local","command":["hive-daemon"]}`) {
		t.Fatalf("Render() bytes = %s, want unrelated values and managed MCPs", output.Bytes)
	}

	_, err = BuildWizardReconcileRequest(WizardReconcileInput{
		SelectedAgents: []string{"opencode"}, Root: root,
		EvidencePath: filepath.Join(root, "state", "recovery.json"), RenderedOutputs: []RenderedManagedOutput{output},
	})
	if err == nil {
		t.Fatal("BuildWizardReconcileRequest() error = nil, want unprovenanced existing config rejection")
	}
}

func TestOpenCodeGlobalAdapterProvenanceBackedOutputIsAcceptedByWizardBuilder(t *testing.T) {
	root := t.TempDir()
	content := []byte(`{"theme":"night","mcp":{"hive":{"type":"local"}}}`)
	writeOpenCodeFixture(t, filepath.Join(root, ".config", "opencode", "opencode.json"), string(content))
	provenance := &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest(content)}
	output, err := NewOpenCodeGlobalAdapter(osFS{}, root).RenderWithProvenance(OpenCodeManagedMCPs{"hive": `{"type":"local"}`}, provenance)
	if err != nil {
		t.Fatalf("RenderWithProvenance() error = %v", err)
	}
	request, err := BuildWizardReconcileRequest(WizardReconcileInput{SelectedAgents: []string{"opencode"}, Root: root, EvidencePath: filepath.Join(root, "state", "recovery.json"), RenderedOutputs: []RenderedManagedOutput{output}})
	if err != nil {
		t.Fatalf("BuildWizardReconcileRequest() error = %v", err)
	}
	if request.StorePlan.Blocked() || len(request.StorePlan.Operations) != 0 {
		t.Fatalf("StorePlan = %#v, want accepted provenance-backed stable output", request.StorePlan)
	}
}

func TestOpenCodeGlobalAdapterRenderWithProvenanceReplacesStaleManagedMCPAndPreservesUnrelatedKeys(t *testing.T) {
	root := t.TempDir()
	content := []byte(`{"theme":"night","provider":{"custom":{"api":"keep"}},"mcp":{"hive":{"type":"local","command":["stale-daemon"]},"other":{"type":"remote","url":"https://example.test"}}}`)
	writeOpenCodeFixture(t, filepath.Join(root, ".config", "opencode", "opencode.json"), string(content))
	provenance := &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest(content)}

	output, err := NewOpenCodeGlobalAdapter(osFS{}, root).RenderWithProvenance(OpenCodeManagedMCPs{
		"hive": `{"type":"local","command":["current-daemon"]}`,
	}, provenance)
	if err != nil {
		t.Fatalf("RenderWithProvenance() error = %v", err)
	}
	if output.Existing == nil || output.Existing.Provenance != provenance || !jsonEqual(output.Existing.Bytes, content) {
		t.Fatalf("RenderWithProvenance() existing = %#v, want exact proven stale artifact", output.Existing)
	}
	if !strings.Contains(string(output.Bytes), `"command":["current-daemon"]`) ||
		!strings.Contains(string(output.Bytes), `"theme":"night"`) ||
		!strings.Contains(string(output.Bytes), `"custom":{"api":"keep"}`) ||
		!strings.Contains(string(output.Bytes), `"other":{"type":"remote","url":"https://example.test"}`) {
		t.Fatalf("RenderWithProvenance() bytes = %s, want current managed MCP and preserved unrelated keys", output.Bytes)
	}
	request, err := BuildWizardReconcileRequest(WizardReconcileInput{
		SelectedAgents: []string{"opencode"}, Root: root,
		EvidencePath: filepath.Join(root, "state", "recovery.json"), RenderedOutputs: []RenderedManagedOutput{output},
	})
	if err != nil {
		t.Fatalf("BuildWizardReconcileRequest() error = %v", err)
	}
	if request.StorePlan.Blocked() || len(request.StorePlan.Operations) != 1 {
		t.Fatalf("StorePlan = %#v, want one authorized replacement", request.StorePlan)
	}
	operation := request.StorePlan.Operations[0]
	if operation.Provenance.ManagedIdentity != openCodeGlobalConfigIdentity ||
		operation.Provenance.Location != openCodeGlobalConfigLocation ||
		operation.Provenance.ManifestDigest != managedOutputDigest(output.Bytes) {
		t.Fatalf("replacement provenance = %#v, want current rendered artifact binding", operation.Provenance)
	}
}

func TestOpenCodeGlobalAdapterRenderWithProvenanceRemovesOnlyOmittedProvenManagedMCPs(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".config", "opencode", "opencode.json")
	content := []byte(`{"theme":"night","mcp":{"hive":{"type":"local","command":["stale-daemon"]},"context7":{"type":"remote","url":"https://stale-context7.test"},"user":{"type":"remote","url":"https://user.test"}}}`)
	writeOpenCodeFixture(t, configPath, string(content))
	provenance := &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest(content)}
	adapter := NewOpenCodeGlobalAdapter(osFS{}, root)

	inventory, err := adapter.InventoryWithProvenance(provenance)
	if err != nil {
		t.Fatalf("InventoryWithProvenance() error = %v", err)
	}
	if len(inventory) != 1 || inventory[0].Provenance != provenance || !jsonEqual(inventory[0].Bytes, content) {
		t.Fatalf("InventoryWithProvenance() = %#v, want exactly the proven prior global artifact", inventory)
	}

	desired := OpenCodeManagedMCPs{"context7": `{"type":"remote","url":"https://current-context7.test"}`}
	output, err := adapter.RenderWithProvenance(desired, provenance)
	if err != nil {
		t.Fatalf("RenderWithProvenance() error = %v", err)
	}
	var rendered struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	if err := json.Unmarshal(output.Bytes, &rendered); err != nil {
		t.Fatalf("Unmarshal rendered output: %v", err)
	}
	if _, found := rendered.MCP["hive"]; found {
		t.Fatalf("rendered MCPs = %s, want omitted proven hive removed", output.Bytes)
	}
	if !jsonEqual(rendered.MCP["context7"], []byte(desired["context7"])) ||
		!jsonEqual(rendered.MCP["user"], []byte(`{"type":"remote","url":"https://user.test"}`)) {
		t.Fatalf("rendered MCPs = %s, want desired context7 and preserved user MCP", output.Bytes)
	}

	request, err := BuildWizardReconcileRequest(WizardReconcileInput{SelectedAgents: []string{"opencode"}, Root: root, EvidencePath: filepath.Join(root, "state", "recovery.json"), RenderedOutputs: []RenderedManagedOutput{output}})
	if err != nil {
		t.Fatalf("BuildWizardReconcileRequest() error = %v", err)
	}
	if request.StorePlan.Blocked() || len(request.StorePlan.Operations) != 1 || request.StorePlan.Operations[0].Provenance.ManifestDigest != managedOutputDigest(output.Bytes) {
		t.Fatalf("StorePlan = %#v, want one desired-byte-provenanced replacement", request.StorePlan)
	}

	writeOpenCodeFixture(t, configPath, string(output.Bytes))
	runAgain, err := adapter.RenderWithProvenance(desired, &request.StorePlan.Operations[0].Provenance)
	if err != nil {
		t.Fatalf("second RenderWithProvenance() error = %v", err)
	}
	if !jsonEqual(runAgain.Bytes, output.Bytes) {
		t.Fatalf("second render = %s, want converged bytes %s", runAgain.Bytes, output.Bytes)
	}
	secondRequest, err := BuildWizardReconcileRequest(WizardReconcileInput{SelectedAgents: []string{"opencode"}, Root: root, EvidencePath: filepath.Join(root, "state", "recovery.json"), RenderedOutputs: []RenderedManagedOutput{runAgain}})
	if err != nil {
		t.Fatalf("second BuildWizardReconcileRequest() error = %v", err)
	}
	if secondRequest.StorePlan.Blocked() || len(secondRequest.StorePlan.Operations) != 0 {
		t.Fatalf("second StorePlan = %#v, want converged no-op", secondRequest.StorePlan)
	}
}

func TestOpenCodeGlobalAdapterRenderWithProvenanceRejectsUnprovenReplacement(t *testing.T) {
	content := []byte(`{"mcp":{"hive":{"type":"local","command":["stale-daemon"]}}}`)
	tests := []struct {
		name       string
		provenance *reconcile.Provenance
		managed    OpenCodeManagedMCPs
	}{
		{name: "missing provenance"},
		{name: "mismatched digest", provenance: &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest([]byte("different"))}},
		{name: "mismatched location", provenance: &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: "other.json", ManifestDigest: managedOutputDigest(content)}},
		{name: "proven artifact lacks desired managed name", provenance: &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest(content)}, managed: OpenCodeManagedMCPs{"context7": `{"type":"remote","url":"https://context7.test"}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenCodeFixture(t, filepath.Join(root, ".config", "opencode", "opencode.json"), string(content))
			managed := tt.managed
			if managed == nil {
				managed = OpenCodeManagedMCPs{"hive": `{"type":"local","command":["current-daemon"]}`}
			}
			output, err := NewOpenCodeGlobalAdapter(osFS{}, root).RenderWithProvenance(managed, tt.provenance)
			if err == nil {
				t.Fatal("RenderWithProvenance() error = nil, want unproven replacement rejection")
			}
			if output.Bytes != nil || strings.Contains(err.Error(), root) {
				t.Fatalf("RenderWithProvenance() = %#v, %v; want no output and sanitized error", output, err)
			}
		})
	}
}

func TestOpenCodeGlobalAdapterInventoryIsAllowlistOnlyAndRejectsUnsafeInputs(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".config", "opencode", "opencode.json")
	writeOpenCodeFixture(t, configPath, `{"mcp":{"other":{"token":"preserve"}}}`)
	adapter := NewOpenCodeGlobalAdapter(osFS{}, root)

	inventory, err := adapter.Inventory()
	if err != nil {
		t.Fatalf("Inventory() error = %v", err)
	}
	if len(inventory) != 0 {
		t.Fatalf("Inventory() = %#v, want no non-Jarvis entries", inventory)
	}
	if _, err := adapter.Render(OpenCodeManagedMCPs{"unmanaged": `{}`}); err == nil {
		t.Fatal("Render() error = nil, want non-allowlisted managed name rejection")
	}
}

func TestOpenCodeGlobalAdapterRejectsAmbiguousOrMalformedConfigurationBeforeOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		fs      OpenCodeConfigFS
	}{
		{name: "malformed JSON", content: `{`},
		{name: "managed entry lacks provenance", content: `{"mcp":{"hive":{"type":"local"}}}`},
		{name: "duplicate ownership evidence", content: `{"mcp":{"hive":{"type":"local"},"context7":{"type":"remote"}}}`},
		{name: "unreadable", fs: failingOpenCodeFS{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.fs == nil {
				writeOpenCodeFixture(t, filepath.Join(root, ".config", "opencode", "opencode.json"), tt.content)
				tt.fs = osFS{}
			}
			output, err := NewOpenCodeGlobalAdapter(tt.fs, root).Render(OpenCodeManagedMCPs{"hive": `{"type":"local"}`})
			if err == nil {
				t.Fatal("Render() error = nil, want safe rejection")
			}
			if output.Bytes != nil || strings.Contains(err.Error(), root) {
				t.Fatalf("Render() = %#v, %v; want no output and sanitized error", output, err)
			}
		})
	}
}

func TestOpenCodeGlobalAdapterAcceptsOnlyMatchingProvenance(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".config", "opencode", "opencode.json")
	content := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	writeOpenCodeFixture(t, configPath, string(content))
	adapter := NewOpenCodeGlobalAdapter(osFS{}, root)

	_, err := adapter.InventoryWithProvenance(&reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest([]byte("wrong"))})
	if err == nil {
		t.Fatal("InventoryWithProvenance() error = nil, want mismatched provenance rejection")
	}
	provenance := &reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, ManifestDigest: managedOutputDigest(content)}
	inventory, err := adapter.InventoryWithProvenance(provenance)
	if err != nil {
		t.Fatalf("InventoryWithProvenance() error = %v", err)
	}
	if len(inventory) != 1 || inventory[0].Provenance != provenance || inventory[0].Location != openCodeGlobalConfigLocation {
		t.Fatalf("InventoryWithProvenance() = %#v, want one proven global artifact", inventory)
	}
}

type failingOpenCodeFS struct{}

func (failingOpenCodeFS) ReadFile(string) ([]byte, error) {
	return nil, errors.New("read /private/token=secret")
}

func writeOpenCodeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

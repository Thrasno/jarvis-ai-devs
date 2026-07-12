package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

func TestBuildWizardReconcileRequestMapsGlobalOpenCodeAndClaudeDefinitions(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, "state", "recovery.json")
	request, err := BuildWizardReconcileRequest(WizardReconcileInput{
		SelectedAgents: []string{"opencode", "claude"},
		Root:           root,
		EvidencePath:   evidencePath,
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: openCodeGlobalConfigIdentity,
			Location: openCodeGlobalConfigLocation,
			Bytes:    []byte(`{"mcp":{"hive":{}}}`),
		}},
		ClaudeHive:     nativeMCPDefinition("hive", "hive-secret"),
		ClaudeContext7: nativeMCPDefinition("context7", "context7-secret"),
	})
	if err != nil {
		t.Fatalf("BuildWizardReconcileRequest() error = %v", err)
	}
	if request.EvidencePath != evidencePath {
		t.Fatalf("EvidencePath = %q, want %q", request.EvidencePath, evidencePath)
	}
	if len(request.StorePlan.Operations) != 1 || request.StorePlan.Operations[0].Location != openCodeGlobalConfigLocation {
		t.Fatalf("StorePlan = %#v, want global OpenCode config artifact", request.StorePlan)
	}
	if len(request.DesiredMCPs) != 2 || request.DesiredMCPs[0].Identity != "hive" || request.DesiredMCPs[1].Identity != "context7" {
		t.Fatalf("DesiredMCPs = %#v, want Hive and Context7 definitions", request.DesiredMCPs)
	}
	for _, definition := range request.DesiredMCPs {
		if definition.Scope != nativeMCPUserScope {
			t.Fatalf("definition %#v has scope %q, want %q", definition, definition.Scope, nativeMCPUserScope)
		}
	}
}

func TestProductionExecutorExecuteWizardSkipsNativeForZeroAgents(t *testing.T) {
	root := t.TempDir()
	calls := 0
	executor := ProductionExecutor{reconcile: func(request ReconcileInstallRequest, native NativeMCPReplacer) (ReconcileInstallResult, error) {
		calls++
		if native != nil || len(request.DesiredMCPs) != 0 {
			t.Fatalf("native reconciliation input = %#v, %#v; want no native work", native, request.DesiredMCPs)
		}
		return ReconcileInstallResult{Native: NativeMCPResult{Phase: NativeMCPSkipped}}, nil
	}}
	result, err := executor.ExecuteWizard(WizardReconcileInput{
		Root: root, EvidencePath: filepath.Join(root, "state", "recovery.json"),
	})
	if err != nil {
		t.Fatalf("ExecuteWizard() error = %v", err)
	}
	if calls != 1 || result.Native.Phase != NativeMCPSkipped {
		t.Fatalf("calls/result = %d/%#v, want one Store-only skip", calls, result.Native)
	}
}

func TestProductionExecutorExecuteWizardRejectsInvalidInputBeforeReconciliation(t *testing.T) {
	valid := func(root string) WizardReconcileInput {
		return WizardReconcileInput{
			SelectedAgents: []string{"claude", "opencode"},
			Root:           root,
			EvidencePath:   filepath.Join(root, "state", "recovery.json"),
			RenderedOutputs: []RenderedManagedOutput{{
				Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, Bytes: []byte(`{}`),
			}},
			ClaudeHive: nativeMCPDefinition("hive", "hive-secret"), ClaudeContext7: nativeMCPDefinition("context7", "context7-secret"),
		}
	}
	tests := []struct {
		name   string
		mutate func(*WizardReconcileInput)
	}{
		{name: "unknown selection", mutate: func(input *WizardReconcileInput) { input.SelectedAgents = []string{"other"} }},
		{name: "missing root", mutate: func(input *WizardReconcileInput) { input.Root = "" }},
		{name: "missing evidence", mutate: func(input *WizardReconcileInput) { input.EvidencePath = "" }},
		{name: "OpenCode artifact mismatch", mutate: func(input *WizardReconcileInput) { input.RenderedOutputs[0].Location = "project/opencode.json" }},
		{name: "unprovenanced OpenCode inventory", mutate: func(input *WizardReconcileInput) {
			input.RenderedOutputs[0].Existing = &reconcile.Artifact{Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, Bytes: []byte("user-owned")}
		}},
		{name: "missing Claude Context7", mutate: func(input *WizardReconcileInput) { input.ClaudeContext7 = NativeMCPDefinition{} }},
		{name: "Claude project scope", mutate: func(input *WizardReconcileInput) { input.ClaudeHive.Scope = "project" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			input := valid(root)
			tt.mutate(&input)
			calls := 0
			executor := ProductionExecutor{reconcile: func(ReconcileInstallRequest, NativeMCPReplacer) (ReconcileInstallResult, error) {
				calls++
				return ReconcileInstallResult{}, nil
			}}
			if _, err := executor.ExecuteWizard(input); err == nil {
				t.Fatal("ExecuteWizard() error = nil, want validation rejection")
			}
			if calls != 0 {
				t.Fatalf("reconciliation calls = %d, want 0 before mutation", calls)
			}
			if _, err := os.Stat(filepath.Join(root, "state")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovery evidence directory = %v, want no mutation", err)
			}
		})
	}
}

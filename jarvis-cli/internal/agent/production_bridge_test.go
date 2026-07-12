package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

func TestFileCompensationStoreRejectsPathsOutsideRenderedManagedOutputs(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{
		Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed"),
	}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}

	if err := store.Write("user/notes.md", []byte("replacement"), reconcile.Provenance{}); err == nil {
		t.Fatal("Write() error = nil, want rejection for user-owned path")
	}
	if _, err := store.Snapshot("../outside"); err == nil {
		t.Fatal("Snapshot() error = nil, want rejection for unsafe path")
	}
}

func TestFileCompensationStoreRejectsManagedPathSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "user-owned")
	if err := os.WriteFile(outside, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "claude"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "claude", "CLAUDE.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md"}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if err := store.Write("claude/CLAUDE.md", []byte("managed"), reconcile.Provenance{}); err == nil {
		t.Fatal("Write() error = nil, want symlink rejection")
	}
	got, readErr := os.ReadFile(outside)
	if readErr != nil || string(got) != "preserve" {
		t.Fatalf("outside file = %q, %v; want preserved bytes", got, readErr)
	}
}

func TestFileCompensationStoreRejectsSymlinkRootBeforeAnyMutation(t *testing.T) {
	outside := t.TempDir()
	rootLink := filepath.Join(t.TempDir(), "managed-root")
	if err := os.Symlink(outside, rootLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := NewFileCompensationStore(rootLink, []RenderedManagedOutput{{
		Identity: "jarvis-instructions", Location: "claude/CLAUDE.md",
	}})
	if err == nil {
		t.Fatal("NewFileCompensationStore() error = nil, want symlink-root rejection")
	}
	if strings.Contains(err.Error(), rootLink) || strings.Contains(err.Error(), outside) {
		t.Fatalf("NewFileCompensationStore() error leaked filesystem path: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "claude", "CLAUDE.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside managed artifact = %v, want no mutation", statErr)
	}
}

func TestFileCompensationStoreRechecksRootBeforeMutation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{
		Identity: "jarvis-instructions", Location: "claude/CLAUDE.md",
	}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err = store.Write("claude/CLAUDE.md", []byte("managed"), reconcile.Provenance{})
	if err == nil {
		t.Fatal("Write() error = nil, want swapped-root rejection")
	}
	if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), outside) {
		t.Fatalf("Write() error leaked filesystem path: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "claude", "CLAUDE.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside managed artifact = %v, want no mutation", statErr)
	}
}

func TestProductionExecutorPersistsRecoveryEvidenceForFreshReload(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, "state", "recovery.json")
	executor := ProductionExecutor{reconcile: func(request ReconcileInstallRequest, _ NativeMCPReplacer) (ReconcileInstallResult, error) {
		evidence, err := reconcile.NewFileRecoveryEvidenceStore(request.EvidencePath)
		if err != nil {
			return ReconcileInstallResult{}, err
		}
		if err := evidence.PersistDegradedRecovery(reconcile.RecoveryEvidence{
			FailedTarget: "claude/token=super-secret",
		}); err != nil {
			return ReconcileInstallResult{}, err
		}
		return ReconcileInstallResult{}, errors.New("Store persistence token=super-secret failed")
	}}

	_, err := executor.Execute(ProductionReconcileInput{
		Root: root, EvidencePath: evidencePath,
		RenderedOutputs: []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed")}},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want degraded reconciliation failure")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), evidencePath) {
		t.Fatalf("Execute() error leaked evidence failure detail: %v", err)
	}

	fresh, loadErr := reconcile.NewFileRecoveryEvidenceStore(evidencePath)
	if loadErr != nil {
		t.Fatalf("fresh NewFileRecoveryEvidenceStore() error = %v", loadErr)
	}
	got, loadErr := fresh.LoadDegradedRecovery()
	if loadErr != nil {
		t.Fatalf("fresh LoadDegradedRecovery() error = %v", loadErr)
	}
	if got.FailedTarget != "claude/<redacted>" || got.RecoveryAction == "" {
		t.Fatalf("fresh evidence = %#v, want sanitized durable recovery evidence", got)
	}
}

func TestProductionExecutorSanitizesFilesystemAndNativeAdapterFailures(t *testing.T) {
	tests := []struct {
		name      string
		executor  ProductionExecutor
		input     ProductionReconcileInput
		forbidden []string
	}{
		{
			name: "filesystem persistence", executor: ProductionExecutor{reconcile: func(ReconcileInstallRequest, NativeMCPReplacer) (ReconcileInstallResult, error) {
				return ReconcileInstallResult{}, errors.New("write /private/config token=super-secret")
			}},
			forbidden: []string{"/private/config", "super-secret"},
		},
		{
			name: "native command", executor: ProductionExecutor{Native: &compositionNativeMCP{err: errors.New("command claude mcp add token=super-secret /private/config")}},
			input:     ProductionReconcileInput{SelectedAgents: []string{"claude"}, ClaudeMCPs: []NativeMCPDefinition{nativeMCPDefinition("hive", "desired-secret")}},
			forbidden: []string{"command claude", "/private/config", "super-secret", "desired-secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			input.Root = t.TempDir()
			input.EvidencePath = filepath.Join(input.Root, "state", "recovery.json")
			input.RenderedOutputs = []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed")}}

			_, err := tt.executor.Execute(input)
			if err == nil {
				t.Fatal("Execute() error = nil, want sanitized adapter failure")
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("Execute() error leaked %q: %v", forbidden, err)
				}
			}
		})
	}
}

func TestProductionExecutorBuildsManagedStorePlanAndSkipsNativeWithoutClaude(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, "state", "recovery.json")
	input := ProductionReconcileInput{
		SelectedAgents: []string{"opencode"},
		Root:           root,
		EvidencePath:   evidencePath,
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: "opencode-mcp", Location: "opencode/opencode.json", Bytes: []byte(`{"mcp":{}}`),
		}},
	}

	native := &compositionNativeMCP{}
	result, err := (ProductionExecutor{Native: native}).Execute(input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Native.Phase != NativeMCPSkipped {
		t.Fatalf("native phase = %q, want %q", result.Native.Phase, NativeMCPSkipped)
	}
	if native.calls != 0 {
		t.Fatalf("native calls = %d, want deterministic no-agent skip", native.calls)
	}
	got, err := os.ReadFile(filepath.Join(root, "opencode", "opencode.json"))
	if err != nil || string(got) != `{"mcp":{}}` {
		t.Fatalf("managed OpenCode output = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Dir(evidencePath)); err != nil {
		t.Fatalf("evidence parent was not created: %v", err)
	}
}

func TestProductionPlanUsesClaudeUserScopeAndKeepsOpenCodeOutOfNativeManager(t *testing.T) {
	input := ProductionReconcileInput{
		SelectedAgents: []string{"claude", "opencode"},
		Root:           t.TempDir(),
		EvidencePath:   filepath.Join(t.TempDir(), "recovery.json"),
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: "opencode-mcp", Location: "opencode/opencode.json", Bytes: []byte(`{"mcp":{"hive":{}}}`),
		}},
		ClaudeMCPs: []NativeMCPDefinition{nativeMCPDefinition("hive", "secret")},
	}

	request, err := BuildProductionReconcileRequest(input)
	if err != nil {
		t.Fatalf("BuildProductionReconcileRequest() error = %v", err)
	}
	if len(request.DesiredMCPs) != 1 || request.DesiredMCPs[0].Scope != nativeMCPUserScope {
		t.Fatalf("native definitions = %#v, want one Claude user-scope definition", request.DesiredMCPs)
	}
	if len(request.StorePlan.Operations) != 1 || request.StorePlan.Operations[0].Location != "opencode/opencode.json" {
		t.Fatalf("Store plan = %#v, want independent OpenCode JSON output", request.StorePlan)
	}
}

func TestProductionExecutorPreservesUnprovenancedManagedLocation(t *testing.T) {
	root := t.TempDir()
	location := "claude/CLAUDE.md"
	path := filepath.Join(root, filepath.FromSlash(location))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("user-owned"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := (ProductionExecutor{}).Execute(ProductionReconcileInput{
		Root: root, EvidencePath: filepath.Join(root, "recovery.json"),
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: "jarvis-instructions", Location: location, Bytes: []byte("managed"),
			Existing: &reconcile.Artifact{Identity: "jarvis-instructions", Location: location, Bytes: []byte("user-owned")},
		}},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want unprovenanced collision failure")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "user-owned" {
		t.Fatalf("user-owned file = %q, %v; want preserved bytes", got, readErr)
	}
}

func TestProductionPlanRejectsClaudeDefinitionsOutsideUserScope(t *testing.T) {
	_, err := BuildProductionReconcileRequest(ProductionReconcileInput{
		SelectedAgents: []string{"claude"}, Root: t.TempDir(), EvidencePath: filepath.Join(t.TempDir(), "recovery.json"),
		ClaudeMCPs: []NativeMCPDefinition{{Identity: "hive", Scope: "project", AddArgs: []string{"mcp", "add", "--scope", "project", "hive"}}},
	})
	if err == nil {
		t.Fatal("BuildProductionReconcileRequest() error = nil, want non-user Claude scope rejection")
	}
}

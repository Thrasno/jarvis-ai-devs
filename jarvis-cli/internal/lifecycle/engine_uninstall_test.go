package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func TestEngineUninstall_RejectsUnsupportedModes(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "soft mode", mode: "soft"},
		{name: "purge mode", mode: "purge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": &fakeProviderAdapter{name: "claude"}}, HomeDir: t.TempDir()})

			_, err := engine.Uninstall("claude", tt.mode)
			if err == nil {
				t.Fatal("expected unsupported mode error")
			}
			var lerr *LifecycleError
			if !errors.As(err, &lerr) || lerr.Code != "unsupported_uninstall_mode" {
				t.Fatalf("expected unsupported_uninstall_mode, got %v", err)
			}
		})
	}
}

func TestEngineUninstall_BacksUpBeforeMutationsAndVerifiesAfter(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name:     "claude",
		observed: ObservedProviderState{},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	result, err := engine.Uninstall("claude", "provider")
	if err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if adapter.applyCalls == 0 {
		t.Fatal("expected uninstall to mutate managed assets")
	}
	if len(adapter.applyStages) == 0 || adapter.applyStages[0] != "backup-complete" {
		t.Fatalf("expected backup before uninstall apply, got stages=%v", adapter.applyStages)
	}
	if adapter.verifyCalls < 1 {
		t.Fatalf("expected uninstall post-verify gate to run, verify calls=%d", adapter.verifyCalls)
	}
	if result.Applied == 0 {
		t.Fatal("expected uninstall result to report applied operations")
	}
}

func TestEngineUninstall_AllModeCleansLedgerAfterSuccessfulVerify(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{name: "claude", observed: ObservedProviderState{}}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	if _, _, err := engine.ledger.LoadOrBootstrap("claude"); err != nil {
		t.Fatalf("bootstrap ledger: %v", err)
	}

	result, err := engine.Uninstall("claude", "all")
	if err != nil {
		t.Fatalf("Uninstall all returned error: %v", err)
	}
	if !result.LedgerRemoved {
		t.Fatal("expected all mode uninstall to remove ledger")
	}
	if _, err := os.Stat(engine.ledger.path()); err == nil {
		t.Fatal("expected ledger file to be removed after all uninstall")
	}
}

func TestEngineUninstall_AllModeKeepsLedgerWhenAnyProviderFails(t *testing.T) {
	home := t.TempDir()
	claude := &fakeProviderAdapter{name: "claude", observed: ObservedProviderState{}}
	opencode := &fakeProviderAdapter{name: "opencode", observed: ObservedProviderState{}, applyErr: errors.New("opencode uninstall failed")}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": claude, "opencode": opencode}, HomeDir: home})

	if _, _, err := engine.ledger.LoadOrBootstrap("claude"); err != nil {
		t.Fatalf("bootstrap ledger: %v", err)
	}

	_, err := engine.Uninstall("all", "all")
	if err == nil || !strings.Contains(err.Error(), "opencode uninstall failed") {
		t.Fatalf("expected failing provider error, got %v", err)
	}
	if claude.applyCalls != 1 || opencode.applyCalls != 1 {
		t.Fatalf("expected all-mode uninstall to attempt providers before ledger cleanup, claude=%d opencode=%d", claude.applyCalls, opencode.applyCalls)
	}
	if _, err := os.Stat(engine.ledger.path()); err != nil {
		t.Fatalf("ledger must remain when all-provider uninstall is only partially complete: %v", err)
	}
}

func TestEngineUninstall_UsesOwnedBoundariesOnly(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{name: "opencode", observed: ObservedProviderState{}}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"opencode": adapter}, HomeDir: home})

	_, err := engine.Uninstall("opencode", "provider")
	if err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	for _, applied := range adapter.appliedAssets {
		if applied == "non-owned-custom-section" {
			t.Fatalf("uninstall must not touch non-owned boundary, got %q", applied)
		}
	}
	for _, target := range adapter.backupTargetPaths {
		if filepath.Clean(target) == "non-owned-custom-section" {
			t.Fatalf("backup targets must be managed-only, got %q", target)
		}
	}
}

func TestFakeCompliantOpenCodeConfig_IncludesSDDHiveGrantEvidence(t *testing.T) {
	config := fakeCompliantOpenCodeConfig()
	requiredSubagents := []string{
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}
	requiredTools := []string{
		"hive_mem_search",
		"hive_mem_get_observation",
		"hive_mem_save",
		"hive_mem_context",
		"hive_mem_session_summary",
	}

	for _, subagent := range requiredSubagents {
		evidence := config.SDDSubagentHiveGrantEvidence[subagent]
		for _, tool := range requiredTools {
			if !hasAllowEvidence(evidence, tool) {
				t.Fatalf("fake compliant OpenCode config missing allow evidence for %s:%s", subagent, tool)
			}
		}
	}
}

func hasAllowEvidence(evidence []sddruntime.OpenCodePermissionEvidence, tool string) bool {
	for _, entry := range evidence {
		if entry.Key == tool && entry.Action == "allow" {
			return true
		}
	}
	return false
}

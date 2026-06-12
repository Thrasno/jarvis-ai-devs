package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// TestObserveArtifact_PromptHook_OpenCode_PluginsHiveTs asserts that for the
// opencode agent, prompt_hook observation uses plugins/hive.ts (a file, not a
// directory). Old hive-hooks/ directory must NOT count as present.
func TestObserveArtifact_PromptHook_OpenCode_PluginsHiveTs(t *testing.T) {
	plan, err := sddruntime.Build("opencode")
	if err != nil {
		t.Fatalf("Build opencode: %v", err)
	}

	promptHookArtifact := findManagedArtifact(plan.Contract.ManagedArtifacts, "prompt_hook")
	if promptHookArtifact == nil {
		t.Fatal("prompt_hook artifact not found in contract")
	}

	t.Run("plugins/hive.ts present => Exists true", func(t *testing.T) {
		dir := t.TempDir()
		pluginDir := filepath.Join(dir, "plugins")
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("mkdir plugins: %v", err)
		}
		if err := os.WriteFile(filepath.Join(pluginDir, "hive.ts"), []byte("// hive plugin"), 0644); err != nil {
			t.Fatalf("write hive.ts: %v", err)
		}

		result, err := observeArtifactForAgent(dir, plan.Paths, *promptHookArtifact, "opencode")
		if err != nil {
			t.Fatalf("observeArtifactForAgent: %v", err)
		}
		if !result.Exists {
			t.Error("Exists must be true when plugins/hive.ts is present")
		}
	})

	t.Run("only hive-hooks/ dir present => Exists false", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "hive-hooks"), 0755); err != nil {
			t.Fatalf("mkdir hive-hooks: %v", err)
		}

		result, err := observeArtifactForAgent(dir, plan.Paths, *promptHookArtifact, "opencode")
		if err != nil {
			t.Fatalf("observeArtifactForAgent: %v", err)
		}
		if result.Exists {
			t.Error("Exists must be false when only legacy hive-hooks/ dir exists for opencode")
		}
	})

	t.Run("plugins/hive.ts absent => Exists false", func(t *testing.T) {
		dir := t.TempDir()

		result, err := observeArtifactForAgent(dir, plan.Paths, *promptHookArtifact, "opencode")
		if err != nil {
			t.Fatalf("observeArtifactForAgent: %v", err)
		}
		if result.Exists {
			t.Error("Exists must be false when plugins/hive.ts does not exist")
		}
	})
}

// TestObserveArtifact_PromptHook_Claude_HiveHooksDir asserts that for the
// claude agent, prompt_hook observation still uses hive-hooks/ directory.
func TestObserveArtifact_PromptHook_Claude_HiveHooksDir(t *testing.T) {
	plan, err := sddruntime.Build("claude")
	if err != nil {
		t.Fatalf("Build claude: %v", err)
	}

	promptHookArtifact := findManagedArtifact(plan.Contract.ManagedArtifacts, "prompt_hook")
	if promptHookArtifact == nil {
		t.Fatal("prompt_hook artifact not found in contract")
	}

	t.Run("hive-hooks/ dir present => Exists true", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "hive-hooks"), 0755); err != nil {
			t.Fatalf("mkdir hive-hooks: %v", err)
		}

		result, err := observeArtifactForAgent(dir, plan.Paths, *promptHookArtifact, "claude")
		if err != nil {
			t.Fatalf("observeArtifactForAgent: %v", err)
		}
		if !result.Exists {
			t.Error("Exists must be true when hive-hooks/ dir is present for claude")
		}
	})

	t.Run("hive-hooks/ dir absent => Exists false", func(t *testing.T) {
		dir := t.TempDir()

		result, err := observeArtifactForAgent(dir, plan.Paths, *promptHookArtifact, "claude")
		if err != nil {
			t.Fatalf("observeArtifactForAgent: %v", err)
		}
		if result.Exists {
			t.Error("Exists must be false when hive-hooks/ dir is absent for claude")
		}
	})
}

func findManagedArtifact(artifacts []sddruntime.ManagedArtifact, id string) *sddruntime.ManagedArtifact {
	for i := range artifacts {
		if artifacts[i].ID == id {
			return &artifacts[i]
		}
	}
	return nil
}

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// preMigrationConfig is a schema-2 config.yaml that still owns the per-phase
// model assignments, with no manifest beside it: the machine upgrading into the
// version that moved them into ~/.jarvis/state.yaml.
const preMigrationConfig = `schema_version: 2
api_url: https://hivemem.dev
persona_preset: gentleman
persona_preset_source: user
scope: local-only
sdd:
  claude_phase_models:
    apply:
      model: opus
      effort: high
  opencode_phase_models:
    apply:
      provider_id: anthropic
      model_id: claude-sonnet-4
`

func seedPreMigrationConfig(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, ".jarvis", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte(preMigrationConfig), 0o600); err != nil {
		t.Fatalf("seed config.yaml: %v", err)
	}
	return path
}

// Observing the runtime is what `jarvis doctor`, `jarvis verify` and a dry-run
// reconcile do, and none of them may mutate the user's configuration as a side
// effect of looking at it. Reading the assignments must still answer with the
// values config.yaml holds on a machine that has not migrated yet, otherwise the
// runtime is verified against the contract defaults instead of the user's
// choices.
func TestLoadDesiredPhaseModels_ReadsThePreMigrationConfigWithoutWritingAnything(t *testing.T) {
	home := isolateTestHome(t)
	configPath := seedPreMigrationConfig(t, home)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	models, err := loadDesiredPhaseModels()
	if err != nil {
		t.Fatalf("loadDesiredPhaseModels: %v", err)
	}

	statePath, err := state.Path()
	if err != nil {
		t.Fatalf("state path: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("observing the runtime created state.yaml, stat err = %v", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("re-read config.yaml: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("observing the runtime rewrote config.yaml:\n%s", after)
	}

	if got := models.Claude["apply"]; got.Model != "opus" || got.Effort != "high" {
		t.Errorf("claude apply assignment = %+v, want the value config.yaml holds", got)
	}
	if got := models.OpenCode["apply"]; got.ProviderID != "anthropic" || got.ModelID != "claude-sonnet-4" {
		t.Errorf("opencode apply assignment = %+v, want the value config.yaml holds", got)
	}
}

// The read-only path must not answer differently from the machine that already
// migrated: the same config.yaml has to produce the same assignments either way.
func TestLoadDesiredPhaseModels_MatchesThePostMigrationAnswer(t *testing.T) {
	home := isolateTestHome(t)
	seedPreMigrationConfig(t, home)

	observed, err := loadDesiredPhaseModels()
	if err != nil {
		t.Fatalf("loadDesiredPhaseModels before migrating: %v", err)
	}

	if _, err := state.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	migrated, err := loadDesiredPhaseModels()
	if err != nil {
		t.Fatalf("loadDesiredPhaseModels after migrating: %v", err)
	}

	if observed.Claude["apply"] != migrated.Claude["apply"] {
		t.Errorf("claude apply before = %+v, after = %+v", observed.Claude["apply"], migrated.Claude["apply"])
	}
	if observed.OpenCode["apply"] != migrated.OpenCode["apply"] {
		t.Errorf("opencode apply before = %+v, after = %+v", observed.OpenCode["apply"], migrated.OpenCode["apply"])
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// readRawConfig returns config.yaml as an untyped mapping, which is the only way
// to see keys AppConfig no longer spells.
func readRawConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".jarvis", "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse config.yaml: %v", err)
	}
	return raw
}

// writeRawConfig writes config.yaml verbatim, bypassing Save.
func writeRawConfig(t *testing.T, home, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

// TestSave_NeverWritesTheManifest is the disjointness guarantee the bridge used
// to break on purpose. config.Save has no business in ~/.jarvis/state.yaml, and
// with the bridge gone it must not create or touch one.
func TestSave_NeverWritesTheManifest(t *testing.T) {
	isolateHome(t)

	if err := Save(&AppConfig{APIURL: DefaultAPIURL}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := state.Load(); err == nil {
		t.Fatal("config.Save created a manifest; the stores are meant to be disjoint")
	}

	manifest := state.New()
	manifest.Persona = "neutra"
	manifest.Skills = []string{"go-testing"}
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	if err := Save(&AppConfig{APIURL: "https://example.invalid"}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if got.Persona != "neutra" || len(got.Skills) != 1 {
		t.Errorf("config.Save modified the manifest: %+v", got)
	}
}

// TestSaveThenMigrate_DoesNotStrandReplayFieldsOnAnUnmigratedMachine is the gap
// that opens the moment the replay fields leave the struct.
//
// A machine upgrading into this version still has them in config.yaml and no
// manifest. Nothing forces state.Migrate to run before some command loads and
// saves the config, and a save that wrote only the keys the struct spells would
// erase them before the migration ever saw them.
func TestSaveThenMigrate_DoesNotStrandReplayFieldsOnAnUnmigratedMachine(t *testing.T) {
	home := isolateHome(t)
	writeRawConfig(t, home, `schema_version: 2
api_url: https://hivemem.dev
persona_preset: neutra
persona_preset_source: user
selected_skills:
  - go-testing
  - work-unit-commits
configured_agents:
  - claude
scope: local+cloud
sdd:
  phase_models:
    sdd-apply:
      opencode: opus
      claude: opus
install:
  completed: true
  agents:
    claude:
      configured: true
      instructions_path: /home/u/.claude/CLAUDE.md
      config_path: /home/u/.claude/settings.json
`)

	// A plain command: load the config, change something config still owns, save.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.Version = "1.0.0"
	if err := Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Only now does the machine migrate.
	if _, err := state.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Persona != "neutra" || manifest.PersonaSource != state.PersonaSourceUser {
		t.Errorf("persona = (%q, %q), want the values config.yaml carried", manifest.Persona, manifest.PersonaSource)
	}
	if len(manifest.Skills) != 2 {
		t.Errorf("skills = %v, want both values config.yaml carried", manifest.Skills)
	}
	if len(manifest.InstalledAgents) != 1 || manifest.InstalledAgents[0].ConfigPath != "/home/u/.claude/settings.json" {
		t.Errorf("agents = %+v, want the record config.yaml carried under install.agents", manifest.InstalledAgents)
	}
	if manifest.Scope != state.ScopeLocalCloud {
		t.Errorf("scope = %q, want the value config.yaml carried", manifest.Scope)
	}
	if manifest.PhaseModels.Aliases["sdd-apply"].Claude != "opus" {
		t.Errorf("phase models = %+v, want the value config.yaml carried", manifest.PhaseModels.Aliases)
	}

	// And once migration has moved them, config.yaml stops carrying them.
	raw := readRawConfig(t, home)
	for _, key := range state.ReplayConfigKeys() {
		if _, present := raw[key]; present {
			t.Errorf("config.yaml still carries replay key %q after migration", key)
		}
	}
	if install, _ := raw["install"].(map[string]any); install["agents"] != nil {
		t.Error("config.yaml still carries install.agents after migration")
	}
	if install, _ := raw["install"].(map[string]any); install["completed"] != true {
		t.Errorf("migration dropped a key config.yaml still owns: install=%v", install)
	}
}

// TestSave_PreservesForeignKeysButHonoursDeletionOfItsOwn pins both halves of
// the merge rule: another writer's key survives, and a key this struct spells is
// authoritative even when the caller clears it.
func TestSave_PreservesForeignKeysButHonoursDeletionOfItsOwn(t *testing.T) {
	home := isolateHome(t)
	writeRawConfig(t, home, `schema_version: 3
api_url: https://hivemem.dev
experimental_feature: kept
cloud:
  email: dev@example.com
  sync_configured: true
install:
  completed: true
  future_key: also kept
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Cloud == nil || cfg.Cloud.Email != "dev@example.com" {
		t.Fatalf("cloud block did not load: %+v", cfg.Cloud)
	}

	// Clearing the cloud link is how the local-only scope removes it. The legacy
	// flat email key mirrors the block both ways, so it has to be cleared too or
	// normalization rebuilds the block from it.
	cfg.Cloud = nil
	cfg.Email = ""
	if err := Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw := readRawConfig(t, home)
	if raw["experimental_feature"] != "kept" {
		t.Errorf("a key this struct does not own was clobbered: %v", raw)
	}
	install, _ := raw["install"].(map[string]any)
	if install["future_key"] != "also kept" {
		t.Errorf("a nested key this struct does not own was clobbered: %v", install)
	}
	if install["completed"] != true {
		t.Errorf("install.completed was lost: %v", install)
	}
	if _, present := raw["cloud"]; present {
		t.Errorf("clearing the cloud link must remove the block, got %v", raw["cloud"])
	}
}

// TestInstallCompleted_IsNotSticky is the regression guard for the trap that
// made the bridge necessary.
//
// install.completed used to be recomputed from IsReadyForReconfigure on every
// load and save, while IsReadyForReconfigure itself required install.completed.
// A first installation that saved before its manifest was populated persisted
// false and could never report ready again. The flag is now recorded by the
// installer and only read.
func TestInstallCompleted_IsNotSticky(t *testing.T) {
	isolateHome(t)

	// The installer records its own completion. Nothing about the manifest is
	// known to config at this point.
	cfg := &AppConfig{APIURL: DefaultAPIURL}
	cfg.Install.Mode = string(ConfigStatusReconfigure)
	cfg.Install.Completed = true
	if err := Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !cfg.Install.Completed {
		t.Fatal("saving cleared the completion the installer just recorded")
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reloaded.Install.Completed {
		t.Fatal("install.completed did not survive a save and load")
	}

	complete := RecordedInstall{Complete: true, Populated: true}
	if !reloaded.IsReadyForReconfigure(complete) {
		t.Error("a completed install with a complete manifest must be ready to reconfigure")
	}
	if got := reloaded.ConfigStatus(complete); got != ConfigStatusReconfigure {
		t.Errorf("config status = %q, want reconfigure", got)
	}

	// The manifest half is genuinely required: config.yaml alone cannot say yes.
	if reloaded.IsReadyForReconfigure(RecordedInstall{}) {
		t.Error("an empty manifest must not read as a complete installation")
	}
	if got := reloaded.ConfigStatus(RecordedInstall{}); got != ConfigStatusRecover {
		t.Errorf("config status with a completed flag but an empty manifest = %q, want recover", got)
	}
}

package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// configV2Fixture is a schema-2 config.yaml with every replay field populated.
const configV2Fixture = `schema_version: 2
api_url: https://hivemem.dev
persona_preset: gentleman
persona_preset_source: user
preset: gentleman
selected_skills:
  - go-testing
  - retired-skill-no-longer-in-catalog
configured_agents:
  - claude
  - opencode
scope: local+cloud
cloud:
  email: dev@example.com
  sync_configured: true
install:
  mode: reconfigure
  completed: true
  agents:
    claude:
      configured: true
      instructions_path: /home/u/.claude/CLAUDE.md
      config_path: /home/u/.claude/settings.json
    opencode:
      configured: true
      instructions_path: /home/u/.config/opencode/AGENTS.md
sdd:
  phase_models:
    apply:
      opencode: sonnet
      claude: opus
  opencode_phase_models:
    apply:
      provider_id: anthropic
      model_id: claude-sonnet-4
      effort: high
  claude_phase_models:
    apply:
      model: opus
      effort: high
version: 1.2.3
`

func writeConfig(t *testing.T, home, content string) string {
	t.Helper()
	path := filepath.Join(home, ".jarvis", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	return path
}

func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return out
}

func TestMigrate_MovesEveryReplayFieldIntoTheManifest(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, configV2Fixture)

	res, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !res.Migrated {
		t.Fatal("Migrated = false, want true for a schema-2 config")
	}
	if strings.TrimSpace(res.Notice) == "" {
		t.Error("Notice is empty after a durable migration")
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load migrated manifest: %v", err)
	}
	if got.Persona != "gentleman" || got.PersonaSource != PersonaSourceUser {
		t.Errorf("persona = %q/%q, want gentleman/user", got.Persona, got.PersonaSource)
	}
	if strings.Join(got.Skills, ",") != "go-testing,retired-skill-no-longer-in-catalog" {
		t.Errorf("skills = %#v", got.Skills)
	}
	if got.Scope != ScopeLocalCloud {
		t.Errorf("scope = %q, want %q", got.Scope, ScopeLocalCloud)
	}
	if !got.SelectionConfigured {
		t.Error("selection_configured = false, want true for a config with configured agents")
	}
	if len(got.InstalledAgents) != 2 {
		t.Fatalf("installed_agents = %#v, want 2 entries", got.InstalledAgents)
	}
	if got.InstalledAgents[0].ID != "claude" || got.InstalledAgents[1].ID != "opencode" {
		t.Errorf("installed_agents order = %#v, want configured_agents order preserved", got.InstalledAgents)
	}
	if got.InstalledAgents[0].ConfigPath != "/home/u/.claude/settings.json" {
		t.Errorf("claude config_path = %q, want the install.agents value carried over", got.InstalledAgents[0].ConfigPath)
	}
	if got.PhaseModels.Aliases["apply"].Claude != "opus" {
		t.Errorf("phase_models.aliases = %#v", got.PhaseModels.Aliases)
	}
	if got.PhaseModels.OpenCode["apply"].ModelID != "claude-sonnet-4" {
		t.Errorf("phase_models.opencode = %#v", got.PhaseModels.OpenCode)
	}
	if got.PhaseModels.Claude["apply"].Effort != "high" {
		t.Errorf("phase_models.claude = %#v", got.PhaseModels.Claude)
	}
}

func TestMigrate_RemovesReplayFieldsFromConfigAndAdvancesItToSchema3(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, configV2Fixture)

	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := readYAMLMap(t, configPath)
	if v, _ := cfg["schema_version"].(int); v != 3 {
		t.Errorf("config.yaml schema_version = %v, want 3", cfg["schema_version"])
	}

	// Fields move; they are never copied. None of these may remain readable
	// from config.yaml once the manifest owns them.
	for _, key := range []string{
		"persona_preset",
		"persona_preset_source",
		"preset",
		"selected_skills",
		"configured_agents",
		"scope",
		"sdd",
	} {
		if _, ok := cfg[key]; ok {
			t.Errorf("config.yaml still carries replay field %q after migration", key)
		}
	}

	install, ok := cfg["install"].(map[string]any)
	if !ok {
		t.Fatalf("config.yaml install = %#v, want a mapping", cfg["install"])
	}
	if _, ok := install["agents"]; ok {
		t.Error("config.yaml still carries install.agents after migration")
	}

	// Fields config.yaml still owns must survive untouched.
	if cfg["api_url"] != "https://hivemem.dev" {
		t.Errorf("api_url = %#v, want it preserved", cfg["api_url"])
	}
	if cfg["version"] != "1.2.3" {
		t.Errorf("version = %#v, want it preserved", cfg["version"])
	}
	if install["mode"] != "reconfigure" {
		t.Errorf("install.mode = %#v, want it preserved", install["mode"])
	}
	cloud, ok := cfg["cloud"].(map[string]any)
	if !ok || cloud["email"] != "dev@example.com" {
		t.Errorf("cloud = %#v, want it preserved", cfg["cloud"])
	}
}

func TestMigrate_StoresAreDisjointAfterMigration(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, configV2Fixture)

	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := readYAMLMap(t, configPath)
	manifest := readYAMLMap(t, filepath.Join(home, ".jarvis", "state.yaml"))

	if len(cfg) == 0 || len(manifest) == 0 {
		t.Fatalf("expected both stores to be populated; cfg=%d manifest=%d keys", len(cfg), len(manifest))
	}
	for key := range manifest {
		if key == "schema_version" {
			// Each store versions itself independently; the versions are not a
			// shared value and cannot disagree about anything.
			continue
		}
		if _, ok := cfg[key]; ok {
			t.Errorf("field %q is readable from both config.yaml and state.yaml", key)
		}
	}
}

// Decision D4: migration is not gated on replay-readiness. It must still run on
// a schema-2 config whose replay fields were never populated; replay blocks
// afterwards on the missing agents list, not during migration.
func TestMigrate_RunsBeforeValidationBlocksOnAnUnpopulatedConfig(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, "schema_version: 2\napi_url: https://hivemem.dev\n")

	res, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate on an unpopulated config: %v", err)
	}
	if !res.Migrated {
		t.Fatal("Migrated = false; migration must run even with no replay fields populated")
	}

	cfg := readYAMLMap(t, configPath)
	if v, _ := cfg["schema_version"].(int); v != 3 {
		t.Errorf("config.yaml schema_version = %v, want 3", cfg["schema_version"])
	}

	st, err := Load()
	if err != nil {
		t.Fatalf("Load after migrating an unpopulated config: %v", err)
	}
	err = st.ValidateForReplay()
	if err == nil {
		t.Fatal("ValidateForReplay succeeded; it must block on the missing agents list")
	}
	if !strings.Contains(err.Error(), "installed_agents") {
		t.Errorf("blocking error %q does not name the missing agents list", err)
	}
}

// The success notice is a claim about durability. A migration that fails before
// the manifest write completes must never tell the user the migration happened,
// and must leave config.yaml at its pre-migration schema version.
func TestMigrate_WithholdsNoticeWhenTheWriteFails(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, configV2Fixture)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	// A directory at the manifest path makes the durable write fail.
	if err := os.MkdirAll(filepath.Join(home, ".jarvis", "state.yaml"), 0755); err != nil {
		t.Fatalf("mkdir state.yaml: %v", err)
	}

	res, err := Migrate()
	if err == nil {
		t.Fatal("Migrate succeeded despite an undeliverable manifest write")
	}
	if res.Migrated {
		t.Error("Migrated = true after a failed write")
	}
	if strings.TrimSpace(res.Notice) != "" {
		t.Errorf("Notice = %q, want it withheld until the write is durable", res.Notice)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("config.yaml was modified even though the manifest write failed")
	}
	cfg := readYAMLMap(t, configPath)
	if v, _ := cfg["schema_version"].(int); v != 2 {
		t.Errorf("config.yaml schema_version = %v, want it left at 2", cfg["schema_version"])
	}
}

func TestMigrate_IsANoOpOnAnAlreadyMigratedConfig(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, "schema_version: 3\napi_url: https://hivemem.dev\n")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	res, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if res.Migrated {
		t.Error("Migrated = true for a config already at schema 3")
	}
	if strings.TrimSpace(res.Notice) != "" {
		t.Errorf("Notice = %q, want no notice for a no-op", res.Notice)
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "state.yaml")); !os.IsNotExist(err) {
		t.Error("a no-op migration wrote state.yaml")
	}

	after, _ := os.ReadFile(configPath)
	if string(before) != string(after) {
		t.Error("a no-op migration rewrote config.yaml")
	}
}

func TestMigrate_IsANoOpWhenNoConfigExists(t *testing.T) {
	home := isolateHome(t)

	res, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate on a fresh machine: %v", err)
	}
	if res.Migrated {
		t.Error("Migrated = true with no config.yaml present")
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "state.yaml")); !os.IsNotExist(err) {
		t.Error("migration wrote state.yaml with no config.yaml to migrate from")
	}
}

func TestMigrate_DropsAgentsThatWereNeverConfigured(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, `schema_version: 2
configured_agents:
  - claude
install:
  agents:
    claude:
      configured: true
      instructions_path: /home/u/.claude/CLAUDE.md
    kilo:
      configured: false
`)

	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	st, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(st.InstalledAgents) != 1 || st.InstalledAgents[0].ID != "claude" {
		t.Fatalf("installed_agents = %#v, want only the configured claude agent", st.InstalledAgents)
	}
}

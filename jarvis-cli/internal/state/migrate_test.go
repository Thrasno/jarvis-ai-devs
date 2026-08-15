package state

import (
	"errors"
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

// The other half of the same rule: install.agents may record a configured agent
// that configured_agents forgot, and that record is the only evidence the agent
// was ever installed. Dropping it would lose the ownership proof a later
// cleanup needs.
func TestMigrate_CarriesOverAConfiguredAgentTheOrderedListForgot(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, `schema_version: 2
configured_agents:
  - claude
install:
  agents:
    claude:
      configured: true
    opencode:
      configured: true
      config_path: /home/u/.config/opencode/opencode.json
`)

	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	st, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(st.InstalledAgents) != 2 || st.InstalledAgents[1].ID != "opencode" {
		t.Fatalf("installed_agents = %#v, want opencode carried over after claude", st.InstalledAgents)
	}
}

func TestMigrate_LeavesAnExistingManifestAlone(t *testing.T) {
	home := isolateHome(t)
	// A config.yaml the bridge already emptied: still schema 2, no replay keys.
	writeConfig(t, home, "schema_version: 2\napi_url: https://hivemem.dev\n")

	owned := New()
	owned.Persona = "neutra"
	owned.Skills = []string{"go-testing"}
	if err := Save(owned); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	res, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if res.Migrated {
		t.Error("Migrated = true, want false when the manifest already owns the fields")
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Persona != "neutra" || len(got.Skills) != 1 {
		t.Fatalf("Migrate overwrote the manifest from a stripped config.yaml: %+v", got)
	}
}

// TestMigrate_SelectionConfiguredRecordsBeingAskedNotMerelyBeingDetected holds
// the distinction the field exists for: "asked, selected nothing" is not the
// same as "never asked". Migration is one-way and runs once per machine, so a
// value derived from the wrong evidence is written permanently.
//
// The two table cases are the two directions the presence-counting derivation
// gets wrong. Detected-but-unconfigured agents are not a selection: the wizard
// records what it found before the user answers. An explicitly empty
// configured_agents list is one: the wizard only writes the key after asking.
func TestMigrate_SelectionConfiguredRecordsBeingAskedNotMerelyBeingDetected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		want   bool
	}{
		{
			name: "agents detected but none configured is not a selection",
			config: `schema_version: 2
install:
  agents:
    claude:
      configured: false
    opencode:
      configured: false
`,
			want: false,
		},
		{
			name: "an explicitly empty list is a selection of nothing",
			config: `schema_version: 2
configured_agents: []
`,
			want: true,
		},
		{
			name: "a configured agent is a selection",
			config: `schema_version: 2
install:
  agents:
    claude:
      configured: true
`,
			want: true,
		},
		{
			name: "no agent evidence at all is not a selection",
			config: `schema_version: 2
persona_preset: neutral
`,
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateHome(t)
			writeConfig(t, home, tc.config)

			if _, err := Migrate(); err != nil {
				t.Fatalf("Migrate: %v", err)
			}
			st, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if st.SelectionConfigured != tc.want {
				t.Errorf("selection_configured = %v, want %v (installed_agents = %#v)",
					st.SelectionConfigured, tc.want, st.InstalledAgents)
			}
		})
	}
}

// TestMigrate_StillMovesReplayFieldsFromASchema3ConfigThatStillCarriesThem
// closes the window between config.yaml losing the replay fields from the
// AppConfig struct and the manifest existing.
//
// On a machine that has not migrated yet, any plain config.Save advances
// config.yaml to the current schema version while the replay keys are still in
// the file. Keying the no-op purely off that version number would then declare
// the migration already done and strand the user's persona, skills, agents,
// scope and phase models in a file nothing reads any more. The keys themselves
// are the honest signal, so the version alone is not allowed to stop the move.
func TestMigrate_StillMovesReplayFieldsFromASchema3ConfigThatStillCarriesThem(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, `schema_version: 3
api_url: https://hivemem.dev
persona_preset: neutra
persona_preset_source: user
selected_skills:
  - go-testing
configured_agents:
  - claude
install:
  completed: true
  agents:
    claude:
      configured: true
      instructions_path: /home/u/.claude/CLAUDE.md
      config_path: /home/u/.claude/settings.json
`)

	result, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !result.Migrated {
		t.Fatal("a schema-3 config that still carries the replay keys has not migrated yet")
	}

	manifest, err := Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Persona != "neutra" || manifest.PersonaSource != PersonaSourceUser {
		t.Errorf("manifest persona = (%q, %q), want the values stranded in config.yaml", manifest.Persona, manifest.PersonaSource)
	}
	if len(manifest.Skills) != 1 || manifest.Skills[0] != "go-testing" {
		t.Errorf("manifest skills = %v, want the value stranded in config.yaml", manifest.Skills)
	}
	if len(manifest.InstalledAgents) != 1 || manifest.InstalledAgents[0].ID != "claude" {
		t.Errorf("manifest agents = %+v, want the value stranded in config.yaml", manifest.InstalledAgents)
	}

	rewritten, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	for _, key := range ReplayConfigKeys() {
		if strings.Contains(string(rewritten), key+":") {
			t.Errorf("config.yaml still carries replay key %q:\n%s", key, rewritten)
		}
	}
}

// TestMigrate_IsStillANoOpOnACleanAlreadyMigratedConfig is the other half of the
// rule above: once the keys are gone, the migration must not run again.
func TestMigrate_IsStillANoOpOnACleanAlreadyMigratedConfig(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, "schema_version: 3\napi_url: https://hivemem.dev\ninstall:\n  completed: true\n")

	result, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if result.Migrated || result.Notice != "" {
		t.Fatalf("a clean migrated config must not migrate again, got %+v", result)
	}
	if _, err := Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no manifest must be written, got err=%v", err)
	}
}

// Migration is a compatibility boundary: it must accept whatever an older
// release or a hand-edited config.yaml holds. The release this replaces
// normalized a non-canonical persona_preset_source and scope on every load, so
// values the manifest does not recognize reached nobody. Copying them verbatim
// makes Validate reject them and the migration fail, which strands every replay
// field in a file nothing reads any more.
func TestMigrate_NormalizesNonCanonicalPersonaSourceAndScope(t *testing.T) {
	cases := []struct {
		name       string
		config     string
		wantSource PersonaSource
		wantScope  Scope
	}{
		{
			name: "unrecognized values with no cloud link",
			config: `schema_version: 2
persona_preset: gentleman
persona_preset_source: external
scope: weird
`,
			wantSource: PersonaSourceBuiltin,
			wantScope:  ScopeLocalOnly,
		},
		{
			name: "case and padding are not a different value",
			config: `schema_version: 2
persona_preset: gentleman
persona_preset_source: "  User  "
scope: weird
cloud:
  email: dev@example.com
`,
			wantSource: PersonaSourceUser,
			wantScope:  ScopeLocalCloud,
		},
		{
			name: "a configured sync is a stored cloud link",
			config: `schema_version: 2
persona_preset: gentleman
scope: nonsense
cloud:
  sync_configured: true
`,
			wantSource: PersonaSourceBuiltin,
			wantScope:  ScopeLocalCloud,
		},
		{
			name: "a legacy top-level email is a stored cloud link",
			config: `schema_version: 2
persona_preset: gentleman
email: dev@example.com
`,
			wantSource: PersonaSourceBuiltin,
			wantScope:  ScopeLocalCloud,
		},
		{
			name: "an absent scope with no cloud link is local-only",
			config: `schema_version: 2
persona_preset: gentleman
`,
			wantSource: PersonaSourceBuiltin,
			wantScope:  ScopeLocalOnly,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateHome(t)
			writeConfig(t, home, tc.config)

			result, err := Migrate()
			if err != nil {
				t.Fatalf("Migrate must tolerate a non-canonical config.yaml: %v", err)
			}
			if !result.Migrated {
				t.Fatal("Migrated = false; the replay fields must still move")
			}

			manifest, err := Load()
			if err != nil {
				t.Fatalf("Load after migrating: %v", err)
			}
			if manifest.PersonaSource != tc.wantSource {
				t.Errorf("persona_source = %q, want %q", manifest.PersonaSource, tc.wantSource)
			}
			if manifest.Scope != tc.wantScope {
				t.Errorf("scope = %q, want %q", manifest.Scope, tc.wantScope)
			}
			if manifest.Persona != "gentleman" {
				t.Errorf("persona = %q, want gentleman; normalization must not drop the recorded choice", manifest.Persona)
			}
		})
	}
}

// Migration is a compatibility boundary: every legacy string it carries over
// reaches State.Validate, which rejects a whitespace-only value. A value the
// previous release normalized away must not become a hard migration failure
// that strands every replay field in a file nothing reads any more.
func TestMigrate_NormalizesWhitespaceOnlyLegacyValues(t *testing.T) {
	cases := []struct {
		name   string
		config string
		assert func(t *testing.T, manifest *State)
	}{
		{
			name: "a whitespace-only persona_preset falls back to preset",
			config: `schema_version: 2
persona_preset: "   "
preset: "  gentleman  "
`,
			assert: func(t *testing.T, manifest *State) {
				if manifest.Persona != "gentleman" {
					t.Errorf("persona = %q, want gentleman carried over from preset", manifest.Persona)
				}
			},
		},
		{
			name: "a whitespace-only persona reads as an unrecorded one",
			config: `schema_version: 2
persona_preset: "   "
preset: "  "
selected_skills:
  - go-testing
`,
			assert: func(t *testing.T, manifest *State) {
				if manifest.Persona != "" {
					t.Errorf("persona = %q, want the unrecorded value", manifest.Persona)
				}
				if slug, _ := manifest.ResolvedPersona(); slug != DefaultPersona {
					t.Errorf("ResolvedPersona = %q, want %q", slug, DefaultPersona)
				}
			},
		},
		{
			name: "a whitespace-only skill entry is dropped without touching the others",
			config: `schema_version: 2
persona_preset: gentleman
selected_skills:
  - go-testing
  - "   "
  - retired-skill-no-longer-in-catalog
`,
			assert: func(t *testing.T, manifest *State) {
				if strings.Join(manifest.Skills, ",") != "go-testing,retired-skill-no-longer-in-catalog" {
					t.Errorf("skills = %#v; only the blank entry may be dropped", manifest.Skills)
				}
			},
		},
		{
			name: "a skills list of nothing but blanks stays a recorded answer of none",
			config: `schema_version: 2
persona_preset: gentleman
selected_skills:
  - "   "
`,
			assert: func(t *testing.T, manifest *State) {
				if manifest.Skills == nil || len(manifest.Skills) != 0 {
					t.Errorf("skills = %#v, want a present but empty list", manifest.Skills)
				}
			},
		},
		{
			name: "a whitespace-only agent id is not an installed agent",
			config: `schema_version: 2
persona_preset: gentleman
configured_agents:
  - "   "
  - claude
install:
  agents:
    claude:
      configured: true
      instructions_path: /home/u/.claude/CLAUDE.md
      config_path: /home/u/.claude/settings.json
`,
			assert: func(t *testing.T, manifest *State) {
				if len(manifest.InstalledAgents) != 1 || manifest.InstalledAgents[0].ID != "claude" {
					t.Errorf("installed_agents = %#v, want only claude", manifest.InstalledAgents)
				}
			},
		},
		{
			name: "whitespace-only agent paths read as unrecorded paths",
			config: `schema_version: 2
persona_preset: gentleman
configured_agents:
  - claude
install:
  agents:
    claude:
      configured: true
      instructions_path: "   "
      config_path: "  /home/u/.claude/settings.json  "
`,
			assert: func(t *testing.T, manifest *State) {
				if len(manifest.InstalledAgents) != 1 {
					t.Fatalf("installed_agents = %#v, want one entry", manifest.InstalledAgents)
				}
				if manifest.InstalledAgents[0].InstructionsPath != "" {
					t.Errorf("instructions_path = %q, want the unrecorded value", manifest.InstalledAgents[0].InstructionsPath)
				}
				if manifest.InstalledAgents[0].ConfigPath != "/home/u/.claude/settings.json" {
					t.Errorf("config_path = %q, want the padding removed", manifest.InstalledAgents[0].ConfigPath)
				}
			},
		},
		{
			// The release being replaced dropped an unnamed phase on every
			// config.Load. Carrying one into the manifest fails Validate, which
			// aborts the migration and then blocks every manifest write, leaving
			// no self-service exit.
			name: "an unnamed phase is not a phase-model assignment",
			config: `schema_version: 2
persona_preset: gentleman
sdd:
  phase_models:
    "  ":
      opencode: sonnet
      claude: sonnet
    "  Apply  ":
      opencode: "  Sonnet  "
      claude: opus
  opencode_phase_models:
    "":
      provider_id: "  anthropic  "
      model_id: "  sonnet  "
  claude_phase_models:
    "  VERIFY  ":
      model: "  opus  "
`,
			assert: func(t *testing.T, manifest *State) {
				if _, ok := manifest.PhaseModels.Aliases["apply"]; !ok {
					t.Errorf("phase_models.aliases = %#v, want the padded phase carried over as apply", manifest.PhaseModels.Aliases)
				}
				if got := manifest.PhaseModels.Aliases["apply"].OpenCode; got != "sonnet" {
					t.Errorf("aliases[apply].opencode = %q, want the padding removed", got)
				}
				if len(manifest.PhaseModels.Aliases) != 1 {
					t.Errorf("phase_models.aliases = %#v, want the unnamed phase dropped", manifest.PhaseModels.Aliases)
				}
				if len(manifest.PhaseModels.OpenCode) != 0 {
					t.Errorf("opencode_phase_models = %#v, want the unnamed phase dropped", manifest.PhaseModels.OpenCode)
				}
				if got := manifest.PhaseModels.Claude["verify"].Model; got != "opus" {
					t.Errorf("claude_phase_models[verify].model = %q, want the padding removed", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateHome(t)
			writeConfig(t, home, tc.config)

			result, err := Migrate()
			if err != nil {
				t.Fatalf("Migrate must tolerate whitespace-only legacy values: %v", err)
			}
			if !result.Migrated {
				t.Fatal("Migrated = false; the replay fields must still move")
			}

			manifest, err := Load()
			if err != nil {
				t.Fatalf("Load after migrating: %v", err)
			}
			tc.assert(t, manifest)
		})
	}
}

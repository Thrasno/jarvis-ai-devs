package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome sets HOME to a fresh temp dir and registers cleanup.
// This is mandatory to prevent tests from touching the real ~/.jarvis.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	setHomeEnv(t, home)
	return home
}

func setHomeEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}

func TestIsConfigured_ReturnsFalseWhenNoFile(t *testing.T) {
	isolateHome(t)

	if IsConfigured() {
		t.Fatal("expected IsConfigured()=false for a fresh home dir with no config file")
	}
}

func TestIsConfigured_ReturnsFalseWhenEmpty(t *testing.T) {
	home := isolateHome(t)

	// Create the directory and an empty config file.
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatalf("create .jarvis dir: %v", err)
	}
	emptyPath := filepath.Join(jarvisDir, "config.yaml")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("write empty config: %v", err)
	}

	// Empty file means no email — should not be considered configured.
	if IsConfigured() {
		t.Fatal("expected IsConfigured()=false when config file is empty")
	}
}

func TestIsConfigured_ReturnsTrueWhenValid(t *testing.T) {
	isolateHome(t)

	cfg := &AppConfig{
		SchemaVersion:  2,
		APIURL:         DefaultAPIURL,
		PersonaPreset:  "tony-stark",
		SelectedSkills: []string{"core-memory"},
		Install: InstallState{
			Completed: true,
			Agents: map[string]AgentState{
				"claude": {Configured: true, InstructionsPath: "/tmp/CLAUDE.md"},
			},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !IsConfigured() {
		t.Fatal("expected IsConfigured()=true after saving a valid config")
	}
}

func TestSave_CreatesDirectoryIfMissing(t *testing.T) {
	home := isolateHome(t)

	// ~/.jarvis does not exist yet.
	jarvisDir := filepath.Join(home, ".jarvis")
	if _, err := os.Stat(jarvisDir); !os.IsNotExist(err) {
		t.Fatal("expected .jarvis dir to NOT exist before Save")
	}

	cfg := &AppConfig{
		SchemaVersion:  2,
		APIURL:         DefaultAPIURL,
		PersonaPreset:  "argentino",
		SelectedSkills: []string{"core-memory"},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Directory must now exist.
	if _, err := os.Stat(jarvisDir); err != nil {
		t.Fatalf("expected .jarvis dir to exist after Save, got: %v", err)
	}
	// Config file must exist inside it.
	cfgPath := filepath.Join(jarvisDir, "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config.yaml to exist after Save, got: %v", err)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	isolateHome(t)

	original := &AppConfig{
		SchemaVersion:    2,
		APIURL:           "https://custom.api.example.com",
		PersonaPreset:    "tony-stark",
		SelectedSkills:   []string{"core-memory", "testing"},
		ConfiguredAgents: []string{"claude", "opencode"},
		Cloud:            &CloudConfig{Email: "rhodey@war.machine", SyncConfigured: true},
		Install: InstallState{
			Mode:      "reconfigure",
			Completed: true,
			Agents: map[string]AgentState{
				"claude":   {Configured: true, InstructionsPath: "/a", ConfigPath: "/b"},
				"opencode": {Configured: true, InstructionsPath: "/c", ConfigPath: "/d"},
			},
		},
		Version: "2.0.0",
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if loaded.Cloud == nil || loaded.Cloud.Email != original.Cloud.Email {
		t.Errorf("Cloud.Email: got %#v, want %q", loaded.Cloud, original.Cloud.Email)
	}
	if loaded.APIURL != original.APIURL {
		t.Errorf("APIURL: got %q, want %q", loaded.APIURL, original.APIURL)
	}
	if loaded.PersonaPreset != original.PersonaPreset {
		t.Errorf("PersonaPreset: got %q, want %q", loaded.PersonaPreset, original.PersonaPreset)
	}
	if len(loaded.SelectedSkills) != len(original.SelectedSkills) {
		t.Fatalf("SelectedSkills length: got %d, want %d", len(loaded.SelectedSkills), len(original.SelectedSkills))
	}
	if loaded.Version != original.Version {
		t.Errorf("Version: got %q, want %q", loaded.Version, original.Version)
	}
	if len(loaded.ConfiguredAgents) != len(original.ConfiguredAgents) {
		t.Errorf("ConfiguredAgents length: got %d, want %d",
			len(loaded.ConfiguredAgents), len(original.ConfiguredAgents))
	} else {
		for i, a := range original.ConfiguredAgents {
			if loaded.ConfiguredAgents[i] != a {
				t.Errorf("ConfiguredAgents[%d]: got %q, want %q",
					i, loaded.ConfiguredAgents[i], a)
			}
		}
	}
}

func TestLoad_MigratesLegacyV1ConfigToV2(t *testing.T) {
	home := isolateHome(t)
	legacy := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"email: legacy@example.com",
		"preset: argentino",
		"configured_agents:",
		"  - claude",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}

	if cfg.SchemaVersion != 2 {
		t.Fatalf("expected schema_version=2 after migration, got %d", cfg.SchemaVersion)
	}
	if cfg.Cloud == nil || cfg.Cloud.Email != "legacy@example.com" {
		t.Fatalf("expected migrated cloud email, got %#v", cfg.Cloud)
	}
	if cfg.PersonaPreset != "argentino" {
		t.Fatalf("expected migrated persona_preset=argentino, got %q", cfg.PersonaPreset)
	}
	if len(cfg.ConfiguredAgents) != 1 || cfg.ConfiguredAgents[0] != "claude" {
		t.Fatalf("expected migrated configured_agents=[claude], got %v", cfg.ConfiguredAgents)
	}
}

func TestLoad_DefaultsPersonaPresetSourceToBuiltinForLegacyConfig(t *testing.T) {
	home := isolateHome(t)
	legacy := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"persona_preset: argentino",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}

	if cfg.PersonaPresetSource != "builtin" {
		t.Fatalf("expected persona_preset_source=builtin, got %q", cfg.PersonaPresetSource)
	}
}

func TestLoad_NormalizesPersonaPresetSourceValues(t *testing.T) {
	tests := []struct {
		name     string
		rawValue string
		want     string
	}{
		{name: "valid user source", rawValue: " user ", want: "user"},
		{name: "invalid source falls back to builtin", rawValue: "external", want: "builtin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateHome(t)
			raw := strings.Join([]string{
				"api_url: https://hivemem.dev",
				"persona_preset: custom-mentor",
				"persona_preset_source: " + tt.rawValue,
			}, "\n")
			if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(raw), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if cfg.PersonaPresetSource != tt.want {
				t.Fatalf("PersonaPresetSource = %q, want %q", cfg.PersonaPresetSource, tt.want)
			}
		})
	}
}

func TestConfigStatus_ReadyWithoutCloudEmail(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion:  2,
		APIURL:         DefaultAPIURL,
		PersonaPreset:  "argentino",
		SelectedSkills: []string{"core-memory"},
		Install: InstallState{
			Completed: true,
			Agents: map[string]AgentState{
				"claude": {Configured: true, InstructionsPath: "/tmp/CLAUDE.md", ConfigPath: "/tmp/settings.json"},
			},
		},
	}

	if !cfg.IsReadyForReconfigure() {
		t.Fatal("expected IsReadyForReconfigure=true for complete local config without cloud email")
	}
	if got := cfg.ConfigStatus(); got != ConfigStatusReconfigure {
		t.Fatalf("expected ConfigStatusReconfigure, got %q", got)
	}
}

func TestConfigStatus_RecoverWhenPartiallyConfigured(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion: 2,
		APIURL:        DefaultAPIURL,
		Install:       InstallState{Completed: true},
	}

	if cfg.IsReadyForReconfigure() {
		t.Fatal("expected IsReadyForReconfigure=false when required local fields are missing")
	}
	if got := cfg.ConfigStatus(); got != ConfigStatusRecover {
		t.Fatalf("expected ConfigStatusRecover for partial state, got %q", got)
	}
}

func TestLoad_DefaultsScopeFromLegacyCloudState(t *testing.T) {
	home := isolateHome(t)
	legacy := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"email: legacy@example.com",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}

	if cfg.Scope != ScopeLocalCloud {
		t.Fatalf("expected scope=%q from legacy cloud state, got %q", ScopeLocalCloud, cfg.Scope)
	}
}

func TestLoad_DefaultsScopeToLocalOnlyWithoutCloudState(t *testing.T) {
	home := isolateHome(t)
	legacy := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"persona_preset: argentino",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}

	if cfg.Scope != ScopeLocalOnly {
		t.Fatalf("expected scope=%q without cloud state, got %q", ScopeLocalOnly, cfg.Scope)
	}
}

func TestLoad_InitializesSDDPhaseModelsContainerForLegacyConfig(t *testing.T) {
	home := isolateHome(t)
	legacy := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"persona_preset: argentino",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.SDD.PhaseModels == nil {
		t.Fatal("expected SDD.PhaseModels to be initialized")
	}
}

func TestSaveLoad_PersistsSDDPhaseModels(t *testing.T) {
	isolateHome(t)
	cfg := defaultConfig()
	cfg.SDD.PhaseModels = map[string]PhaseModelSelection{
		"default":   {OpenCode: "sonnet", Claude: "haiku"},
		"sdd-apply": {OpenCode: "opus", Claude: "sonnet"},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := loaded.SDD.PhaseModels["sdd-apply"]
	if got.OpenCode != "opus" || got.Claude != "sonnet" {
		t.Fatalf("unexpected sdd-apply phase models: %+v", got)
	}
}

func TestSaveLoad_PersistsOpenCodePhaseModelAssignments(t *testing.T) {
	isolateHome(t)
	cfg := defaultConfig()
	cfg.SDD.OpenCodePhaseModels = map[string]OpenCodeModelAssignment{
		"sdd-apply": {
			ProviderID: "openai",
			ModelID:    "gpt-5.1-codex-max",
			Effort:     "high",
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := loaded.SDD.OpenCodePhaseModels["sdd-apply"]
	if got.ProviderID != "openai" || got.ModelID != "gpt-5.1-codex-max" || got.Effort != "high" {
		t.Fatalf("unexpected OpenCode assignment: %+v", got)
	}
}

func TestLoad_LegacyPhaseModelsClaudeLoadsWithEmptyClaudeEffort(t *testing.T) {
	home := isolateHome(t)
	raw := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"persona_preset: argentino",
		"sdd:",
		"  phase_models:",
		"    sdd-design:",
		"      claude: opus",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.SDD.PhaseModels["sdd-design"].Claude != "opus" {
		t.Fatalf("legacy PhaseModels Claude = %q, want opus", cfg.SDD.PhaseModels["sdd-design"].Claude)
	}
	if cfg.SDD.ClaudePhaseModels == nil {
		t.Fatal("expected SDD.ClaudePhaseModels to be initialized")
	}
	if got := cfg.SDD.ClaudePhaseModels["sdd-design"].Effort; got != "" {
		t.Fatalf("Claude effort = %q, want empty inherited/default effort", got)
	}
}

func TestLoad_ReturnsErrorWhenFileCorrupt(t *testing.T) {
	home := isolateHome(t)

	// Write invalid YAML to the config path.
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(jarvisDir, "config.yaml")
	corruptYAML := []byte("email: [\nbad yaml: {unclosed")
	if err := os.WriteFile(cfgPath, corruptYAML, 0644); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load() to return an error for corrupt YAML, got nil")
	}
}

func TestSave_ReturnsErrorOnNilConfig(t *testing.T) {
	isolateHome(t)
	err := Save(nil)
	if err == nil || !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("expected nil-config error, got %v", err)
	}
}

func TestLoad_DefaultConfigRespectsEnvOverride(t *testing.T) {
	isolateHome(t)
	t.Setenv("JARVIS_API_URL", "https://override.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIURL != "https://override.example" {
		t.Fatalf("expected APIURL override, got %q", cfg.APIURL)
	}
}

func TestConfigStatusSetupWhenConfigNil(t *testing.T) {
	var cfg *AppConfig
	if got := cfg.ConfigStatus(); got != ConfigStatusSetup {
		t.Fatalf("expected ConfigStatusSetup for nil cfg, got %q", got)
	}
}

func TestIsReadyForReconfigure_FailsWhenConfiguredAgentStateMissing(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion:    2,
		APIURL:           DefaultAPIURL,
		PersonaPreset:    "argentino",
		SelectedSkills:   []string{"core-memory"},
		ConfiguredAgents: []string{"claude"},
		Install: InstallState{
			Completed: true,
			Agents:    map[string]AgentState{},
		},
	}

	if cfg.IsReadyForReconfigure() {
		t.Fatal("expected IsReadyForReconfigure=false when configured agent state is missing")
	}
}

// TestLayer1Content_ContainsAllRequiredSections verifies that Layer1Content includes
// behavior-only runtime guardrails while deferring protocol details to protocol.hive.
func TestLayer1Content_ContainsAllRequiredSections(t *testing.T) {
	content := Layer1Content()

	required := []string{
		// PROJECT CONTEXT
		"PROJECT CONTEXT",
		"git remote get-url origin",
		"basename",
		`"default"`,
		// Canonical Hive protocol boundary
		"Hive Protocol Source Boundary",
		"protocol.hive",
		"jarvis-cli/embed/hive-protocol.md",
		"Layer1 MUST NOT duplicate the Hive protocol body",
		// Contextual skill loading guardrail
		"Contextual Skill Loading Self-Check",
		"Before every response",
		"matches an installed skill",
		"load that skill before task-specific work",
		// Persona/artifact language guardrail
		"Persona Scope and Artifact Language",
		"Persona voice applies only to direct user replies",
		"Generated technical artifacts default to English",
		"Hive",
		"jarvis CLI",
		".jarvis/skill-registry.md",
		".jarvis/skills/<skill>/SKILL.md",
		// Hive-specific behavior summary
		"scope",
		// SDD DAG without retired QA gate
		"SDD DAG: `proposal → specs → tasks → apply → verify → archive`",
		"Apply-progress continuity",
		// Hive-specific
		"project",
	}

	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Errorf("Layer1Content missing required string %q", want)
		}
	}

	for _, protocolBodyMarker := range []string{"PROACTIVE SAVE TRIGGERS", "SESSION CLOSE PROTOCOL", "FORMAT FOR mem_save"} {
		if strings.Contains(content, protocolBodyMarker) {
			t.Errorf("Layer1Content must not duplicate protocol.hive body marker %q", protocolBodyMarker)
		}
	}
}

// TestLayer1Content_NoEngramReferences verifies that Layer1Content contains no
// references to "Engram" (the old memory system) in any casing.
func TestLayer1Content_NoEngramReferences(t *testing.T) {
	content := Layer1Content()

	if strings.Contains(content, "Engram") {
		t.Error("Layer1Content must not contain 'Engram' (old memory system reference)")
	}
	if strings.Contains(content, "engram") {
		t.Error("Layer1Content must not contain 'engram' (old memory system reference)")
	}
}

func TestLayer1Content_NoRetiredSDDQAReferences(t *testing.T) {
	content := Layer1Content()

	for _, forbidden := range []string{"sdd-qa", "qa-signoff", "qa-checklist"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("Layer1Content must not contain retired QA gate reference %q", forbidden)
		}
	}
}

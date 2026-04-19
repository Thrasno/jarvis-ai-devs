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
	t.Setenv("HOME", home)
	return home
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

// TestLayer1Content_ContainsAllRequiredSections verifies that Layer1Content includes
// all 10 required sections of the full Hive protocol (R2).
func TestLayer1Content_ContainsAllRequiredSections(t *testing.T) {
	content := Layer1Content()

	required := []string{
		// PROJECT CONTEXT
		"PROJECT CONTEXT",
		"git remote get-url origin",
		"basename",
		`"default"`,
		// PROACTIVE SAVE TRIGGERS + self-check
		"PROACTIVE SAVE TRIGGERS",
		"Self-check after EVERY task",
		// mem_save format fields
		"scope",
		"topic_key",
		"What",
		"Why",
		"Where",
		"Learned",
		// Topic update rules
		"Different topics MUST NOT overwrite",
		"mem_suggest_topic_key",
		// Search protocol
		"mem_context",
		"mem_get_observation",
		// Session close protocol
		"SESSION CLOSE PROTOCOL",
		"mem_session_summary",
		"Goal",
		"Discoveries",
		"Accomplished",
		"Next Steps",
		"Relevant Files",
		// SDD with sdd-qa
		"sdd-qa",
		// Hive-specific
		"mem_sync",
		"project",
		// Core tool
		"mem_save",
	}

	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Errorf("Layer1Content missing required string %q", want)
		}
	}

	// AFTER COMPACTION — case-insensitive check
	lowerContent := strings.ToLower(content)
	if !strings.Contains(lowerContent, "after compaction") {
		t.Error("Layer1Content missing 'AFTER COMPACTION' section (case-insensitive)")
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

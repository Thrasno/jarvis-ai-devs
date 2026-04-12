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
		Email:  "tony@stark.io",
		APIURL: DefaultAPIURL,
		Preset: "tony-stark",
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
		Email:  "pepper@stark.io",
		APIURL: DefaultAPIURL,
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
		Email:            "rhodey@war.machine",
		APIURL:           "https://custom.api.example.com",
		Preset:           "tony-stark",
		ConfiguredAgents: []string{"claude", "opencode"},
		Version:          "2.0.0",
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if loaded.Email != original.Email {
		t.Errorf("Email: got %q, want %q", loaded.Email, original.Email)
	}
	if loaded.APIURL != original.APIURL {
		t.Errorf("APIURL: got %q, want %q", loaded.APIURL, original.APIURL)
	}
	if loaded.Preset != original.Preset {
		t.Errorf("Preset: got %q, want %q", loaded.Preset, original.Preset)
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

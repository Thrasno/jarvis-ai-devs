package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// populatedConfig carries a value in every replay field the manifest takes over.
func populatedConfig() *AppConfig {
	return &AppConfig{
		APIURL:              DefaultAPIURL,
		PersonaPreset:       "neutra",
		PersonaPresetSource: "user",
		SelectedSkills:      []string{"go-testing", "work-unit-commits"},
		ConfiguredAgents:    []string{"claude"},
		Scope:               ScopeLocalCloud,
		Install: InstallState{
			Agents: map[string]AgentState{"claude": {
				Configured:       true,
				InstructionsPath: "/home/u/.claude/CLAUDE.md",
				ConfigPath:       "/home/u/.claude/settings.json",
			}},
		},
		SDD: SDDConfig{
			PhaseModels:         map[string]PhaseModelSelection{"apply": {OpenCode: "sonnet", Claude: "opus"}},
			OpenCodePhaseModels: map[string]OpenCodeModelAssignment{"apply": {ProviderID: "anthropic", ModelID: "sonnet"}},
			ClaudePhaseModels:   map[string]ClaudeModelAssignment{"apply": {Model: "opus"}},
		},
	}
}

// writeLegacyConfig writes a pre-bridge config.yaml, bypassing Save.
func writeLegacyConfig(t *testing.T, home string, cfg *AppConfig) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), data, 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

// assertReplayFields checks every field the manifest took over.
func assertReplayFields(t *testing.T, got *AppConfig) {
	t.Helper()
	agent := got.Install.Agents["claude"]
	switch {
	case got.PersonaPreset != "neutra" || got.PersonaPresetSource != "user",
		len(got.SelectedSkills) != 2 || got.SelectedSkills[0] != "go-testing",
		len(got.ConfiguredAgents) != 1 || got.ConfiguredAgents[0] != "claude",
		!agent.Configured || agent.ConfigPath != "/home/u/.claude/settings.json",
		got.Scope != ScopeLocalCloud,
		got.SDD.PhaseModels["apply"].Claude != "opus",
		got.SDD.OpenCodePhaseModels["apply"].ModelID != "sonnet",
		got.SDD.ClaudePhaseModels["apply"].Model != "opus":
		t.Fatalf("replay fields not served: %+v agent=%+v", got, agent)
	}
}

// The defining test: a persona and a skill selection set through AppConfig
// survive Save and a fresh Load, while config.yaml stops carrying them.
func TestSaveThenLoad_MovesReplayFieldsIntoTheManifest(t *testing.T) {
	home := isolateHome(t)
	if err := Save(populatedConfig()); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	assertReplayFields(t, got)

	// Disjointness is a property of the written file, not of the struct.
	data, err := os.ReadFile(filepath.Join(home, ".jarvis", "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse config.yaml: %v", err)
	}
	for _, key := range state.ReplayConfigKeys() {
		if _, present := raw[key]; present {
			t.Errorf("config.yaml still carries replay key %q", key)
		}
	}
	if install, _ := raw["install"].(map[string]any); install["agents"] != nil {
		t.Error("config.yaml still carries install.agents")
	}
	if raw["api_url"] != DefaultAPIURL {
		t.Errorf("config.yaml lost a key it still owns: %v", raw["api_url"])
	}
}

// config.yaml stays authoritative until the manifest exists, and never after.
func TestLoad_ManifestTakesOverFromConfigYAML(t *testing.T) {
	home := isolateHome(t)
	writeLegacyConfig(t, home, populatedConfig())

	got, err := Load()
	if err != nil {
		t.Fatalf("load without manifest: %v", err)
	}
	assertReplayFields(t, got)

	manifest := state.New()
	manifest.Persona = "gentleman"
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	if got, err = Load(); err != nil {
		t.Fatalf("load with manifest: %v", err)
	}
	if got.PersonaPreset != "gentleman" {
		t.Fatalf("the manifest must win over config.yaml, got %q", got.PersonaPreset)
	}
}

// Wiring state.Migrate() into a command is the next slice's job; without the
// bridge it would leave every consumer reading empty replay fields.
func TestLoad_AfterStateMigrateKeepsServingTheReplayFields(t *testing.T) {
	home := isolateHome(t)
	writeLegacyConfig(t, home, populatedConfig())

	if _, err := state.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	assertReplayFields(t, got)
}

func TestSave_PreservesManifestFieldsConfigDoesNotOwn(t *testing.T) {
	isolateHome(t)
	manifest := state.New()
	manifest.Persona = "neutra"
	manifest.Statusline = state.StatuslineState{Decided: true, Enabled: true}
	manifest.ManagedAssetDigest = "sha256:cafe"
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	if err := Save(populatedConfig()); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if !got.Statusline.ShouldManage() || got.ManagedAssetDigest != "sha256:cafe" {
		t.Errorf("config.Save cleared manifest fields it does not own: %+v", got)
	}
}

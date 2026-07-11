package main

import (
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
)

func TestPersonaSetCmd_UsesResolverAndPipeline_ForBuiltinAndUserPreset(t *testing.T) {
	tests := []struct {
		name              string
		inputSlug         string
		expectedSlug      string
		expectedSource    string
		seedUserPresetYML string
	}{
		{
			name:           "builtin preset stores builtin source",
			inputSlug:      "Neutra",
			expectedSlug:   "neutra",
			expectedSource: "builtin",
		},
		{
			name:           "user preset stores user source",
			inputSlug:      "Mi Persona",
			expectedSlug:   "mi-persona",
			expectedSource: "user",
			seedUserPresetYML: `schema_version: 2
name: mi-persona
display_name: Mi Persona
presentation:
  language: es-rioplatense
  register: warm-direct
  vocabulary: rioplatense
  cadence: energetic
  humor: warm
  emotional_range: supportive
  verbosity: balanced
  formatting: structured
  teaching_metaphors: architecture
  examples: practical
  address_pack: gentleman
  phrase_pack: gentleman
  anti_caricature: grounded
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempHome := isolateTestHome(t)

			if err := os.MkdirAll(filepath.Join(tempHome, ".claude"), 0o755); err != nil {
				t.Fatalf("create .claude dir: %v", err)
			}

			if tt.seedUserPresetYML != "" {
				if _, err := persona.SaveUserPresetFile(tt.expectedSlug, []byte(tt.seedUserPresetYML)); err != nil {
					t.Fatalf("seed user preset: %v", err)
				}
			}

			if err := config.Save(&config.AppConfig{
				PersonaPreset:       "argentino",
				PersonaPresetSource: "user",
				Preset:              "argentino",
			}); err != nil {
				t.Fatalf("seed config: %v", err)
			}

			if err := personaSetCmd.RunE(personaSetCmd, []string{tt.inputSlug}); err != nil {
				t.Fatalf("persona set returned error: %v", err)
			}

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("load config after persona set: %v", err)
			}

			if cfg.PersonaPreset != tt.expectedSlug {
				t.Fatalf("persona_preset = %q, want %q", cfg.PersonaPreset, tt.expectedSlug)
			}
			if cfg.PersonaPresetSource != tt.expectedSource {
				t.Fatalf("persona_preset_source = %q, want %q", cfg.PersonaPresetSource, tt.expectedSource)
			}

			if _, err := os.Stat(filepath.Join(tempHome, ".claude", "CLAUDE.md")); err != nil {
				t.Fatalf("expected CLAUDE.md to exist: %v", err)
			}
			if _, err := os.Stat(filepath.Join(tempHome, ".claude", "settings.json")); err != nil {
				t.Fatalf("expected settings.json to exist: %v", err)
			}
		})
	}
}

// TestPersonaSetCmd_ClaudeAgent_CreatesOutputStyle verifies that when persona set
// is called with ClaudeAgent (which supports output-styles), both CLAUDE.md and
// the output-style file are created.
func TestPersonaSetCmd_ClaudeAgent_CreatesOutputStyle(t *testing.T) {
	// Setup temp directories
	tempHome := isolateTestHome(t)
	tempClaudeDir := filepath.Join(tempHome, ".claude")

	// Create ClaudeAgent config directory
	if err := os.MkdirAll(tempClaudeDir, 0755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	// Verify HOME was set
	currentHome, _ := os.UserHomeDir()
	if currentHome != tempHome {
		t.Fatalf("HOME not set correctly: got %q, want %q", currentHome, tempHome)
	}

	// Load a test preset
	resolved, err := persona.ResolvePresetV2(jarvis.PersonaFS, "neutra")
	if err != nil {
		t.Fatalf("ResolvePresetV2 failed: %v", err)
	}

	// Detect agents AFTER setting HOME env var
	agents := agent.Detect(jarvis.TemplatesFS)
	var claudeAgent agent.Agent
	for _, a := range agents {
		if a.Name() == "claude" {
			claudeAgent = a
			break
		}
	}
	if claudeAgent == nil {
		var names []string
		for _, a := range agents {
			names = append(names, a.Name())
		}
		t.Fatalf("ClaudeAgent not detected (config dir exists at %s, agents found: %v)", tempClaudeDir, names)
	}

	// Verify ClaudeAgent supports output-styles
	if !claudeAgent.SupportsOutputStyles() {
		t.Fatal("ClaudeAgent should support output-styles")
	}

	// Call WriteInstructions (required before WriteOutputStyle)
	layer2 := persona.RenderLayer2V2(resolved.Preset)
	if err := claudeAgent.WriteInstructions(config.Layer1Content(), layer2, nil); err != nil {
		t.Fatalf("WriteInstructions failed: %v", err)
	}

	// Call WriteOutputStyleV2 through the V2 adapter.
	claudeV2, ok := claudeAgent.(persona.PresetV2Agent)
	if !ok {
		t.Fatal("ClaudeAgent must support schema-v2 output styles")
	}
	if err := claudeV2.WriteOutputStyleV2(resolved.Preset); err != nil {
		t.Fatalf("WriteOutputStyleV2 failed: %v", err)
	}

	// ASSERT: CLAUDE.md should exist
	claudeMd := filepath.Join(tempClaudeDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMd); os.IsNotExist(err) {
		t.Error("CLAUDE.md was not created")
	}

	// ASSERT: output-style file should exist
	outputStylePath := filepath.Join(tempClaudeDir, "output-styles", "Neutra.md")
	if _, err := os.Stat(outputStylePath); os.IsNotExist(err) {
		t.Error("output-style file was not created")
	}

	// ASSERT: settings.json should contain outputStyle key
	settingsPath := filepath.Join(tempClaudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("settings.json was not created")
	} else {
		data, _ := os.ReadFile(settingsPath)
		if !contains(string(data), `"outputStyle"`) {
			t.Error("settings.json missing outputStyle key")
		}
		if !contains(string(data), `"Neutra"`) {
			t.Error("settings.json missing Neutra value")
		}
	}
}

// TestPersonaSetCmd_OpenCodeAgent_NoOutputStyle verifies that when persona set
// is called with OpenCodeAgent (which does NOT support output-styles), only
// AGENTS.md is created and no output-style files are written.
func TestPersonaSetCmd_OpenCodeAgent_NoOutputStyle(t *testing.T) {
	// Setup temp directories
	tempHome := isolateTestHome(t)
	tempOpenCodeDir := filepath.Join(tempHome, ".config", "opencode")

	// Create OpenCodeAgent config directory
	if err := os.MkdirAll(tempOpenCodeDir, 0755); err != nil {
		t.Fatalf("failed to create opencode dir: %v", err)
	}

	// Load a test preset
	resolved, err := persona.ResolvePresetV2(jarvis.PersonaFS, "neutra")
	if err != nil {
		t.Fatalf("ResolvePresetV2 failed: %v", err)
	}

	// Detect agents AFTER setting HOME env var
	agents := agent.Detect(jarvis.TemplatesFS)
	var openCodeAgent agent.Agent
	for _, a := range agents {
		if a.Name() == "opencode" {
			openCodeAgent = a
			break
		}
	}
	if openCodeAgent == nil {
		var names []string
		for _, a := range agents {
			names = append(names, a.Name())
		}
		t.Fatalf("OpenCodeAgent not detected (config dir exists at %s, agents found: %v)", tempOpenCodeDir, names)
	}

	// Verify OpenCodeAgent does NOT support output-styles
	if openCodeAgent.SupportsOutputStyles() {
		t.Fatal("OpenCodeAgent should NOT support output-styles")
	}

	// Call WriteInstructions
	layer2 := persona.RenderLayer2V2(resolved.Preset)
	if err := openCodeAgent.WriteInstructions(config.Layer1Content(), layer2, nil); err != nil {
		t.Fatalf("WriteInstructions failed: %v", err)
	}

	// Call WriteOutputStyleV2 (should be no-op)
	openCodeV2, ok := openCodeAgent.(persona.PresetV2Agent)
	if !ok {
		t.Fatal("OpenCodeAgent must support schema-v2 output styles")
	}
	if err := openCodeV2.WriteOutputStyleV2(resolved.Preset); err != nil {
		t.Fatalf("WriteOutputStyleV2 should not error: %v", err)
	}

	// ASSERT: AGENTS.md should exist
	agentsMd := filepath.Join(tempOpenCodeDir, "AGENTS.md")
	if _, err := os.Stat(agentsMd); os.IsNotExist(err) {
		t.Error("AGENTS.md was not created")
	}

	// ASSERT: No output-style directory should exist
	outputStyleDir := filepath.Join(tempOpenCodeDir, "output-styles")
	if _, err := os.Stat(outputStyleDir); !os.IsNotExist(err) {
		t.Error("output-styles directory should not exist for OpenCodeAgent")
	}

	// ASSERT: No settings.json should be created
	settingsPath := filepath.Join(tempOpenCodeDir, "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings.json should not exist for OpenCodeAgent")
	}
}

func TestApplyPersonaPresetV2UsesCanonicalPipeline(t *testing.T) {
	tempHome := isolateTestHome(t)
	if err := os.MkdirAll(filepath.Join(tempHome, ".claude"), 0o755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	agents := agent.Detect(jarvis.TemplatesFS)
	resolved := &persona.ResolvedPresetV2{
		Slug:   "custom-mentor",
		Source: persona.PresetSourceUser,
		Preset: &persona.PresetV2{
			Name: "custom-mentor",
			Presentation: persona.PresentationV2{
				Language: "en-us", Register: "friendly-professional", Vocabulary: "plain-technical", Cadence: "measured",
				Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured",
				TeachingMetaphors: "construction", Examples: "practical", AddressPack: "peer", PhrasePack: "plain", AntiCaricature: "grounded",
			},
		},
	}

	if err := applyPersonaPresetV2(agents, resolved, persona.ApplyOptions{Layer1: config.Layer1Content()}); err != nil {
		t.Fatalf("applyPersonaPresetV2() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(tempHome, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !contains(string(content), "### Presentation") || contains(string(content), "### Behavioral Rules") {
		t.Fatalf("V2 CLI adapter rendered unexpected Layer2:\n%s", content)
	}
}

func TestResolvePersonaSetPresetUsesValidatedV2Route(t *testing.T) {
	resolved, err := resolvePersonaSetPreset(jarvis.PersonaFS, "Neutra")
	if err != nil {
		t.Fatalf("resolvePersonaSetPreset: %v", err)
	}
	if resolved == nil || resolved.Slug != "neutra" || resolved.Preset.SchemaVersion != 2 {
		t.Fatalf("resolved = %+v, want validated V2 neutra", resolved)
	}
}

func TestResolvePersonaSetSelectionRejectsLegacyCustomProfileWithMigrationGuidance(t *testing.T) {
	home := isolateTestHome(t)
	legacyPath := filepath.Join(home, ".jarvis", "personas", "legacy-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy preset dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n"), 0o644); err != nil {
		t.Fatalf("write legacy preset: %v", err)
	}

	_, err := resolvePersonaSetPreset(jarvis.PersonaFS, "legacy custom")
	if err == nil || !contains(err.Error(), "migrate") {
		t.Fatalf("resolvePersonaSetSelection() error = %v, want actionable migration guidance", err)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

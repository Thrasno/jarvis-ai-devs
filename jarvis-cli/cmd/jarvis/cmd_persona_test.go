package main

import (
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
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
			seedUserPresetYML: `name: mi-persona
display_name: Mi Persona
description: Persona de usuario para tests
tone:
  formality: casual
  directness: high
  humor: warm
  language: es-rioplatense
communication_style:
  verbosity: high
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["che"]
  confirmations: ["dale"]
  transitions: ["a ver"]
  sign_offs: ["vamos"]
notes: |
  ## Voice & Tone
  Habla claro y directo.

  ## Behavior Rules
  Priorizá claridad y ejemplos.

  ## Collaboration Protocol
  Confirmá supuestos antes de avanzar.

  ## Boundaries
  No inventes datos.
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempHome := t.TempDir()
			t.Setenv("HOME", tempHome)

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
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
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
	preset, err := persona.LoadPreset(jarvis.PersonaFS, "neutra")
	if err != nil {
		t.Fatalf("LoadPreset failed: %v", err)
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
	layer2 := persona.RenderLayer2(preset)
	if err := claudeAgent.WriteInstructions(config.Layer1Content(), layer2, nil); err != nil {
		t.Fatalf("WriteInstructions failed: %v", err)
	}

	// Call WriteOutputStyle (the new functionality being tested)
	if err := claudeAgent.WriteOutputStyle(preset); err != nil {
		t.Fatalf("WriteOutputStyle failed: %v", err)
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
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	tempOpenCodeDir := filepath.Join(tempHome, ".config", "opencode")

	// Create OpenCodeAgent config directory
	if err := os.MkdirAll(tempOpenCodeDir, 0755); err != nil {
		t.Fatalf("failed to create opencode dir: %v", err)
	}

	// Load a test preset
	preset, err := persona.LoadPreset(jarvis.PersonaFS, "neutra")
	if err != nil {
		t.Fatalf("LoadPreset failed: %v", err)
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
	layer2 := persona.RenderLayer2(preset)
	if err := openCodeAgent.WriteInstructions(config.Layer1Content(), layer2, nil); err != nil {
		t.Fatalf("WriteInstructions failed: %v", err)
	}

	// Call WriteOutputStyle (should be no-op)
	if err := openCodeAgent.WriteOutputStyle(preset); err != nil {
		t.Fatalf("WriteOutputStyle should not error: %v", err)
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

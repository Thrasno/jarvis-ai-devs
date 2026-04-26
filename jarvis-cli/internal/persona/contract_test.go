package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

func TestPresetContract(t *testing.T) {
	t.Run("resolver canonicalizes slug and source", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		userYAML := []byte(`name: mi-persona
display_name: Mi Persona
description: Preset user para contrato
tone:
  formality: balanced
  directness: high
  humor: warm
  language: en-us
communication_style:
  verbosity: high
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["hey"]
  confirmations: ["done"]
  transitions: ["now"]
  sign_offs: ["bye"]
notes: |
  ## Voice & Tone
  Keep it clear.

  ## Behavior Rules
  Prefer practical outcomes.

  ## Collaboration Protocol
  Confirm assumptions.

  ## Boundaries
  Do not fabricate data.
`)
		if _, err := SaveUserPresetFile("mi-persona", userYAML); err != nil {
			t.Fatalf("SaveUserPresetFile: %v", err)
		}

		tests := []struct {
			name       string
			requested  string
			wantSlug   string
			wantSource PresetSource
		}{
			{name: "builtin", requested: "NeuTra", wantSlug: "neutra", wantSource: PresetSourceBuiltin},
			{name: "user-defined", requested: "Mi Persona", wantSlug: "mi-persona", wantSource: PresetSourceUser},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				resolved, err := ResolvePreset(jarvis.PersonaFS, tt.requested)
				if err != nil {
					t.Fatalf("ResolvePreset(%q): %v", tt.requested, err)
				}
				if resolved.Slug != tt.wantSlug {
					t.Fatalf("resolved slug = %q, want %q", resolved.Slug, tt.wantSlug)
				}
				if resolved.Source != tt.wantSource {
					t.Fatalf("resolved source = %q, want %q", resolved.Source, tt.wantSource)
				}
			})
		}
	})

	t.Run("validation enforces structural and editorial contract", func(t *testing.T) {
		tests := []struct {
			name    string
			yaml    string
			wantErr string
		}{
			{
				name: "valid canonical preset",
				yaml: `name: valid
display_name: Valid
description: Valid preset
tone:
  formality: balanced
  directness: high
  humor: warm
  language: en-us
communication_style:
  verbosity: high
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["hey"]
  confirmations: ["done"]
  transitions: ["now"]
  sign_offs: ["bye"]
notes: |
  # Valid Persona

  ## Core Principle
  Keep it clear.

  ## Behavior
  Prefer practical outcomes.

  ## When Asking Questions
  Confirm assumptions.
`,
			},
			{
				name: "reject missing required field",
				yaml: `name: broken
display_name: Broken
description: missing tone
communication_style:
  verbosity: high
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["hey"]
  confirmations: ["done"]
`,
				wantErr: "missing required field: tone",
			},
			{
				name: "reject malformed editorial notes",
				yaml: `name: broken-notes
display_name: Broken Notes
description: invalid notes
tone:
  formality: balanced
  directness: high
  humor: warm
  language: en-us
communication_style:
  verbosity: high
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["hey"]
  confirmations: ["done"]
  transitions: ["now"]
  sign_offs: ["bye"]
notes: |
  # Broken Notes

  ## Missing Required Sections
  This should fail.
`,
				wantErr: "invalid notes template",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := ValidatePreset([]byte(tt.yaml))
				if tt.wantErr == "" {
					if err != nil {
						t.Fatalf("ValidatePreset() unexpected error: %v", err)
					}
					return
				}
				if err == nil {
					t.Fatalf("ValidatePreset() expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidatePreset() error = %q, want contains %q", err.Error(), tt.wantErr)
				}
			})
		}
	})

	t.Run("apply pipeline keeps no-mix semantics and persists canonical slug+source", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		userYAML := []byte(`name: mi-persona
display_name: Mi Persona
description: Preset user para contrato
tone:
  formality: balanced
  directness: high
  humor: warm
  language: en-us
communication_style:
  verbosity: high
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["hey"]
  confirmations: ["done"]
  transitions: ["now"]
  sign_offs: ["bye"]
notes: |
  ## Voice & Tone
  Keep it clear.

  ## Behavior Rules
  Prefer practical outcomes.

  ## Collaboration Protocol
  Confirm assumptions.

  ## Boundaries
  Do not fabricate data.
`)
		if _, err := SaveUserPresetFile("mi-persona", userYAML); err != nil {
			t.Fatalf("SaveUserPresetFile: %v", err)
		}

		if err := config.Save(&config.AppConfig{PersonaPreset: "argentino", PersonaPresetSource: "builtin", Preset: "argentino"}); err != nil {
			t.Fatalf("seed config: %v", err)
		}

		agent := newPipelineAgentStub("claude", true)
		agents := []PresetAgent{agent}

		userResolved, err := ResolvePreset(jarvis.PersonaFS, "Mi Persona")
		if err != nil {
			t.Fatalf("ResolvePreset user: %v", err)
		}
		if err := ApplyPresetPipeline(agents, userResolved, ApplyOptions{
			Layer1:               "layer1",
			PreviousPresetSlug:   "argentino",
			PreviousPresetSource: PresetSourceBuiltin,
			PersistConfig:        true,
		}); err != nil {
			t.Fatalf("ApplyPresetPipeline user: %v", err)
		}

		cfgAfterUser, err := config.Load()
		if err != nil {
			t.Fatalf("load config after user apply: %v", err)
		}
		if cfgAfterUser.PersonaPreset != "mi-persona" {
			t.Fatalf("persona_preset after user apply = %q, want mi-persona", cfgAfterUser.PersonaPreset)
		}
		if cfgAfterUser.PersonaPresetSource != "user" {
			t.Fatalf("persona_preset_source after user apply = %q, want user", cfgAfterUser.PersonaPresetSource)
		}

		builtinResolved, err := ResolvePreset(jarvis.PersonaFS, "Neutra")
		if err != nil {
			t.Fatalf("ResolvePreset builtin: %v", err)
		}
		if err := ApplyPresetPipeline(agents, builtinResolved, ApplyOptions{
			Layer1:               "layer1",
			PreviousPresetSlug:   userResolved.Slug,
			PreviousPresetSource: userResolved.Source,
			PersistConfig:        true,
		}); err != nil {
			t.Fatalf("ApplyPresetPipeline builtin: %v", err)
		}

		cfgAfterBuiltin, err := config.Load()
		if err != nil {
			t.Fatalf("load config after builtin apply: %v", err)
		}
		if cfgAfterBuiltin.PersonaPreset != "neutra" {
			t.Fatalf("persona_preset after builtin apply = %q, want neutra", cfgAfterBuiltin.PersonaPreset)
		}
		if cfgAfterBuiltin.PersonaPresetSource != "builtin" {
			t.Fatalf("persona_preset_source after builtin apply = %q, want builtin", cfgAfterBuiltin.PersonaPresetSource)
		}

		if got := agent.settings["outputStyle"]; got != "Neutra" {
			t.Fatalf("settings.outputStyle = %q, want %q", got, "Neutra")
		}
		if _, exists := agent.outputFiles["MiPersona.md"]; exists {
			t.Fatalf("found residual previous output-style file MiPersona.md")
		}
		if _, exists := agent.outputFiles["Neutra.md"]; !exists {
			t.Fatalf("missing current output-style file Neutra.md")
		}

		cfgPath := filepath.Join(home, ".jarvis", "config.yaml")
		if _, err := os.Stat(cfgPath); err != nil {
			t.Fatalf("expected config to persist at %s: %v", cfgPath, err)
		}
	})
}

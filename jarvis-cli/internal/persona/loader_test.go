package persona

import (
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
)

func TestLoadPreset(t *testing.T) {
	presetNames := []string{
		"argentino",
		"tony-stark",
		"neutra",
		"yoda",
		"sargento",
		"asturiano",
		"galleguinho",
	}

	for _, name := range presetNames {
		t.Run("load_"+name, func(t *testing.T) {
			preset, err := LoadPreset(jarvis.PersonaFS, name)
			if err != nil {
				t.Fatalf("LoadPreset(%q) failed: %v", name, err)
			}
			if preset.Name == "" {
				t.Error("preset.Name is empty")
			}
			if preset.DisplayName == "" {
				t.Error("preset.DisplayName is empty")
			}
			if preset.Tone.Language == "" {
				t.Error("preset.Tone.Language is empty")
			}
		})
	}
}

func TestLoadPreset_Unknown(t *testing.T) {
	_, err := LoadPreset(jarvis.PersonaFS, "unknown-preset-xyz")
	if err == nil {
		t.Fatal("expected error for unknown preset, got nil")
	}
}

func TestListPresets(t *testing.T) {
	presets, err := ListPresets(jarvis.PersonaFS)
	if err != nil {
		t.Fatalf("ListPresets() failed: %v", err)
	}

	if len(presets) < 7 {
		t.Errorf("expected at least 7 presets, got %d", len(presets))
	}

	for _, p := range presets {
		if p.Name == "" {
			t.Error("preset with empty name found")
		}
	}
}

func TestValidateCustom(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid custom preset",
			content: `
name: my-persona
display_name: "My Persona"
description: "Test persona"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hello"]
  confirmations: ["Done"]
  transitions: ["Note:"]
  sign_offs: ["The end."]
`,
		},
		{
			name: "missing tone.formality",
			content: `
name: bad-persona
display_name: "Bad"
description: "Missing tone formality"
tone:
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hi"]
  confirmations: ["OK"]
`,
			wantErr: true,
		},
		{
			name: "contains protected Layer1 field",
			content: `
name: bad-persona
display_name: "Bad"
description: "Has Layer1 field"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hi"]
  confirmations: ["OK"]
expertise: "This should not be here"
`,
			wantErr: true,
		},
		{
			name: "invalid language value",
			content: `
name: bad-lang
display_name: "Bad"
description: "Invalid language"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: klingon
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hi"]
  confirmations: ["OK"]
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCustom([]byte(tt.content))
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRenderLayer2(t *testing.T) {
	preset, err := LoadPreset(jarvis.PersonaFS, "argentino")
	if err != nil {
		t.Fatalf("LoadPreset failed: %v", err)
	}

	rendered := RenderLayer2(preset)
	if len(rendered) == 0 {
		t.Error("RenderLayer2 produced empty string")
	}
	if rendered == "" {
		t.Error("RenderLayer2 returned empty content")
	}
}

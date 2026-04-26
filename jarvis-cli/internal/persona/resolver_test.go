package persona

import (
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
)

func TestNormalizeSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "lowercase", input: "YODA", want: "yoda"},
		{name: "spaces to hyphen", input: "Tony Stark", want: "tony-stark"},
		{name: "trim spaces", input: "  Argentino  ", want: "argentino"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSlug(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolvePreset(t *testing.T) {
	const customPresetYAML = `
name: custom-mentor
display_name: "Custom Mentor"
description: "User-defined preset"
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
  greetings: ["Hey"]
  confirmations: ["Done"]
  transitions: ["Now"]
  sign_offs: ["Bye"]
notes: |
  Keep things practical.
`

	tests := []struct {
		name           string
		requestedSlug  string
		setupUserPreset bool
		wantSource     PresetSource
		wantSlug       string
		wantErr        bool
	}{
		{
			name:          "resolve builtin slug",
			requestedSlug: "Tony Stark",
			wantSource:    PresetSourceBuiltin,
			wantSlug:      "tony-stark",
		},
		{
			name:            "resolve user-defined slug",
			requestedSlug:   "Custom Mentor",
			setupUserPreset: true,
			wantSource:      PresetSourceUser,
			wantSlug:        "custom-mentor",
		},
		{
			name:          "missing slug fails fast",
			requestedSlug: "does-not-exist",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			if tt.setupUserPreset {
				presetPath := filepath.Join(home, ".jarvis", "personas", "custom-mentor.yaml")
				if err := os.MkdirAll(filepath.Dir(presetPath), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", filepath.Dir(presetPath), err)
				}
				if err := os.WriteFile(presetPath, []byte(customPresetYAML), 0o644); err != nil {
					t.Fatalf("WriteFile(%q): %v", presetPath, err)
				}
			}

			resolved, err := ResolvePreset(jarvis.PersonaFS, tt.requestedSlug)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolvePreset(%q) expected error, got nil", tt.requestedSlug)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePreset(%q) unexpected error: %v", tt.requestedSlug, err)
			}

			if resolved == nil {
				t.Fatalf("ResolvePreset(%q) returned nil result", tt.requestedSlug)
			}
			if resolved.Source != tt.wantSource {
				t.Fatalf("ResolvePreset(%q) source = %q, want %q", tt.requestedSlug, resolved.Source, tt.wantSource)
			}
			if resolved.Slug != tt.wantSlug {
				t.Fatalf("ResolvePreset(%q) slug = %q, want %q", tt.requestedSlug, resolved.Slug, tt.wantSlug)
			}
			if resolved.Preset == nil {
				t.Fatalf("ResolvePreset(%q) returned nil preset", tt.requestedSlug)
			}
		})
	}
}

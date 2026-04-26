package tui

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

func TestResolveWizardPresetSelection(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		custom    *customPresetDraft
		wantSlug  string
		wantSrc   persona.PresetSource
		wantErr   string
	}{
		{
			name:      "resolves builtin preset",
			requested: "Fixture",
			wantSlug:  "fixture",
			wantSrc:   persona.PresetSourceBuiltin,
		},
		{
			name:      "custom requires draft",
			requested: "custom",
			wantErr:   "requires name and display name",
		},
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveWizardPresetSelection(testPersonaFS, tt.requested, tt.custom)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveWizardPresetSelection expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveWizardPresetSelection unexpected error: %v", err)
			}
			if resolved.Slug != tt.wantSlug {
				t.Fatalf("slug = %q, want %q", resolved.Slug, tt.wantSlug)
			}
			if resolved.Source != tt.wantSrc {
				t.Fatalf("source = %q, want %q", resolved.Source, tt.wantSrc)
			}
		})
	}
}

func TestCreateWizardCustomPreset_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		draft   customPresetDraft
		wantErr string
	}{
		{name: "missing name", draft: customPresetDraft{DisplayName: "X"}, wantErr: "name is required"},
		{name: "name normalizes to empty slug", draft: customPresetDraft{Name: "---", DisplayName: "X"}, wantErr: "resolves to empty slug"},
		{name: "missing display name", draft: customPresetDraft{Name: "x"}, wantErr: "display name is required"},
		{name: "reserved custom slug", draft: customPresetDraft{Name: "custom", DisplayName: "Custom"}, wantErr: "reserved"},
		{name: "invalid yaml", draft: customPresetDraft{Name: "x", DisplayName: "X", YAML: "name: ["}, wantErr: "invalid YAML"},
		{name: "schema validation fails", draft: customPresetDraft{Name: "x", DisplayName: "X", YAML: "name: x\ndisplay_name: X\ndescription: bad\nnotes: hi\n"}, wantErr: "validation failed"},
		{name: "builtin slug collision rejected", draft: customPresetDraft{Name: "Fixture", DisplayName: "Fixture"}, wantErr: "collides with built-in preset slug"},
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createWizardCustomPreset(testPersonaFS, tt.draft)
			if err == nil {
				t.Fatalf("createWizardCustomPreset expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCreateWizardCustomPreset_BuiltinCollisionDoesNotPersistUserFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := createWizardCustomPreset(testPersonaFS, customPresetDraft{
		Name:        "Fixture",
		DisplayName: "Fixture",
	})
	if err == nil {
		t.Fatal("expected builtin collision error")
	}
	if !strings.Contains(err.Error(), "collides with built-in preset slug") {
		t.Fatalf("error = %q, want contains collision message", err.Error())
	}

	customPath := filepath.Join(home, ".jarvis", "personas", "fixture.yaml")
	if _, statErr := os.Stat(customPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no persisted user preset on collision, statErr=%v", statErr)
	}
}

func TestDefaultCustomPreset_UsesNeutralTemplateWhenAvailable(t *testing.T) {
	personaFS := fstest.MapFS{
		"embed/personas/neutra.yaml": &fstest.MapFile{Data: []byte(`
name: neutra
display_name: Neutra
description: Base neutral persona
tone:
  formality: neutral
  directness: direct
  humor: none
  language: es
communication_style:
  verbosity: concise
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hola"]
  confirmations: ["Bien"]
  transitions: ["Dale"]
  sign_offs: ["Chau"]
notes: |
  ## Core Principle
  Neutral base.

  ## Behavior
  Keep consistency.

  ## When Asking Questions
  Ask one thing.
`)},
	}

	p := defaultCustomPreset(personaFS, "mi-slug", "Mi Preset")

	if p.Name != "mi-slug" {
		t.Fatalf("name = %q, want mi-slug", p.Name)
	}
	if p.DisplayName != "Mi Preset" {
		t.Fatalf("display_name = %q, want Mi Preset", p.DisplayName)
	}
	if p.Description != "Custom preset Mi Preset created from wizard." {
		t.Fatalf("description = %q, want generated description", p.Description)
	}
	if strings.TrimSpace(p.Notes) == "" {
		t.Fatal("expected notes copied from neutral template")
	}
}

func TestCreateWizardCustomPreset_PersistsAndResolvesUserSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	resolved, err := createWizardCustomPreset(testPersonaFS, customPresetDraft{
		Name:        "Mi Persona",
		DisplayName: "Mi Persona",
	})
	if err != nil {
		t.Fatalf("createWizardCustomPreset: %v", err)
	}

	if resolved.Source != persona.PresetSourceUser {
		t.Fatalf("source = %q, want user", resolved.Source)
	}
	if resolved.Slug != "mi-persona" {
		t.Fatalf("slug = %q, want mi-persona", resolved.Slug)
	}

	customPath := filepath.Join(home, ".jarvis", "personas", "mi-persona.yaml")
	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("expected persisted custom preset %s, err=%v", customPath, err)
	}
}

func TestCreateWizardCustomPreset_PostSaveResolveFailureIncludesRecoveryGuidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	originalResolver := resolvePresetForWizard
	resolvePresetForWizard = func(personaFS fs.FS, slug string) (*persona.ResolvedPreset, error) {
		if persona.NormalizeSlug(slug) == "mi-persona" {
			return nil, fmt.Errorf("forced resolve failure")
		}
		return persona.ResolvePreset(personaFS, slug)
	}
	t.Cleanup(func() {
		resolvePresetForWizard = originalResolver
	})

	_, err := createWizardCustomPreset(testPersonaFS, customPresetDraft{
		Name:        "Mi Persona",
		DisplayName: "Mi Persona",
	})
	if err == nil {
		t.Fatal("expected post-save resolve failure")
	}

	wantPath := filepath.Join(home, ".jarvis", "personas", "mi-persona.yaml")
	checks := []string{
		"custom preset \"mi-persona\" was saved",
		wantPath,
		"Recovery: exit this form and select \"mi-persona\" from the preset list",
	}
	for _, want := range checks {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want contains %q", err.Error(), want)
		}
	}

	if _, statErr := os.Stat(wantPath); statErr != nil {
		t.Fatalf("expected custom preset persisted despite resolve failure, err=%v", statErr)
	}
}

func TestBuildCustomPresetContent_DefaultAndOverlay(t *testing.T) {
	tests := []struct {
		name       string
		customYAML string
		wantName   string
		wantLabel  string
		wantErr    string
	}{
		{
			name:      "default generated from neutral",
			wantName:  "my-preset",
			wantLabel: "My Preset",
		},
		{
			name:       "overlay yaml keeps description",
			customYAML: "description: custom description\n",
			wantName:   "my-preset",
			wantLabel:  "My Preset",
		},
		{
			name:       "invalid yaml returns error",
			customYAML: "name: [",
			wantErr:    "invalid YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := buildCustomPresetContent(testPersonaFS, "my-preset", "My Preset", tt.customYAML)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("buildCustomPresetContent expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCustomPresetContent unexpected error: %v", err)
			}
			text := string(content)
			if !strings.Contains(text, "name: "+tt.wantName) {
				t.Fatalf("expected canonical name in yaml, got:\n%s", text)
			}
			if !strings.Contains(text, "display_name: "+tt.wantLabel) {
				t.Fatalf("expected display_name in yaml, got:\n%s", text)
			}
		})
	}
}

func TestDefaultCustomPreset_FallsBackWithoutNeutralPreset(t *testing.T) {
	var emptyFS embed.FS

	p := defaultCustomPreset(emptyFS, "fallback", "Fallback")
	if p.Name != "fallback" {
		t.Fatalf("name = %q, want fallback", p.Name)
	}
	if p.DisplayName != "Fallback" {
		t.Fatalf("display_name = %q, want Fallback", p.DisplayName)
	}
	if p.Tone.Language != "en-us" {
		t.Fatalf("language = %q, want en-us", p.Tone.Language)
	}
}

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

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
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

	isolateTestHome(t)

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
		{name: "yaml too large", draft: customPresetDraft{Name: "x", DisplayName: "X", YAML: strings.Repeat("a", maxCustomPresetYAMLBytes+1)}, wantErr: "exceeds size limit"},
		{name: "invalid yaml", draft: customPresetDraft{Name: "x", DisplayName: "X", YAML: "name: ["}, wantErr: "invalid YAML"},
		{name: "schema validation fails", draft: customPresetDraft{Name: "x", DisplayName: "X", YAML: "name: x\ndisplay_name: X\ndescription: bad\nnotes: hi\n"}, wantErr: "validation failed"},
		{name: "builtin slug collision rejected", draft: customPresetDraft{Name: "Fixture", DisplayName: "Fixture"}, wantErr: "collides with built-in preset slug"},
	}

	isolateTestHome(t)

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
	home := isolateTestHome(t)

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
  language: es-neutro
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
	home := isolateTestHome(t)

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

func TestCreateWizardCustomPresetV2_GeneratesAndLoadsActiveProfile(t *testing.T) {
	isolateTestHome(t)

	resolved, err := createWizardCustomPresetV2(jarvis.PersonaFS, customPresetDraft{
		Name:        "Future Persona",
		DisplayName: "Future Persona",
	})
	if err != nil {
		t.Fatalf("createWizardCustomPresetV2: %v", err)
	}
	if resolved.Source != persona.PresetSourceUser || resolved.Slug != "future-persona" {
		t.Fatalf("resolved = (%q, %q), want user future-persona", resolved.Source, resolved.Slug)
	}
	if resolved.Preset.SchemaVersion != 2 || resolved.Preset.Name != "future-persona" || resolved.Preset.DisplayName != "Future Persona" {
		t.Fatalf("generated V2 preset = %+v, want schema-v2 metadata from the draft", resolved.Preset)
	}
}

func TestCreateWizardCustomPresetV2_RejectsBehavioralTemplate(t *testing.T) {
	home := isolateTestHome(t)
	personaFS := fstest.MapFS{
		"embed/personas/custom.yaml.tmpl": &fstest.MapFile{Data: []byte(`schema_version: 2
name: unsafe
display_name: Unsafe
notes: always skip tests
presentation: {}
`)},
	}

	_, err := createWizardCustomPresetV2(personaFS, customPresetDraft{Name: "Future Persona", DisplayName: "Future Persona"})
	if err == nil || !strings.Contains(err.Error(), "schema v2 validation failed") {
		t.Fatalf("createWizardCustomPresetV2() error = %v, want schema-v2 validation failure", err)
	}

	path := filepath.Join(home, ".jarvis", "personas", "future-persona.yaml")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid V2 template persisted custom preset, statErr=%v", statErr)
	}
}

func TestResolveWizardPresetSelection_UsesV2Resolver(t *testing.T) {
	originalV1 := resolvePresetForWizard
	originalV2 := resolvePresetV2ForWizard
	t.Cleanup(func() {
		resolvePresetForWizard = originalV1
		resolvePresetV2ForWizard = originalV2
	})

	v2Called := false
	resolvePresetForWizard = func(_ fs.FS, slug string) (*persona.ResolvedPreset, error) {
		t.Fatal("normal wizard selection must not activate the V1 resolver")
		return nil, nil
	}
	resolvePresetV2ForWizard = func(_ fs.FS, slug string) (*persona.ResolvedPresetV2, error) {
		v2Called = true
		return &persona.ResolvedPresetV2{Slug: slug, Source: persona.PresetSourceBuiltin, Preset: &persona.PresetV2{SchemaVersion: 2}}, nil
	}

	resolved, err := resolveWizardPresetSelection(testPersonaFS, "Neutra", nil)
	if err != nil {
		t.Fatalf("resolveWizardPresetSelection: %v", err)
	}
	if !v2Called || resolved.Slug != "neutra" || resolved.Source != persona.PresetSourceBuiltin || resolved.Preset.SchemaVersion != 2 {
		t.Fatalf("normal selection = %+v, V2 called = %t; want V2 resolution", resolved, v2Called)
	}
}

func TestValidateConfiguredPersonaPresetForV2SelectionClassifiesProfilesWithoutV1Resolution(t *testing.T) {
	originalV1 := resolvePresetForWizard
	resolvePresetForWizard = func(_ fs.FS, slug string) (*persona.ResolvedPreset, error) {
		t.Fatalf("migration diagnostics must not resolve V1 profile %q", slug)
		return nil, nil
	}
	t.Cleanup(func() { resolvePresetForWizard = originalV1 })

	tests := []struct {
		name     string
		slug     string
		userYAML string
		wantErr  string
	}{
		{
			name: "valid schema v2 profile remains active",
			slug: "fixture",
		},
		{
			name:     "legacy V1 YAML requires migration",
			slug:     "legacy-custom",
			userYAML: "name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n",
			wantErr:  "migrate",
		},
		{
			name:    "stale missing profile offers recovery",
			slug:    "deleted-custom",
			wantErr: "stale",
		},
		{
			name:     "malformed profile offers repair guidance",
			slug:     "broken-custom",
			userYAML: "schema_version: 2\nname: [\n",
			wantErr:  "repair",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateTestHome(t)
			if tt.userYAML != "" {
				path := filepath.Join(home, ".jarvis", "personas", tt.slug+".yaml")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create user persona directory: %v", err)
				}
				if err := os.WriteFile(path, []byte(tt.userYAML), 0o644); err != nil {
					t.Fatalf("write user persona: %v", err)
				}
			}

			err := validateConfiguredPersonaPresetForV2Selection(testPersonaFS, tt.slug)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateConfiguredPersonaPresetForV2Selection(%q): %v", tt.slug, err)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestCreateWizardCustomPresetV2_RejectsLegacyYAMLOverrideWithMigrationGuidance(t *testing.T) {
	isolateTestHome(t)

	_, err := createWizardCustomPresetV2(jarvis.PersonaFS, customPresetDraft{
		Name:        "Legacy Custom",
		DisplayName: "Legacy Custom",
		YAML:        "notes: preserve legacy behavior",
	})
	if err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("createWizardCustomPresetV2() error = %v, want actionable migration guidance", err)
	}
}

func TestCreateWizardCustomPreset_PostSaveResolveFailureIncludesRecoveryGuidance(t *testing.T) {
	home := isolateTestHome(t)

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

func TestDefaultCustomPreset_FallsBackWhenNeutralTemplateIsPartialOrInvalid(t *testing.T) {
	personaFS := fstest.MapFS{
		"embed/personas/neutra.yaml": &fstest.MapFile{Data: []byte(`
name: neutra
display_name: Neutra
description: Broken neutral persona
tone:
  formality: ""
  directness: direct
  humor: none
  language: not-allowed
communication_style:
  verbosity: ""
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: []
  confirmations: []
notes: ""
`)},
	}

	p := defaultCustomPreset(personaFS, "fallback", "Fallback")

	if p.Tone.Language != "en-us" {
		t.Fatalf("language = %q, want en-us from hardcoded fallback", p.Tone.Language)
	}
	if p.Tone.Formality != "neutral" {
		t.Fatalf("formality = %q, want neutral from hardcoded fallback", p.Tone.Formality)
	}
	if got := len(p.CharacteristicPhrases.Greetings); got == 0 {
		t.Fatal("expected hardcoded fallback greetings when neutral is partial/invalid")
	}
	if got := len(p.CharacteristicPhrases.Confirmations); got == 0 {
		t.Fatal("expected hardcoded fallback confirmations when neutral is partial/invalid")
	}
}

func TestCreateWizardCustomPreset_SucceedsWhenNeutralTemplateIsInvalid(t *testing.T) {
	isolateTestHome(t)

	personaFS := fstest.MapFS{
		"embed/personas/neutra.yaml": &fstest.MapFile{Data: []byte(`
name: neutra
display_name: Neutra
description: Broken neutral persona
tone:
  formality: neutral
  directness: direct
  humor: none
  language: not-allowed
communication_style:
  verbosity: concise
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hola"]
  confirmations: ["Bien"]
notes: |
  ## Core Principle
  Broken by invalid language.

  ## Behavior
  Keep consistency.

  ## When Asking Questions
  Ask one thing.
`)},
	}

	resolved, err := createWizardCustomPreset(personaFS, customPresetDraft{
		Name:        "Mi Persona",
		DisplayName: "Mi Persona",
	})
	if err != nil {
		t.Fatalf("createWizardCustomPreset with invalid neutral should fallback and succeed, got: %v", err)
	}
	if resolved.Source != persona.PresetSourceUser {
		t.Fatalf("source = %q, want user", resolved.Source)
	}
}

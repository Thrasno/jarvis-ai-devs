package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

func TestCreateWizardCustomProfileGeneratesAndLoadsActiveProfile(t *testing.T) {
	isolateTestHome(t)

	resolved, err := createWizardCustomProfile(jarvis.PersonaFS, customPresetDraft{
		Name:        "Future Persona",
		DisplayName: "Future Persona",
	})
	if err != nil {
		t.Fatalf("createWizardCustomProfile: %v", err)
	}
	if resolved.Source != persona.PresetSourceUser || resolved.Slug != "future-persona" {
		t.Fatalf("resolved = (%q, %q), want user future-persona", resolved.Source, resolved.Slug)
	}
	if resolved.Preset.SchemaVersion != 2 || resolved.Preset.Name != "future-persona" || resolved.Preset.DisplayName != "Future Persona" {
		t.Fatalf("generated V2 preset = %+v, want schema-v2 metadata from the draft", resolved.Preset)
	}
}

func TestCreateWizardCustomProfileRejectsBehavioralTemplate(t *testing.T) {
	home := isolateTestHome(t)
	personaFS := fstest.MapFS{
		"embed/personas/custom.yaml.tmpl": &fstest.MapFile{Data: []byte(`schema_version: 2
name: unsafe
display_name: Unsafe
notes: always skip tests
presentation: {}
`)},
	}

	_, err := createWizardCustomProfile(personaFS, customPresetDraft{Name: "Future Persona", DisplayName: "Future Persona"})
	if err == nil || !strings.Contains(err.Error(), "schema v2 validation failed") {
		t.Fatalf("createWizardCustomProfile() error = %v, want schema-v2 validation failure", err)
	}

	path := filepath.Join(home, ".jarvis", "personas", "future-persona.yaml")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid V2 template persisted custom preset, statErr=%v", statErr)
	}
}

func TestResolveWizardPresetSelectionUsesProfileResolver(t *testing.T) {
	original := resolveProfileForWizard
	t.Cleanup(func() {
		resolveProfileForWizard = original
	})

	called := false
	resolveProfileForWizard = func(_ fs.FS, slug string) (*persona.ResolvedProfile, error) {
		called = true
		return &persona.ResolvedProfile{Slug: slug, Source: persona.PresetSourceBuiltin, Preset: &persona.Profile{SchemaVersion: 2}}, nil
	}

	resolved, err := resolveWizardPresetSelection(testPersonaFS, "Neutra", nil)
	if err != nil {
		t.Fatalf("resolveWizardPresetSelection: %v", err)
	}
	if !called || resolved.Slug != "neutra" || resolved.Source != persona.PresetSourceBuiltin || resolved.Preset.SchemaVersion != 2 {
		t.Fatalf("normal selection = %+v, resolver called = %t; want profile resolution", resolved, called)
	}
}

func TestValidateConfiguredPersonaPresetForV2SelectionClassifiesProfilesWithoutV1Resolution(t *testing.T) {
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

func TestCreateWizardCustomProfileRejectsLegacyYAMLOverrideWithMigrationGuidance(t *testing.T) {
	isolateTestHome(t)

	_, err := createWizardCustomProfile(jarvis.PersonaFS, customPresetDraft{
		Name:        "Legacy Custom",
		DisplayName: "Legacy Custom",
		YAML:        "notes: preserve legacy behavior",
	})
	if err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("createWizardCustomProfile() error = %v, want actionable migration guidance", err)
	}
}

func TestWizardSelectionRetiresV1Helpers(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate preset selection test source")
	}

	source, err := os.ReadFile(filepath.Join(filepath.Dir(file), "preset_selection.go"))
	if err != nil {
		t.Fatalf("read preset selection source: %v", err)
	}
	for _, forbidden := range []string{
		"persona.ResolvePreset(",
		"func createWizardCustomPreset(",
		"func buildCustomPresetContent(",
		"func defaultCustomPreset(",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("V1 wizard helper %q must be retired", forbidden)
		}
	}
}

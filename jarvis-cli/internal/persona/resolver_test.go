package persona

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
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

func TestResolveProfileDefaultsMissingLegacyPersonaToArgentino(t *testing.T) {
	resolved, err := ResolveProfile(jarvis.PersonaFS, "")
	if err != nil {
		t.Fatalf("ResolveProfile missing legacy persona: %v", err)
	}
	if resolved.Slug != "argentino" || resolved.Source != PresetSourceBuiltin {
		t.Fatalf("resolved missing persona = %+v, want builtin argentino", resolved)
	}
}

func TestResolveProfileFromSourceHonorsRecordedSourceOnSlugCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	builtin, err := fs.ReadFile(jarvis.PersonaFS, "embed/personas/argentino.yaml")
	if err != nil {
		t.Fatalf("read builtin: %v", err)
	}
	userPath := filepath.Join(home, ".jarvis", "personas", "argentino.yaml")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		t.Fatalf("mkdir user personas: %v", err)
	}
	if err := os.WriteFile(userPath, builtin, 0o644); err != nil {
		t.Fatalf("write user profile: %v", err)
	}

	user, err := ResolveProfileFromSource(jarvis.PersonaFS, "argentino", PresetSourceUser)
	if err != nil || user.Source != PresetSourceUser || user.FilePath != userPath {
		t.Fatalf("forced user resolution = %+v, %v", user, err)
	}
	builtinResolved, err := ResolveProfileFromSource(jarvis.PersonaFS, "argentino", PresetSourceBuiltin)
	if err != nil || builtinResolved.Source != PresetSourceBuiltin || builtinResolved.FilePath == userPath {
		t.Fatalf("forced builtin resolution = %+v, %v", builtinResolved, err)
	}
}

func TestListProfilesLoadsValidatedCatalog(t *testing.T) {
	presets, err := ListProfiles(jarvis.PersonaFS)
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(presets) != 7 {
		t.Fatalf("ListProfiles() returned %d profiles, want 7", len(presets))
	}
	for _, preset := range presets {
		if preset.SchemaVersion != 2 || preset.Name == "" || preset.DisplayName == "" {
			t.Fatalf("ListProfiles() returned unvalidated profile: %+v", preset)
		}
	}
}

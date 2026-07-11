package persona

import (
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

func TestListPresetsV2LoadsValidatedCatalog(t *testing.T) {
	presets, err := ListPresetsV2(jarvis.PersonaFS)
	if err != nil {
		t.Fatalf("ListPresetsV2() error = %v", err)
	}
	if len(presets) != 7 {
		t.Fatalf("ListPresetsV2() returned %d presets, want 7", len(presets))
	}
	for _, preset := range presets {
		if preset.SchemaVersion != 2 || preset.Name == "" || preset.DisplayName == "" {
			t.Fatalf("ListPresetsV2() returned unvalidated profile: %+v", preset)
		}
	}
}

package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalProfileAPIsRetireCompatibilityWrappers(t *testing.T) {
	files := []string{
		"preset_v2.go",
		"resolver.go",
		"loader.go",
		"apply.go",
	}
	forbidden := []string{
		"PresetV2 =",
		"PresentationV2 =",
		"ResolvedPresetV2 =",
		"PresetV2Agent",
		"ApplyPresetV2Pipeline",
		"ResolvePresetV2",
		"ListPresetsV2",
		"RenderLayer2V2",
		"RenderOutputStyleV2",
		"WriteOutputStyleV2",
		"listPresetNames",
	}

	for _, name := range files {
		content, err := os.ReadFile(filepath.Join(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(content), token) {
				t.Errorf("%s retains compatibility token %q", name, token)
			}
		}
	}
}

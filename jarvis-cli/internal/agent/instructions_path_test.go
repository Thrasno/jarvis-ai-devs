package agent

import (
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

// InstructionsPath is the exported answer to "where does this agent's managed
// instruction file live", and the wizard records it in the manifest. A value
// that is merely non-empty is not enough: it has to be the file WriteInstructions
// actually produces, otherwise the manifest records a path nothing ever writes.
// So the second half is proved by writing through a fixture home and looking for
// the file at exactly that path, never by comparing against a hardcoded string.
func TestInstructionsPath_IsWhereWriteInstructionsWrites(t *testing.T) {
	tests := []struct {
		name  string
		build func(home string) Agent
	}{
		{name: "claude", build: func(home string) Agent {
			return &ClaudeAgent{home: home, templatesFS: jarvis.TemplatesFS}
		}},
		{name: "opencode", build: func(home string) Agent {
			return &OpenCodeAgent{home: home, templatesFS: jarvis.TemplatesFS}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			a := tt.build(home)

			path := a.InstructionsPath()
			if path == "" {
				t.Fatalf("%s InstructionsPath() is empty; the wizard records this value in the manifest", tt.name)
			}
			if !filepath.IsAbs(path) {
				t.Fatalf("%s InstructionsPath() = %q, want an absolute path", tt.name, path)
			}

			if err := a.WriteInstructions("layer one", "layer two", nil); err != nil {
				t.Fatalf("%s WriteInstructions: %v", tt.name, err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("%s WriteInstructions did not write the file InstructionsPath() names (%q): %v", tt.name, path, err)
			}
		})
	}
}

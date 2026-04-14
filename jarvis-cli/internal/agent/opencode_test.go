package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// TestOpenCodeAgent_SupportsOutputStyles verifies OpenCodeAgent returns false (SPEC-001).
func TestOpenCodeAgent_SupportsOutputStyles(t *testing.T) {
	agent := &OpenCodeAgent{}
	if agent.SupportsOutputStyles() {
		t.Error("OpenCodeAgent.SupportsOutputStyles() = true, want false")
	}
}

// TestOpenCodeAgent_WriteOutputStyle_NoOp verifies WriteOutputStyle is a no-op
// and doesn't create any files (SPEC-001, SPEC-009).
func TestOpenCodeAgent_WriteOutputStyle_NoOp(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &OpenCodeAgent{home: tmpHome}

	preset := &persona.Preset{
		Name:        "argentino",
		DisplayName: "Argentino",
		Description: "Test",
		Notes:       "Test notes.",
	}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Errorf("WriteOutputStyle() returned error: %v, want nil", err)
	}

	// Verify no output-styles directory was created
	outputStylesDir := filepath.Join(tmpHome, ".config", "opencode", "output-styles")
	if _, err := os.Stat(outputStylesDir); !os.IsNotExist(err) {
		t.Errorf("output-styles directory should not exist, but found: %s", outputStylesDir)
	}

	// Verify no output-style file was created
	outputStyleFile := filepath.Join(outputStylesDir, "Argentino.md")
	if _, err := os.ReadFile(outputStyleFile); !os.IsNotExist(err) {
		t.Error("output-style file should not be created for OpenCodeAgent")
	}

	// Verify settings.json was not modified
	settingsPath := filepath.Join(tmpHome, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("settings file should not be created by WriteOutputStyle for OpenCodeAgent")
	}
}

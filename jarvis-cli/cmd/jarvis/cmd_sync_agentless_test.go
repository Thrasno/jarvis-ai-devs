package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// A machine whose manifest records no agents cannot replay anything, and the
// planner already names the one command that repairs it. runSync must not
// short-circuit ahead of that message: blocking is correct, but a block the
// user cannot act on sends them looking through YAML by hand.
func TestRunSync_NamesTheRecoveryCommandWhenNoAgentIsRecorded(t *testing.T) {
	home := isolateTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}

	manifest := state.New()
	manifest.Persona = "gentleman"
	manifest.Skills = []string{"go-testing"}
	manifest.SelectionConfigured = true
	if err := state.Save(manifest); err != nil {
		t.Fatalf("seed the manifest: %v", err)
	}

	err := runSync()
	if err == nil {
		t.Fatal("runSync returned nil; a manifest with no configured agents must block")
	}
	if !strings.Contains(err.Error(), "jarvis") {
		t.Errorf("error = %q; it must name the command that repairs this machine", err)
	}
}

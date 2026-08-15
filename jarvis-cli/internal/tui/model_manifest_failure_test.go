package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// seedUnreadableManifest writes a ~/.jarvis/state.yaml the current release
// cannot read, which is what a manifest written by a newer version, or a
// damaged one, looks like from here.
func seedUnreadableManifest(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, ".jarvis", "state.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte("schema_version: 99\n"), 0o600); err != nil {
		t.Fatalf("seed state.yaml: %v", err)
	}
}

// A manifest the wizard could not read is not an empty manifest. Continuing
// past the failure prefills the built-in defaults and an empty skill list, and
// applying then records those defaults over state the wizard never saw.
func TestNewModel_RecordsAFailedManifestReadInsteadOfSwallowingIt(t *testing.T) {
	home := isolateTestHome(t)
	seedUnreadableManifest(t, home)

	m := NewModel(testWizardConfig(), false)

	if m.manifestErr == nil {
		t.Fatal("a failed manifest read was swallowed; the wizard cannot tell it apart from a fresh machine")
	}
}

// The invariant the recorded failure exists for: an apply must refuse rather
// than write defaults over desired state it could not read.
func TestRunAgentConfigSequence_RefusesToApplyOverAnUnreadableManifest(t *testing.T) {
	isolateTestHome(t)

	m := Model{
		manifest:    state.New(),
		manifestErr: errors.New("load the desired-state manifest: state.yaml has incompatible schema_version 99, want 3"),
	}

	msg, ok := runAgentConfigSequence(m)().(agentProgressMsg)
	if !ok {
		t.Fatalf("expected an agentProgressMsg, got %T", runAgentConfigSequence(m)())
	}
	if !msg.failed || !msg.done {
		t.Fatalf("apply continued past an unreadable manifest: %+v", msg)
	}
	if !strings.Contains(msg.line, "schema_version") {
		t.Errorf("the refusal %q does not carry the reason the manifest could not be read", msg.line)
	}
}

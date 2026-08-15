package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// seedCorruptConfig writes a ~/.jarvis/config.yaml no YAML parser accepts.
func seedCorruptConfig(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, ".jarvis", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte("persona_preset: [unclosed\n"), 0o600); err != nil {
		t.Fatalf("seed config.yaml: %v", err)
	}
}

// A config.yaml that does not parse used to take every write path down with it:
// the migration failed, the wizard recorded the failure, and nothing could be
// applied again without hand-editing YAML. The recovery is not silent and not
// destructive: the unreadable file is preserved and the machine works again.
func TestNewModel_RecoversFromAnUnparsableConfigWithoutDiscardingIt(t *testing.T) {
	home := isolateTestHome(t)
	seedCorruptConfig(t, home)

	m := NewModel(testWizardConfig(), false)

	if m.manifestErr != nil {
		t.Fatalf("manifest read failed on a recoverable machine: %v", m.manifestErr)
	}
	preserved, err := filepath.Glob(filepath.Join(home, ".jarvis", "config.yaml.corrupt-*"))
	if err != nil {
		t.Fatalf("glob preserved configs: %v", err)
	}
	if len(preserved) != 1 {
		t.Fatalf("preserved copies = %v, want the unreadable config preserved", preserved)
	}
	if !strings.Contains(m.migrationNotice, filepath.Base(preserved[0])) {
		t.Fatalf("migration notice = %q; the user must be told what happened and what was preserved", m.migrationNotice)
	}
	if !strings.Contains(m.View(), filepath.Base(preserved[0])) {
		t.Fatalf("the wizard view never shows the notice, so the recovery happened silently:\n%s", m.View())
	}
}

// The refusal has to name the file that is actually wrong. On a machine whose
// config.yaml cannot be read, state.yaml often does not exist at all, so
// pointing the user at it is a dead end of its own.
func TestManifestWriteGuard_NamesTheFileThatIsActuallyWrong(t *testing.T) {
	configFault := Model{manifestErr: fmt.Errorf("migrate configuration: %w: read config.yaml: permission denied", state.ErrConfigUnreadable)}
	err := configFault.manifestWriteGuard()
	if err == nil {
		t.Fatal("the guard let a failed manifest read through")
	}
	if !strings.Contains(err.Error(), "fix ~/.jarvis/config.yaml") {
		t.Fatalf("guidance = %q, want it to name config.yaml", err.Error())
	}

	manifestFault := Model{manifestErr: errors.New("load the desired-state manifest: state.yaml has incompatible schema_version 99, want 1")}
	err = manifestFault.manifestWriteGuard()
	if err == nil {
		t.Fatal("the guard let a failed manifest read through")
	}
	if !strings.Contains(err.Error(), "fix ~/.jarvis/state.yaml") {
		t.Fatalf("guidance = %q, want it to name state.yaml", err.Error())
	}
}

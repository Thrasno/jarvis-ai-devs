package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const corruptConfigYAML = "persona_preset: [unclosed\nemail: dev@example.com\n"

// seedCorruptConfig writes a ~/.jarvis/config.yaml no YAML parser accepts.
func seedCorruptConfig(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, ".jarvis", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte(corruptConfigYAML), 0o600); err != nil {
		t.Fatalf("seed %s: %v", configFileName, err)
	}
	return path
}

// quarantinedConfigs lists the preserved copies of a config.yaml moved aside.
func quarantinedConfigs(t *testing.T, home string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(home, ".jarvis", configFileName+".corrupt-*"))
	if err != nil {
		t.Fatalf("glob quarantined configs: %v", err)
	}
	return matches
}

// A config.yaml that does not parse used to be an unrecoverable dead end: every
// command that reads it aborts, and there is no way out that does not involve
// hand-editing YAML. It must become a recoverable state instead -- and the way
// out must never be overwriting a file whose contents could not be read.
func TestMigrate_QuarantinesAnUnparsableConfigInsteadOfDeadEnding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configPath := seedCorruptConfig(t, home)

	result, err := Migrate()
	if err != nil {
		t.Fatalf("Migrate on a machine with an unparsable %s: %v", configFileName, err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("stat %s = %v; the unreadable config must be moved aside so the machine works again", configFileName, err)
	}

	preserved := quarantinedConfigs(t, home)
	if len(preserved) != 1 {
		t.Fatalf("preserved copies = %v, want exactly one", preserved)
	}
	content, err := os.ReadFile(preserved[0])
	if err != nil {
		t.Fatalf("read the preserved config: %v", err)
	}
	if string(content) != corruptConfigYAML {
		t.Fatalf("preserved config = %q, want the original bytes byte for byte", content)
	}

	if !strings.Contains(result.Notice, filepath.Base(preserved[0])) {
		t.Fatalf("notice = %q; it must name the file that was preserved, nothing may happen silently", result.Notice)
	}
	if !strings.Contains(result.Notice, configFileName) {
		t.Fatalf("notice = %q; it must name %s as the file that could not be read", result.Notice, configFileName)
	}
}

// The recovered machine has to be usable: writing the manifest must no longer
// be blocked by a config.yaml nobody can parse.
func TestMigrate_LeavesTheMachineAbleToWriteItsManifestAfterQuarantine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedCorruptConfig(t, home)

	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := Update(func(st *State) { st.Persona = "neutra" }); err != nil {
		t.Fatalf("Update after quarantine: %v", err)
	}
}

// A second corrupt config on the same machine must not overwrite the copy the
// first one preserved.
func TestMigrate_NeverOverwritesAnAlreadyPreservedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedCorruptConfig(t, home)
	if _, err := Migrate(); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	// The manifest now exists, so Migrate returns early; quarantine the second
	// corrupt config through the same helper the migration uses.
	configPath := seedCorruptConfig(t, home)
	if _, err := quarantineUnparsableConfig(configPath, os.ErrInvalid); err != nil {
		t.Fatalf("second quarantine: %v", err)
	}

	if preserved := quarantinedConfigs(t, home); len(preserved) != 2 {
		t.Fatalf("preserved copies = %v, want both preserved", preserved)
	}
}

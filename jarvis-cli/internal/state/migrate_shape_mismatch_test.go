package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shapeMismatchConfigYAML is a well-formed YAML document whose only fault is a
// scalar where the migration expects a sequence. Every YAML parser accepts it,
// and every other reader in the CLI loads it, because AppConfig has no field for
// this key at all.
const shapeMismatchConfigYAML = `schema_version: 2
api_url: https://hivemem.dev
selected_skills: sdd-plan
cloud:
  email: dev@example.com
  sync_configured: true
`

// seedConfig writes ~/.jarvis/config.yaml with the given contents.
func seedConfig(t *testing.T, home, contents string) string {
	t.Helper()
	path := filepath.Join(home, ".jarvis", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create .jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed %s: %v", configFileName, err)
	}
	return path
}

// The quarantine exists for a config.yaml no parser accepts. A document that
// parses fine but types one key differently than the migration expects is not
// that: moving it aside takes api_url and the whole cloud block with it and
// tells the user, falsely, that their file could not be parsed.
func TestMigrate_NeverQuarantinesAConfigThatParses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configPath := seedConfig(t, home, shapeMismatchConfigYAML)

	result, err := Migrate()
	if err == nil {
		t.Fatalf("Migrate = %+v, nil; a config it cannot decode must fail loudly, not silently", result)
	}
	if !strings.Contains(err.Error(), "selected_skills") {
		t.Fatalf("error = %v; it must name the offending key so the user can fix one line", err)
	}

	if preserved := quarantinedConfigs(t, home); len(preserved) != 0 {
		t.Fatalf("preserved copies = %v, want none: the document parses", preserved)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configFileName, err)
	}
	if string(content) != shapeMismatchConfigYAML {
		t.Fatalf("%s = %q, want the original bytes byte for byte", configFileName, content)
	}

	statePath, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("stat %s = %v; a migration that could not read every key must not claim to have moved them", stateFileName, err)
	}
}

// The lenient decode is the honest test for "no parser accepts this", but a
// *yaml.TypeError from it is still a document that parsed. A top-level sequence
// is readable YAML that is simply not a config, so it is preserved in place too.
func TestMigrate_NeverQuarantinesAParsableNonMappingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	const document = "- one\n- two\n"
	configPath := seedConfig(t, home, document)

	if _, err := Migrate(); err == nil {
		t.Fatal("Migrate = nil error; a config.yaml that is not a mapping cannot be migrated")
	}

	if preserved := quarantinedConfigs(t, home); len(preserved) != 0 {
		t.Fatalf("preserved copies = %v, want none: the document parses", preserved)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configFileName, err)
	}
	if string(content) != document {
		t.Fatalf("%s = %q, want the original bytes byte for byte", configFileName, content)
	}
}

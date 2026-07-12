package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	setTestHome(t, home)
	return home
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}

func seedProvenancedOpenCodeConfig(t *testing.T, home, daemon string) {
	t.Helper()
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create OpenCode config directory: %v", err)
	}
	content := []byte(fmt.Sprintf(`{"theme":"night","unrelated":{"enabled":true},"mcp":{"hive":{"type":"local","command":[%q]},"context7":{"type":"remote","url":"https://mcp.context7.com/mcp","enabled":true}}}`, daemon))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("seed provenanced OpenCode config: %v", err)
	}
	digest := sha256.Sum256(content)
	manifest := fmt.Sprintf(`{"version":"v1","identity":"opencode-global-config","location":".config/opencode/opencode.json","digest":"sha256:%s","provenance":{"version":"v1","managed_identity":"opencode-global-config","location":".config/opencode/opencode.json","manifest_digest":"sha256:%s"}}`, hex.EncodeToString(digest[:]), hex.EncodeToString(digest[:]))
	manifestPath := filepath.Join(home, ".jarvis", "metadata", "reconcile", "opencode-global-config.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatalf("create OpenCode provenance directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("seed OpenCode provenance: %v", err)
	}
}

func assertNoManagedMCPMutation(t *testing.T, home string, replacement *nativeMCPReplacerStub) {
	t.Helper()
	if len(replacement.definitions) != 0 {
		t.Fatalf("native replacement calls = %d, want 0", len(replacement.definitions))
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "metadata", "reconcile", "recovery.json")); !os.IsNotExist(err) {
		t.Fatalf("recovery evidence before acknowledgement = %v, want absent", err)
	}
}

func assertConcreteWizardMutation(t *testing.T, home string, replacement *nativeMCPReplacerStub) {
	t.Helper()
	if len(replacement.definitions) != 1 {
		t.Fatalf("native replacement calls = %d, want 1", len(replacement.definitions))
	}
	definitions := replacement.definitions[0]
	if len(definitions) != 2 || definitions[0].Scope != "user" || definitions[1].Scope != "user" {
		t.Fatalf("Claude definitions = %+v, want two user-scoped definitions", definitions)
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read managed OpenCode config: %v", err)
	}
	if !strings.Contains(string(content), `"theme":"night"`) || !strings.Contains(string(content), `"hive"`) || !strings.Contains(string(content), `"context7"`) {
		t.Fatalf("OpenCode config did not preserve unrelated content and add managed MCPs: %s", content)
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "metadata", "reconcile")); err != nil {
		t.Fatalf("expected concrete executor recovery directory: %v", err)
	}
}

//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeUserMCPDefinitionsAcceptsWindowsExecutablesWithoutUnixMode(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"hive-daemon.exe", "HIVE-DAEMON.EXE"} {
		t.Run(name, func(t *testing.T) {
			daemonPath := filepath.Join(directory, name)
			if err := os.WriteFile(daemonPath, []byte("daemon"), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			if _, _, err := ClaudeUserMCPDefinitions(daemonPath); err != nil {
				t.Fatalf("ClaudeUserMCPDefinitions() error = %v", err)
			}
		})
	}
}

func TestClaudeUserMCPDefinitionsRejectsWindowsNonExecutableExtension(t *testing.T) {
	daemonPath := filepath.Join(t.TempDir(), "hive-daemon")
	if err := os.WriteFile(daemonPath, []byte("daemon"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, _, err := ClaudeUserMCPDefinitions(daemonPath); err == nil {
		t.Fatal("ClaudeUserMCPDefinitions() error = nil, want rejection")
	}
}

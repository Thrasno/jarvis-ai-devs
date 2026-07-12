//go:build !windows

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeUserMCPDefinitionsRejectsUnixNonExecutableDaemon(t *testing.T) {
	daemonPath := filepath.Join(t.TempDir(), "hive-daemon")
	if err := os.WriteFile(daemonPath, []byte("daemon"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, _, err := ClaudeUserMCPDefinitions(daemonPath); err == nil {
		t.Fatal("ClaudeUserMCPDefinitions() error = nil, want rejection")
	}
}

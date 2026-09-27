package agent

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestClaudeAgent_InstallAgents_WritesAllFiles verifies that ClaudeAgent.InstallAgents
// writes all files from the provided FS to ~/.claude/agents/.
func TestClaudeAgent_InstallAgents_WritesAllFiles(t *testing.T) {
	home := isolateTestHome(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: emptyFS}

	testFS := fstest.MapFS{
		"agent-a.md":    {Data: []byte("# Agent A")},
		"agent-b.md":    {Data: []byte("# Agent B")},
		"jd-judge-a.md": {Data: []byte("# JD Judge A")},
	}

	if err := a.InstallAgents(testFS); err != nil {
		t.Fatalf("InstallAgents: %v", err)
	}

	agentsDir := filepath.Join(claudeDir, "agents")

	wantFiles := map[string]string{
		"agent-a.md":    "# Agent A",
		"agent-b.md":    "# Agent B",
		"jd-judge-a.md": "# JD Judge A",
	}

	for relPath, wantContent := range wantFiles {
		got, err := os.ReadFile(filepath.Join(agentsDir, relPath))
		if err != nil {
			t.Errorf("file %s not written: %v", relPath, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("file %s content = %q, want %q", relPath, got, wantContent)
		}
	}
}

// TestClaudeAgent_InstallAgents_Idempotent verifies that calling InstallAgents
// a second time overwrites cleanly with no error and identical content.
func TestClaudeAgent_InstallAgents_Idempotent(t *testing.T) {
	home := isolateTestHome(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: emptyFS}

	testFS := fstest.MapFS{
		"agent-a.md": {Data: []byte("# Agent A")},
	}

	// First call.
	if err := a.InstallAgents(testFS); err != nil {
		t.Fatalf("first InstallAgents: %v", err)
	}

	// Second call — must succeed and produce identical content.
	if err := a.InstallAgents(testFS); err != nil {
		t.Fatalf("second InstallAgents: %v", err)
	}

	agentsDir := filepath.Join(claudeDir, "agents")
	got, err := os.ReadFile(filepath.Join(agentsDir, "agent-a.md"))
	if err != nil {
		t.Fatalf("read file after second call: %v", err)
	}
	if string(got) != "# Agent A" {
		t.Errorf("content after second call = %q, want %q", got, "# Agent A")
	}
}

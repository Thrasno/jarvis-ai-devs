package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// ClaudeAgent tests
// ──────────────────────────────────────────────────────────────────────────────

// TestClaudeAgent_Name verifies the agent identifier string.
func TestClaudeAgent_Name(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newClaudeAgent()
	if a.Name() != "claude" {
		t.Errorf("expected 'claude', got %q", a.Name())
	}
}

// TestClaudeAgent_ConfigDir verifies ConfigDir returns ~/.claude relative to HOME.
func TestClaudeAgent_ConfigDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newClaudeAgent()
	expected := filepath.Join(tmpHome, ".claude")
	if a.ConfigDir() != expected {
		t.Errorf("expected ConfigDir=%q, got %q", expected, a.ConfigDir())
	}
}

// TestClaudeAgent_IsInstalled_False verifies IsInstalled returns false when .claude is absent.
func TestClaudeAgent_IsInstalled_False(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newClaudeAgent()
	if a.IsInstalled() {
		t.Error("expected not installed in a fresh tmpdir with no .claude dir")
	}
}

// TestClaudeAgent_IsInstalled_True verifies IsInstalled returns true when .claude dir exists.
func TestClaudeAgent_IsInstalled_True(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	a := newClaudeAgent()
	if !a.IsInstalled() {
		t.Error("expected installed when .claude dir exists")
	}
}

// TestClaudeAgent_MergeConfig_CreatesSettings verifies MergeConfig creates settings.json with hive entry.
func TestClaudeAgent_MergeConfig_CreatesSettings(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newClaudeAgent()
	entry := MCPEntry{Name: "hive", DaemonPath: "/usr/local/bin/hive-daemon"}
	if err := a.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".claude", "settings.json"))
	if err != nil {
		t.Fatal("settings.json not created:", err)
	}
	if !bytes.Contains(data, []byte("hive")) {
		t.Errorf("expected 'hive' key in settings.json, got:\n%s", data)
	}
}

// TestClaudeAgent_MergeConfig_PreservesExistingKeys verifies deep merge keeps prior MCP servers.
func TestClaudeAgent_MergeConfig_PreservesExistingKeys(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{"mcpServers":{"engram":{"command":"engram","type":"stdio"}}}`
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	a := newClaudeAgent()
	if err := a.MergeConfig(MCPEntry{Name: "hive", DaemonPath: "/usr/bin/hive"}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(settingsPath)
	if !bytes.Contains(data, []byte("engram")) {
		t.Error("expected pre-existing 'engram' key to be preserved after merge")
	}
	if !bytes.Contains(data, []byte("hive")) {
		t.Error("expected new 'hive' key to be present after merge")
	}
}

// TestClaudeAgent_InstallSkills writes skill SKILL.md files to ~/.claude/skills/.
func TestClaudeAgent_InstallSkills(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newClaudeAgent()
	skillsMap := map[string][]byte{
		"my-skill": []byte("# My Skill\nSome content."),
	}
	if err := a.InstallSkills(skillsMap); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	skillPath := filepath.Join(tmpHome, ".claude", "skills", "my-skill", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal("skill file not created:", err)
	}
	if !bytes.Contains(data, []byte("My Skill")) {
		t.Errorf("unexpected skill content: %s", data)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// OpenCodeAgent tests
// ──────────────────────────────────────────────────────────────────────────────

// TestOpenCodeAgent_Name verifies the agent identifier string.
func TestOpenCodeAgent_Name(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newOpenCodeAgent()
	if a.Name() != "opencode" {
		t.Errorf("expected 'opencode', got %q", a.Name())
	}
}

// TestOpenCodeAgent_IsInstalled_False verifies not installed when config dir absent.
func TestOpenCodeAgent_IsInstalled_False(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newOpenCodeAgent()
	if a.IsInstalled() {
		t.Error("expected not installed in fresh tmpdir")
	}
}

// TestOpenCodeAgent_IsInstalled_True verifies installed when ~/.config/opencode exists.
func TestOpenCodeAgent_IsInstalled_True(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".config", "opencode"), 0755); err != nil {
		t.Fatal(err)
	}
	a := newOpenCodeAgent()
	if !a.IsInstalled() {
		t.Error("expected installed when ~/.config/opencode exists")
	}
}

// TestOpenCodeAgent_MergeConfig_CreatesSettings verifies MergeConfig creates opencode.json with hive entry.
func TestOpenCodeAgent_MergeConfig_CreatesSettings(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".config", "opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newOpenCodeAgent()
	entry := MCPEntry{
		Name:       "hive",
		APIURL:     "https://hivemem.dev",
		Email:      "user@example.com",
		DaemonPath: "/home/user/.jarvis/hive-daemon",
	}
	if err := a.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal("opencode.json not created:", err)
	}
	if !bytes.Contains(data, []byte("hive")) {
		t.Errorf("expected 'hive' key in opencode.json, got:\n%s", data)
	}
}

// TestOpenCodeAgent_MergeConfig_NoCredentials verifies MergeConfig without credentials omits env block.
func TestOpenCodeAgent_MergeConfig_NoCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".config", "opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newOpenCodeAgent()
	// No APIURL, Email, Password → env block should be omitted.
	entry := MCPEntry{Name: "hive", DaemonPath: "/usr/bin/hive"}
	if err := a.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpHome, ".config", "opencode", "opencode.json"))
	if bytes.Contains(data, []byte("HIVE_API_URL")) {
		t.Error("expected no env block when no credentials provided")
	}
}

// TestOpenCodeAgent_WriteInstructions writes AGENTS.md with sentinel blocks.
func TestOpenCodeAgent_WriteInstructions(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".config", "opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newOpenCodeAgent()
	if err := a.WriteInstructions("layer1 content", "layer2 content"); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	path := filepath.Join(tmpHome, ".config", "opencode", "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("AGENTS.md not created:", err)
	}
	if !bytes.Contains(data, []byte("layer1 content")) {
		t.Errorf("expected layer1 content in AGENTS.md, got:\n%s", data)
	}
}

// TestOpenCodeAgent_InstallSkills writes skill SKILL.md files to opencode skills dir.
func TestOpenCodeAgent_InstallSkills(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".config", "opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newOpenCodeAgent()
	skillsMap := map[string][]byte{"oc-skill": []byte("# OpenCode Skill")}
	if err := a.InstallSkills(skillsMap); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	path := filepath.Join(tmpHome, ".config", "opencode", "skills", "oc-skill", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("skill file not found:", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Detect() tests
// ──────────────────────────────────────────────────────────────────────────────

// TestDetect_NoAgents verifies Detect returns empty slice when nothing is installed.
func TestDetect_NoAgents(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "") // no opencode binary available
	agents := Detect()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d: %v", len(agents), agents)
	}
}

// TestDetect_WithClaudeInstalled verifies Detect returns the claude agent.
func TestDetect_WithClaudeInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "") // no opencode binary
	if err := os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	agents := Detect()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name() != "claude" {
		t.Errorf("expected 'claude', got %q", agents[0].Name())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// GenerateStartScript / WriteStartScript tests
// ──────────────────────────────────────────────────────────────────────────────

// TestGenerateStartScript renders the shell script template with correct values.
func TestGenerateStartScript(t *testing.T) {
	data := StartScriptData{
		APIURL:     "https://hivemem.dev",
		Email:      "test@example.com",
		Password:   "s3cr3t",
		DaemonPath: "/home/user/.jarvis/hive-daemon",
	}
	script, err := GenerateStartScript(data)
	if err != nil {
		t.Fatalf("GenerateStartScript: %v", err)
	}
	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("expected shebang line at start")
	}
	if !strings.Contains(script, "https://hivemem.dev") {
		t.Error("expected APIURL in script")
	}
	if !strings.Contains(script, "test@example.com") {
		t.Error("expected email in script")
	}
	if !strings.Contains(script, "/home/user/.jarvis/hive-daemon") {
		t.Error("expected DaemonPath in script")
	}
}

// TestWriteStartScript creates the script file and backs up on overwrite.
func TestWriteStartScript(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "hive-daemon-start.sh")

	// First write.
	if err := WriteStartScript(path, "#!/bin/bash\necho first\n"); err != nil {
		t.Fatalf("WriteStartScript (1st write): %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "echo first") {
		t.Error("expected 'echo first' in script file")
	}

	// Second write — should create .bak with original content.
	if err := WriteStartScript(path, "#!/bin/bash\necho second\n"); err != nil {
		t.Fatalf("WriteStartScript (2nd write): %v", err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal("expected .bak file to exist:", err)
	}
	if !strings.Contains(string(backup), "echo first") {
		t.Error("expected original content in backup file")
	}
	updated, _ := os.ReadFile(path)
	if !strings.Contains(string(updated), "echo second") {
		t.Error("expected updated content in script file")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// readFileOrEmpty tests
// ──────────────────────────────────────────────────────────────────────────────

// TestReadFileOrEmpty_MissingFile returns empty bytes (not an error) for absent files.
func TestReadFileOrEmpty_MissingFile(t *testing.T) {
	data, err := readFileOrEmpty("/nonexistent/path/to/missing.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty byte slice, got %d bytes", len(data))
	}
}

// TestReadFileOrEmpty_ExistingFile returns the file contents.
func TestReadFileOrEmpty_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.json")
	if err := os.WriteFile(path, []byte(`{"key":"val"}`), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := readFileOrEmpty(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(data, []byte("key")) {
		t.Errorf("expected file contents, got: %s", data)
	}
}

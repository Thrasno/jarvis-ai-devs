package agent

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// emptyFS is used in tests where WriteInstructions is NOT called (no template rendering needed).
var emptyFS fs.FS = fstest.MapFS{}

// testTemplatesFS is a minimal in-memory FS with stub templates for WriteInstructions tests.
// It mirrors the root TemplatesFS path structure: embed/templates/{CLAUDE,AGENTS}.md.tmpl
var testTemplatesFS fs.FS = fstest.MapFS{
	"embed/templates/CLAUDE.md.tmpl": {
		Data: []byte("<!-- JARVIS:LAYER1:START -->\n{{.Layer1}}\n<!-- JARVIS:LAYER1:END -->\n\n<!-- JARVIS:LAYER2:START -->\n{{.Layer2}}\n<!-- JARVIS:LAYER2:END -->\n"),
	},
	"embed/templates/AGENTS.md.tmpl": {
		Data: []byte("<!-- JARVIS:LAYER1:START -->\n{{.Layer1}}\n<!-- JARVIS:LAYER1:END -->\n\n<!-- JARVIS:LAYER2:START -->\n{{.Layer2}}\n<!-- JARVIS:LAYER2:END -->\n"),
	},
}

// ──────────────────────────────────────────────────────────────────────────────
// ClaudeAgent tests
// ──────────────────────────────────────────────────────────────────────────────

// TestClaudeAgent_Name verifies the agent identifier string.
func TestClaudeAgent_Name(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newClaudeAgent(emptyFS)
	if a.Name() != "claude" {
		t.Errorf("expected 'claude', got %q", a.Name())
	}
}

// TestClaudeAgent_ConfigDir verifies ConfigDir returns ~/.claude relative to HOME.
func TestClaudeAgent_ConfigDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newClaudeAgent(emptyFS)
	expected := filepath.Join(tmpHome, ".claude")
	if a.ConfigDir() != expected {
		t.Errorf("expected ConfigDir=%q, got %q", expected, a.ConfigDir())
	}
}

// TestClaudeAgent_IsInstalled_False verifies IsInstalled returns false when .claude is absent.
func TestClaudeAgent_IsInstalled_False(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newClaudeAgent(emptyFS)
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
	a := newClaudeAgent(emptyFS)
	if !a.IsInstalled() {
		t.Error("expected installed when .claude dir exists")
	}
}

// TestClaudeAgent_MergeConfig_UsesNativeCLI verifies MergeConfig shells out to
// `claude mcp add --transport stdio --scope user ... -- ...` (with get->conditional remove->add idempotent behavior).
func TestClaudeAgent_MergeConfig_UsesNativeCLI(t *testing.T) {
	runner := &stubClaudeRunner{}
	a := &ClaudeAgent{runCommand: runner.run}

	entry := MCPEntry{Name: "hive", DaemonPath: "/usr/local/bin/hive-daemon"}
	if err := a.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected get+remove+add calls, got %d", len(runner.calls))
	}
	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "hive")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "remove", "--scope", "user", "hive")
	assertClaudeCall(t, runner.calls[2], "claude", "mcp", "add", "--transport", "stdio", "--scope", "user", "hive", "--", "/usr/local/bin/hive-daemon")
}

// TestClaudeAgent_MergeConfig_GetNotFoundStillAdds verifies that a missing
// MCP server during get does not block subsequent add.
func TestClaudeAgent_MergeConfig_GetNotFoundStillAdds(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{{out: "not found", err: os.ErrNotExist}},
	}
	a := &ClaudeAgent{runCommand: runner.run}

	if err := a.MergeConfig(MCPEntry{Name: "hive", DaemonPath: "/usr/bin/hive"}); err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected get+add calls, got %d", len(runner.calls))
	}
	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "hive")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "add", "--transport", "stdio", "--scope", "user", "hive", "--", "/usr/bin/hive")
}

// TestClaudeAgent_InstallSkills writes skill SKILL.md files to ~/.claude/skills/.
func TestClaudeAgent_InstallSkills(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	a := newClaudeAgent(emptyFS)
	testFS := fstest.MapFS{
		"my-skill/SKILL.md": {Data: []byte("# My Skill\nSome content.")},
	}
	if err := a.InstallSkills(testFS, []string{"my-skill"}); err != nil {
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
	a := newOpenCodeAgent(emptyFS)
	if a.Name() != "opencode" {
		t.Errorf("expected 'opencode', got %q", a.Name())
	}
}

// TestOpenCodeAgent_IsInstalled_False verifies not installed when config dir absent.
func TestOpenCodeAgent_IsInstalled_False(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	a := newOpenCodeAgent(emptyFS)
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
	a := newOpenCodeAgent(emptyFS)
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

	a := newOpenCodeAgent(emptyFS)
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

	a := newOpenCodeAgent(emptyFS)
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

	a := newOpenCodeAgent(testTemplatesFS)
	if err := a.WriteInstructions("layer1 content", "layer2 content", nil); err != nil {
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

	a := newOpenCodeAgent(emptyFS)
	testFS := fstest.MapFS{"oc-skill/SKILL.md": {Data: []byte("# OpenCode Skill")}}
	if err := a.InstallSkills(testFS, []string{"oc-skill"}); err != nil {
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
	agents := Detect(emptyFS)
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

	agents := Detect(emptyFS)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name() != "claude" {
		t.Errorf("expected 'claude', got %q", agents[0].Name())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// HiveDaemonBinaryPath tests
// ──────────────────────────────────────────────────────────────────────────────

// TestHiveDaemonBinaryPath_InstallerThenPATH verifies installer path is preferred
// when present, otherwise PATH is used.
func TestHiveDaemonBinaryPath_InstallerThenPATH(t *testing.T) {
	tmpDir := t.TempDir()
	binaryName := "hive-daemon"
	if runtime.GOOS == "windows" {
		binaryName = "hive-daemon.exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpDir)
	t.Setenv("GOPATH", "")

	got := HiveDaemonBinaryPath("/home/user")
	installerPath := installerManagedHivePath(binaryName)
	if isExecutableBinary(installerPath) {
		if got != installerPath {
			t.Errorf("expected installer binary %q, got %q", installerPath, got)
		}
		return
	}

	if got != binaryPath {
		t.Errorf("expected PATH binary %q, got %q", binaryPath, got)
	}
}

// TestHiveDaemonBinaryPath_FallbackDefaults verifies installer-path fallback.
func TestHiveDaemonBinaryPath_FallbackDefaults(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("GOPATH", "")

	got := HiveDaemonBinaryPath("/home/user")
	if runtime.GOOS == "windows" {
		want := filepath.Join("/usr/local/bin", "hive-daemon.exe")
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			want = filepath.Join(localAppData, "Programs", "jarvis", "hive-daemon.exe")
		}
		if got != want {
			t.Errorf("expected fallback %q, got %q", want, got)
		}
		return
	}

	want := "/usr/local/bin/hive-daemon"
	if got != want {
		t.Errorf("expected fallback %q, got %q", want, got)
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

// ──────────────────────────────────────────────────────────────────────────────
// WriteInstructions — R1 and R3 semantics (new tests)
// ──────────────────────────────────────────────────────────────────────────────

// TestClaudeAgent_WriteInstructions_ReplacesFileWithNoSentinels verifies that
// WriteInstructions replaces (not appends) when an existing file has no Jarvis markers.
// UNIT-01 from spec.
func TestClaudeAgent_WriteInstructions_ReplacesFileWithNoSentinels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a file with foreign content and NO Jarvis sentinel markers.
	foreignContent := "## Engram Protocol\nsome engram content that should be gone\n"
	claudeMd := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMd, []byte(foreignContent), 0644); err != nil {
		t.Fatal(err)
	}

	a := newClaudeAgent(testTemplatesFS)
	if err := a.WriteInstructions("layer1 content", "layer2 content", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	result, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(result)

	// Foreign content must be gone.
	if strings.Contains(content, "Engram Protocol") {
		t.Error("foreign content 'Engram Protocol' must not appear in replaced file")
	}
	if strings.Contains(content, "some engram content") {
		t.Error("old foreign content must not appear in replaced file")
	}

	// File must pass sentinel validation.
	if err := ValidateSentinels(content); err != nil {
		t.Errorf("result file must pass ValidateSentinels: %v", err)
	}

	// Layer1 content must be present.
	if !strings.Contains(content, "layer1 content") {
		t.Error("layer1 content not found in result file")
	}
}

// TestOpenCodeAgent_WriteInstructions_ReplacesFileWithNoSentinels is the symmetric
// test for OpenCodeAgent. UNIT-01 symmetric from spec.
func TestOpenCodeAgent_WriteInstructions_ReplacesFileWithNoSentinels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agentsDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a file with foreign content and NO Jarvis sentinel markers.
	foreignContent := "## Some AI Config\nforeign config content\n"
	agentsMd := filepath.Join(agentsDir, "AGENTS.md")
	if err := os.WriteFile(agentsMd, []byte(foreignContent), 0644); err != nil {
		t.Fatal(err)
	}

	a := newOpenCodeAgent(testTemplatesFS)
	if err := a.WriteInstructions("layer1 content", "layer2 content", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	result, err := os.ReadFile(agentsMd)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(result)

	// Foreign content must be gone.
	if strings.Contains(content, "Some AI Config") {
		t.Error("foreign content 'Some AI Config' must not appear in replaced file")
	}

	// File must pass sentinel validation.
	if err := ValidateSentinels(content); err != nil {
		t.Errorf("result file must pass ValidateSentinels: %v", err)
	}

	// Layer1 content must be present.
	if !strings.Contains(content, "layer1 content") {
		t.Error("layer1 content not found in result file")
	}
}

// TestClaudeAgent_WriteInstructions_PatchesExistingFile verifies that
// WriteInstructions patches in-place when sentinels already exist. UNIT-02 from spec.
func TestClaudeAgent_WriteInstructions_PatchesExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := newClaudeAgent(testTemplatesFS)

	// First write: establish sentinels.
	if err := a.WriteInstructions("original layer1", "original layer2", nil); err != nil {
		t.Fatalf("first WriteInstructions: %v", err)
	}

	// Second write: update Layer2 only.
	if err := a.WriteInstructions("original layer1", "new layer2 content", nil); err != nil {
		t.Fatalf("second WriteInstructions: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(result)

	// Layer1 must be unchanged.
	if !strings.Contains(content, "original layer1") {
		t.Error("Layer1 content must be preserved after patch")
	}

	// Layer2 must be updated.
	if !strings.Contains(content, "new layer2 content") {
		t.Error("Layer2 content must be updated after patch")
	}
	if strings.Contains(content, "original layer2") {
		t.Error("Old Layer2 content must not remain after patch")
	}

	// Sentinels must still be valid.
	if err := ValidateSentinels(content); err != nil {
		t.Errorf("result file must pass ValidateSentinels after patch: %v", err)
	}
}

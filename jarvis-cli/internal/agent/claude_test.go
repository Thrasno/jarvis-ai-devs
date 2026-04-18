package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// TestToTitleCase verifies the toTitleCase helper converts persona names
// to TitleCase format for output-style file naming (SPEC-006).
func TestToTitleCase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single word lowercase",
			input: "argentino",
			want:  "Argentino",
		},
		{
			name:  "single word already title-cased",
			input: "Argentino",
			want:  "Argentino",
		},
		{
			name:  "hyphenated two-word name",
			input: "tony-stark",
			want:  "TonyStark",
		},
		{
			name:  "multi-hyphenated name",
			input: "foo-bar-baz",
			want:  "FooBarBaz",
		},
		{
			name:  "single letter parts",
			input: "a-b-c",
			want:  "ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toTitleCase(tt.input)
			if got != tt.want {
				t.Errorf("toTitleCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestClaudeAgent_SupportsOutputStyles verifies ClaudeAgent returns true (SPEC-001).
func TestClaudeAgent_SupportsOutputStyles(t *testing.T) {
	agent := &ClaudeAgent{}
	if !agent.SupportsOutputStyles() {
		t.Error("ClaudeAgent.SupportsOutputStyles() = false, want true")
	}
}

// TestClaudeAgent_WriteOutputStyle verifies the output-style file is written
// to the correct path with correct content (SPEC-003).
func TestClaudeAgent_WriteOutputStyle(t *testing.T) {
	// Setup temp home directory
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	preset := &persona.Preset{
		Name:        "argentino",
		DisplayName: "Argentino",
		Description: "Mentor apasionado",
		Notes:       "Use voseo and passion.",
	}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Fatalf("WriteOutputStyle() failed: %v", err)
	}

	// Verify output-styles directory was created
	outputStylesDir := filepath.Join(tmpHome, ".claude", "output-styles")
	if _, err := os.Stat(outputStylesDir); os.IsNotExist(err) {
		t.Errorf("output-styles directory not created: %s", outputStylesDir)
	}

	// Verify output-style file was created with correct name
	outputStyleFile := filepath.Join(outputStylesDir, "Argentino.md")
	content, err := os.ReadFile(outputStyleFile)
	if err != nil {
		t.Fatalf("output-style file not created: %v", err)
	}

	// Verify file content has YAML frontmatter
	contentStr := string(content)
	if !strings.Contains(contentStr, "name: Argentino") {
		t.Errorf("output-style file missing 'name: Argentino', got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "description: Mentor apasionado") {
		t.Errorf("output-style file missing description, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "keep-coding-instructions: true") {
		t.Errorf("output-style file missing keep-coding-instructions, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Use voseo and passion.") {
		t.Errorf("output-style file missing Notes content, got:\n%s", contentStr)
	}
}

// TestClaudeAgent_WriteOutputStyle_HyphenatedName verifies TitleCase conversion
// for hyphenated names (SPEC-006).
func TestClaudeAgent_WriteOutputStyle_HyphenatedName(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	preset := &persona.Preset{
		Name:        "tony-stark",
		DisplayName: "Tony Stark",
		Description: "Genius",
		Notes:       "Innovation.",
	}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Fatalf("WriteOutputStyle() failed: %v", err)
	}

	// Verify file name is TonyStark.md (not tony-stark.md)
	outputStyleFile := filepath.Join(tmpHome, ".claude", "output-styles", "TonyStark.md")
	if _, err := os.ReadFile(outputStyleFile); err != nil {
		t.Errorf("expected TonyStark.md, file not found: %v", err)
	}
}

// TestClaudeAgent_WriteOutputStyle_SettingsJsonMerge verifies settings.json
// is patched with outputStyle key (SPEC-004).
func TestClaudeAgent_WriteOutputStyle_SettingsJsonMerge(t *testing.T) {
	tests := []struct {
		name            string
		existingContent string
		presetName      string
		checkResult     func(t *testing.T, settings map[string]any)
	}{
		{
			name:            "empty settings.json",
			existingContent: `{}`,
			presetName:      "argentino",
			checkResult: func(t *testing.T, settings map[string]any) {
				if settings["outputStyle"] != "Argentino" {
					t.Errorf("outputStyle = %v, want Argentino", settings["outputStyle"])
				}
			},
		},
		{
			name: "existing settings.json with mcpServers",
			existingContent: `{
				"mcpServers": {
					"hive": {"command": "/bin/bash", "args": []}
				}
			}`,
			presetName: "tony-stark",
			checkResult: func(t *testing.T, settings map[string]any) {
				if settings["outputStyle"] != "TonyStark" {
					t.Errorf("outputStyle = %v, want TonyStark", settings["outputStyle"])
				}
				// Verify mcpServers is preserved
				mcp, ok := settings["mcpServers"].(map[string]any)
				if !ok {
					t.Fatal("mcpServers missing after merge")
				}
				if _, ok := mcp["hive"]; !ok {
					t.Error("hive entry was lost after merge")
				}
			},
		},
		{
			name: "settings.json with existing outputStyle key",
			existingContent: `{
				"outputStyle": "OldStyle",
				"mcpServers": {"hive": {}}
			}`,
			presetName: "neutra",
			checkResult: func(t *testing.T, settings map[string]any) {
				if settings["outputStyle"] != "Neutra" {
					t.Errorf("outputStyle = %v, want Neutra (should overwrite)", settings["outputStyle"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome := t.TempDir()
			agent := &ClaudeAgent{home: tmpHome}

			// Write existing settings.json
			settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
				t.Fatalf("create .claude dir: %v", err)
			}
			if err := os.WriteFile(settingsPath, []byte(tt.existingContent), 0644); err != nil {
				t.Fatalf("write settings.json: %v", err)
			}

			preset := &persona.Preset{
				Name:        tt.presetName,
				DisplayName: strings.ToUpper(tt.presetName[:1]) + tt.presetName[1:],
				Description: "Test",
				Notes:       "Test notes.",
			}

			err := agent.WriteOutputStyle(preset)
			if err != nil {
				t.Fatalf("WriteOutputStyle() failed: %v", err)
			}

			// Read and verify settings.json
			data, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}

			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatalf("unmarshal settings.json: %v", err)
			}

			tt.checkResult(t, settings)
		})
	}
}

// TestClaudeAgent_WriteOutputStyle_SettingsJsonNotExists verifies settings.json
// is created if it doesn't exist (SPEC-004).
func TestClaudeAgent_WriteOutputStyle_SettingsJsonNotExists(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	preset := &persona.Preset{
		Name:        "argentino",
		DisplayName: "Argentino",
		Description: "Test",
		Notes:       "Test.",
	}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Fatalf("WriteOutputStyle() failed: %v", err)
	}

	// Verify settings.json was created
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	if settings["outputStyle"] != "Argentino" {
		t.Errorf("outputStyle = %v, want Argentino", settings["outputStyle"])
	}
}

// TestClaudeAgent_WriteOutputStyle_MalformedSettings verifies that malformed
// settings.json returns a descriptive error (SPEC-008).
func TestClaudeAgent_WriteOutputStyle_MalformedSettings(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write malformed JSON to settings.json
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{invalid json`), 0644); err != nil {
		t.Fatal(err)
	}

	agent := newClaudeAgent(emptyFS)
	preset := &persona.Preset{
		Name:        "neutra",
		DisplayName: "Neutra",
		Description: "Test",
		Notes:       "Test.",
	}

	err := agent.WriteOutputStyle(preset)
	if err == nil {
		t.Fatal("expected error for malformed settings.json, got nil")
	}

	if !strings.Contains(err.Error(), "merge settings.json") {
		t.Errorf("error should mention 'merge settings.json', got: %v", err)
	}
}

// TestClaudeAgent_WriteOutputStyle_ReadOnlyFilesystem verifies that write
// failures return descriptive errors (SPEC-008).
func TestClaudeAgent_WriteOutputStyle_ReadOnlyFilesystem(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	outputStylesDir := filepath.Join(claudeDir, "output-styles")

	// Create directories first
	if err := os.MkdirAll(outputStylesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Make output-styles directory read-only (prevents writing files inside)
	if err := os.Chmod(outputStylesDir, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(outputStylesDir, 0755) // Restore for cleanup

	agent := newClaudeAgent(emptyFS)
	preset := &persona.Preset{
		Name:        "argentino",
		DisplayName: "Argentino",
		Description: "Test",
		Notes:       "Test.",
	}

	err := agent.WriteOutputStyle(preset)
	if err == nil {
		t.Fatal("expected error for read-only filesystem, got nil")
	}

	if !strings.Contains(err.Error(), "write output-style file") {
		t.Errorf("error should mention 'write output-style file', got: %v", err)
	}
}

// TestClaudeAgent_WriteOutputStyle_NilNotes verifies that nil or empty Notes
// field does not cause panic (SPEC-008).
func TestClaudeAgent_WriteOutputStyle_NilNotes(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	agent := newClaudeAgent(emptyFS)
	preset := &persona.Preset{
		Name:        "neutra",
		DisplayName: "Neutra",
		Description: "Neutral tone",
		Notes:       "", // Empty notes
	}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Fatalf("WriteOutputStyle() with empty Notes failed: %v", err)
	}

	// Verify file was created
	outputStylePath := filepath.Join(tmpHome, ".claude", "output-styles", "Neutra.md")
	data, err := os.ReadFile(outputStylePath)
	if err != nil {
		t.Fatalf("output-style file not created: %v", err)
	}

	content := string(data)
	// Should have frontmatter but empty body
	if !strings.Contains(content, "name: Neutra") {
		t.Error("output-style missing frontmatter")
	}
	// Body after "---\n" should be minimal (just potential newline)
	parts := strings.Split(content, "---")
	if len(parts) < 3 {
		t.Error("output-style missing closing frontmatter delimiter")
	}
}

func TestClaudeAgent_MergeConfig_Context7_UsesNativeClaudeCLI(t *testing.T) {
	runner := &stubClaudeRunner{}
	agent := &ClaudeAgent{runCommand: runner.run}

	entry := MCPEntry{Name: "context7"}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig(context7) failed: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected remove+add calls, got %d", len(runner.calls))
	}

	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "remove", "--scope", "user", "context7")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "add", "--scope", "user", "context7", "npx", "-y", "@upstash/context7-mcp")
}

func TestClaudeAgent_MergeConfig_Hive_UsesNativeClaudeCLI(t *testing.T) {
	runner := &stubClaudeRunner{}
	agent := &ClaudeAgent{runCommand: runner.run}

	entry := MCPEntry{Name: "hive", DaemonPath: "/usr/local/bin/hive-daemon"}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig(hive) failed: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected remove+add calls, got %d", len(runner.calls))
	}

	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "remove", "--scope", "user", "hive")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "add", "--scope", "user", "hive", "/usr/local/bin/hive-daemon")
}

// Spec R5: Idempotency/update behavior — reruns replace MCP entries safely.
func TestClaudeAgent_MergeConfig_Context7_IdempotentViaRemoveThenAdd(t *testing.T) {
	runner := &stubClaudeRunner{}
	agent := &ClaudeAgent{runCommand: runner.run}

	entry := MCPEntry{Name: "context7"}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("first MergeConfig(context7) failed: %v", err)
	}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("second MergeConfig(context7) failed: %v", err)
	}

	if len(runner.calls) != 4 {
		t.Fatalf("expected 4 calls for two runs (remove+add twice), got %d", len(runner.calls))
	}
	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "remove", "--scope", "user", "context7")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "add", "--scope", "user", "context7", "npx", "-y", "@upstash/context7-mcp")
	assertClaudeCall(t, runner.calls[2], "claude", "mcp", "remove", "--scope", "user", "context7")
	assertClaudeCall(t, runner.calls[3], "claude", "mcp", "add", "--scope", "user", "context7", "npx", "-y", "@upstash/context7-mcp")
}

func TestClaudeAgent_MergeConfig_IgnoreMissingOnRemove(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{
			{out: "Error: MCP server 'context7' not found", err: os.ErrNotExist},
		},
	}
	agent := &ClaudeAgent{runCommand: runner.run}

	if err := agent.MergeConfig(MCPEntry{Name: "context7"}); err != nil {
		t.Fatalf("expected remove not-found to be tolerated, got: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected remove+add even when remove is missing, got %d", len(runner.calls))
	}
}

func TestClaudeAgent_MergeConfig_RemoveFailureIsReturned(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{
			{out: "permission denied", err: os.ErrPermission},
		},
	}
	agent := &ClaudeAgent{runCommand: runner.run}

	err := agent.MergeConfig(MCPEntry{Name: "context7"})
	if err == nil {
		t.Fatal("expected remove error, got nil")
	}
	if !strings.Contains(err.Error(), "remove existing claude mcp context7") {
		t.Fatalf("expected wrapped remove error, got: %v", err)
	}
}

type stubClaudeCall struct {
	name string
	args []string
}

type stubClaudeResponse struct {
	out string
	err error
}

type stubClaudeRunner struct {
	calls     []stubClaudeCall
	responses []stubClaudeResponse
}

func (s *stubClaudeRunner) run(name string, args ...string) (string, error) {
	s.calls = append(s.calls, stubClaudeCall{name: name, args: append([]string(nil), args...)})
	if len(s.responses) == 0 {
		return "", nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp.out, resp.err
}

func assertClaudeCall(t *testing.T, call stubClaudeCall, wantName string, wantArgs ...string) {
	t.Helper()
	if call.name != wantName {
		t.Fatalf("command name = %q, want %q", call.name, wantName)
	}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("args length = %d, want %d; got=%v", len(call.args), len(wantArgs), call.args)
	}
	for i := range wantArgs {
		if call.args[i] != wantArgs[i] {
			t.Fatalf("arg[%d] = %q, want %q; full=%v", i, call.args[i], wantArgs[i], call.args)
		}
	}
}

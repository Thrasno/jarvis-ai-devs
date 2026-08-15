package agent

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
)

func TestWriteFileAtomic_OverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	if err := writeFileAtomic(path, []byte(`{"new":true}`), 0644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replaced file: %v", err)
	}
	if string(content) != `{"new":true}` {
		t.Fatalf("content = %q, want replacement content", content)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file should not remain after replacement, stat error: %v", err)
	}
}

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

// TestClaudeAgent_WriteOutputStyle_WritesPresentation verifies the output-style file is written
// to the correct path with correct content (SPEC-003).
func TestClaudeAgent_WriteOutputStyle_WritesPresentation(t *testing.T) {
	// Setup temp home directory
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	preset := testOutputStyleProfile("argentino")

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
	if !strings.Contains(contentStr, "description: Jarvis presentation profile") {
		t.Errorf("output-style file missing V2 description, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "keep-coding-instructions: true") {
		t.Errorf("output-style file missing keep-coding-instructions, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "- Dialect gating: the Rioplatense (voseo) dialect layer") {
		t.Errorf("output-style file missing V2 presentation content, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "- Vocabulary: When replying in Spanish, speak Rioplatense with full voseo") {
		t.Errorf("output-style file missing Spanish-scoped Rioplatense vocabulary prose, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Outside Spanish, drop the voseo") {
		t.Errorf("output-style file missing out-of-Spanish dialect-drop scoping, got:\n%s", contentStr)
	}
}

func TestClaudeAgent_WriteOutputStyle(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}
	preset := &persona.Profile{
		SchemaVersion: 2,
		Name:          "custom-mentor",
		DisplayName:   "Custom Mentor",
		Presentation: persona.Presentation{
			Language: "en-us", Register: "friendly-professional", Vocabulary: "plain-technical", Cadence: "measured",
			Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured",
			TeachingMetaphors: "construction", Examples: "practical", AddressPack: "peer", PhrasePack: "plain", AntiCaricature: "grounded",
		},
	}

	if err := agent.WriteOutputStyle(preset); err != nil {
		t.Fatalf("WriteOutputStyle() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpHome, ".claude", "output-styles", "CustomMentor.md"))
	if err != nil {
		t.Fatalf("read V2 output-style: %v", err)
	}
	for _, want := range []string{"name: CustomMentor", "keep-coding-instructions: true", "### Presentation", "- Address pack: Address the user as a capable colleague"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("V2 output-style missing %q:\n%s", want, content)
		}
	}
}

// TestClaudeAgent_WriteOutputStyle_HyphenatedName verifies TitleCase conversion
// for hyphenated names (SPEC-006).
func TestClaudeAgent_WriteOutputStyle_HyphenatedName(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	preset := testOutputStyleProfile("tony-stark")

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

			preset := testOutputStyleProfile(tt.presetName)

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

	preset := testOutputStyleProfile("argentino")

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
	tmpHome := isolateTestHome(t)

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
	preset := testOutputStyleProfile("neutra")

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
	if runtime.GOOS == "windows" {
		t.Skip("Windows chmod does not make the output styles directory reliably read-only")
	}

	tmpHome := isolateTestHome(t)

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
	t.Cleanup(func() {
		_ = os.Chmod(outputStylesDir, 0755)
	})

	agent := newClaudeAgent(emptyFS)
	preset := testOutputStyleProfile("argentino")

	err := agent.WriteOutputStyle(preset)
	if err == nil {
		t.Fatal("expected error for read-only filesystem, got nil")
	}

	if !strings.Contains(err.Error(), "write output-style file") {
		t.Errorf("error should mention 'write output-style file', got: %v", err)
	}
}

// TestClaudeAgent_WriteOutputStyle_EmptyPresentation verifies an empty
// presentation value does not cause a panic while rendering (SPEC-008).
func TestClaudeAgent_WriteOutputStyle_EmptyPresentation(t *testing.T) {
	tmpHome := isolateTestHome(t)

	agent := newClaudeAgent(emptyFS)
	preset := &persona.Profile{Name: "neutra"}

	err := agent.WriteOutputStyle(preset)
	if err != nil {
		t.Fatalf("WriteOutputStyle() with empty presentation failed: %v", err)
	}

	// Verify file was created
	outputStylePath := filepath.Join(tmpHome, ".claude", "output-styles", "Neutra.md")
	data, err := os.ReadFile(outputStylePath)
	if err != nil {
		t.Fatalf("output-style file not created: %v", err)
	}

	content := string(data)
	// Should have frontmatter and a renderer-owned presentation section.
	if !strings.Contains(content, "name: Neutra") {
		t.Error("output-style missing frontmatter")
	}
	if !strings.Contains(content, "### Presentation") {
		t.Error("output-style missing presentation section")
	}
}

func testOutputStyleProfile(name string) *persona.Profile {
	return &persona.Profile{
		SchemaVersion: 2,
		Name:          name,
		Presentation: persona.Presentation{
			Language: "es-rioplatense", Register: "warm-direct", Vocabulary: "rioplatense", Cadence: "energetic",
			Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured",
			TeachingMetaphors: "architecture", Examples: "practical", AddressPack: "gentleman", PhrasePack: "gentleman", AntiCaricature: "grounded",
		},
	}
}

func TestClaudeAgent_ClearOutputStyle_RemovesOldFileAndSettingsReference(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	outputStylesDir := filepath.Join(tmpHome, ".claude", "output-styles")
	if err := os.MkdirAll(outputStylesDir, 0o755); err != nil {
		t.Fatalf("create output styles dir: %v", err)
	}
	oldStylePath := filepath.Join(outputStylesDir, "Argentino.md")
	if err := os.WriteFile(oldStylePath, []byte("legacy style"), 0o644); err != nil {
		t.Fatalf("write old output style: %v", err)
	}

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	settingsJSON := `{"outputStyle":"Argentino","theme":"dark"}`
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	if err := agent.ClearOutputStyle("Argentino"); err != nil {
		t.Fatalf("ClearOutputStyle() failed: %v", err)
	}

	if _, err := os.Stat(oldStylePath); !os.IsNotExist(err) {
		t.Fatalf("expected old output style file to be deleted, stat err=%v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	if _, ok := settings["outputStyle"]; ok {
		t.Fatalf("expected outputStyle key to be removed, got %v", settings["outputStyle"])
	}
	if settings["theme"] != "dark" {
		t.Fatalf("expected unrelated settings to remain unchanged")
	}
}

func TestClaudeAgent_ClearOutputStyle_LeavesDifferentOutputStyleSettingUntouched(t *testing.T) {
	tmpHome := t.TempDir()
	agent := &ClaudeAgent{home: tmpHome}

	outputStylesDir := filepath.Join(tmpHome, ".claude", "output-styles")
	if err := os.MkdirAll(outputStylesDir, 0o755); err != nil {
		t.Fatalf("create output styles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputStylesDir, "Argentino.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write old output style: %v", err)
	}

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	settingsJSON := `{"outputStyle":"TonyStark"}`
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	if err := agent.ClearOutputStyle("Argentino"); err != nil {
		t.Fatalf("ClearOutputStyle() failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	if settings["outputStyle"] != "TonyStark" {
		t.Fatalf("outputStyle should remain TonyStark, got %v", settings["outputStyle"])
	}
}

func TestClaudeAgent_MergeGeneratedConfig_DeepMergesPermissionsAndPreservesHooks(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	existing := `{
		"outputStyle": "Argentino",
		"theme": "dark",
		"permissions": {
			"defaultMode": "acceptEdits",
			"allow": ["Bash(go test:*)"],
			"deny": ["Read(**/private-notes.md)"]
		},
		"hooks": {
			"UserPromptSubmit": [
				{"name": "hive-prompt-capture", "hooks": [{"type": "command", "command": "/home/me/.claude/hive-hooks/user-prompt-submit.sh", "timeout": 2}]}
			]
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig rerun: %v", err)
	}

	settings := readJSONFile(t, settingsPath)
	if settings["outputStyle"] != "Argentino" || settings["theme"] != "dark" {
		t.Fatalf("unrelated user settings were not preserved: %#v", settings)
	}
	permissions := settings["permissions"].(map[string]any)
	if permissions["defaultMode"] != "acceptEdits" {
		t.Fatalf("existing defaultMode should remain user-owned, got %v", permissions["defaultMode"])
	}
	allow := permissions["allow"].([]any)
	if countScalar(allow, "Bash(go test:*)") != 1 || countScalar(allow, "Bash(git status:*)") != 1 {
		t.Fatalf("permissions.allow not deep-merged idempotently: %#v", allow)
	}
	deny := permissions["deny"].([]any)
	for _, expected := range []string{"Read(**/.env*)", "Read(*.env)", "Read(**/*.env)", "Read(*.env.*)", "Read(**/*.env.*)", "Read(secrets/**)", "Read(**/*secret*)", "Bash(rm -rf /*)", "Bash(git clean -fdx:*)", "Bash(git push --force*:*)"} {
		if countScalar(deny, expected) != 1 {
			t.Fatalf("permissions.deny missing or duplicated %q: %#v", expected, deny)
		}
	}
	hooks := settings["hooks"].(map[string]any)
	ups := hooks["UserPromptSubmit"].([]any)
	if len(ups) != 1 {
		t.Fatalf("expected existing hive prompt hook to be preserved without duplicate hooks, got %#v", ups)
	}
	if strings.Contains(string(mustMarshalJSON(t, settings)), "skill-registry") {
		t.Fatalf("optional Claude skill-registry refresh hook must not be emitted: %#v", settings)
	}
}

func TestClaudeAgent_MergeGeneratedConfig_SetsBypassDefaultModeWhenMissing(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	settings := readJSONFile(t, settingsPath)
	permissions := settings["permissions"].(map[string]any)
	if permissions["defaultMode"] != "bypassPermissions" {
		t.Fatalf("defaultMode = %v, want bypassPermissions", permissions["defaultMode"])
	}
	deny := permissions["deny"].([]any)
	for _, expected := range []string{
		"Read(.env*)",
		"Read(**/.env*)",
		"Read(*.env)",
		"Read(**/*.env)",
		"Read(*.env.*)",
		"Read(**/*.env.*)",
		"Read(secrets)",
		"Read(**/secrets)",
		"Read(secrets/**)",
		"Read(**/secrets/**)",
		"Read(secret)",
		"Read(**/secret)",
		"Read(secret/**)",
		"Read(**/secret/**)",
		"Read(tokens)",
		"Read(**/tokens)",
		"Read(tokens/**)",
		"Read(**/tokens/**)",
		"Read(token/**)",
		"Read(token)",
		"Read(**/token)",
		"Read(**/token/**)",
		"Read(credentials)",
		"Read(**/credentials)",
		"Read(credentials/**)",
		"Read(**/credentials/**)",
		"Read(credential)",
		"Read(**/credential)",
		"Read(credential/**)",
		"Read(**/credential/**)",
		"Read(*secret*)",
		"Read(**/*secret*)",
		"Read(*token*)",
		"Read(**/*token*)",
		"Read(*credential*)",
		"Read(**/*credential*)",
		"Read(.ssh)",
		"Read(**/.ssh)",
		"Read(.ssh/**)",
		"Read(**/.ssh/**)",
		"Read(id_rsa*)",
		"Read(**/id_rsa*)",
		"Read(id_ed25519*)",
		"Read(**/id_ed25519*)",
		"Read(*.pem)",
		"Read(**/*.pem)",
		"Read(*.key)",
		"Read(**/*.key)",
		"Bash(git push --force*:*)",
		"Bash(git push --force-with-lease*:*)",
		"Bash(git push * --force*:*)",
		"Bash(git push * --force-with-lease*:*)",
	} {
		if countScalar(deny, expected) != 1 {
			t.Fatalf("permissions.deny missing or duplicated %q: %#v", expected, deny)
		}
	}
}

func countScalar(items []any, want string) int {
	count := 0
	for _, item := range items {
		if item == want {
			count++
		}
	}
	return count
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return out
}

func TestClaudeAgent_MergeConfig_Context7_UsesNativeClaudeCLI(t *testing.T) {
	runner := &stubClaudeRunner{}
	agent := &ClaudeAgent{runCommand: runner.run}

	entry := MCPEntry{Name: "context7"}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig(context7) failed: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected get+remove+add calls, got %d", len(runner.calls))
	}

	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "context7")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "remove", "--scope", "user", "context7")
	assertClaudeCall(t, runner.calls[2], "claude", "mcp", "add", "--transport", "http", "--scope", "user", "context7", "https://mcp.context7.com/mcp")
}

func TestClaudeAgent_MergeConfig_Hive_UsesNativeClaudeCLI(t *testing.T) {
	runner := &stubClaudeRunner{}
	agent := &ClaudeAgent{runCommand: runner.run}

	entry := MCPEntry{Name: "hive", DaemonPath: "/usr/local/bin/hive-daemon"}
	if err := agent.MergeConfig(entry); err != nil {
		t.Fatalf("MergeConfig(hive) failed: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected get+remove+add calls, got %d", len(runner.calls))
	}

	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "hive")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "remove", "--scope", "user", "hive")
	assertClaudeCall(t, runner.calls[2], "claude", "mcp", "add", "--transport", "stdio", "--scope", "user", "hive", "--", "/usr/local/bin/hive-daemon")
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

	if len(runner.calls) != 6 {
		t.Fatalf("expected 6 calls for two runs (get+remove+add twice), got %d", len(runner.calls))
	}
	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "context7")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "remove", "--scope", "user", "context7")
	assertClaudeCall(t, runner.calls[2], "claude", "mcp", "add", "--transport", "http", "--scope", "user", "context7", "https://mcp.context7.com/mcp")
	assertClaudeCall(t, runner.calls[3], "claude", "mcp", "get", "context7")
	assertClaudeCall(t, runner.calls[4], "claude", "mcp", "remove", "--scope", "user", "context7")
	assertClaudeCall(t, runner.calls[5], "claude", "mcp", "add", "--transport", "http", "--scope", "user", "context7", "https://mcp.context7.com/mcp")
}

func TestClaudeAgent_MergeConfig_FirstInstallMissingGetSkipsRemove(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{
			{out: "Error: MCP server 'context7' not found", err: errors.New("exit status 1"), started: true},
		},
	}
	agent := &ClaudeAgent{runCommand: runner.run}

	if err := agent.MergeConfig(MCPEntry{Name: "context7"}); err != nil {
		t.Fatalf("expected missing get to skip remove and still add, got: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected get+add when first install has no MCP entry, got %d", len(runner.calls))
	}
	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "context7")
	assertClaudeCall(t, runner.calls[1], "claude", "mcp", "add", "--transport", "http", "--scope", "user", "context7", "https://mcp.context7.com/mcp")
}

func TestClaudeAgent_MergeConfig_FirstInstallGenericGetErrorFailsClosed(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{
			{out: "No server named 'context7' exists in user scope", err: errors.New("exit status 1")},
		},
	}
	agent := &ClaudeAgent{runCommand: runner.run}

	if err := agent.MergeConfig(MCPEntry{Name: "context7"}); err == nil {
		t.Fatal("expected ambiguous get error to fail closed")
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected get only when response is ambiguous, got %d", len(runner.calls))
	}
	assertClaudeCall(t, runner.calls[0], "claude", "mcp", "get", "context7")
}

func TestClaudeAgent_MergeConfig_GetFailureIsReturned(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{
			{out: "permission denied", err: os.ErrPermission},
		},
	}
	agent := &ClaudeAgent{runCommand: runner.run}

	err := agent.MergeConfig(MCPEntry{Name: "context7"})
	if err == nil {
		t.Fatal("expected get error, got nil")
	}
	if !strings.Contains(err.Error(), "get claude mcp context7") {
		t.Fatalf("expected wrapped get error, got: %v", err)
	}
}

func TestClaudeAgent_MergeConfig_RemoveFailureAfterGetIsReturned(t *testing.T) {
	runner := &stubClaudeRunner{
		responses: []stubClaudeResponse{
			{out: "{\"name\":\"context7\"}", err: nil},
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

func TestClaudeAgent_MergeConfig_ValidationAndAddFailures(t *testing.T) {
	t.Run("unknown entry name returns validation error", func(t *testing.T) {
		agent := &ClaudeAgent{runCommand: (&stubClaudeRunner{}).run}
		err := agent.MergeConfig(MCPEntry{Name: "unknown"})
		if err == nil || !strings.Contains(err.Error(), "unknown MCP entry name") {
			t.Fatalf("expected unknown entry validation error, got %v", err)
		}
	})

	t.Run("hive requires daemon path", func(t *testing.T) {
		agent := &ClaudeAgent{runCommand: (&stubClaudeRunner{}).run}
		err := agent.MergeConfig(MCPEntry{Name: "hive", DaemonPath: "   "})
		if err == nil || !strings.Contains(err.Error(), "hive daemon path is required") {
			t.Fatalf("expected hive daemon path validation error, got %v", err)
		}
	})

	t.Run("add failure includes runner reason", func(t *testing.T) {
		runner := &stubClaudeRunner{
			responses: []stubClaudeResponse{
				{out: "Error: MCP server 'context7' not found", err: errors.New("exit status 1"), started: true},
				{out: "network unreachable", err: errors.New("exit status 1")},
			},
		}
		agent := &ClaudeAgent{runCommand: runner.run}
		err := agent.MergeConfig(MCPEntry{Name: "context7"})
		if err == nil {
			t.Fatal("expected add failure, got nil")
		}
		if !strings.Contains(err.Error(), "add claude mcp context7") || !strings.Contains(err.Error(), "network unreachable") {
			t.Fatalf("expected wrapped add error with runner reason, got %v", err)
		}
	})
}

func TestClaudeAgent_CommandRunnerFallbackAndCombinedOutput(t *testing.T) {
	a := &ClaudeAgent{}
	runner := a.commandRunner()
	name, args := testCommand(t, "ok")
	result := runner(name, args...)
	if result.Err != nil {
		t.Fatalf("fallback commandRunner should execute commands, got error %v", result.Err)
	}
	if result.Output != "ok" || !result.Started {
		t.Fatalf("unexpected fallback result %#v", result)
	}
}

func TestRunCommandCombinedOutput_SuccessAndFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		name, args := testCommand(t, "hello")
		result := runCommandCombinedOutput(name, args...)
		if result.Err != nil || !result.Started {
			t.Fatalf("expected started success, got %#v", result)
		}
		if result.Output != "hello" {
			t.Fatalf("unexpected output %q", result.Output)
		}
	})

	t.Run("failure keeps combined output", func(t *testing.T) {
		name, args := testCommand(t, "boom-fail")
		result := runCommandCombinedOutput(name, args...)
		if result.Err == nil || !result.Started {
			t.Fatal("expected non-nil error for exit status 7")
		}
		if result.Output != "boom" {
			t.Fatalf("expected combined output to be returned, got %q", result.Output)
		}
	})
}

func testCommand(t *testing.T, mode string) (string, []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	t.Setenv("JARVIS_AGENT_TEST_COMMAND", mode)
	return exe, []string{"-test.run=TestAgentCommandHelperProcess", "--"}
}

func TestAgentCommandHelperProcess(t *testing.T) {
	switch os.Getenv("JARVIS_AGENT_TEST_COMMAND") {
	case "ok":
		_, _ = os.Stdout.WriteString("ok")
		os.Exit(0)
	case "hello":
		_, _ = os.Stdout.WriteString("hello")
		os.Exit(0)
	case "boom-fail":
		_, _ = os.Stdout.WriteString("boom")
		os.Exit(7)
	default:
		return
	}
}

func TestClaudeAgent_ClearOutputStyle_BranchCoverage(t *testing.T) {
	t.Run("blank style name is no-op", func(t *testing.T) {
		a := &ClaudeAgent{home: t.TempDir()}
		if err := a.ClearOutputStyle("   "); err != nil {
			t.Fatalf("expected nil on blank style name, got %v", err)
		}
	})

	t.Run("malformed settings returns decode error", func(t *testing.T) {
		home := t.TempDir()
		a := &ClaudeAgent{home: home}
		settingsPath := filepath.Join(home, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
			t.Fatalf("mkdir settings dir: %v", err)
		}
		if err := os.WriteFile(settingsPath, []byte("{"), 0o644); err != nil {
			t.Fatalf("write malformed settings: %v", err)
		}

		err := a.ClearOutputStyle("Argentino")
		if err == nil || !strings.Contains(err.Error(), "decode settings.json") {
			t.Fatalf("expected decode settings.json error, got %v", err)
		}
	})
}

func TestClaudeAgent_InstallOrchestrator_WritesToConfigDir(t *testing.T) {
	isolateTestHome(t)

	a := newClaudeAgent(testTemplatesFS)
	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("create claude dir: %v", err)
	}

	if err := a.InstallOrchestrator([]byte("# orchestrator\n")); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}

	dest := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read installed orchestrator: %v", err)
	}
	if !strings.Contains(string(content), "orchestrator") {
		t.Fatalf("unexpected orchestrator content: %q", string(content))
	}
}

type stubClaudeCall struct {
	name string
	args []string
}

type stubClaudeResponse struct {
	out     string
	err     error
	started bool
}

type stubClaudeRunner struct {
	calls     []stubClaudeCall
	responses []stubClaudeResponse
}

func (s *stubClaudeRunner) run(name string, args ...string) claudeCommandResult {
	s.calls = append(s.calls, stubClaudeCall{name: name, args: append([]string(nil), args...)})
	if len(s.responses) == 0 {
		return claudeCommandResult{Started: true}
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return claudeCommandResult{Output: resp.out, Err: resp.err, Started: resp.started || resp.err == nil}
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

// ── InstallStatusline tests (TDD: RED → GREEN → REFACTOR) ────────────────────

// TestClaudeAgent_InstallStatusline_FreshInstall verifies that when the
// statusline script does not exist, InstallStatusline writes the script at 0755,
// merges the statusLine key into settings.json, and never calls confirm.
func TestClaudeAgent_InstallStatusline_FreshInstall(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}

	confirmCalled := false
	confirm := func() bool {
		confirmCalled = true
		return true
	}

	if err := a.InstallStatusline(testHooksFS, confirm); err != nil {
		t.Fatalf("InstallStatusline: %v", err)
	}

	if confirmCalled {
		t.Error("confirm must not be called on fresh install (file absent)")
	}

	// Script must be written.
	scriptPath := filepath.Join(claudeDir, "statusline-command.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("statusline script not written: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0100 == 0 {
		t.Errorf("statusline script must be executable, mode=%v", info.Mode())
	}

	// statusLine key must be present in settings.json.
	settings := readStatuslineSettings(t, filepath.Join(claudeDir, "settings.json"))
	sl, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("statusLine key missing or wrong type in settings.json: %#v", settings)
	}
	if sl["type"] != "command" {
		t.Errorf("statusLine.type = %v, want command", sl["type"])
	}
	if sl["command"] != "bash ~/.claude/statusline-command.sh" {
		t.Errorf("statusLine.command = %v, want bash ~/.claude/statusline-command.sh", sl["command"])
	}
}

// TestClaudeAgent_InstallStatusline_Skip verifies that when the script already
// exists and confirm returns false, nothing is written and settings.json is
// left unchanged.
func TestClaudeAgent_InstallStatusline_Skip(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}

	// Pre-write an existing script with known content.
	scriptPath := filepath.Join(claudeDir, "statusline-command.sh")
	originalContent := []byte("#!/bin/bash\necho old-statusline\n")
	if err := os.WriteFile(scriptPath, originalContent, 0755); err != nil {
		t.Fatalf("write existing script: %v", err)
	}

	// Pre-write settings.json without statusLine.
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	confirm := func() bool { return false }

	if err := a.InstallStatusline(testHooksFS, confirm); err != nil {
		t.Fatalf("InstallStatusline: %v", err)
	}

	// Script must be unchanged.
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if string(content) != string(originalContent) {
		t.Errorf("script was modified on skip; got %q", string(content))
	}

	// settings.json must not have statusLine.
	settings := readStatuslineSettings(t, settingsPath)
	if _, ok := settings["statusLine"]; ok {
		t.Errorf("statusLine key must not be present after skip, got %#v", settings)
	}
}

// TestClaudeAgent_InstallStatusline_Overwrite verifies that when the script
// already exists and confirm returns true, the script is overwritten at 0755
// and the statusLine key is merged into settings.json.
func TestClaudeAgent_InstallStatusline_Overwrite(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}

	// Pre-write an old script.
	scriptPath := filepath.Join(claudeDir, "statusline-command.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho old\n"), 0755); err != nil {
		t.Fatalf("write existing script: %v", err)
	}

	// Pre-write settings.json with a sibling key to verify preservation.
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	confirm := func() bool { return true }

	if err := a.InstallStatusline(testHooksFS, confirm); err != nil {
		t.Fatalf("InstallStatusline: %v", err)
	}

	// Script must be replaced with embedded content.
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script after overwrite: %v", err)
	}
	expectedContent, err := fs.ReadFile(testHooksFS, "embed/hooks/claude/statusline-command.sh")
	if err != nil {
		t.Fatalf("read embedded script: %v", err)
	}
	if string(content) != string(expectedContent) {
		t.Errorf("script content after overwrite = %q, want embedded content", string(content))
	}

	// statusLine key must be present; sibling key must be preserved.
	settings := readStatuslineSettings(t, settingsPath)
	if settings["theme"] != "dark" {
		t.Errorf("sibling settings key lost after overwrite; theme = %v", settings["theme"])
	}
	sl, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("statusLine key missing after overwrite: %#v", settings)
	}
	if sl["command"] != "bash ~/.claude/statusline-command.sh" {
		t.Errorf("statusLine.command = %v", sl["command"])
	}
}

// TestClaudeAgent_InstallStatusline_SettingsIsolationOnSkip verifies that a
// pre-existing unrelated settings key is not modified and statusLine is absent
// after a skip.
func TestClaudeAgent_InstallStatusline_SettingsIsolationOnSkip(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}

	// Pre-write script so confirm will be called.
	scriptPath := filepath.Join(claudeDir, "statusline-command.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho existing\n"), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	// Pre-write settings.json with an unrelated key and statusLine present.
	settingsPath := filepath.Join(claudeDir, "settings.json")
	seed := `{"outputStyle":"Gentleman","permissions":{"allow":["Bash(go test:*)"]}}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	confirm := func() bool { return false }

	if err := a.InstallStatusline(testHooksFS, confirm); err != nil {
		t.Fatalf("InstallStatusline: %v", err)
	}

	// settings.json must be byte-identical to seed (not modified at all).
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if string(raw) != seed {
		t.Errorf("settings.json was modified on skip\ngot:  %q\nwant: %q", string(raw), seed)
	}
}

// readStatuslineSettings reads and unmarshals settings.json from path.
func readStatuslineSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	return m
}

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
)

// ── Real-world JSON fixtures ──────────────────────────────────────────────────

// claudeSettingsFixture is a realistic ~/.claude/settings.json with an existing
// engram MCP server already registered.
const claudeSettingsFixture = `{
  "mcpServers": {
    "engram": {
      "command": "/home/andres/go/bin/engram",
      "args": ["mcp", "--tools=agent"],
      "type": "stdio"
    }
  }
}`

// openCodeFixture is a realistic ~/.config/opencode/opencode.json with plugin,
// mcp, and agent sections already present.
const openCodeFixture = `{
  "plugin": ["opencode-gemini-auth@latest"],
  "mcp": {
    "context7": {
      "enabled": true,
      "type": "remote",
      "url": "https://mcp.context7.com/mcp"
    }
  },
  "agent": {
    "gentleman": {
      "description": "Senior Architect",
      "mode": "primary"
    }
  }
}`

// hivePatchForClaude is the patch that MergeConfig injects for Claude.
const hivePatchForClaude = `{
  "mcpServers": {
    "hive": {
      "command": "/home/user/.jarvis/hive-daemon-start.sh",
      "args": [],
      "type": "stdio"
    }
  }
}`

// hivePatchForOpenCode is the patch that MergeConfig injects for OpenCode.
const hivePatchForOpenCode = `{
  "mcp": {
    "hive": {
      "command": ["/home/user/.jarvis/hive-daemon-start.sh"],
      "type": "local"
    }
  }
}`

// ── Task 5.2 tests ────────────────────────────────────────────────────────────

func TestMergeJSON_ClaudeSettings_PreservesEngram(t *testing.T) {
	out, err := MergeJSON([]byte(claudeSettingsFixture), []byte(hivePatchForClaude))
	if err != nil {
		t.Fatalf("MergeJSON: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	mcp, ok := result["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcpServers to be an object")
	}

	// engram must still be present (user's existing entry preserved).
	if _, ok := mcp["engram"]; !ok {
		t.Error("engram entry was lost after merge — existing user config must be preserved")
	}

	// hive must be added.
	if _, ok := mcp["hive"]; !ok {
		t.Error("hive entry was not added by merge")
	}
}

func TestMergeJSON_OpenCodeConfig_PreservesMCPAndAgents(t *testing.T) {
	out, err := MergeJSON([]byte(openCodeFixture), []byte(hivePatchForOpenCode))
	if err != nil {
		t.Fatalf("MergeJSON: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// mcp section must contain both context7 (pre-existing) and hive (new).
	mcp, ok := result["mcp"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp to be an object")
	}
	if _, ok := mcp["context7"]; !ok {
		t.Error("context7 MCP entry was lost — must be preserved")
	}
	if _, ok := mcp["hive"]; !ok {
		t.Error("hive MCP entry was not added")
	}

	// agent section must be preserved untouched.
	agents, ok := result["agent"].(map[string]any)
	if !ok {
		t.Fatal("expected agent section to be an object")
	}
	if _, ok := agents["gentleman"]; !ok {
		t.Error("gentleman agent was lost — must be preserved")
	}

	// plugin array must also be preserved.
	plugins, ok := result["plugin"].([]any)
	if !ok || len(plugins) == 0 {
		t.Error("plugin array was lost — must be preserved")
	}
}

func TestMergeJSON_Idempotent(t *testing.T) {
	// First merge.
	once, err := MergeJSON([]byte(claudeSettingsFixture), []byte(hivePatchForClaude))
	if err != nil {
		t.Fatalf("first MergeJSON: %v", err)
	}

	// Second merge of same patch into the already-merged result.
	twice, err := MergeJSON(once, []byte(hivePatchForClaude))
	if err != nil {
		t.Fatalf("second MergeJSON: %v", err)
	}

	var r1, r2 map[string]any
	if err := json.Unmarshal(once, &r1); err != nil {
		t.Fatalf("unmarshal once: %v", err)
	}
	if err := json.Unmarshal(twice, &r2); err != nil {
		t.Fatalf("unmarshal twice: %v", err)
	}

	// Number of top-level keys must be identical (no duplication).
	if len(r1) != len(r2) {
		t.Errorf("idempotency violated: first merge has %d keys, second has %d", len(r1), len(r2))
	}

	// The hive entry must appear exactly once.
	mcp1 := r1["mcpServers"].(map[string]any)
	mcp2 := r2["mcpServers"].(map[string]any)
	if len(mcp1) != len(mcp2) {
		t.Errorf("idempotency violated in mcpServers: once=%d entries, twice=%d entries",
			len(mcp1), len(mcp2))
	}
}

func TestClaudeAgent_WriteInstructions_CreatesSentinels(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	// Create ~/.claude directory so IsInstalled() would be true.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer 1 — Hive Memory Protocol"
	layer2 := "# Layer 2 — Tony Stark Persona"

	if err := a.WriteInstructions(layer1, layer2, nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	// Both sentinel pairs must be present.
	for _, marker := range []string{Layer1Start, Layer1End, Layer2Start, Layer2End} {
		if !strings.Contains(content, marker) {
			t.Errorf("expected sentinel marker %q in CLAUDE.md, not found", marker)
		}
	}

	// Content must appear between the correct markers.
	if !strings.Contains(content, layer1) {
		t.Errorf("layer1 content not found in CLAUDE.md")
	}
	if !strings.Contains(content, layer2) {
		t.Errorf("layer2 content not found in CLAUDE.md")
	}
}

func TestClaudeAgent_WriteInstructions_PatchesLayer2Only(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer1 — Memory system instructions"
	layer2v1 := "# Layer2 v1 — argentino persona"
	layer2v2 := "# Layer2 v2 — tony-stark persona"

	// First write.
	if err := a.WriteInstructions(layer1, layer2v1, nil); err != nil {
		t.Fatalf("first WriteInstructions: %v", err)
	}

	// Second write with different Layer2.
	if err := a.WriteInstructions(layer1, layer2v2, nil); err != nil {
		t.Fatalf("second WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	// Layer1 block must be unchanged.
	if !strings.Contains(content, layer1) {
		t.Errorf("layer1 content missing after second write")
	}

	// Layer2 must have the NEW content.
	if !strings.Contains(content, layer2v2) {
		t.Errorf("layer2 v2 content missing after second write")
	}

	// OLD Layer2 must NOT be present anymore.
	if strings.Contains(content, layer2v1) {
		t.Errorf("old layer2 v1 content still present — should have been replaced")
	}
}

func TestOpenCodeAgent_InstallSkills_CreatesSkillDirs(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	a := &OpenCodeAgent{home: home}

	testFS := fstest.MapFS{
		"go-testing/SKILL.md": {Data: []byte("# Go Testing Skill\n\nUse table-driven tests.")},
		"sdd-apply/SKILL.md":  {Data: []byte("# SDD Apply Skill\n\nImplement tasks from spec.")},
	}
	selectedIDs := []string{"go-testing", "sdd-apply"}

	if err := a.InstallSkills(testFS, selectedIDs); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	// Each skill must have its own directory with SKILL.md inside.
	expected := map[string]string{
		"go-testing": "# Go Testing Skill\n\nUse table-driven tests.",
		"sdd-apply":  "# SDD Apply Skill\n\nImplement tasks from spec.",
	}
	for skillID, expectedContent := range expected {
		skillPath := filepath.Join(home, ".config", "opencode", "skills", skillID, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("skill %q: expected SKILL.md at %s, got error: %v", skillID, skillPath, err)
			continue
		}
		if string(data) != expectedContent {
			t.Errorf("skill %q: content mismatch\n  got:  %q\n  want: %q",
				skillID, string(data), expectedContent)
		}
	}
}

// ── TASK-4.4: Integration test for jarvis sync protocol injection ────────────

// TestProtocolInjection_FirstSync verifies that WriteInstructions injects
// the Hive protocol with jarvis:hive-protocol markers on first sync.
func TestProtocolInjection_FirstSync(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	// Create ~/.claude directory so IsInstalled() would be true.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	// Simulate jarvis sync by calling WriteInstructions
	layer1 := "# Layer 1 — Hive Memory Protocol"
	layer2 := "# Layer 2 — Tony Stark Persona"

	if err := a.WriteInstructions(layer1, layer2, nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	// Verify Hive protocol markers are present
	if !strings.Contains(content, HiveProtocolStart) {
		t.Error("expected Hive protocol start marker, not found")
	}
	if !strings.Contains(content, HiveProtocolEnd) {
		t.Error("expected Hive protocol end marker, not found")
	}

	// Verify protocol content is present (check for a characteristic string)
	if !strings.Contains(content, "mem_save") || !strings.Contains(content, "mem_search") {
		t.Error("expected Hive protocol content between markers, not found")
	}

	// Verify old gentle-ai markers are NOT present
	if strings.Contains(content, OldEngramStart) || strings.Contains(content, OldEngramEnd) {
		t.Error("old gentle-ai protocol markers should not be present after first sync")
	}
}

// TestProtocolInjection_Idempotency verifies that running WriteInstructions
// twice produces the same result (no duplicate markers).
func TestProtocolInjection_Idempotency(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer 1 — Hive Memory Protocol"
	layer2 := "# Layer 2 — Tony Stark Persona"

	// First sync
	if err := a.WriteInstructions(layer1, layer2, nil); err != nil {
		t.Fatalf("first WriteInstructions: %v", err)
	}

	firstData, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md after first sync: %v", err)
	}

	// Second sync (same content)
	if err := a.WriteInstructions(layer1, layer2, nil); err != nil {
		t.Fatalf("second WriteInstructions: %v", err)
	}

	secondData, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md after second sync: %v", err)
	}

	// Content should be identical (idempotent)
	if string(firstData) != string(secondData) {
		t.Error("second sync produced different content — expected idempotent behavior")
	}

	// Verify exactly one occurrence of start marker
	startCount := strings.Count(string(secondData), HiveProtocolStart)
	if startCount != 1 {
		t.Errorf("found %d occurrences of Hive protocol start marker, want exactly 1", startCount)
	}

	// Verify exactly one occurrence of end marker
	endCount := strings.Count(string(secondData), HiveProtocolEnd)
	if endCount != 1 {
		t.Errorf("found %d occurrences of Hive protocol end marker, want exactly 1", endCount)
	}
}

// TestProtocolInjection_CleansUpOldMarkers verifies that if CLAUDE.md contains
// old gentle-ai:engram-protocol markers, they are removed and replaced with
// the new jarvis:hive-protocol markers.
func TestProtocolInjection_CleansUpOldMarkers(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	// Pre-create CLAUDE.md with old gentle-ai markers
	oldContent := `# Some Instructions

<!-- gentle-ai:engram-protocol -->
OLD PROTOCOL CONTENT HERE
<!-- /gentle-ai:engram-protocol -->

More content
`
	claudePath := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte(oldContent), 0644); err != nil {
		t.Fatalf("write initial CLAUDE.md: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer 1 — Hive Memory Protocol"
	layer2 := "# Layer 2 — Tony Stark Persona"

	// Run WriteInstructions (should clean up old markers)
	if err := a.WriteInstructions(layer1, layer2, nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md after cleanup: %v", err)
	}
	content := string(data)

	// Verify old markers are removed
	if strings.Contains(content, OldEngramStart) {
		t.Error("old gentle-ai start marker still present after cleanup")
	}
	if strings.Contains(content, OldEngramEnd) {
		t.Error("old gentle-ai end marker still present after cleanup")
	}
	if strings.Contains(content, "OLD PROTOCOL CONTENT HERE") {
		t.Error("old protocol content still present after cleanup")
	}

	// Verify new markers are present
	if !strings.Contains(content, HiveProtocolStart) {
		t.Error("new Hive protocol start marker not found")
	}
	if !strings.Contains(content, HiveProtocolEnd) {
		t.Error("new Hive protocol end marker not found")
	}

	// Verify exactly one new protocol block
	startCount := strings.Count(content, HiveProtocolStart)
	if startCount != 1 {
		t.Errorf("found %d Hive protocol start markers, want exactly 1", startCount)
	}
}

// TestProtocolInjection_ProtocolContentCorrect verifies that the embedded
// Hive protocol content is correctly injected between the markers.
func TestProtocolInjection_ProtocolContentCorrect(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer 1 — Hive Memory Protocol"
	layer2 := "# Layer 2 — Tony Stark Persona"

	if err := a.WriteInstructions(layer1, layer2, nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	// Extract content between markers
	startIdx := strings.Index(content, HiveProtocolStart)
	endIdx := strings.Index(content, HiveProtocolEnd)
	if startIdx == -1 || endIdx == -1 {
		t.Fatal("protocol markers not found in CLAUDE.md")
	}

	protocolSection := content[startIdx+len(HiveProtocolStart) : endIdx]

	// Verify key protocol sections are present
	requiredSections := []string{
		"PROACTIVE SAVE TRIGGERS",
		"mem_save",
		"mem_search",
		"mem_context",
		"SESSION CLOSE PROTOCOL",
		"AFTER COMPACTION",
	}

	for _, section := range requiredSections {
		if !strings.Contains(protocolSection, section) {
			t.Errorf("protocol section missing required content: %q", section)
		}
	}

	// Verify the protocol content matches the embedded hive-protocol.md
	expectedProtocol := getHiveProtocol()
	if !strings.Contains(protocolSection, expectedProtocol) {
		// At minimum, check that the first 100 characters match
		if len(expectedProtocol) > 100 && !strings.Contains(protocolSection, expectedProtocol[:100]) {
			t.Error("injected protocol content does not match embedded hive-protocol.md")
		}
	}
}

// ── T-CLI-1: InstallPromptHook tests ─────────────────────────────────────────

// testHooksFS is a minimal in-memory FS that mirrors embed/hooks/ for tests.
// The 6 Claude session/prompt shell scripts were deleted in hooks-go-native migration;
// only the statusline script and the OpenCode plugin remain.
var testHooksFS = fstest.MapFS{
	"embed/hooks/claude/statusline-command.sh": {Data: []byte("#!/bin/bash\necho statusline\n")},
	"embed/hooks/opencode/hive.ts":             {Data: []byte("export const Hive = {}")},
}

// TestClaudeAgent_InstallSessionHooks verifies that InstallSessionHooks:
//   - patches settings.json with SessionStart and Stop hook entries using inline commands
//   - does NOT write any script files (hooks-go-native migration)
func TestClaudeAgent_InstallSessionHooks(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("InstallSessionHooks: %v", err)
	}

	// No script files should be written.
	hookDir := filepath.Join(claudeDir, "hive-hooks")
	for _, script := range []string{"session-start.sh", "session-start.ps1", "session-stop.sh", "session-stop.ps1"} {
		if _, err := os.Stat(filepath.Join(hookDir, script)); !os.IsNotExist(err) {
			t.Errorf("script %q must not be written after hooks-go-native migration", script)
		}
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings := readSettingsMap(t, settingsPath)

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing 'hooks' object")
	}

	// SessionStart entry must be present with inline command.
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok || len(sessionStart) == 0 {
		t.Fatal("settings.json missing hooks.SessionStart array")
	}
	foundStart := false
	for _, entry := range sessionStart {
		entryMap, ok := entry.(map[string]any)
		if !ok || entryMap["name"] != "hive-session-start" {
			continue
		}
		foundStart = true
		// Verify inline command (not a script path)
		innerHooks, _ := entryMap["hooks"].([]any)
		if len(innerHooks) == 0 {
			t.Fatal("hive-session-start missing inner hooks")
		}
		hm, _ := innerHooks[0].(map[string]any)
		cmd, _ := hm["command"].(string)
		if !strings.Contains(cmd, "hook") || !strings.Contains(cmd, "session-start") {
			t.Errorf("hive-session-start command %q must reference 'hook session-start'", cmd)
		}
		if strings.HasSuffix(cmd, ".sh") || strings.HasSuffix(cmd, ".ps1") {
			t.Errorf("hive-session-start command must not be a script path, got %q", cmd)
		}
	}
	if !foundStart {
		t.Error("hive-session-start entry not found in SessionStart hooks")
	}

	// Stop entry must be present.
	stop, ok := hooks["Stop"].([]any)
	if !ok || len(stop) == 0 {
		t.Fatal("settings.json missing hooks.Stop array")
	}
	if _, ok := hooks["SessionEnd"]; ok {
		t.Fatal("settings.json must not register hive-session-stop under SessionEnd")
	}
	foundStop := false
	for _, entry := range stop {
		if entryMap, ok := entry.(map[string]any); ok && entryMap["name"] == "hive-session-stop" {
			foundStop = true
		}
	}
	if !foundStop {
		t.Error("hive-session-stop entry not found in Stop hooks")
	}
}

// TestClaudeAgent_InstallSessionHooks_Idempotent verifies that calling
// InstallSessionHooks twice leaves exactly one hive-session-start entry under
// SessionStart and exactly one hive-session-stop entry under Stop.
func TestClaudeAgent_InstallSessionHooks_Idempotent(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	// First call.
	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("first InstallSessionHooks: %v", err)
	}

	// Second call — must be idempotent.
	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("second InstallSessionHooks: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings := readSettingsMap(t, settingsPath)

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing 'hooks' object")
	}

	// Exactly one hive-session-start entry.
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatal("settings.json missing hooks.SessionStart array")
	}
	startCount := 0
	for _, entry := range sessionStart {
		if entryMap, ok := entry.(map[string]any); ok && entryMap["name"] == "hive-session-start" {
			startCount++
		}
	}
	if startCount != 1 {
		t.Errorf("idempotency: expected exactly 1 hive-session-start entry, got %d", startCount)
	}

	// Exactly one hive-session-stop entry.
	stop, ok := hooks["Stop"].([]any)
	if !ok {
		t.Fatal("settings.json missing hooks.Stop array")
	}
	stopCount := 0
	for _, entry := range stop {
		if entryMap, ok := entry.(map[string]any); ok && entryMap["name"] == "hive-session-stop" {
			stopCount++
		}
	}
	if stopCount != 1 {
		t.Errorf("idempotency: expected exactly 1 hive-session-stop entry, got %d", stopCount)
	}
}

// TestClaudeAgent_InstallPromptHook verifies that InstallPromptHook:
//   - patches settings.json with a UserPromptSubmit hook entry using an inline command
//   - does NOT write any script file (hooks-go-native migration)
//   - is idempotent (hook appears exactly once after two calls)
//   - preserves pre-existing UserPromptSubmit entries (R-9 mitigation)
func TestClaudeAgent_InstallPromptHook(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	// First call.
	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("first InstallPromptHook: %v", err)
	}

	// No script files should be written.
	for _, script := range []string{"user-prompt-submit.sh", "user-prompt-submit.ps1"} {
		path := filepath.Join(claudeDir, "hive-hooks", script)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("script %q must not be written after hooks-go-native migration", path)
		}
	}

	// settings.json must exist and be valid JSON.
	settingsPath := filepath.Join(claudeDir, "settings.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json is invalid JSON: %v", err)
	}

	// hooks.UserPromptSubmit must be an array with at least one entry containing type=command.
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing 'hooks' object")
	}
	ups, ok := hooks["UserPromptSubmit"].([]any)
	if !ok || len(ups) == 0 {
		t.Fatal("settings.json missing hooks.UserPromptSubmit array")
	}

	foundInlineCommand := false
	for _, entry := range ups {
		entryMap, ok := entry.(map[string]any)
		if !ok || entryMap["name"] != "hive-prompt-capture" {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if hMap["type"] == "command" {
				cmd, _ := hMap["command"].(string)
				if strings.Contains(cmd, "hook") && strings.Contains(cmd, "prompt-submit") {
					foundInlineCommand = true
				}
			}
		}
	}
	if !foundInlineCommand {
		t.Errorf("no UserPromptSubmit hook with inline 'hook prompt-submit' command found")
	}

	// Second call — must be idempotent (hook appears exactly once).
	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("second InstallPromptHook: %v", err)
	}

	raw2, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json after second call: %v", err)
	}
	var settings2 map[string]any
	if err := json.Unmarshal(raw2, &settings2); err != nil {
		t.Fatalf("settings.json invalid JSON after second call: %v", err)
	}
	hooks2 := settings2["hooks"].(map[string]any)
	ups2 := hooks2["UserPromptSubmit"].([]any)

	hiveCount := 0
	for _, entry := range ups2 {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if entryMap["name"] == "hive-prompt-capture" {
			hiveCount++
		}
	}
	if hiveCount != 1 {
		t.Errorf("idempotency: expected exactly 1 hive-prompt-capture entry, got %d", hiveCount)
	}
}

// TestClaudeAgent_InstallPromptHook_RegistersInlineCommandOnAllPlatforms verifies
// that InstallPromptHook uses an inline jarvis command on all platforms after migration.
func TestClaudeAgent_InstallPromptHook_RegistersInlineCommandOnAllPlatforms(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			home := t.TempDir()
			setTestHome(t, home)

			claudeDir := filepath.Join(home, ".claude")
			if err := os.MkdirAll(claudeDir, 0755); err != nil {
				t.Fatalf("mkdir .claude: %v", err)
			}

			origExe := osExecutable
			osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
			t.Cleanup(func() { osExecutable = origExe })

			a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
			if err := a.InstallPromptHook(testHooksFS); err != nil {
				t.Fatalf("InstallPromptHook(%s): %v", goos, err)
			}

			command := claudeHookCommandFromSettings(t, filepath.Join(claudeDir, "settings.json"))
			if !strings.Contains(command, "hook") || !strings.Contains(command, "prompt-submit") {
				t.Errorf("(%s) hook command %q must reference 'hook prompt-submit'", goos, command)
			}
			if strings.HasSuffix(command, ".sh") || strings.HasSuffix(command, ".ps1") {
				t.Errorf("(%s) hook command must not be a script path, got %q", goos, command)
			}
		})
	}
}

func TestClaudeAgent_InstallPromptHook_PreservesExistingHooksAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	existing := map[string]any{
		"env": map[string]any{"KEEP_ME": "yes"},
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"name": "my-existing-hook",
					"hooks": []any{
						map[string]any{"type": "command", "command": `/usr/local/bin/existing-hook`},
					},
				},
			},
		},
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	existingBytes, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, existingBytes, 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("first InstallPromptHook: %v", err)
	}
	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("second InstallPromptHook: %v", err)
	}

	settings := readSettingsMap(t, settingsPath)
	if env, ok := settings["env"].(map[string]any); !ok || env["KEEP_ME"] != "yes" {
		t.Fatalf("unrelated top-level settings were not preserved: %#v", settings["env"])
	}

	hooks := settings["hooks"].(map[string]any)
	ups := hooks["UserPromptSubmit"].([]any)
	foundExisting := false
	hiveCount := 0
	for _, entry := range ups {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		switch entryMap["name"] {
		case "my-existing-hook":
			foundExisting = true
		case "hive-prompt-capture":
			hiveCount++
		}
	}
	if !foundExisting {
		t.Fatal("existing hook was not preserved")
	}
	if hiveCount != 1 {
		t.Fatalf("expected exactly one hive-prompt-capture hook after rerun, got %d", hiveCount)
	}
}

func withClaudeGOOS(t *testing.T, goos string) {
	t.Helper()
	previous := claudeRuntimeGOOS
	claudeRuntimeGOOS = goos
	t.Cleanup(func() { claudeRuntimeGOOS = previous })
}

func claudeHookCommandFromSettings(t *testing.T, settingsPath string) string {
	t.Helper()
	settings := readSettingsMap(t, settingsPath)
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing hooks object")
	}
	ups, ok := hooks["UserPromptSubmit"].([]any)
	if !ok {
		t.Fatal("settings.json missing UserPromptSubmit hooks")
	}
	for _, entry := range ups {
		entryMap, ok := entry.(map[string]any)
		if !ok || entryMap["name"] != "hive-prompt-capture" {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			t.Fatal("hive-prompt-capture missing inner hooks")
		}
		for _, hook := range innerHooks {
			hookMap, ok := hook.(map[string]any)
			if !ok || hookMap["type"] != "command" {
				continue
			}
			command, ok := hookMap["command"].(string)
			if !ok {
				t.Fatalf("command is not a string: %#v", hookMap["command"])
			}
			return command
		}
	}
	t.Fatal("hive-prompt-capture command hook not found")
	return ""
}

func readSettingsMap(t *testing.T, settingsPath string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json is invalid JSON: %v", err)
	}
	return settings
}

// TestClaudeAgent_InstallPromptHook_PreservesExistingHooks verifies that
// pre-existing UserPromptSubmit entries are NOT removed (R-9 mitigation).
func TestClaudeAgent_InstallPromptHook_PreservesExistingHooks(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	// Pre-seed settings.json with an existing hook.
	existing := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"name": "my-existing-hook",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/usr/local/bin/my-hook",
						},
					},
				},
			},
		},
	}
	existingBytes, _ := json.MarshalIndent(existing, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, existingBytes, 0644); err != nil {
		t.Fatalf("write pre-existing settings.json: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks := settings["hooks"].(map[string]any)
	ups := hooks["UserPromptSubmit"].([]any)

	// Both the original hook AND the new hive hook must be present.
	foundExisting := false
	foundHive := false
	for _, entry := range ups {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		switch entryMap["name"] {
		case "my-existing-hook":
			foundExisting = true
		case "hive-prompt-capture":
			foundHive = true
		}
	}

	if !foundExisting {
		t.Error("pre-existing hook 'my-existing-hook' was removed — R-9 violation")
	}
	if !foundHive {
		t.Error("hive-prompt-capture hook not found after InstallPromptHook")
	}
}

// TestOpenCodeAgent_InstallPromptHook verifies that InstallPromptHook writes
// the TypeScript plugin to ~/.config/opencode/plugins/hive.ts.
func TestOpenCodeAgent_InstallPromptHook(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	a := &OpenCodeAgent{home: home}

	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "hive.ts")
	data, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("plugin not found at %s: %v", pluginPath, err)
	}

	if string(data) != "export const Hive = {}" {
		t.Errorf("plugin content mismatch: got %q", string(data))
	}
}

func TestWriteInstructions_ReapplyRewritesManagedAndPreservesUserContent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent Agent
		path  string
	}{
		{name: "claude", agent: &ClaudeAgent{home: t.TempDir(), templatesFS: testTemplatesFS}, path: "CLAUDE.md"},
		{name: "opencode", agent: &OpenCodeAgent{home: t.TempDir(), templatesFS: testTemplatesFS}, path: "AGENTS.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.agent.ConfigDir(), 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}

			const (
				layer1 = "# managed layer1"
				layer2 = "# managed layer2"
			)
			if err := tc.agent.WriteInstructions(layer1, layer2, nil); err != nil {
				t.Fatalf("first WriteInstructions: %v", err)
			}

			instructionsPath := filepath.Join(tc.agent.ConfigDir(), tc.path)
			seeded := "# user-header\n" +
				Layer1Start + "\nSTALE-L1\n" + Layer1End + "\n" +
				"# user-middle\n" +
				Layer2Start + "\nSTALE-L2\n" + Layer2End + "\n" +
				"# user-footer\n"
			if err := os.WriteFile(instructionsPath, []byte(seeded), 0644); err != nil {
				t.Fatalf("seed instructions: %v", err)
			}

			if err := tc.agent.WriteInstructions(layer1, layer2, nil); err != nil {
				t.Fatalf("reapply WriteInstructions: %v", err)
			}

			updatedBytes, err := os.ReadFile(instructionsPath)
			if err != nil {
				t.Fatalf("read instructions: %v", err)
			}
			updated := string(updatedBytes)

			if strings.Contains(updated, "STALE-L1") || strings.Contains(updated, "STALE-L2") {
				t.Fatalf("managed stale content was not rewritten")
			}
			if !strings.Contains(updated, layer1) || !strings.Contains(updated, layer2) {
				t.Fatalf("managed content not restored to expected values")
			}
			for _, userLine := range []string{"# user-header", "# user-middle", "# user-footer"} {
				if !strings.Contains(updated, userLine) {
					t.Fatalf("user content outside managed boundaries lost: %s", userLine)
				}
			}
		})
	}
}

func TestWriteInstructions_ReapplyIsIdempotentOnCompliantInstall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent Agent
		path  string
	}{
		{name: "claude", agent: &ClaudeAgent{home: t.TempDir(), templatesFS: testTemplatesFS}, path: "CLAUDE.md"},
		{name: "opencode", agent: &OpenCodeAgent{home: t.TempDir(), templatesFS: testTemplatesFS}, path: "AGENTS.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.agent.ConfigDir(), 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}

			const (
				layer1 = "# managed layer1"
				layer2 = "# managed layer2"
			)
			if err := tc.agent.WriteInstructions(layer1, layer2, nil); err != nil {
				t.Fatalf("first WriteInstructions: %v", err)
			}

			instructionsPath := filepath.Join(tc.agent.ConfigDir(), tc.path)
			first, err := os.ReadFile(instructionsPath)
			if err != nil {
				t.Fatalf("read first instructions: %v", err)
			}

			if err := tc.agent.WriteInstructions(layer1, layer2, nil); err != nil {
				t.Fatalf("second WriteInstructions: %v", err)
			}

			second, err := os.ReadFile(instructionsPath)
			if err != nil {
				t.Fatalf("read second instructions: %v", err)
			}

			if string(first) != string(second) {
				t.Fatalf("reapply is not idempotent for compliant installation")
			}
		})
	}
}

func TestWriteInstructions_ReapplyDeterministicallyRegeneratesPartialManagedState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		agent Agent
		path  string
	}{
		{name: "claude", agent: &ClaudeAgent{home: t.TempDir(), templatesFS: testTemplatesFS}, path: "CLAUDE.md"},
		{name: "opencode", agent: &OpenCodeAgent{home: t.TempDir(), templatesFS: testTemplatesFS}, path: "AGENTS.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.agent.ConfigDir(), 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}

			const (
				layer1 = "# managed layer1"
				layer2 = "# managed layer2"
			)
			if err := tc.agent.WriteInstructions(layer1, layer2, nil); err != nil {
				t.Fatalf("first WriteInstructions: %v", err)
			}

			instructionsPath := filepath.Join(tc.agent.ConfigDir(), tc.path)
			baselineBytes, err := os.ReadFile(instructionsPath)
			if err != nil {
				t.Fatalf("read baseline instructions: %v", err)
			}
			baseline := string(baselineBytes)

			partial := strings.Replace(baseline, Layer2End, "", 1)
			if err := os.WriteFile(instructionsPath, []byte(partial), 0644); err != nil {
				t.Fatalf("write partial instructions: %v", err)
			}

			if err := tc.agent.WriteInstructions(layer1, layer2, nil); err != nil {
				t.Fatalf("reapply WriteInstructions from partial state: %v", err)
			}

			regeneratedBytes, err := os.ReadFile(instructionsPath)
			if err != nil {
				t.Fatalf("read regenerated instructions: %v", err)
			}

			regenerated := string(regeneratedBytes)
			if regenerated != baseline {
				t.Fatalf("regenerated managed state is not deterministic")
			}
		})
	}
}

// ── Orchestrator import injection tests ──────────────────────────────────────

// TestClaudeAgent_WriteInstructions_InjectsOrchestratorImport verifies that
// WriteInstructions injects the @./sdd-orchestrator.md block inside
// <!-- jarvis:orchestrator-import --> markers in CLAUDE.md.
func TestClaudeAgent_WriteInstructions_InjectsOrchestratorImport(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	// OrchestratorImportStart and OrchestratorImportEnd markers must be present.
	if !strings.Contains(content, OrchestratorImportStart) {
		t.Errorf("CLAUDE.md missing %q marker", OrchestratorImportStart)
	}
	if !strings.Contains(content, OrchestratorImportEnd) {
		t.Errorf("CLAUDE.md missing %q marker", OrchestratorImportEnd)
	}

	// The @import line must appear between the markers.
	startIdx := strings.Index(content, OrchestratorImportStart)
	endIdx := strings.Index(content, OrchestratorImportEnd)
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		t.Fatalf("orchestrator import markers not found or malformed in CLAUDE.md")
	}
	between := content[startIdx+len(OrchestratorImportStart) : endIdx]
	if !strings.Contains(between, "@./sdd-orchestrator.md") {
		t.Errorf("CLAUDE.md orchestrator import block must contain '@./sdd-orchestrator.md', got: %q", between)
	}
}

// TestClaudeAgent_WriteInstructions_OrchestratorImportIdempotent verifies that
// running WriteInstructions twice produces exactly one orchestrator-import block.
func TestClaudeAgent_WriteInstructions_OrchestratorImportIdempotent(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("first WriteInstructions: %v", err)
	}
	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("second WriteInstructions: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)

	startCount := strings.Count(content, OrchestratorImportStart)
	if startCount != 1 {
		t.Errorf("expected exactly 1 %q marker, got %d", OrchestratorImportStart, startCount)
	}
	endCount := strings.Count(content, OrchestratorImportEnd)
	if endCount != 1 {
		t.Errorf("expected exactly 1 %q marker, got %d", OrchestratorImportEnd, endCount)
	}
}

// TestOpenCodeJSONTemplate_OrchestratorPromptUsesFileInjection verifies that the
// embedded opencode.json.tmpl uses {file:./sdd-orchestrator.md} for the
// sdd-orchestrator agent prompt field.
func TestOpenCodeJSONTemplate_OrchestratorPromptUsesFileInjection(t *testing.T) {
	templateBytes, err := os.ReadFile("../../embed/templates/opencode.json.tmpl")
	if err != nil {
		t.Skipf("opencode.json.tmpl not found at embed path (may not exist yet): %v", err)
	}
	template := string(templateBytes)

	if !strings.Contains(template, `"prompt": "{file:./sdd-orchestrator.md}"`) {
		t.Errorf("opencode.json.tmpl sdd-orchestrator agent prompt must be {file:./sdd-orchestrator.md}, got template:\n%s", template)
	}
	if strings.Contains(template, "Read and follow") {
		t.Errorf("opencode.json.tmpl must not contain old prose prompt 'Read and follow'")
	}
	if !strings.Contains(template, `"permission": {"question": "allow", "task":`) {
		t.Errorf("opencode.json.tmpl sdd-orchestrator must allow the native question tool, got template:\n%s", template)
	}
}

func TestGeneratedRuntimeAcceptance_RenderedArtifactsProveGuardrails(t *testing.T) {
	preset, err := persona.ResolveProfile(jarvis.PersonaFS, "argentino")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	layer1 := config.Layer1Content()
	layer2 := persona.RenderLayer2(preset.Preset)
	skills := []config.SkillInfo{{Name: "sdd-apply", Description: "Implement SDD tasks", Trigger: "implementing SDD tasks"}}

	for _, tc := range []struct {
		name  string
		agent Agent
		file  string
	}{
		{name: "claude", agent: &ClaudeAgent{home: t.TempDir(), templatesFS: jarvis.TemplatesFS}, file: "CLAUDE.md"},
		{name: "opencode", agent: &OpenCodeAgent{home: t.TempDir(), templatesFS: jarvis.TemplatesFS}, file: "AGENTS.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.agent.ConfigDir(), 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}
			if err := tc.agent.WriteInstructions(layer1, layer2, skills); err != nil {
				t.Fatalf("WriteInstructions: %v", err)
			}

			renderedBytes, err := os.ReadFile(filepath.Join(tc.agent.ConfigDir(), tc.file))
			if err != nil {
				t.Fatalf("read rendered instructions: %v", err)
			}
			rendered := string(renderedBytes)
			if got := strings.Count(rendered, HiveProtocolStart); got != 1 {
				t.Fatalf("Hive protocol start marker count = %d, want 1\n%s", got, rendered)
			}
			if got := strings.Count(rendered, HiveProtocolEnd); got != 1 {
				t.Fatalf("Hive protocol end marker count = %d, want 1\n%s", got, rendered)
			}
			if got := strings.Count(rendered, "# Hive Persistent Memory — Protocol"); got != 1 {
				t.Fatalf("Hive protocol body count = %d, want 1\n%s", got, rendered)
			}
			projection, err := config.ProjectInstruction(rendered)
			if err != nil {
				t.Fatalf("ProjectInstruction: %v", err)
			}
			if projection.Layer1 != layer1 {
				t.Fatal("Layer1 projection does not derive from the canonical source")
			}
			if got := strings.Count(rendered, config.TechnicalContractContent()); got != 1 {
				t.Fatalf("canonical technical contract count = %d, want 1", got)
			}
			assertNoGeneratedProductMemoryBackendWording(t, rendered)
		})
	}
}

func TestGeneratedRuntimeAcceptance_ConfigArtifactsProveRolloutSafety(t *testing.T) {
	openCodeHome := t.TempDir()
	openCode := &OpenCodeAgent{home: openCodeHome, templatesFS: jarvis.TemplatesFS}
	if err := os.MkdirAll(openCode.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir opencode dir: %v", err)
	}
	if err := openCode.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("OpenCode MergeGeneratedConfig: %v", err)
	}
	opencodeSettings := readJSONFile(t, filepath.Join(openCode.ConfigDir(), "opencode.json"))
	if opencodeSettings["share"] != "disabled" {
		t.Fatalf("OpenCode share = %v, want disabled", opencodeSettings["share"])
	}
	if opencodeSettings["default_agent"] != "sdd-orchestrator" {
		t.Fatalf("OpenCode default_agent = %v, want sdd-orchestrator", opencodeSettings["default_agent"])
	}
	assertOpenCodeGlobalPermissionGuards(t, opencodeSettings)
	agents := opencodeSettings["agent"].(map[string]any)
	orchestrator := agents["sdd-orchestrator"].(map[string]any)
	if orchestrator["mode"] != "primary" {
		t.Fatalf("sdd-orchestrator mode = %v, want primary", orchestrator["mode"])
	}
	orchestratorPerm := orchestrator["permission"].(map[string]any)
	if orchestratorPerm["question"] != "allow" {
		t.Fatalf("sdd-orchestrator question permission = %v, want allow", orchestratorPerm["question"])
	}
	taskPerm := orchestratorPerm["task"].(map[string]any)
	if taskPerm["sdd-orchestrator"] == "allow" {
		t.Fatal("sdd-orchestrator must not allow task delegation to itself")
	}
	for _, allowed := range append(openCodeSDDSubagents(), openCodeJudgmentDaySubagents()...) {
		if _, ok := agents[allowed]; !ok {
			t.Fatalf("sdd-orchestrator allows undefined task target %q", allowed)
		}
		if taskPerm[allowed] != "allow" {
			t.Fatalf("sdd-orchestrator task permission for %s = %v, want allow", allowed, taskPerm[allowed])
		}
	}
	for target, permission := range taskPerm {
		if permission != "allow" || target == "*" {
			continue
		}
		if target == "general" || target == "explore" {
			continue
		}
		if _, ok := agents[target]; !ok {
			t.Fatalf("sdd-orchestrator task permission allows undefined agent %q", target)
		}
	}
	assertOpenCodeSubagentsHiddenAndMode(t, agents, openCodeSDDSubagents())
	assertOpenCodeSubagentsHiddenAndMode(t, agents, openCodeJudgmentDaySubagents())
	assertNoGeneratedProductMemoryBackendWording(t, string(mustMarshalJSON(t, opencodeSettings)))

	claudeHome := t.TempDir()
	claude := &ClaudeAgent{home: claudeHome, templatesFS: testTemplatesFS}
	if err := os.MkdirAll(claude.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })
	if err := claude.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}
	if err := claude.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("Claude MergeGeneratedConfig: %v", err)
	}
	claudeSettings := readJSONFile(t, filepath.Join(claude.ConfigDir(), "settings.json"))
	permissions := claudeSettings["permissions"].(map[string]any)
	if countScalar(permissions["allow"].([]any), "Bash(go test:*)") != 1 {
		t.Fatalf("Claude permissions.allow must include go test exactly once: %#v", permissions["allow"])
	}
	if countScalar(permissions["deny"].([]any), "Read(**/.env*)") != 1 {
		t.Fatalf("Claude permissions.deny must include env read guard exactly once: %#v", permissions["deny"])
	}
	hooks := claudeSettings["hooks"].(map[string]any)
	hookEntries := hooks["UserPromptSubmit"].([]any)
	foundHiveHook := false
	for _, entry := range hookEntries {
		entryMap, ok := entry.(map[string]any)
		if ok && entryMap["name"] == "hive-prompt-capture" {
			foundHiveHook = true
		}
	}
	if !foundHiveHook {
		t.Fatalf("Claude settings must preserve existing Hive prompt-capture hook: %#v", hookEntries)
	}
	assertNoGeneratedProductMemoryBackendWording(t, string(mustMarshalJSON(t, claudeSettings)))
}

func assertOpenCodeGlobalPermissionGuards(t *testing.T, settings map[string]any) {
	t.Helper()
	permissions := settings["permission"].(map[string]any)
	bash := permissions["bash"].(map[string]any)
	for _, guard := range []string{"git push --force*", "git reset --hard*", "git clean -fdx*", "rm -rf /*"} {
		if bash[guard] != "ask" {
			t.Fatalf("OpenCode global bash guard %q = %v, want ask", guard, bash[guard])
		}
	}
	read := permissions["read"].(map[string]any)
	for _, guard := range []string{"**/.env*", "**/*secret*", "**/*token*", "**/*credential*", "**/id_rsa*"} {
		if read[guard] != "deny" {
			t.Fatalf("OpenCode global read guard %q = %v, want deny", guard, read[guard])
		}
	}
}

func assertOpenCodeSubagentsHiddenAndMode(t *testing.T, agents map[string]any, names []string) {
	t.Helper()
	for _, name := range names {
		agent, ok := agents[name].(map[string]any)
		if !ok {
			t.Fatalf("OpenCode subagent %q is not defined", name)
		}
		if agent["mode"] != "subagent" {
			t.Fatalf("OpenCode subagent %q mode = %v, want subagent", name, agent["mode"])
		}
		if agent["hidden"] != true {
			t.Fatalf("OpenCode subagent %q hidden = %v, want true", name, agent["hidden"])
		}
	}
}

func assertNoGeneratedProductMemoryBackendWording(t *testing.T, content string) {
	t.Helper()
	for _, forbidden := range []string{"Engram", "engram"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("generated Jarvis artifact leaked forbidden memory backend wording %q\n%s", forbidden, content)
		}
	}
}

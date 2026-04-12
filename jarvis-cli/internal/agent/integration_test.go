package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	t.Setenv("HOME", home)

	// Create ~/.claude directory so IsInstalled() would be true.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer 1 — Hive Memory Protocol"
	layer2 := "# Layer 2 — Tony Stark Persona"

	if err := a.WriteInstructions(layer1, layer2); err != nil {
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
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	layer1 := "# Layer1 — Memory system instructions"
	layer2v1 := "# Layer2 v1 — argentino persona"
	layer2v2 := "# Layer2 v2 — tony-stark persona"

	// First write.
	if err := a.WriteInstructions(layer1, layer2v1); err != nil {
		t.Fatalf("first WriteInstructions: %v", err)
	}

	// Second write with different Layer2.
	if err := a.WriteInstructions(layer1, layer2v2); err != nil {
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
	t.Setenv("HOME", home)

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

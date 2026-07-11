package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ── Unit tests for removeHookEntriesByCommand ─────────────────────────────────

func TestRemoveHookEntriesByCommand_RemovesMatchingEntry(t *testing.T) {
	input := `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [{"type":"command","command":"'/usr/bin/jarvis' hook prompt-submit","timeout":2}]
      }
    ]
  }
}`
	result := removeHookEntriesByCommand([]byte(input), "UserPromptSubmit", "'/usr/bin/jarvis' hook prompt-submit")

	var root map[string]any
	if err := json.Unmarshal(result, &root); err != nil {
		t.Fatalf("result is invalid JSON: %v", err)
	}
	hooks := root["hooks"].(map[string]any)
	entries := hooks["UserPromptSubmit"].([]any)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after removal, got %d", len(entries))
	}
}

func TestRemoveHookEntriesByCommand_LeavesNonMatchingEntries(t *testing.T) {
	input := `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "name": "other-hook",
        "hooks": [{"type":"command","command":"'/usr/bin/jarvis' skill-registry refresh","timeout":5}]
      },
      {
        "hooks": [{"type":"command","command":"'/usr/bin/jarvis' hook prompt-submit","timeout":2}]
      }
    ]
  }
}`
	result := removeHookEntriesByCommand([]byte(input), "UserPromptSubmit", "'/usr/bin/jarvis' hook prompt-submit")

	var root map[string]any
	if err := json.Unmarshal(result, &root); err != nil {
		t.Fatalf("result is invalid JSON: %v", err)
	}
	hooks := root["hooks"].(map[string]any)
	entries := hooks["UserPromptSubmit"].([]any)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (other-hook preserved), got %d", len(entries))
	}
	em := entries[0].(map[string]any)
	if em["name"] != "other-hook" {
		t.Errorf("preserved entry name = %v, want 'other-hook'", em["name"])
	}
}

func TestRemoveHookEntriesByCommand_RemovesMultipleDuplicates(t *testing.T) {
	input := `{
  "hooks": {
    "UserPromptSubmit": [
      {"hooks": [{"command":"'/usr/bin/jarvis' hook prompt-submit"}]},
      {"hooks": [{"command":"'/usr/bin/jarvis' hook prompt-submit"}]},
      {"name":"keeper","hooks": [{"command":"something-else"}]}
    ]
  }
}`
	result := removeHookEntriesByCommand([]byte(input), "UserPromptSubmit", "'/usr/bin/jarvis' hook prompt-submit")

	var root map[string]any
	if err := json.Unmarshal(result, &root); err != nil {
		t.Fatalf("result is invalid JSON: %v", err)
	}
	hooks := root["hooks"].(map[string]any)
	entries := hooks["UserPromptSubmit"].([]any)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after removing 2 duplicates, got %d", len(entries))
	}
}

func TestRemoveHookEntriesByCommand_NoMatchReturnsOriginalBytes(t *testing.T) {
	input := `{"hooks":{"UserPromptSubmit":[{"name":"x","hooks":[{"command":"other"}]}]}}`
	result := removeHookEntriesByCommand([]byte(input), "UserPromptSubmit", "no-match")
	if string(result) != input {
		t.Errorf("expected original bytes unchanged, got %q", result)
	}
}

func TestRemoveHookEntriesByCommand_EmptySettingsReturnsOriginal(t *testing.T) {
	empty := []byte{}
	result := removeHookEntriesByCommand(empty, "UserPromptSubmit", "cmd")
	if len(result) != 0 {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestRemoveHookEntriesByCommand_InvalidJSONReturnsOriginal(t *testing.T) {
	bad := []byte(`{not json}`)
	result := removeHookEntriesByCommand(bad, "UserPromptSubmit", "cmd")
	if string(result) != string(bad) {
		t.Errorf("expected original bytes on invalid JSON, got %q", result)
	}
}

func TestRemoveHookEntriesByCommand_MissingHooksKeyReturnsOriginal(t *testing.T) {
	input := `{"outputStyle":"Gentleman"}`
	result := removeHookEntriesByCommand([]byte(input), "UserPromptSubmit", "cmd")
	if string(result) != input {
		t.Errorf("expected original bytes when no hooks key, got %q", result)
	}
}

func TestRemoveHookEntriesByCommand_MissingEventReturnsOriginal(t *testing.T) {
	input := `{"hooks":{"Stop":[]}}`
	result := removeHookEntriesByCommand([]byte(input), "UserPromptSubmit", "cmd")
	if string(result) != input {
		t.Errorf("expected original bytes when event is absent, got %q", result)
	}
}

// ── Unit tests for removeHookEntriesByCommandToken ────────────────────────────

// TestRemoveHookEntriesByCommandToken_StripsEntriesAcrossBinaryPaths proves the
// token matcher removes managed entries regardless of the absolute binary path
// embedded in the command (upgrade path drift).
func TestRemoveHookEntriesByCommandToken_StripsEntriesAcrossBinaryPaths(t *testing.T) {
	input := `{
  "hooks": {
    "SubagentStop": [
      {
        "hooks": [{"type":"command","command":"'/old/install/bin/jarvis' hook subagent-stop","timeout":10}]
      },
      {
        "hooks": [{"type":"command","command":"'/new/install/bin/jarvis' hook subagent-stop","timeout":10}]
      }
    ]
  }
}`
	result := removeHookEntriesByCommandToken([]byte(input), "SubagentStop", " hook subagent-stop")

	var root map[string]any
	if err := json.Unmarshal(result, &root); err != nil {
		t.Fatalf("result is invalid JSON: %v", err)
	}
	hooks := root["hooks"].(map[string]any)
	entries := hooks["SubagentStop"].([]any)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after token removal across paths, got %d", len(entries))
	}
}

// TestRemoveHookEntriesByCommandToken_KeepsUnrelatedUserHooks proves a
// user-authored jarvis hook invoking a different subcommand is preserved.
func TestRemoveHookEntriesByCommandToken_KeepsUnrelatedUserHooks(t *testing.T) {
	input := `{
  "hooks": {
    "SubagentStop": [
      {
        "name": "user-custom",
        "hooks": [{"type":"command","command":"'/usr/bin/jarvis' hook custom-thing"}]
      },
      {
        "hooks": [{"type":"command","command":"'/usr/bin/jarvis' hook subagent-stop"}]
      }
    ]
  }
}`
	result := removeHookEntriesByCommandToken([]byte(input), "SubagentStop", " hook subagent-stop")

	var root map[string]any
	if err := json.Unmarshal(result, &root); err != nil {
		t.Fatalf("result is invalid JSON: %v", err)
	}
	hooks := root["hooks"].(map[string]any)
	entries := hooks["SubagentStop"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 surviving user entry, got %d", len(entries))
	}
	em := entries[0].(map[string]any)
	if em["name"] != "user-custom" {
		t.Errorf("preserved entry name = %v, want 'user-custom'", em["name"])
	}
}

// TestRemoveHookEntriesByCommandToken_ReturnsInputWhenNoMatchOrUnparseable
// documents that empty, unparseable, and no-match inputs return the bytes
// unchanged, matching the exact-match helper's early-return semantics.
func TestRemoveHookEntriesByCommandToken_ReturnsInputWhenNoMatchOrUnparseable(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty settings", input: ""},
		{name: "invalid JSON", input: `{not json}`},
		{name: "no matching token", input: `{"hooks":{"SubagentStop":[{"hooks":[{"command":"'/usr/bin/jarvis' hook prompt-submit"}]}]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeHookEntriesByCommandToken([]byte(tt.input), "SubagentStop", " hook subagent-stop")
			if string(result) != tt.input {
				t.Errorf("expected input returned unchanged, got %q", result)
			}
		})
	}
}

// ── Unit tests for hookEntryContainsCommand ───────────────────────────────────

func TestHookEntryContainsCommand_Match(t *testing.T) {
	entry := map[string]any{
		"hooks": []any{
			map[string]any{"command": "jarvis hook prompt-submit"},
		},
	}
	if !hookEntryContainsCommand(entry, "jarvis hook prompt-submit") {
		t.Error("expected true for matching command")
	}
}

func TestHookEntryContainsCommand_NoMatch(t *testing.T) {
	entry := map[string]any{
		"hooks": []any{
			map[string]any{"command": "other-command"},
		},
	}
	if hookEntryContainsCommand(entry, "jarvis hook prompt-submit") {
		t.Error("expected false for non-matching command")
	}
}

func TestHookEntryContainsCommand_NotAMap(t *testing.T) {
	if hookEntryContainsCommand("not-a-map", "cmd") {
		t.Error("expected false for non-map entry")
	}
}

func TestHookEntryContainsCommand_NoHooksField(t *testing.T) {
	entry := map[string]any{"name": "x"}
	if hookEntryContainsCommand(entry, "cmd") {
		t.Error("expected false when entry has no hooks field")
	}
}

// ── Integration tests: legacy-entry deduplication ─────────────────────────────

// TestClaudeAgent_InstallPromptHook_DeduplicatesLegacyEntries verifies that when
// settings.json already contains old-format UserPromptSubmit entries (no "name"
// field) with the same command, InstallPromptHook removes them and leaves exactly
// one "hive-prompt-capture" entry.
func TestClaudeAgent_InstallPromptHook_DeduplicatesLegacyEntries(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	command := "'/usr/local/bin/jarvis' hook prompt-submit"

	// Seed settings.json with two legacy entries (no "name" field) plus an
	// unrelated entry that must be preserved.
	legacySettings := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				// legacy entry 1 — no name
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": command, "timeout": 2},
					},
				},
				// legacy entry 2 — no name (duplicate)
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": command, "timeout": 2},
					},
				},
				// unrelated entry that must survive
				map[string]any{
					"name": "other-tool",
					"hooks": []any{
						map[string]any{"type": "command", "command": "other-tool refresh", "timeout": 5},
					},
				},
			},
		},
	}
	raw, err := json.MarshalIndent(legacySettings, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), raw, 0644); err != nil {
		t.Fatalf("write legacy settings.json: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks := settings["hooks"].(map[string]any)
	entries := hooks["UserPromptSubmit"].([]any)

	if n := countEntriesByName(entries, "hive-prompt-capture"); n != 1 {
		t.Errorf("expected exactly 1 hive-prompt-capture entry, got %d", n)
	}
	if n := countEntriesByName(entries, "other-tool"); n != 1 {
		t.Errorf("unrelated entry 'other-tool' must be preserved, got %d", n)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 total entries (hive-prompt-capture + other-tool), got %d", len(entries))
	}
}

// TestClaudeAgent_InstallSessionHooks_DeduplicatesLegacyEntries verifies that when
// settings.json already contains old-format SessionStart/Stop entries (no "name"
// field) with the same commands, InstallSessionHooks removes them and leaves exactly
// one "hive-session-start" and one "hive-session-stop" entry.
func TestClaudeAgent_InstallSessionHooks_DeduplicatesLegacyEntries(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	startCmd := "'/usr/local/bin/jarvis' hook session-start"
	stopCmd := "'/usr/local/bin/jarvis' hook session-stop"

	// Seed with two legacy entries per event (no "name" field).
	legacySettings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{map[string]any{"command": startCmd}}},
				map[string]any{"hooks": []any{map[string]any{"command": startCmd}}},
			},
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{"command": stopCmd}}},
				map[string]any{"hooks": []any{map[string]any{"command": stopCmd}}},
			},
		},
	}
	raw, err := json.MarshalIndent(legacySettings, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), raw, 0644); err != nil {
		t.Fatalf("write legacy settings.json: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("InstallSessionHooks: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks := settings["hooks"].(map[string]any)

	sessionStart := hooks["SessionStart"].([]any)
	if n := countEntriesByName(sessionStart, "hive-session-start"); n != 1 {
		t.Errorf("expected exactly 1 hive-session-start entry, got %d", n)
	}
	if len(sessionStart) != 1 {
		t.Errorf("expected 1 total SessionStart entry, got %d", len(sessionStart))
	}

	stop := hooks["Stop"].([]any)
	if n := countEntriesByName(stop, "hive-session-stop"); n != 1 {
		t.Errorf("expected exactly 1 hive-session-stop entry, got %d", n)
	}
	if len(stop) != 1 {
		t.Errorf("expected 1 total Stop entry, got %d", len(stop))
	}
}

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveHookCommand extracts the command string from the first inner hook
// inside the named top-level hook entry (by name).
func resolveHookCommand(t *testing.T, hooks map[string]any, hookType, entryName string) string {
	t.Helper()
	entries, ok := hooks[hookType].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("settings.json missing hooks.%s array", hookType)
	}
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok || entryMap["name"] != entryName {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok || len(innerHooks) == 0 {
			t.Fatalf("entry %q missing inner hooks", entryName)
		}
		hookMap, ok := innerHooks[0].(map[string]any)
		if !ok {
			t.Fatalf("inner hook for %q is not a map", entryName)
		}
		cmd, ok := hookMap["command"].(string)
		if !ok {
			t.Fatalf("inner hook command for %q is not a string", entryName)
		}
		return cmd
	}
	t.Fatalf("entry %q not found in hooks.%s", entryName, hookType)
	return ""
}

// countEntriesByName counts how many entries in a hooks array have the given name.
func countEntriesByName(entries []any, name string) int {
	count := 0
	for _, entry := range entries {
		if em, ok := entry.(map[string]any); ok && em["name"] == name {
			count++
		}
	}
	return count
}

// countEntriesByToken counts entries whose nested hooks[].command contains the
// given managed subcommand token. Path-agnostic: the count is stable across
// binary-path drift and a stripped "name" field.
func countEntriesByToken(entries []any, token string) int {
	count := 0
	for _, entry := range entries {
		em, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := em["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, token) {
				count++
				break
			}
		}
	}
	return count
}

// seedSettings writes settings.json into the claude dir for a test home.
func seedSettings(t *testing.T, claudeDir string, settings map[string]any) {
	t.Helper()
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshal seed settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), raw, 0644); err != nil {
		t.Fatalf("write seed settings.json: %v", err)
	}
}

// nativeHookEntry builds a Claude-NORMALIZED (name-stripped) hook group entry
// wrapping a single inline command.
func nativeHookEntry(command string) map[string]any {
	return map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	}
}

// setupClaudeHome creates a temp home with a .claude dir and stubs osExecutable
// to the given absolute path, returning the agent and the claude dir.
func setupClaudeHome(t *testing.T, exe string) (*ClaudeAgent, string) {
	t.Helper()
	home := t.TempDir()
	setTestHome(t, home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	origExe := osExecutable
	osExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { osExecutable = origExe })
	return &ClaudeAgent{home: home, templatesFS: testTemplatesFS}, claudeDir
}

// ── InstallRegistryAutomation idempotency (token cleanup) ─────────────────────

func TestInstallRegistryAutomation_IdempotentAfterNameStripped(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	// Prior entry in Claude-normalized shape (no "name"), stale path.
	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				nativeHookEntry(`'/old/bin/jarvis' skill-registry refresh --quiet --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`),
			},
		},
	})

	if err := a.InstallRegistryAutomation(testHooksFS); err != nil {
		t.Fatalf("InstallRegistryAutomation: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["UserPromptSubmit"].([]any)
	if n := countEntriesByToken(entries, registryRefreshHookToken); n != 1 {
		t.Errorf("expected exactly 1 skill-registry entry after name-stripped reinstall, got %d", n)
	}
}

func TestInstallRegistryAutomation_IdempotentAcrossBinaryPathDrift(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"name":  "jarvis-skill-registry-refresh",
					"hooks": []any{map[string]any{"type": "command", "command": `'/stale/path/jarvis' skill-registry refresh --quiet --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`}},
				},
			},
		},
	})

	if err := a.InstallRegistryAutomation(testHooksFS); err != nil {
		t.Fatalf("InstallRegistryAutomation: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["UserPromptSubmit"].([]any)
	if n := countEntriesByToken(entries, registryRefreshHookToken); n != 1 {
		t.Errorf("expected exactly 1 skill-registry entry after path drift, got %d", n)
	}
}

func TestInstallRegistryAutomation_PreservesOtherUserPromptSubmitHooks(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				// Hive prompt-capture entry (different managed token) survives.
				map[string]any{
					"name":  "hive-prompt-capture",
					"hooks": []any{map[string]any{"type": "command", "command": "'/usr/local/bin/jarvis' hook prompt-submit"}},
				},
				// Unrelated user hook survives.
				map[string]any{
					"name":  "user-audit",
					"hooks": []any{map[string]any{"type": "command", "command": "'/usr/bin/jarvis' hook custom-audit"}},
				},
				// Stale skill-registry entry to be deduped.
				nativeHookEntry(`'/old/bin/jarvis' skill-registry refresh --quiet --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`),
			},
		},
	})

	if err := a.InstallRegistryAutomation(testHooksFS); err != nil {
		t.Fatalf("InstallRegistryAutomation: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["UserPromptSubmit"].([]any)
	if n := countEntriesByToken(entries, registryRefreshHookToken); n != 1 {
		t.Errorf("expected exactly 1 skill-registry entry, got %d", n)
	}
	if n := countEntriesByToken(entries, promptSubmitHookToken); n != 1 {
		t.Errorf("hive-prompt-capture entry must be preserved, got %d", n)
	}
	if n := countEntriesByName(entries, "user-audit"); n != 1 {
		t.Errorf("unrelated user hook must be preserved, got %d", n)
	}
}

// ── InstallCompactHook idempotency (token cleanup) ────────────────────────────

func TestInstallCompactHook_IdempotentAfterNameStripped(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				// Normalized shape (no "name") with matcher retained.
				map[string]any{
					"matcher": "compact",
					"hooks":   []any{map[string]any{"type": "command", "command": "'/usr/local/bin/jarvis' hook session-compact"}},
				},
			},
		},
	})

	if err := a.InstallCompactHook(); err != nil {
		t.Fatalf("InstallCompactHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if n := countEntriesByToken(entries, sessionCompactHookToken); n != 1 {
		t.Errorf("expected exactly 1 session-compact entry after name-stripped reinstall, got %d", n)
	}
}

func TestInstallCompactHook_IdempotentWithNamePresent(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"name":    "hive-session-compact",
					"matcher": "compact",
					"hooks":   []any{map[string]any{"type": "command", "command": "'/usr/local/bin/jarvis' hook session-compact"}},
				},
			},
		},
	})

	if err := a.InstallCompactHook(); err != nil {
		t.Fatalf("InstallCompactHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if n := countEntriesByToken(entries, sessionCompactHookToken); n != 1 {
		t.Errorf("expected exactly 1 session-compact entry with name present, got %d", n)
	}
}

func TestInstallCompactHook_IdempotentAcrossBinaryPathDrift(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"name":    "hive-session-compact",
					"matcher": "compact",
					"hooks":   []any{map[string]any{"type": "command", "command": "'/stale/path/jarvis' hook session-compact"}},
				},
			},
		},
	})

	if err := a.InstallCompactHook(); err != nil {
		t.Fatalf("InstallCompactHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if n := countEntriesByToken(entries, sessionCompactHookToken); n != 1 {
		t.Errorf("expected exactly 1 session-compact entry after path drift, got %d", n)
	}
}

// ── InstallSubagentStopHook idempotency (token cleanup) ───────────────────────

func TestInstallSubagentStopHook_IdempotentAfterNameStripped(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SubagentStop": []any{
				nativeHookEntry("'/usr/local/bin/jarvis' hook subagent-stop"),
			},
		},
	})

	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("InstallSubagentStopHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SubagentStop"].([]any)
	if n := countEntriesByToken(entries, subagentStopHookToken); n != 1 {
		t.Errorf("expected exactly 1 subagent-stop entry after name-stripped reinstall, got %d", n)
	}
}

func TestInstallSubagentStopHook_IdempotentWithNamePresent(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SubagentStop": []any{
				map[string]any{
					"name":  "hive-subagent-stop",
					"hooks": []any{map[string]any{"type": "command", "command": "'/usr/local/bin/jarvis' hook subagent-stop"}},
				},
			},
		},
	})

	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("InstallSubagentStopHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SubagentStop"].([]any)
	if n := countEntriesByToken(entries, subagentStopHookToken); n != 1 {
		t.Errorf("expected exactly 1 subagent-stop entry with name present, got %d", n)
	}
}

func TestInstallSubagentStopHook_IdempotentAcrossBinaryPathDrift(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SubagentStop": []any{
				nativeHookEntry("'/stale/path/jarvis' hook subagent-stop"),
			},
		},
	})

	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("InstallSubagentStopHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SubagentStop"].([]any)
	if n := countEntriesByToken(entries, subagentStopHookToken); n != 1 {
		t.Errorf("expected exactly 1 subagent-stop entry after path drift, got %d", n)
	}
}

func TestInstallSubagentStopHook_PreservesUnrelatedJarvisHookWithDifferentSubcommand(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SubagentStop": []any{
				map[string]any{
					"name":  "user-subagent-audit",
					"hooks": []any{map[string]any{"type": "command", "command": "'/usr/bin/jarvis' hook custom-subagent"}},
				},
				nativeHookEntry("'/old/bin/jarvis' hook subagent-stop"),
			},
		},
	})

	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("InstallSubagentStopHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["SubagentStop"].([]any)
	if n := countEntriesByToken(entries, subagentStopHookToken); n != 1 {
		t.Errorf("expected exactly 1 managed subagent-stop entry, got %d", n)
	}
	if n := countEntriesByName(entries, "user-subagent-audit"); n != 1 {
		t.Errorf("unrelated jarvis hook with different subcommand must be preserved, got %d", n)
	}
}

// ── D4 regression: session + prompt hooks survive binary path drift ───────────

func TestInstallSessionHooks_IdempotentAcrossBinaryPathDrift(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				nativeHookEntry("'/stale/path/jarvis' hook session-start"),
			},
			"Stop": []any{
				nativeHookEntry("'/stale/path/jarvis' hook session-stop"),
			},
		},
	})

	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("InstallSessionHooks: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks := settings["hooks"].(map[string]any)
	if n := countEntriesByToken(hooks["SessionStart"].([]any), sessionStartHookToken); n != 1 {
		t.Errorf("expected exactly 1 session-start entry after path drift, got %d", n)
	}
	if n := countEntriesByToken(hooks["Stop"].([]any), sessionStopHookToken); n != 1 {
		t.Errorf("expected exactly 1 session-stop entry after path drift, got %d", n)
	}
}

func TestInstallPromptHook_IdempotentAcrossBinaryPathDrift(t *testing.T) {
	a, claudeDir := setupClaudeHome(t, "/usr/local/bin/jarvis")

	seedSettings(t, claudeDir, map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				nativeHookEntry("'/stale/path/jarvis' hook prompt-submit"),
			},
		},
	})

	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	entries := settings["hooks"].(map[string]any)["UserPromptSubmit"].([]any)
	if n := countEntriesByToken(entries, promptSubmitHookToken); n != 1 {
		t.Errorf("expected exactly 1 prompt-submit entry after path drift, got %d", n)
	}
}

// ── Task 3.2: InstallSessionHooks uses inline commands ───────────────────────

// TestClaudeAgent_InstallSessionHooks_UsesInlineCommands verifies that after
// migration, InstallSessionHooks emits inline jarvis commands instead of script paths.
func TestClaudeAgent_InstallSessionHooks_UsesInlineCommands(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	// Override executable resolution for test
	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("InstallSessionHooks: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing 'hooks' object")
	}

	// SessionStart must use inline command
	startCmd := resolveHookCommand(t, hooks, "SessionStart", "hive-session-start")
	if !strings.Contains(startCmd, "jarvis") || !strings.Contains(startCmd, "hook") || !strings.Contains(startCmd, "session-start") {
		t.Errorf("SessionStart command %q does not reference 'jarvis hook session-start'", startCmd)
	}
	if strings.HasSuffix(startCmd, ".sh") || strings.HasSuffix(startCmd, ".ps1") {
		t.Errorf("SessionStart command must not reference a script file, got %q", startCmd)
	}

	// Stop must use inline command
	stopCmd := resolveHookCommand(t, hooks, "Stop", "hive-session-stop")
	if !strings.Contains(stopCmd, "jarvis") || !strings.Contains(stopCmd, "hook") || !strings.Contains(stopCmd, "session-stop") {
		t.Errorf("Stop command %q does not reference 'jarvis hook session-stop'", stopCmd)
	}
	if strings.HasSuffix(stopCmd, ".sh") || strings.HasSuffix(stopCmd, ".ps1") {
		t.Errorf("Stop command must not reference a script file, got %q", stopCmd)
	}
}

// TestClaudeAgent_InstallSessionHooks_NoScriptFilesCreated verifies that
// no session script files are written to hive-hooks/ after migration.
func TestClaudeAgent_InstallSessionHooks_NoScriptFilesCreated(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("InstallSessionHooks: %v", err)
	}

	for _, script := range []string{"session-start.sh", "session-start.ps1", "session-stop.sh", "session-stop.ps1"} {
		path := filepath.Join(claudeDir, "hive-hooks", script)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("script file %q must not be written after migration; stat error: %v", path, err)
		}
	}
}

// TestClaudeAgent_InstallSessionHooks_Idempotent_InlineMode verifies idempotency
// with inline commands (no duplicate entries after two calls).
func TestClaudeAgent_InstallSessionHooks_Idempotent_InlineMode(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("first InstallSessionHooks: %v", err)
	}
	if err := a.InstallSessionHooks(testHooksFS); err != nil {
		t.Fatalf("second InstallSessionHooks: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks := settings["hooks"].(map[string]any)

	sessionStart := hooks["SessionStart"].([]any)
	if n := countEntriesByName(sessionStart, "hive-session-start"); n != 1 {
		t.Errorf("expected exactly 1 hive-session-start entry, got %d", n)
	}

	stop := hooks["Stop"].([]any)
	if n := countEntriesByName(stop, "hive-session-stop"); n != 1 {
		t.Errorf("expected exactly 1 hive-session-stop entry, got %d", n)
	}
}

// ── Task 3.3: InstallCompactHook ─────────────────────────────────────────────

// TestClaudeAgent_InstallCompactHook_AddsSessionStartEntry verifies that
// InstallCompactHook appends a second SessionStart entry with matcher "compact".
func TestClaudeAgent_InstallCompactHook_AddsSessionStartEntry(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallCompactHook(); err != nil {
		t.Fatalf("InstallCompactHook: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing 'hooks' object")
	}

	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok || len(sessionStart) == 0 {
		t.Fatal("settings.json missing hooks.SessionStart array")
	}

	foundCompact := false
	for _, entry := range sessionStart {
		em, ok := entry.(map[string]any)
		if !ok || em["name"] != "hive-session-compact" {
			continue
		}
		foundCompact = true

		// Must have matcher: "compact"
		if em["matcher"] != "compact" {
			t.Errorf("hive-session-compact entry missing matcher='compact', got %v", em["matcher"])
		}

		// Command must reference jarvis hook session-compact
		innerHooks, ok := em["hooks"].([]any)
		if !ok || len(innerHooks) == 0 {
			t.Fatal("hive-session-compact missing inner hooks")
		}
		hookMap, ok := innerHooks[0].(map[string]any)
		if !ok {
			t.Fatal("inner hook is not a map")
		}
		cmd, ok := hookMap["command"].(string)
		if !ok {
			t.Fatal("inner hook command is not a string")
		}
		if !strings.Contains(cmd, "jarvis") || !strings.Contains(cmd, "hook") || !strings.Contains(cmd, "session-compact") {
			t.Errorf("compact hook command %q must reference 'jarvis hook session-compact'", cmd)
		}
	}
	if !foundCompact {
		t.Error("hive-session-compact entry not found in SessionStart hooks")
	}
}

// TestClaudeAgent_InstallCompactHook_Idempotent verifies that calling
// InstallCompactHook twice leaves exactly one hive-session-compact entry.
func TestClaudeAgent_InstallCompactHook_Idempotent(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallCompactHook(); err != nil {
		t.Fatalf("first InstallCompactHook: %v", err)
	}
	if err := a.InstallCompactHook(); err != nil {
		t.Fatalf("second InstallCompactHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks := settings["hooks"].(map[string]any)
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatal("settings.json missing SessionStart")
	}

	if n := countEntriesByName(sessionStart, "hive-session-compact"); n != 1 {
		t.Errorf("idempotency: expected exactly 1 hive-session-compact entry, got %d", n)
	}
}

// ── Task 3.4: InstallPromptHook uses inline command ──────────────────────────

// TestClaudeAgent_InstallPromptHook_UsesInlineCommand verifies that after
// migration, InstallPromptHook emits an inline jarvis command, not a script path.
func TestClaudeAgent_InstallPromptHook_UsesInlineCommand(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing hooks object")
	}

	promptCmd := resolveHookCommand(t, hooks, "UserPromptSubmit", "hive-prompt-capture")
	if !strings.Contains(promptCmd, "jarvis") || !strings.Contains(promptCmd, "hook") || !strings.Contains(promptCmd, "prompt-submit") {
		t.Errorf("prompt hook command %q must reference 'jarvis hook prompt-submit'", promptCmd)
	}
	if strings.HasSuffix(promptCmd, ".sh") || strings.HasSuffix(promptCmd, ".ps1") {
		t.Errorf("prompt hook command must not reference a script file, got %q", promptCmd)
	}
}

// TestClaudeAgent_InstallPromptHook_NoScriptFileCreated verifies no script
// file is written for the prompt hook after migration.
func TestClaudeAgent_InstallPromptHook_NoScriptFileCreated(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallPromptHook(testHooksFS); err != nil {
		t.Fatalf("InstallPromptHook: %v", err)
	}

	for _, script := range []string{"user-prompt-submit.sh", "user-prompt-submit.ps1"} {
		path := filepath.Join(claudeDir, "hive-hooks", script)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("script %q must not be written after migration", path)
		}
	}
}

// ── Task 3.5: InstallSubagentStopHook ────────────────────────────────────────

// TestClaudeAgent_InstallSubagentStopHook_AddsSubagentStopEntry verifies that
// InstallSubagentStopHook writes a SubagentStop entry in settings.json.
func TestClaudeAgent_InstallSubagentStopHook_AddsSubagentStopEntry(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("InstallSubagentStopHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("settings.json missing 'hooks' object")
	}

	subagentStop, ok := hooks["SubagentStop"].([]any)
	if !ok || len(subagentStop) == 0 {
		t.Fatal("settings.json missing hooks.SubagentStop array")
	}

	foundEntry := false
	for _, entry := range subagentStop {
		em, ok := entry.(map[string]any)
		if !ok || em["name"] != "hive-subagent-stop" {
			continue
		}
		foundEntry = true

		innerHooks, ok := em["hooks"].([]any)
		if !ok || len(innerHooks) == 0 {
			t.Fatal("hive-subagent-stop missing inner hooks")
		}
		hookMap, ok := innerHooks[0].(map[string]any)
		if !ok {
			t.Fatal("inner hook is not a map")
		}
		cmd, ok := hookMap["command"].(string)
		if !ok {
			t.Fatal("inner hook command is not a string")
		}
		if !strings.Contains(cmd, "jarvis") || !strings.Contains(cmd, "hook") || !strings.Contains(cmd, "subagent-stop") {
			t.Errorf("subagent-stop command %q must reference 'jarvis hook subagent-stop'", cmd)
		}

		// Must have timeout: 10
		timeout, ok := hookMap["timeout"].(float64)
		if !ok {
			t.Fatal("subagent-stop hook must have a numeric timeout")
		}
		if int(timeout) != 10 {
			t.Errorf("subagent-stop timeout = %v, want 10", timeout)
		}

		// Must have async: true
		asyncVal, ok := hookMap["async"].(bool)
		if !ok || !asyncVal {
			t.Errorf("subagent-stop hook must have async=true, got %v", hookMap["async"])
		}
	}
	if !foundEntry {
		t.Error("hive-subagent-stop entry not found in SubagentStop hooks")
	}
}

// TestClaudeAgent_InstallSubagentStopHook_Idempotent verifies that calling
// InstallSubagentStopHook twice leaves exactly one hive-subagent-stop entry.
func TestClaudeAgent_InstallSubagentStopHook_Idempotent(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	origExe := osExecutable
	osExecutable = func() (string, error) { return "/usr/local/bin/jarvis", nil }
	t.Cleanup(func() { osExecutable = origExe })

	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("first InstallSubagentStopHook: %v", err)
	}
	if err := a.InstallSubagentStopHook(); err != nil {
		t.Fatalf("second InstallSubagentStopHook: %v", err)
	}

	settings := readSettingsMap(t, filepath.Join(claudeDir, "settings.json"))
	hooks := settings["hooks"].(map[string]any)
	subagentStop, ok := hooks["SubagentStop"].([]any)
	if !ok {
		t.Fatal("settings.json missing SubagentStop")
	}

	if n := countEntriesByName(subagentStop, "hive-subagent-stop"); n != 1 {
		t.Errorf("idempotency: expected exactly 1 hive-subagent-stop entry, got %d", n)
	}
}

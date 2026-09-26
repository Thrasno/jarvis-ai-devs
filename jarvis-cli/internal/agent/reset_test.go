package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

const sampleClaudeMD = `# CLAUDE.md

Some user-owned preamble the reset must never touch.

<!-- JARVIS:LAYER1:START -->
layer1 content
<!-- JARVIS:LAYER1:END -->

<!-- JARVIS:LAYER2:START -->
layer2 content
<!-- JARVIS:LAYER2:END -->

Some user-owned trailer the reset must never touch.
`

// --- Clean/fresh tree: empty plan, no writes ---

func TestPlanAndApplyReset_AbsentConfigDir_EmptyPlanNoWrites(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")

	plan, err := PlanReset(sddruntime.PlatformClaude, configDir)
	if err != nil {
		t.Fatalf("PlanReset: %v", err)
	}
	if len(plan.Changes) != 0 {
		t.Fatalf("plan.Changes = %+v, want empty", plan.Changes)
	}

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID != "" || len(result.Changes) != 0 {
		t.Fatalf("result = %+v, want empty", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "backups")); !os.IsNotExist(err) {
		t.Fatalf("backups dir should not be created for an empty plan, stat err = %v", err)
	}
}

func TestApplyReset_IsIdempotent(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	mustWriteFile(t, filepath.Join(configDir, "CLAUDE.md"), sampleClaudeMD)
	mustWriteFile(t, filepath.Join(configDir, "agents", "sdd-init.md"), "agent content")

	first, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("first ApplyReset: %v", err)
	}
	if len(first.Changes) == 0 {
		t.Fatalf("first ApplyReset made no changes, test setup is wrong")
	}

	second, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("second ApplyReset: %v", err)
	}
	if second.SnapshotID != "" || len(second.Changes) != 0 {
		t.Fatalf("second ApplyReset = %+v, want empty (idempotent)", second)
	}
}

// --- Legacy 4R residue removed ---

func TestApplyReset_Claude_RemovesLegacy4RResidueAgentFiles(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	for _, name := range sddruntime.RetiredClaudeReviewAgentBaseNames() {
		mustWriteFile(t, filepath.Join(configDir, "agents", name+".md"), "retired agent")
	}

	plan, err := PlanReset(sddruntime.PlatformClaude, configDir)
	if err != nil {
		t.Fatalf("PlanReset: %v", err)
	}
	if len(plan.Changes) != 4 {
		t.Fatalf("plan.Changes = %+v, want 4 retired agent removals", plan.Changes)
	}

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}
	for _, name := range sddruntime.RetiredClaudeReviewAgentBaseNames() {
		if _, err := os.Stat(filepath.Join(configDir, "agents", name+".md")); !os.IsNotExist(err) {
			t.Fatalf("retired agent %s still present after reset", name)
		}
	}
}

func TestApplyReset_OpenCode_RemovesLegacy4RResidueFromAgentAndPermission(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	content := `{
  "theme": "night",
  "default_agent": "orchestrator",
  "permission": {"task": {"review-risk": "allow", "sdd-apply": "allow"}},
  "agent": {"review-risk": {"mode": "subagent"}, "sdd-apply": {"mode": "subagent"}},
  "mcp": {"hive": {"type": "local", "command": ["/bin/hive"]}, "unrelated": {"type": "remote", "url": "https://example.com"}}
}
`
	mustWriteFile(t, filepath.Join(configDir, "opencode.json"), content)

	result, err := ApplyReset(sddruntime.PlatformOpenCode, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}
	after := mustReadFile(t, filepath.Join(configDir, "opencode.json"))
	if strings.Contains(after, "\"agent\"") || strings.Contains(after, "\"permission\"") || strings.Contains(after, "\"default_agent\"") {
		t.Fatalf("expected agent/permission/default_agent entirely removed, got: %s", after)
	}
	if !strings.Contains(after, "\"theme\": \"night\"") {
		t.Fatalf("unrelated key theme was not preserved: %s", after)
	}
	if strings.Contains(after, "\"hive\"") {
		t.Fatalf("mcp.hive was not removed: %s", after)
	}
	if !strings.Contains(after, "\"unrelated\"") {
		t.Fatalf("unrelated mcp server was not preserved: %s", after)
	}
}

// --- User-added content reported and removed ---

func TestPlanReset_Claude_ReportsUserAddedAgentAsUserAdded(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	mustWriteFile(t, filepath.Join(configDir, "agents", "sdd-init.md"), "owned agent")
	mustWriteFile(t, filepath.Join(configDir, "agents", "my-custom-agent.md"), "user agent")

	plan, err := PlanReset(sddruntime.PlatformClaude, configDir)
	if err != nil {
		t.Fatalf("PlanReset: %v", err)
	}
	var ownedDetail, userDetail string
	for _, c := range plan.Changes {
		switch {
		case strings.Contains(c.Detail, "sdd-init.md"):
			ownedDetail = c.Detail
		case strings.Contains(c.Detail, "my-custom-agent.md"):
			userDetail = c.Detail
		}
	}
	if strings.Contains(ownedDetail, "user-added") {
		t.Fatalf("owned agent incorrectly flagged as user-added: %q", ownedDetail)
	}
	if !strings.Contains(userDetail, "user-added") {
		t.Fatalf("user-added agent not flagged: %q", userDetail)
	}

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}
	if _, err := os.Stat(filepath.Join(configDir, "agents", "my-custom-agent.md")); !os.IsNotExist(err) {
		t.Fatalf("user-added agent still present after reset")
	}
}

func TestPlanReset_OpenCode_ReportsExtraAgentEntries(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	content := `{"agent": {"sdd-init": {"mode": "subagent"}, "my-custom-agent": {"mode": "subagent"}}}`
	mustWriteFile(t, filepath.Join(configDir, "opencode.json"), content)

	plan, err := PlanReset(sddruntime.PlatformOpenCode, configDir)
	if err != nil {
		t.Fatalf("PlanReset: %v", err)
	}
	found := false
	for _, c := range plan.Changes {
		if c.SurfaceID == "opencode.settings.core_keys" {
			found = true
			if !strings.Contains(c.Detail, "my-custom-agent") {
				t.Fatalf("expected extra agent entry reported, got detail: %q", c.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("expected opencode.settings.core_keys change, got %+v", plan.Changes)
	}
}

// --- Unrelated content preserved ---

func TestApplyReset_Claude_PreservesUnrelatedSettingsAndMarkerBoundaries(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	settings := `{
  "theme": "dark",
  "outputStyle": "MyOwnStyle",
  "statusLine": {"type": "command", "command": "bash ~/.claude/my-own-statusline.sh"},
  "permissions": {"allow": ["Bash(git status:*)", "Bash(npm test:*)"], "deny": ["Bash(rm -rf /*)"], "defaultMode": "acceptEdits"}
}
`
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), settings)
	mustWriteFile(t, filepath.Join(configDir, "CLAUDE.md"), sampleClaudeMD)
	mustWriteFile(t, filepath.Join(configDir, "unrelated-notes.txt"), "do not touch me")

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}

	after := mustReadFile(t, filepath.Join(configDir, "settings.json"))
	var decoded map[string]any
	if err := json.Unmarshal([]byte(after), &decoded); err != nil {
		t.Fatalf("settings.json is invalid JSON after reset: %v", err)
	}
	if decoded["theme"] != "dark" {
		t.Fatalf("unrelated key theme lost: %+v", decoded)
	}
	if _, ok := decoded["outputStyle"]; !ok {
		t.Fatalf("user-owned outputStyle was removed: %+v", decoded)
	}
	if _, ok := decoded["statusLine"]; !ok {
		t.Fatalf("user-owned statusLine was removed: %+v", decoded)
	}
	permissions, _ := decoded["permissions"].(map[string]any)
	if permissions == nil {
		t.Fatalf("permissions key lost: %+v", decoded)
	}
	allow, _ := permissions["allow"].([]any)
	if len(allow) != 1 || allow[0] != "Bash(npm test:*)" {
		t.Fatalf("permissions.allow = %+v, want only the user-owned entry preserved", allow)
	}
	deny, _ := permissions["deny"].([]any)
	if len(deny) != 0 {
		t.Fatalf("permissions.deny = %+v, want the Jarvis literal removed", deny)
	}
	if permissions["defaultMode"] != "acceptEdits" {
		t.Fatalf("user-owned defaultMode lost: %+v", permissions)
	}

	claudeMD := mustReadFile(t, filepath.Join(configDir, "CLAUDE.md"))
	if strings.Contains(claudeMD, "layer1 content") || strings.Contains(claudeMD, "layer2 content") {
		t.Fatalf("CLAUDE.md marker block content was not stripped: %s", claudeMD)
	}
	if !strings.Contains(claudeMD, "Some user-owned preamble") || !strings.Contains(claudeMD, "Some user-owned trailer") {
		t.Fatalf("CLAUDE.md content outside markers was not preserved: %s", claudeMD)
	}

	if mustReadFile(t, filepath.Join(configDir, "unrelated-notes.txt")) != "do not touch me" {
		t.Fatalf("unrelated file was mutated")
	}
}

// --- Mixed hook groups ---

func TestApplyReset_Claude_HookGroup_RemovesOnlyMatchingNestedCommandKeepsMatcherAndUnrelated(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	settings := `{
  "hooks": {
    "SessionStart": [
      {
        "name": "mixed-group",
        "matcher": "compact",
        "hooks": [
          {"type": "command", "command": "'/opt/jarvis' hook session-compact", "timeout": 5},
          {"type": "command", "command": "/usr/local/bin/my-tool --flag", "timeout": 3}
        ]
      },
      {
        "name": "hive-session-start",
        "hooks": [
          {"type": "command", "command": "'/opt/jarvis' hook session-start", "timeout": 5}
        ]
      }
    ],
    "SubagentStop": [
      {
        "name": "user-subagent-stopwatch",
        "hooks": [
          {"type": "command", "command": "/opt/other hook subagent-stopwatch", "timeout": 5}
        ]
      }
    ]
  }
}
`
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), settings)

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}

	after := mustReadFile(t, filepath.Join(configDir, "settings.json"))
	var decoded struct {
		Hooks map[string][]struct {
			Name    string `json:"name"`
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(after), &decoded); err != nil {
		t.Fatalf("settings.json is invalid JSON after reset: %v\n%s", err, after)
	}

	sessionStart := decoded.Hooks["SessionStart"]
	if len(sessionStart) != 1 {
		t.Fatalf("SessionStart groups = %+v, want exactly the mixed group to survive", sessionStart)
	}
	mixed := sessionStart[0]
	if mixed.Name != "mixed-group" || mixed.Matcher != "compact" {
		t.Fatalf("mixed group identity/matcher not preserved: %+v", mixed)
	}
	if len(mixed.Hooks) != 1 || !strings.Contains(mixed.Hooks[0].Command, "my-tool") {
		t.Fatalf("mixed group nested hooks = %+v, want only the unrelated command left", mixed.Hooks)
	}

	subagentStop := decoded.Hooks["SubagentStop"]
	if len(subagentStop) != 1 || !strings.Contains(subagentStop[0].Hooks[0].Command, "subagent-stopwatch") {
		t.Fatalf("SubagentStop = %+v, want the near-miss subagent-stopwatch command preserved", subagentStop)
	}
}

// --- Failure midway restores backup ---

func TestApplyReset_FailureMidway_RestoresPriorBytesAndDeletesCreatedContent(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	originalSettings := `{"permissions": {"allow": ["Bash(git status:*)"]}}`
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), originalSettings)
	mustWriteFile(t, filepath.Join(configDir, "CLAUDE.md"), sampleClaudeMD)

	settingsPath := filepath.Join(configDir, "settings.json")
	claudeMDPath := filepath.Join(configDir, "CLAUDE.md")

	base := defaultResetFileOps()
	origWrite := base.writeFile
	base.writeFile = func(path string, data []byte, mode os.FileMode) error {
		if path == claudeMDPath {
			return fmt.Errorf("injected apply failure")
		}
		return origWrite(path, data, mode)
	}

	_, err := applyResetWithOps(sddruntime.PlatformClaude, configDir, home, base)
	if err == nil {
		t.Fatalf("expected an error from the injected failure")
	}
	// Rollback fully restored prior state here, so the caller must see the
	// plain wrapped cause, NOT a *ResetRestoreError: that type is reserved
	// for an incomplete rollback (see the sibling restore-failure test).
	var restoreErr *ResetRestoreError
	if errors.As(err, &restoreErr) {
		t.Fatalf("error = %v (*ResetRestoreError), want a plain wrapped error since rollback fully succeeded", err)
	}
	if !strings.Contains(err.Error(), "injected apply failure") {
		t.Fatalf("error = %q, want it to wrap the injected cause", err)
	}
	if !strings.Contains(err.Error(), "rollback restored prior state") {
		t.Fatalf("error = %q, want it to say rollback restored prior state", err)
	}

	if got := mustReadFile(t, settingsPath); got != originalSettings {
		t.Fatalf("settings.json = %q, want original bytes restored: %q", got, originalSettings)
	}
	if got := mustReadFile(t, claudeMDPath); got != sampleClaudeMD {
		t.Fatalf("CLAUDE.md was mutated despite the injected failure: %q", got)
	}
}

func TestApplyReset_FailureDuringRestore_ReportsSnapshotIDAndUnrecoveredPaths(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	originalSettings := `{"permissions": {"allow": ["Bash(git status:*)"]}}`
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), originalSettings)
	mustWriteFile(t, filepath.Join(configDir, "CLAUDE.md"), sampleClaudeMD)

	settingsPath := filepath.Join(configDir, "settings.json")
	claudeMDPath := filepath.Join(configDir, "CLAUDE.md")

	base := defaultResetFileOps()
	origWrite := base.writeFile
	callCount := map[string]int{}
	base.writeFile = func(path string, data []byte, mode os.FileMode) error {
		callCount[path]++
		switch {
		case path == claudeMDPath:
			return fmt.Errorf("injected apply failure")
		case path == settingsPath && callCount[path] == 2:
			return fmt.Errorf("injected restore failure")
		default:
			return origWrite(path, data, mode)
		}
	}

	_, err := applyResetWithOps(sddruntime.PlatformClaude, configDir, home, base)
	if err == nil {
		t.Fatalf("expected an error from the injected failures")
	}
	restoreErr, ok := err.(*ResetRestoreError)
	if !ok {
		t.Fatalf("error = %T, want *ResetRestoreError", err)
	}
	if restoreErr.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID on the restore error")
	}
	if len(restoreErr.UnrecoveredPaths) != 1 || restoreErr.UnrecoveredPaths[0] != settingsPath {
		t.Fatalf("UnrecoveredPaths = %v, want [%s]", restoreErr.UnrecoveredPaths, settingsPath)
	}
}

// --- Unit-level rollback branch coverage: absent-before path gets deleted ---

func TestRollbackResetMutations_DeletesPathThatWasAbsentBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(path, []byte("new content"), 0o644); err != nil {
		t.Fatalf("seed created file: %v", err)
	}

	ops := defaultResetFileOps()
	applied := []resetFileMutation{
		{Path: path, existed: false, before: nil},
	}
	unrecovered := rollbackResetMutations(applied, ops)
	if len(unrecovered) != 0 {
		t.Fatalf("unrecovered = %v, want empty", unrecovered)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path %s still exists after rollback of a not-existed-before entry", path)
	}
}

func TestPlanReset_UnsupportedPlatform_ReturnsError(t *testing.T) {
	if _, err := PlanReset(sddruntime.Platform("unknown"), t.TempDir()); err == nil {
		t.Fatalf("expected an error for an unsupported platform")
	}
}

func TestApplyReset_SkillsDirectory_RemovesAllFilesRecursively(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	mustWriteFile(t, filepath.Join(configDir, "skills", "foo", "SKILL.md"), "skill")
	mustWriteFile(t, filepath.Join(configDir, "skills", "_shared", "helper.md"), "shared")

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if len(result.Changes) != 2 {
		t.Fatalf("result.Changes = %+v, want 2 skill files removed", result.Changes)
	}
	names := make([]string, 0, len(result.Changes))
	for _, c := range result.Changes {
		names = append(names, c.Detail)
	}
	sort.Strings(names)
	if names[0] != "_shared/helper.md" || names[1] != "foo/SKILL.md" {
		t.Fatalf("result.Changes details = %v", names)
	}
}

// --- R3-002: symlinks under a whole-directory surface ---

func TestApplyReset_Claude_FileSymlink_RemovedWithoutReadingThroughIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows needs elevated privileges")
	}
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	mustWriteFile(t, filepath.Join(configDir, "agents", "sdd-init.md"), "owned agent")

	linkPath := filepath.Join(configDir, "agents", "dangling-link.md")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A dangling target proves the reset never calls os.ReadFile through the
	// symlink: reading through it would fail with ENOENT.
	if err := os.Symlink(filepath.Join(home, "nonexistent-target-xyz"), linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	plan, err := PlanReset(sddruntime.PlatformClaude, configDir)
	if err != nil {
		t.Fatalf("PlanReset: %v", err)
	}
	var symlinkDetail string
	for _, c := range plan.Changes {
		if strings.Contains(c.Detail, "dangling-link.md") {
			symlinkDetail = c.Detail
		}
	}
	if !strings.Contains(symlinkDetail, "symlink") {
		t.Fatalf("expected the symlink entry flagged as a symlink, got %q", symlinkDetail)
	}
	if !strings.Contains(symlinkDetail, "user-added") {
		t.Fatalf("expected the symlink entry flagged as user-added, got %q", symlinkDetail)
	}

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("symlink still present after reset, lstat err = %v", err)
	}
}

func TestApplyReset_Claude_DirectorySymlink_NeverFollowedOrDescendedInto(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows needs elevated privileges")
	}
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")

	targetDir := filepath.Join(home, "outside-target-dir")
	mustWriteFile(t, filepath.Join(targetDir, "untouched.txt"), "must survive")

	linkPath := filepath.Join(configDir, "agents", "linked-dir")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(targetDir, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	plan, err := PlanReset(sddruntime.PlatformClaude, configDir)
	if err != nil {
		t.Fatalf("PlanReset: %v", err)
	}
	for _, c := range plan.Changes {
		if strings.Contains(c.Detail, "untouched.txt") {
			t.Fatalf("plan descended into the symlinked directory, got %+v", plan.Changes)
		}
	}
	var symlinkDetail string
	for _, c := range plan.Changes {
		if strings.Contains(c.Detail, "linked-dir") {
			symlinkDetail = c.Detail
		}
	}
	if !strings.Contains(symlinkDetail, "symlink") {
		t.Fatalf("expected the directory symlink entry flagged as a symlink, got %q", symlinkDetail)
	}

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("directory symlink still present after reset, lstat err = %v", err)
	}
	if got := mustReadFile(t, filepath.Join(targetDir, "untouched.txt")); got != "must survive" {
		t.Fatalf("file behind the symlinked directory was mutated: %q", got)
	}
}

// --- R3-003: restored file mode preserves the original permission bits ---

func TestApplyReset_RollbackRestoresOriginalExecutableBit(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	scriptPath := filepath.Join(configDir, "statusline-command.sh")
	skillPath := filepath.Join(configDir, "skills", "foo", "SKILL.md")
	scriptContent := "#!/bin/sh\necho hi\n"
	mustWriteFile(t, skillPath, "skill")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	base := defaultResetFileOps()
	origRemove := base.removeFile
	base.removeFile = func(path string) error {
		if path == skillPath {
			return fmt.Errorf("injected failure removing skill file")
		}
		return origRemove(path)
	}

	_, err := applyResetWithOps(sddruntime.PlatformClaude, configDir, home, base)
	if err == nil {
		t.Fatalf("expected an error from the injected failure")
	}

	info, statErr := os.Stat(scriptPath)
	if statErr != nil {
		t.Fatalf("statusline-command.sh was not restored: %v", statErr)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("statusline-command.sh mode = %o, want 0755 (executable bit preserved)", info.Mode().Perm())
	}
	if got := mustReadFile(t, scriptPath); got != scriptContent {
		t.Fatalf("statusline-command.sh content = %q, want %q", got, scriptContent)
	}
}

// --- R3-004: an unparseable existing settings file is an explicit error ---

func TestPlanReset_Claude_UnparseableSettings_ReturnsExplicitError(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	settingsPath := filepath.Join(configDir, "settings.json")
	mustWriteFile(t, settingsPath, "{ this is not valid json")

	if _, err := PlanReset(sddruntime.PlatformClaude, configDir); err == nil {
		t.Fatalf("expected an error for an unparseable settings.json")
	} else if !strings.Contains(err.Error(), "settings.json") {
		t.Fatalf("error = %q, want it to name settings.json", err)
	}

	if _, err := ApplyReset(sddruntime.PlatformClaude, configDir, home); err == nil {
		t.Fatalf("expected ApplyReset to also refuse an unparseable settings.json")
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "backups")); !os.IsNotExist(err) {
		t.Fatalf("a refused reset must not create a backup, stat err = %v", err)
	}
	if got := mustReadFile(t, settingsPath); got != "{ this is not valid json" {
		t.Fatalf("the unparseable file must be left untouched, got %q", got)
	}
}

func TestPlanReset_OpenCode_UnparseableSettings_ReturnsExplicitError(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	settingsPath := filepath.Join(configDir, "opencode.json")
	mustWriteFile(t, settingsPath, "not json at all")

	if _, err := PlanReset(sddruntime.PlatformOpenCode, configDir); err == nil {
		t.Fatalf("expected an error for an unparseable opencode.json")
	} else if !strings.Contains(err.Error(), "opencode.json") {
		t.Fatalf("error = %q, want it to name opencode.json", err)
	}
}

// --- R3-005: positive-removal coverage for branches that only had
// preservation tests before ---

func TestApplyReset_Claude_RemovesJarvisPersonaOutputStyle(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	names, err := claudeManagedOutputStyleNames()
	if err != nil || len(names) == 0 {
		t.Fatalf("claudeManagedOutputStyleNames() = %v, %v, want at least one built-in name", names, err)
	}
	settings := fmt.Sprintf("{\"outputStyle\": %q}\n", names[0])
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), settings)

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	after := mustReadFile(t, filepath.Join(configDir, "settings.json"))
	if strings.Contains(after, "outputStyle") {
		t.Fatalf("Jarvis-emitted outputStyle was not removed: %s", after)
	}
	if !hasChangeSurface(result.Changes, "claude.settings.output_style") {
		t.Fatalf("expected claude.settings.output_style change, got %+v", result.Changes)
	}
}

func TestApplyReset_Claude_RemovesExactManagedStatusLineCommand(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	settings := `{"statusLine": {"type": "command", "command": "bash ~/.claude/statusline-command.sh"}}` + "\n"
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), settings)

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	after := mustReadFile(t, filepath.Join(configDir, "settings.json"))
	if strings.Contains(after, "statusLine") {
		t.Fatalf("exact managed statusLine command was not removed: %s", after)
	}
	if !hasChangeSurface(result.Changes, "claude.settings.status_line") {
		t.Fatalf("expected claude.settings.status_line change, got %+v", result.Changes)
	}
}

func TestApplyReset_Claude_RemovesDefaultModeBypassPermissions(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	settings := `{"permissions": {"defaultMode": "bypassPermissions"}}` + "\n"
	mustWriteFile(t, filepath.Join(configDir, "settings.json"), settings)

	result, err := ApplyReset(sddruntime.PlatformClaude, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	after := mustReadFile(t, filepath.Join(configDir, "settings.json"))
	var decoded map[string]any
	if err := json.Unmarshal([]byte(after), &decoded); err != nil {
		t.Fatalf("settings.json is invalid JSON after reset: %v", err)
	}
	permissions, _ := decoded["permissions"].(map[string]any)
	if _, ok := permissions["defaultMode"]; ok {
		t.Fatalf("permissions.defaultMode == bypassPermissions was not removed: %+v", permissions)
	}
	if !hasChangeSurface(result.Changes, "claude.settings.default_mode") {
		t.Fatalf("expected claude.settings.default_mode change, got %+v", result.Changes)
	}
}

const sampleAgentsMD = `# AGENTS.md

Some user-owned preamble the reset must never touch.

<!-- JARVIS:LAYER1:START -->
layer1 content
<!-- JARVIS:LAYER1:END -->

<!-- JARVIS:LAYER2:START -->
layer2 content
<!-- JARVIS:LAYER2:END -->

Some user-owned trailer the reset must never touch.
`

func TestApplyReset_OpenCode_StripsAGENTSMarkerBlock(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	mustWriteFile(t, filepath.Join(configDir, "AGENTS.md"), sampleAgentsMD)

	result, err := ApplyReset(sddruntime.PlatformOpenCode, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	if result.SnapshotID == "" {
		t.Fatalf("expected a snapshot ID")
	}
	after := mustReadFile(t, filepath.Join(configDir, "AGENTS.md"))
	if strings.Contains(after, "layer1 content") || strings.Contains(after, "layer2 content") {
		t.Fatalf("AGENTS.md marker block content was not stripped: %s", after)
	}
	if !strings.Contains(after, "Some user-owned preamble") || !strings.Contains(after, "Some user-owned trailer") {
		t.Fatalf("AGENTS.md content outside markers was not preserved: %s", after)
	}
}

func TestApplyReset_OpenCode_RemovesPluginFiles(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	hivePluginPath := filepath.Join(configDir, "plugins", "hive.ts")
	registryPluginPath := filepath.Join(configDir, "plugins", "skill-registry.ts")
	mustWriteFile(t, hivePluginPath, "hive plugin")
	mustWriteFile(t, registryPluginPath, "registry plugin")

	result, err := ApplyReset(sddruntime.PlatformOpenCode, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	for _, path := range []string{hivePluginPath, registryPluginPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("plugin file %s still present after reset", path)
		}
	}
	if !hasChangeSurface(result.Changes, "opencode.plugins.hive_hook") {
		t.Fatalf("expected opencode.plugins.hive_hook change, got %+v", result.Changes)
	}
	if !hasChangeSurface(result.Changes, "opencode.plugins.registry_hook") {
		t.Fatalf("expected opencode.plugins.registry_hook change, got %+v", result.Changes)
	}
}

func TestApplyReset_OpenCode_RemovesMCPContext7(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	content := `{"mcp": {"context7": {"type": "remote", "url": "https://example.com"}, "other": {"type": "remote", "url": "https://other.example.com"}}}` + "\n"
	mustWriteFile(t, filepath.Join(configDir, "opencode.json"), content)

	result, err := ApplyReset(sddruntime.PlatformOpenCode, configDir, home)
	if err != nil {
		t.Fatalf("ApplyReset: %v", err)
	}
	after := mustReadFile(t, filepath.Join(configDir, "opencode.json"))
	if strings.Contains(after, "context7") {
		t.Fatalf("mcp.context7 was not removed: %s", after)
	}
	if !strings.Contains(after, "other") {
		t.Fatalf("unrelated mcp server was not preserved: %s", after)
	}
	if !hasChangeSurface(result.Changes, "opencode.settings.mcp") {
		t.Fatalf("expected opencode.settings.mcp change, got %+v", result.Changes)
	}
}

func hasChangeSurface(changes []ResetChange, surfaceID string) bool {
	for _, c := range changes {
		if c.SurfaceID == surfaceID {
			return true
		}
	}
	return false
}

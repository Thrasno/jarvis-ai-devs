package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEmbeddedPromptCaptureHooks_PostProjectSessionMetadata(t *testing.T) {
	t.Parallel()

	assertHookContract(t, "opencode plugin", opencodeEmbeddedHookPath(t, "hive.ts"), "session_id: sessionId", "resolveHiveDirectory", "resolveHiveProject")
	assertHookContract(t, "claude shell hook", claudeEmbeddedHookPath(t, "user-prompt-submit.sh"), `session_id: $s`, `directory: $d`, `project: $p`)
	assertHookContract(t, "claude powershell hook", claudeEmbeddedHookPath(t, "user-prompt-submit.ps1"), `$bodyMap['session_id'] = $sessionId`, `$bodyMap['directory'] = $directory`, `$bodyMap['project'] = $project`)
}

func assertHookContract(t *testing.T, name, path string, want ...string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	text := string(content)
	for _, substring := range want {
		if !strings.Contains(text, substring) {
			t.Fatalf("%s prompt payload contract missing %q", name, substring)
		}
	}
	if strings.Contains(strings.ToLower(text), "basename") {
		t.Fatalf("%s must not derive project from a path basename; send directory and let Hive validate it", name)
	}
}

func TestClaudeEmbeddedShellHooks_ParseWithBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable; skipping shell hook syntax test")
	}

	for _, script := range []string{"session-start.sh", "session-stop.sh", "user-prompt-submit.sh"} {
		t.Run(script, func(t *testing.T) {
			path := claudeEmbeddedHookPath(t, script)
			cmd := exec.Command(bash, "-n", "./"+script)
			cmd.Dir = filepath.Dir(path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bash -n %s failed: %v\n%s", script, err, output)
			}
		})
	}
}

func TestClaudeEmbeddedHooks_UseSessionScopedTempMarkers(t *testing.T) {
	for _, script := range []string{
		"session-start.sh",
		"session-stop.sh",
		"user-prompt-submit.sh",
		"session-start.ps1",
		"session-stop.ps1",
		"user-prompt-submit.ps1",
	} {
		t.Run(script, func(t *testing.T) {
			content, err := os.ReadFile(claudeEmbeddedHookPath(t, script))
			if err != nil {
				t.Fatalf("read hook: %v", err)
			}
			text := string(content)
			if strings.Contains(text, ".first-prompt-done") {
				t.Fatalf("%s still uses script-local .first-prompt-done marker", script)
			}
			if !strings.Contains(text, "first-prompt-") {
				t.Fatalf("%s does not name a session-scoped first prompt marker", script)
			}
			if !strings.Contains(text, "jarvis-hive") || !strings.Contains(text, "claude-hooks") {
				t.Fatalf("%s does not store marker under the temp/cache hook state directory", script)
			}
		})
	}
}

func claudeEmbeddedHookPath(t *testing.T, script string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "embed", "hooks", "claude", script))
}

func opencodeEmbeddedHookPath(t *testing.T, script string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "embed", "hooks", "opencode", script))
}

package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestClaudeEmbeddedUserPromptHook_CapturesPromptWithoutJQ(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell hook behavior is covered on Unix runners")
	}

	for _, tt := range []struct {
		name   string
		prompt string
	}{
		{
			name:   "plain prompt",
			prompt: "capture me",
		},
		{
			name:   "quotes and backslashes",
			prompt: `say "hello" and keep C:\tmp\file.go`,
		},
		{
			name:   "literal slash n remains literal",
			prompt: `code sample: fmt.Println("a\nb")`,
		},
		{
			name:   "actual newline remains newline",
			prompt: "line one\nline two",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := runClaudeUserPromptHookWithoutJQ(t, tt.prompt)

			var payload struct {
				Content   string `json:"content"`
				SessionID string `json:"session_id"`
				Directory string `json:"directory"`
				Project   string `json:"project"`
			}
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatalf("prompt POST body is not valid JSON: %v\n%s", err, body)
			}
			if payload.Content != tt.prompt {
				t.Fatalf("prompt content mismatch\nwant: %q\n got: %q\nbody: %s", tt.prompt, payload.Content, body)
			}
			if payload.SessionID != "session-123" {
				t.Fatalf("session_id mismatch: %q", payload.SessionID)
			}
			if payload.Directory != "/workspace/project" {
				t.Fatalf("directory mismatch: %q", payload.Directory)
			}
			if payload.Project != "jarvis-dev" {
				t.Fatalf("project mismatch: %q", payload.Project)
			}
		})
	}
}

func runClaudeUserPromptHookWithoutJQ(t *testing.T, prompt string) string {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable; skipping shell hook behavior test")
	}

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	postFile := filepath.Join(tmp, "curl-post.txt")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-d\" ]; then\n    shift\n    printf '%s' \"$1\" > \"$CURL_POST_FILE\"\n    exit 0\n  fi\n  shift\ndone\nexit 1\n")
	for _, name := range []string{"cat", "cut", "date", "mkdir", "tr"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s unavailable; skipping shell hook behavior test", name)
		}
		if err := os.Symlink(path, filepath.Join(binDir, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}

	input, err := json.Marshal(map[string]string{
		"prompt":     prompt,
		"session_id": "session-123",
		"directory":  "/workspace/project",
		"project":    "jarvis-dev",
	})
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}

	cmd := exec.Command(bash, claudeEmbeddedHookPath(t, "user-prompt-submit.sh"))
	cmd.Dir = tmp
	cmd.Stdin = strings.NewReader(string(input))
	cmd.Env = []string{
		"PATH=" + binDir,
		"CURL_POST_FILE=" + postFile,
		"XDG_RUNTIME_DIR=" + filepath.Join(tmp, "runtime"),
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("user prompt hook failed without jq: %v\n%s", err, output)
	}

	var posted string
	for i := 0; i < 20; i++ {
		content, err := os.ReadFile(postFile)
		if err == nil {
			posted = string(content)
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if posted == "" {
		t.Fatalf("expected prompt POST without jq; hook output: %s", output)
	}
	return posted
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
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

package agent

import (
	"encoding/json"
	"fmt"
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

func TestEmbeddedSkillRegistryAutomationHooks_UseQuietActiveWorktreeRefresh(t *testing.T) {
	t.Parallel()

	assertHookContract(t, "claude registry shell hook", claudeEmbeddedHookPath(t, "skill-registry-refresh.sh"), "skill-registry refresh", "--quiet", "--cwd", "resolve_directory", "timeout {{JARVIS_REFRESH_TIMEOUT_SECONDS}}s", "{{JARVIS_EXECUTABLE}}", `PATH="/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"`)
	assertHookContract(t, "claude registry powershell hook", claudeEmbeddedHookPath(t, "skill-registry-refresh.ps1"), "skill-registry refresh", "--quiet", "--cwd", "Resolve-Directory", "Start-Process", "WaitForExit({{JARVIS_REFRESH_TIMEOUT_MILLIS}})", "{{JARVIS_EXECUTABLE}}")
	assertHookContract(t, "opencode registry plugin", opencodeEmbeddedHookPath(t, "skill-registry.ts"), "skill-registry", "--quiet", "--cwd", "context.directory", "context.worktree", "process.cwd()", "timeout: {{JARVIS_REFRESH_TIMEOUT_MILLIS}}", "{{JARVIS_EXECUTABLE}}")
}

func TestEmbeddedSkillRegistryAutomationHooks_DoNotUseProjectLocalPathLookup(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		path      string
		forbidden []string
	}{
		{
			name: "claude registry shell hook",
			path: claudeEmbeddedHookPath(t, "skill-registry-refresh.sh"),
			forbidden: []string{
				"command -v jarvis",
				" jarvis skill-registry",
			},
		},
		{
			name: "claude registry powershell hook",
			path: claudeEmbeddedHookPath(t, "skill-registry-refresh.ps1"),
			forbidden: []string{
				"Get-Command -Name 'jarvis'",
				"Start-Process -FilePath 'jarvis'",
			},
		},
		{
			name: "opencode registry plugin",
			path: opencodeEmbeddedHookPath(t, "skill-registry.ts"),
			forbidden: []string{
				`execFile("jarvis"`,
				"process.env[\"PATH\"]",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			content, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read hook: %v", err)
			}
			text := string(content)
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s must not use project-local PATH lookup %q:\n%s", tt.name, forbidden, text)
				}
			}
		})
	}
}

func TestOpenCodeRegistryPlugin_PassesInitializerContextToResolver(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(opencodeEmbeddedHookPath(t, "skill-registry.ts"))
	if err != nil {
		t.Fatalf("read opencode registry plugin: %v", err)
	}
	text := string(content)
	assertHookContract(t, "opencode registry plugin", opencodeEmbeddedHookPath(t, "skill-registry.ts"), "function resolveDirectory(context: any, input: any): string", "context.worktree", "context.directory")
	if !strings.Contains(text, "resolveDirectory(context ?? {}, input ?? {})") {
		t.Fatalf("OpenCode registry plugin must pass initializer context into resolveDirectory:\n%s", text)
	}
}

func TestOpenCodeRegistryPlugin_DirectoryResolverBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable; skipping OpenCode resolver behavior contract")
	}

	content, err := os.ReadFile(opencodeEmbeddedHookPath(t, "skill-registry.ts"))
	if err != nil {
		t.Fatalf("read opencode registry plugin: %v", err)
	}
	resolver, err := extractOpenCodeDirectoryResolverForNode(string(content))
	if err != nil {
		t.Fatal(err)
	}

	workdir := t.TempDir()
	script := resolver + `
const assert = require("node:assert/strict")
process.env.HIVE_PROJECT_DIRECTORY = "/env-hive"
process.env.JARVIS_WORKSPACE_DIRECTORY = "/env-jarvis"
process.env.PWD = "/env-pwd"

assert.equal(resolveDirectory(
  { worktree: " /context-worktree ", directory: "/context-directory" },
  { worktree: "/input-worktree", directory: "/input-directory" },
), "/context-worktree")
assert.equal(resolveDirectory(
  { directory: "/context-directory" },
  { worktree: "/input-worktree", directory: "/input-directory" },
), "/context-directory")
assert.equal(resolveDirectory(
  {},
  { worktree: "/input-worktree", directory: "/input-directory" },
), "/input-worktree")
assert.equal(resolveDirectory({}, { directory: "/input-directory" }), "/input-directory")
assert.equal(resolveDirectory({}, {}), "/env-hive")
delete process.env.HIVE_PROJECT_DIRECTORY
assert.equal(resolveDirectory({}, {}), "/env-jarvis")
delete process.env.JARVIS_WORKSPACE_DIRECTORY
assert.equal(resolveDirectory({}, {}), process.cwd())
`
	scriptPath := filepath.Join(workdir, "resolver-contract.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write resolver contract script: %v", err)
	}
	cmd := exec.Command(node, scriptPath)
	cmd.Dir = workdir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode resolver behavior contract failed: %v\n%s", err, output)
	}
}

func extractOpenCodeDirectoryResolverForNode(content string) (string, error) {
	start := strings.Index(content, "function readString")
	end := strings.Index(content, "function refreshSkillRegistry")
	if start < 0 || end < 0 || end <= start {
		return "", fmt.Errorf("could not locate resolver functions")
	}
	resolver := content[start:end]
	replacer := strings.NewReplacer(
		"function readString(value: unknown): string", "function readString(value)",
		"function resolveDirectory(context: any, input: any): string", "function resolveDirectory(context, input)",
	)
	return replacer.Replace(resolver), nil
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

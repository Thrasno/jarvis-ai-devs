package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEmbeddedPromptCaptureHooks_PostProjectSessionMetadata verifies that
// the OpenCode hook plugin posts the required session metadata fields.
// (Claude shell/PS hooks have been replaced by the native Go hook package.)
func TestEmbeddedPromptCaptureHooks_PostProjectSessionMetadata(t *testing.T) {
	t.Parallel()

	assertHookContract(t, "opencode plugin", opencodeEmbeddedHookPath(t, "hive.ts"), "session_id: sessionId", "resolveHiveDirectory", "resolveHiveProject")
}

func TestEmbeddedSkillRegistryAutomationHooks_UseQuietActiveWorktreeRefresh(t *testing.T) {
	t.Parallel()

	assertHookContract(t, "opencode registry plugin", opencodeEmbeddedHookPath(t, "skill-registry.ts"), "skill-registry", "--quiet", "--cwd", "--allow-non-git-root", "context.directory", "context.worktree", "process.cwd()", "refreshed = refreshSkillRegistry", "timeout: {{JARVIS_REFRESH_TIMEOUT_MILLIS}}", "{{JARVIS_EXECUTABLE}}")
}

func TestClaudeSkillRegistryAutomation_DoesNotShipWrapperScripts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"skill-registry-refresh.sh", "skill-registry-refresh.ps1"} {
		path := claudeEmbeddedHookPath(t, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Claude registry automation should use a direct command and not ship %s, stat err=%v", path, err)
		}
	}
}

func TestEmbeddedSkillRegistryAutomationHooks_DoNotUseProjectLocalPathLookup(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		path      string
		forbidden []string
	}{
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
	assertHookContract(t, "opencode registry plugin", opencodeEmbeddedHookPath(t, "skill-registry.ts"), "function resolveDirectory(context: any, input: any): string", "context.directory", "context.worktree")
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
  { worktree: " / ", directory: "/context-directory" },
  { worktree: "/input-worktree", directory: "/input-directory" },
), "/context-directory")
assert.equal(resolveDirectory(
  { directory: "/context-directory" },
  { worktree: "/input-worktree", directory: "/input-directory" },
), "/context-directory")
assert.equal(resolveDirectory(
  {},
  { worktree: "/input-worktree", directory: "/input-directory" },
), "/input-directory")
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

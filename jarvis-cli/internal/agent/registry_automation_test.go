package agent

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func TestInstallRegistryAutomationIfSupported_NoopsForUnsupportedAgent(t *testing.T) {
	a := &unsupportedRegistryAutomationAgent{home: t.TempDir()}

	installed, err := InstallRegistryAutomationIfSupported(a, fstest.MapFS{})
	if err != nil {
		t.Fatalf("InstallRegistryAutomationIfSupported returned error: %v", err)
	}
	if installed {
		t.Fatal("unsupported agent should not report registry automation installation")
	}
	if entries, err := os.ReadDir(a.ConfigDir()); err == nil && len(entries) > 0 {
		t.Fatalf("unsupported agent config dir should remain untouched, got %d entries", len(entries))
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read unsupported config dir: %v", err)
	}
}

func TestClaudeAgent_InstallRegistryAutomation_MergesUserPromptSubmitHookIdempotently(t *testing.T) {
	tmpHome := t.TempDir()
	a := &ClaudeAgent{home: tmpHome}
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("create claude dir: %v", err)
	}
	existing := `{
		"hooks": {
			"UserPromptSubmit": [
				{"name":"hive-prompt-capture","hooks":[{"type":"command","command":"/old/hive.sh","timeout":2}]},
				{"name":"user-owned","hooks":[{"type":"command","command":"/user/tool"}]}
			]
		},
		"theme": "dark"
	}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	previousGOOS := claudeRuntimeGOOS
	claudeRuntimeGOOS = "linux"
	t.Cleanup(func() { claudeRuntimeGOOS = previousGOOS })

	for i := 0; i < 2; i++ {
		installed, err := InstallRegistryAutomationIfSupported(a, jarvis.HooksFS)
		if err != nil {
			t.Fatalf("InstallRegistryAutomationIfSupported run %d: %v", i+1, err)
		}
		if !installed {
			t.Fatalf("Claude should report registry automation installation on run %d", i+1)
		}
	}

	scriptPath := filepath.Join(tmpHome, ".claude", "hive-hooks", "skill-registry-refresh.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read installed registry hook: %v", err)
	}
	if !strings.Contains(string(content), "skill-registry refresh --quiet --cwd") {
		t.Fatalf("installed hook does not run quiet cwd refresh: %s", content)
	}
	executable := currentJarvisExecutableForTest(t)
	if !strings.Contains(string(content), executable) {
		t.Fatalf("installed hook should pin the current jarvis executable %q, got: %s", executable, content)
	}
	if !strings.Contains(string(content), "timeout 3s") || strings.Contains(string(content), "{{JARVIS_") {
		t.Fatalf("installed hook should render timeout and consume placeholders: %s", content)
	}
	if strings.Contains(string(content), "command -v jarvis") || strings.Contains(string(content), " jarvis skill-registry") {
		t.Fatalf("installed hook must not resolve jarvis from PATH or execute bare jarvis: %s", content)
	}

	settings := readJSONFile(t, settingsPath)
	if settings["theme"] != "dark" {
		t.Fatalf("user-owned top-level settings were not preserved: %#v", settings)
	}
	hooks := settings["hooks"].(map[string]any)["UserPromptSubmit"].([]any)
	if countNamedHook(hooks, "jarvis-skill-registry-refresh") != 1 {
		t.Fatalf("expected exactly one registry refresh hook, got %#v", hooks)
	}
	if countNamedHook(hooks, "hive-prompt-capture") != 1 || countNamedHook(hooks, "user-owned") != 1 {
		t.Fatalf("existing Hive/user hooks were not preserved: %#v", hooks)
	}
	registryHook := namedHook(t, hooks, "jarvis-skill-registry-refresh")
	commands := registryHook["hooks"].([]any)
	command := commands[0].(map[string]any)
	if command["type"] != "command" || command["timeout"] != float64(registryAutomationTimeoutSeconds) {
		t.Fatalf("unexpected registry hook command contract: %#v", command)
	}
	if !strings.Contains(command["command"].(string), scriptPath) {
		t.Fatalf("registry hook command should point at installed script, got %#v", command)
	}
}

func TestOpenCodeAgent_InstallRegistryAutomation_WritesSeparatePluginWithoutClobberingHive(t *testing.T) {
	tmpHome := t.TempDir()
	a := &OpenCodeAgent{home: tmpHome}
	pluginDir := filepath.Join(tmpHome, ".config", "opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("create plugin dir: %v", err)
	}
	hivePath := filepath.Join(pluginDir, "hive.ts")
	if err := os.WriteFile(hivePath, []byte("// existing hive plugin\n"), 0o644); err != nil {
		t.Fatalf("write hive plugin: %v", err)
	}

	for i := 0; i < 2; i++ {
		installed, err := InstallRegistryAutomationIfSupported(a, jarvis.HooksFS)
		if err != nil {
			t.Fatalf("InstallRegistryAutomationIfSupported run %d: %v", i+1, err)
		}
		if !installed {
			t.Fatalf("OpenCode should report registry automation installation on run %d", i+1)
		}
	}

	assertFileContent(t, hivePath, "// existing hive plugin\n")
	pluginContent, err := os.ReadFile(filepath.Join(pluginDir, "skill-registry.ts"))
	if err != nil {
		t.Fatalf("read installed registry plugin: %v", err)
	}
	executable := currentJarvisExecutableForTest(t)
	if !strings.Contains(string(pluginContent), executable) {
		t.Fatalf("installed plugin should pin the current jarvis executable %q, got: %s", executable, pluginContent)
	}
	if strings.Contains(string(pluginContent), `execFile("jarvis"`) || strings.Contains(string(pluginContent), "process.env[\"PATH\"]") {
		t.Fatalf("installed plugin must not locate jarvis through PATH: %s", pluginContent)
	}
	if !strings.Contains(string(pluginContent), "timeout: 3000") || strings.Contains(string(pluginContent), "{{JARVIS_") {
		t.Fatalf("installed plugin should render timeout and consume placeholders: %s", pluginContent)
	}
	if _, err := os.Stat(filepath.Join(tmpHome, ".config", "opencode", "opencode.json")); !os.IsNotExist(err) {
		t.Fatalf("registry plugin installation must not require opencode.json mutation, stat err=%v", err)
	}
}

func TestRenderRegistryAutomationEmbeddedAssetsConsumePlaceholders(t *testing.T) {
	for _, tt := range []struct {
		name        string
		path        string
		wantTimeout string
	}{
		{name: "claude shell", path: "embed/hooks/claude/skill-registry-refresh.sh", wantTimeout: "timeout 3s"},
		{name: "claude powershell", path: "embed/hooks/claude/skill-registry-refresh.ps1", wantTimeout: "WaitForExit(3000)"},
		{name: "opencode plugin", path: "embed/hooks/opencode/skill-registry.ts", wantTimeout: "timeout: 3000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			content, err := jarvis.HooksFS.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read embedded asset: %v", err)
			}
			rendered, err := renderRegistryAutomationAssetWithExecutable(tt.path, content, "/opt/jarvis/bin/jarvis")
			if err != nil {
				t.Fatalf("render embedded asset: %v", err)
			}
			text := string(rendered)
			if strings.Contains(text, "{{JARVIS_") {
				t.Fatalf("rendered asset still has Jarvis placeholders: %s", text)
			}
			if !strings.Contains(text, tt.wantTimeout) {
				t.Fatalf("rendered asset missing expected timeout %q: %s", tt.wantTimeout, text)
			}
		})
	}
}

func TestRenderRegistryAutomationAssetRejectsMalformedPlaceholderContract(t *testing.T) {
	for _, tt := range []struct {
		name    string
		path    string
		content string
		wantErr string
	}{
		{
			name:    "missing executable placeholder",
			path:    "embed/hooks/claude/skill-registry-refresh.sh",
			content: "timeout {{JARVIS_REFRESH_TIMEOUT_SECONDS}}s\n",
			wantErr: jarvisExecutablePlaceholder,
		},
		{
			name:    "missing timeout seconds placeholder",
			path:    "embed/hooks/claude/skill-registry-refresh.sh",
			content: "{{JARVIS_EXECUTABLE}}\n",
			wantErr: jarvisRefreshTimeoutSecondsPlaceholder,
		},
		{
			name:    "missing timeout millis placeholder",
			path:    "embed/hooks/opencode/skill-registry.ts",
			content: "{{JARVIS_EXECUTABLE}}\n",
			wantErr: jarvisRefreshTimeoutMillisPlaceholder,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderRegistryAutomationAssetWithExecutable(tt.path, []byte(tt.content), "/usr/local/bin/jarvis")
			if err == nil {
				t.Fatal("expected malformed asset to return an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want placeholder %q", err, tt.wantErr)
			}
		})
	}
}

func TestRenderRegistryAutomationAssetConsumesPlaceholdersAndUsesCentralTimeout(t *testing.T) {
	out, err := renderRegistryAutomationAssetWithExecutable(
		"embed/hooks/opencode/skill-registry.ts",
		[]byte(`const exe = {{JARVIS_EXECUTABLE}}
const timeout = {{JARVIS_REFRESH_TIMEOUT_MILLIS}}
`),
		"/opt/jarvis/bin/jarvis",
	)
	if err != nil {
		t.Fatalf("renderRegistryAutomationAssetWithExecutable: %v", err)
	}
	text := string(out)
	for _, forbidden := range []string{jarvisExecutablePlaceholder, jarvisRefreshTimeoutMillisPlaceholder} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("rendered asset still contains placeholder %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"/opt/jarvis/bin/jarvis"`) {
		t.Fatalf("rendered asset did not JSON-quote executable: %s", text)
	}
	if !strings.Contains(text, "3000") {
		t.Fatalf("rendered asset did not use %dms timeout: %s", registryAutomationTimeoutMillis, text)
	}
}

func currentJarvisExecutableForTest(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve current executable: %v", err)
	}
	return executable
}

func countNamedHook(hooks []any, name string) int {
	count := 0
	for _, raw := range hooks {
		entry, ok := raw.(map[string]any)
		if ok && entry["name"] == name {
			count++
		}
	}
	return count
}

func namedHook(t *testing.T, hooks []any, name string) map[string]any {
	t.Helper()
	for _, raw := range hooks {
		entry, ok := raw.(map[string]any)
		if ok && entry["name"] == name {
			return entry
		}
	}
	t.Fatalf("hook %q not found in %#v", name, hooks)
	return nil
}

type unsupportedRegistryAutomationAgent struct{ home string }

func (a *unsupportedRegistryAutomationAgent) Name() string      { return "unsupported" }
func (a *unsupportedRegistryAutomationAgent) IsInstalled() bool { return true }
func (a *unsupportedRegistryAutomationAgent) ConfigDir() string {
	return filepath.Join(a.home, ".unsupported")
}
func (a *unsupportedRegistryAutomationAgent) MergeConfig(MCPEntry) error { return nil }
func (a *unsupportedRegistryAutomationAgent) WriteInstructions(string, string, []config.SkillInfo) error {
	return nil
}
func (a *unsupportedRegistryAutomationAgent) InstallSkills(fs.FS, []string) error    { return nil }
func (a *unsupportedRegistryAutomationAgent) InstallOrchestrator([]byte) error       { return nil }
func (a *unsupportedRegistryAutomationAgent) SupportsOutputStyles() bool             { return false }
func (a *unsupportedRegistryAutomationAgent) WriteOutputStyle(*persona.Preset) error { return nil }
func (a *unsupportedRegistryAutomationAgent) ClearOutputStyle(string) error          { return nil }
func (a *unsupportedRegistryAutomationAgent) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return sddruntime.Build("claude")
}
func (a *unsupportedRegistryAutomationAgent) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return sddruntime.ObservedRuntime{}, nil
}
func (a *unsupportedRegistryAutomationAgent) InstallPromptHook(fs.FS) error   { return nil }
func (a *unsupportedRegistryAutomationAgent) InstallSessionHooks(fs.FS) error { return nil }

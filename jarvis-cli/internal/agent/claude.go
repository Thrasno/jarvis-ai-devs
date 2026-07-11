package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// Ensure ClaudeAgent implements Agent at compile time.
var _ Agent = (*ClaudeAgent)(nil)

// Ensure ClaudeAgent implements the optional AgentInstaller capability.
var _ AgentInstaller = (*ClaudeAgent)(nil)

type claudeCommandRunner func(name string, args ...string) (string, error)

var claudeRuntimeGOOS = runtime.GOOS

// osExecutable is a package-level variable wrapping os.Executable so tests
// can inject a fake path without spawning a real subprocess.
var osExecutable = os.Executable

// ClaudeAgent implements Agent for Anthropic's Claude Code CLI.
// Config dir: ~/.claude/
// MCP registration contract (persists in ~/.claude.json):
//   - Hive (local daemon): `claude mcp add --transport stdio --scope user hive -- <daemon-path>`
//   - Context7 (remote HTTP): `claude mcp add --transport http --scope user context7 https://mcp.context7.com/mcp`
//
// Settings file (non-MCP): ~/.claude/settings.json (e.g. outputStyle)
// Instructions file: ~/.claude/CLAUDE.md
// Skills dir: ~/.claude/skills/
type ClaudeAgent struct {
	home        string
	templatesFS fs.FS
	runCommand  claudeCommandRunner
}

func newClaudeAgent(fsys fs.FS) *ClaudeAgent {
	home, _ := os.UserHomeDir()
	return &ClaudeAgent{home: home, templatesFS: fsys, runCommand: runCommandCombinedOutput}
}

func (a *ClaudeAgent) Name() string { return "claude" }

func (a *ClaudeAgent) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return runtimePlanFor(a.Name())
}

func (a *ClaudeAgent) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return a.ObserveRuntimeWithConfig(nil)
}

func (a *ClaudeAgent) ObserveRuntimeWithConfig(cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
	plan, err := a.RuntimePlan()
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	return observeRuntimeWithConfig(a.ConfigDir(), plan, cfg)
}

func (a *ClaudeAgent) IsInstalled() bool {
	_, err := os.Stat(a.ConfigDir())
	return err == nil
}

func (a *ClaudeAgent) ConfigDir() string {
	return filepath.Join(a.home, ".claude")
}

func (a *ClaudeAgent) settingsPath() string {
	return filepath.Join(a.ConfigDir(), "settings.json")
}

func (a *ClaudeAgent) instructionsPath() string {
	return filepath.Join(a.ConfigDir(), "CLAUDE.md")
}

func (a *ClaudeAgent) skillsDir() string {
	return filepath.Join(a.ConfigDir(), "skills")
}

// MergeGeneratedConfig installs Jarvis-owned Claude Code settings that are not
// MCP registrations. It deep-merges permission guardrails while preserving
// outputStyle, the Hive prompt-capture hook, and user-owned settings. The
// optional skill-registry refresh hook is intentionally not emitted.
func (a *ClaudeAgent) MergeGeneratedConfig(_ *config.AppConfig) error {
	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	includeDefaultMode, err := shouldIncludeClaudeDefaultMode(existing)
	if err != nil {
		return fmt.Errorf("inspect settings.json permissions: %w", err)
	}

	permissions := map[string]any{
		"allow": []any{
			"Bash(git status:*)",
			"Bash(git diff:*)",
			"Bash(go test:*)",
		},
		"deny": []any{
			"Read(.env*)",
			"Read(**/.env*)",
			"Read(*.env)",
			"Read(**/*.env)",
			"Read(*.env.*)",
			"Read(**/*.env.*)",
			"Read(secrets)",
			"Read(**/secrets)",
			"Read(secrets/**)",
			"Read(**/secrets/**)",
			"Read(secret)",
			"Read(**/secret)",
			"Read(secret/**)",
			"Read(**/secret/**)",
			"Read(tokens)",
			"Read(**/tokens)",
			"Read(tokens/**)",
			"Read(**/tokens/**)",
			"Read(token)",
			"Read(**/token)",
			"Read(token/**)",
			"Read(**/token/**)",
			"Read(credentials)",
			"Read(**/credentials)",
			"Read(credentials/**)",
			"Read(**/credentials/**)",
			"Read(credential)",
			"Read(**/credential)",
			"Read(credential/**)",
			"Read(**/credential/**)",
			"Read(*secret*)",
			"Read(**/*secret*)",
			"Read(*token*)",
			"Read(**/*token*)",
			"Read(*credential*)",
			"Read(**/*credential*)",
			"Read(.ssh)",
			"Read(**/.ssh)",
			"Read(.ssh/**)",
			"Read(**/.ssh/**)",
			"Read(id_rsa*)",
			"Read(**/id_rsa*)",
			"Read(id_ed25519*)",
			"Read(**/id_ed25519*)",
			"Read(*.pem)",
			"Read(**/*.pem)",
			"Read(*.key)",
			"Read(**/*.key)",
			"Bash(rm -rf /*)",
			"Bash(git clean -fdx:*)",
			"Bash(git reset --hard:*)",
			"Bash(git push --force*:*)",
			"Bash(git push --force-with-lease*:*)",
			"Bash(git push * --force*:*)",
			"Bash(git push * --force-with-lease*:*)",
		},
	}
	if includeDefaultMode {
		permissions["defaultMode"] = "bypassPermissions"
	}
	patch := map[string]any{"permissions": permissions}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal Claude generated settings patch: %w", err)
	}
	merged, err := MergeJSON(existing, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json generated config: %w", err)
	}
	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

func shouldIncludeClaudeDefaultMode(existing []byte) (bool, error) {
	if len(strings.TrimSpace(string(existing))) == 0 {
		return true, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(existing, &decoded); err != nil {
		return false, err
	}
	permissions, ok := decoded["permissions"].(map[string]any)
	if !ok {
		return true, nil
	}
	_, exists := permissions["defaultMode"]
	return !exists, nil
}

// MergeConfig registers MCP servers via the native Claude CLI contract.
//
// For idempotent reruns/update behavior, it first checks existence via:
//
//	claude mcp get <name>
//
// If present, it removes the existing entry using:
//
//	claude mcp remove --scope user <name>
//
// If absent, it skips remove and continues directly to add.
// settings.json remains reserved for non-MCP settings (e.g. outputStyle).
func (a *ClaudeAgent) MergeConfig(entry MCPEntry) error {
	addArgs := []string{"mcp", "add"}

	switch entry.Name {
	case "hive":
		if strings.TrimSpace(entry.DaemonPath) == "" {
			return fmt.Errorf("hive daemon path is required")
		}
		addArgs = append(addArgs, "--transport", "stdio", "--scope", "user", entry.Name, "--", entry.DaemonPath)
	case "context7":
		addArgs = append(addArgs, "--transport", "http", "--scope", "user", entry.Name, "https://mcp.context7.com/mcp")
	default:
		return fmt.Errorf("unknown MCP entry name: %s", entry.Name)
	}

	getOut, err := a.commandRunner()("claude", "mcp", "get", entry.Name)
	if err != nil {
		if !isMissingClaudeMCP(getOut, err) {
			return fmt.Errorf("get claude mcp %s: %w", entry.Name, err)
		}
	} else {
		removeOut, removeErr := a.commandRunner()("claude", "mcp", "remove", "--scope", "user", entry.Name)
		if removeErr != nil {
			reason := strings.TrimSpace(removeOut)
			if reason != "" {
				return fmt.Errorf("remove existing claude mcp %s: %w: %s", entry.Name, removeErr, reason)
			}
			return fmt.Errorf("remove existing claude mcp %s: %w", entry.Name, removeErr)
		}
	}

	addOut, err := a.commandRunner()("claude", addArgs...)
	if err != nil {
		reason := strings.TrimSpace(addOut)
		if reason != "" {
			return fmt.Errorf("add claude mcp %s: %w: %s", entry.Name, err, reason)
		}
		return fmt.Errorf("add claude mcp %s: %w", entry.Name, err)
	}

	return nil
}

func (a *ClaudeAgent) commandRunner() claudeCommandRunner {
	if a.runCommand != nil {
		return a.runCommand
	}
	return runCommandCombinedOutput
}

func runCommandCombinedOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func isMissingClaudeMCP(output string, err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	lower := strings.ToLower(output + "\n" + err.Error())
	markers := []string{"not found", "does not exist", "no mcp server", "unknown mcp", "no server named", "no server configured"}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// WriteInstructions writes ~/.claude/CLAUDE.md with Layer1+Layer2 sentinel blocks.
//
// Decision logic:
//   - File absent or empty → render fresh via RenderCLAUDEMd ("created")
//   - File exists with Jarvis sentinels → patch in-place via PatchFile ("updated")
//   - File exists without sentinels → render fresh via RenderCLAUDEMd, replacing foreign content ("replaced")
//
// After determining the final content, the Hive protocol is injected via InjectProtocol.
// Any legacy gentle-ai protocol blocks are cleaned up first via CleanupOldProtocol.
func (a *ClaudeAgent) WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error {
	path := a.instructionsPath()

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	var content string
	if os.IsNotExist(err) || len(existing) == 0 {
		// Create new file from scratch using the canonical template renderer.
		content, err = config.RenderCLAUDEMd(a.templatesFS, layer1, layer2, "", skills)
		if err != nil {
			return fmt.Errorf("render CLAUDE.md: %w", err)
		}
	} else {
		existingStr := string(existing)
		if err := ValidateSentinels(existingStr); err == nil {
			// Sentinels present — patch in-place (preserves user content outside blocks).
			content, err = PatchFile(existingStr, layer1, layer2)
			if err != nil {
				return fmt.Errorf("patch CLAUDE.md sentinels: %w", err)
			}
		} else {
			// Sentinels missing — discard foreign content and render a clean Jarvis file.
			content, err = config.RenderCLAUDEMd(a.templatesFS, layer1, layer2, "", skills)
			if err != nil {
				return fmt.Errorf("render CLAUDE.md (replace): %w", err)
			}
		}
	}

	// Clean up legacy gentle-ai protocol blocks and inject Hive protocol
	content = CleanupOldProtocol(content)
	content = InjectProtocol(content, getHiveProtocol())

	// Clean up legacy orchestrator prose link and upsert the @import block
	content = CleanupOldOrchestratorLink(content)
	content = InjectOrchestratorImport(content)

	return writeFileAtomic(path, []byte(content), 0644)
}

// SupportsOutputStyles returns true for ClaudeAgent since Claude Code has
// native output-style support via ~/.claude/output-styles/.
func (a *ClaudeAgent) SupportsOutputStyles() bool {
	return true
}

// WriteOutputStyle writes a schema-v2 presentation output style.
func (a *ClaudeAgent) WriteOutputStyle(preset *persona.Profile) error {
	return a.writeOutputStyle(preset.Name, persona.RenderOutputStyle(preset))
}

// WriteOutputStyleV2 is retained for compatibility until the remaining test
// fixtures are migrated to the canonical profile API.
func (a *ClaudeAgent) WriteOutputStyleV2(preset *persona.PresetV2) error {
	return a.WriteOutputStyle(preset)
}

func (a *ClaudeAgent) writeOutputStyle(presetName, content string) error {
	// 1. Create output-styles directory
	outputStylesDir := filepath.Join(a.ConfigDir(), "output-styles")
	if err := os.MkdirAll(outputStylesDir, 0755); err != nil {
		return fmt.Errorf("create output-styles dir: %w", err)
	}

	// 2. Write output-style file atomically
	titleCaseName := toTitleCase(presetName)
	outputStylePath := filepath.Join(outputStylesDir, titleCaseName+".md")
	if err := writeFileAtomic(outputStylePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write output-style file: %w", err)
	}

	// 3. Patch settings.json with outputStyle key
	patch := map[string]any{
		"outputStyle": titleCaseName,
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal outputStyle patch: %w", err)
	}

	existingBytes, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}

	merged, err := MergeJSON(existingBytes, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// ClearOutputStyle removes the named output-style file and clears the settings
// outputStyle reference if it currently points to that style.
func (a *ClaudeAgent) ClearOutputStyle(name string) error {
	styleName := strings.TrimSpace(name)
	if styleName == "" {
		return nil
	}

	outputStylePath := filepath.Join(a.ConfigDir(), "output-styles", styleName+".md")
	if err := os.Remove(outputStylePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove output-style file: %w", err)
	}

	settingsBytes, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	if len(strings.TrimSpace(string(settingsBytes))) == 0 {
		return nil
	}

	settings := map[string]any{}
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		return fmt.Errorf("decode settings.json: %w", err)
	}

	current, ok := settings["outputStyle"].(string)
	if !ok || current != styleName {
		return nil
	}

	delete(settings, "outputStyle")
	cleanBytes, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}

	if err := writeFileAtomic(a.settingsPath(), cleanBytes, 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}

	return nil
}

// toTitleCase converts a persona name to TitleCase format for output-style file naming.
// Examples: "argentino" -> "Argentino", "tony-stark" -> "TonyStark"
// Implements SPEC-006 transformation rules.
func toTitleCase(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			parts[i] = string(runes)
		}
	}
	return strings.Join(parts, "")
}

// agentsDir returns the path to ~/.claude/agents/.
func (a *ClaudeAgent) agentsDir() string {
	return filepath.Join(a.ConfigDir(), "agents")
}

// InstallAgents installs named agent definition files from agentsFS to
// ~/.claude/agents/. agentsFS must be a sub-FS rooted at the platform-specific
// embed/agents/claude directory. Install is idempotent: existing files are
// overwritten silently.
func (a *ClaudeAgent) InstallAgents(agentsFS fs.FS) error {
	if agentsFS == nil {
		return fmt.Errorf("InstallAgents: agentsFS is nil")
	}
	dir := a.agentsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	return installAgentsFromFS(dir, agentsFS)
}

func (a *ClaudeAgent) InstallSDDPhaseAgents(cfg *config.AppConfig) error {
	files, err := RenderClaudeSDDPhaseAgents(a.templatesFS, cfg)
	if err != nil {
		return err
	}
	dir := a.agentsDir()
	for _, def := range SDDPhaseAgentDefinitions() {
		name := def.Name + ".md"
		content, ok := files[name]
		if !ok {
			return fmt.Errorf("rendered Claude SDD agent %s missing", name)
		}
		if err := writeFileAtomic(filepath.Join(dir, name), content, 0644); err != nil {
			return fmt.Errorf("write Claude SDD agent %s: %w", name, err)
		}
	}
	return nil
}

// InstallSkills installs selected skills from skillsFS to ~/.claude/skills/.
// skillsFS must be a sub-FS rooted at the embed/skills directory.
// The _shared/ directory is always installed regardless of the selected list.
// Idempotent: existing files are overwritten silently.
func (a *ClaudeAgent) InstallSkills(skillsFS fs.FS, selected []string) error {
	dir := a.skillsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	return installSkillsFromFS(dir, skillsFS, selected)
}

func (a *ClaudeAgent) InstallSkillsWithConfig(skillsFS fs.FS, selected []string, cfg *config.AppConfig) error {
	dir := a.skillsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	sectionClass, err := skillModelSectionClassForPlatform(sddruntime.PlatformClaude, cfg)
	if err != nil {
		return fmt.Errorf("resolve skill model sections: %w", err)
	}
	return installSkillsFromFSWithModelSections(dir, skillsFS, selected, sectionClass)
}

// InstallOrchestrator installs rendered sdd-orchestrator.md to ~/.claude/.
// Idempotent: existing file is overwritten silently.
func (a *ClaudeAgent) InstallOrchestrator(orchestratorContent []byte) error {
	destPath := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	return installOrchestrator(destPath, orchestratorContent)
}

// InstallPromptHook writes the Hive UserPromptSubmit hook for Claude Code.
// After migration to native Go hooks, the Claude implementation emits an inline
// command using the jarvis binary path from os.Executable(). The hooksFS
// parameter is kept for interface compatibility but is ignored for Claude.
func (a *ClaudeAgent) InstallPromptHook(_ fs.FS) error {
	executable, err := osExecutable()
	if err != nil {
		return fmt.Errorf("resolve jarvis executable: %w", err)
	}
	command := shellSingleQuote(executable) + " hook prompt-submit"

	patch := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"name": "hive-prompt-capture",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command,
							"timeout": 2,
						},
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal hook patch: %w", err)
	}

	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	// Strip legacy entries (no "name" field) that match the same command so that
	// re-running jarvis init on an old install does not produce duplicate hooks.
	existing = removeHookEntriesByCommand(existing, "UserPromptSubmit", command)

	merged, err := MergeJSON(existing, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// InstallRegistryAutomation merges the Jarvis project skill registry refresh
// command into Claude Code UserPromptSubmit without replacing the Hive
// prompt-capture hook or user-owned hooks.
func (a *ClaudeAgent) InstallRegistryAutomation(_ fs.FS) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve jarvis executable: %w", err)
	}
	command, err := claudeRegistryRefreshCommand(executable)
	if err != nil {
		return err
	}

	patch := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"name": "jarvis-skill-registry-refresh",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command,
							"timeout": registryAutomationTimeoutSeconds,
						},
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal registry hook patch: %w", err)
	}
	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	merged, err := MergeJSON(existing, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json: %w", err)
	}
	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

func claudeRegistryRefreshCommand(executable string) (string, error) {
	if strings.TrimSpace(executable) == "" || !filepath.IsAbs(executable) {
		return "", fmt.Errorf("resolve jarvis executable: expected absolute path, got %q", executable)
	}
	return shellSingleQuote(executable) + ` skill-registry refresh --quiet --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`, nil
}

// InstallSessionHooks installs the Hive SessionStart and Stop hooks for Claude Code.
// After migration to native Go hooks, this emits inline commands using the jarvis
// binary path from os.Executable(). The hooksFS parameter is kept for interface
// compatibility but is ignored for Claude.
func (a *ClaudeAgent) InstallSessionHooks(_ fs.FS) error {
	executable, err := osExecutable()
	if err != nil {
		return fmt.Errorf("resolve jarvis executable: %w", err)
	}
	startCommand := shellSingleQuote(executable) + " hook session-start"
	stopCommand := shellSingleQuote(executable) + " hook session-stop"

	patch := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"name": "hive-session-start",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": startCommand,
							"timeout": 5,
						},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"name": "hive-session-stop",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": stopCommand,
							"timeout": 5,
							"async":   true,
						},
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal session hooks patch: %w", err)
	}

	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	// Strip legacy entries (no "name" field) that match the same commands so that
	// re-running jarvis init on an old install does not produce duplicate hooks.
	existing = removeHookEntriesByCommand(existing, "SessionStart", startCommand)
	existing = removeHookEntriesByCommand(existing, "Stop", stopCommand)

	merged, err := MergeJSON(existing, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// InstallCompactHook adds a second SessionStart entry with matcher "compact"
// pointing to "jarvis hook session-compact". It is idempotent: if an entry
// named "hive-session-compact" already exists it is not added again.
// This method is on *ClaudeAgent only (not on the AgentInstaller interface)
// because OpenCode has no equivalent matcher concept.
func (a *ClaudeAgent) InstallCompactHook() error {
	executable, err := osExecutable()
	if err != nil {
		return fmt.Errorf("resolve jarvis executable: %w", err)
	}
	command := shellSingleQuote(executable) + " hook session-compact"

	// Check for existing entry before patching (idempotency).
	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	if len(strings.TrimSpace(string(existing))) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(existing, &decoded); err == nil {
			if hooks, ok := decoded["hooks"].(map[string]any); ok {
				if sessionStart, ok := hooks["SessionStart"].([]any); ok {
					for _, entry := range sessionStart {
						if em, ok := entry.(map[string]any); ok && em["name"] == "hive-session-compact" {
							return nil // already installed
						}
					}
				}
			}
		}
	}

	patch := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"name":    "hive-session-compact",
					"matcher": "compact",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command,
							"timeout": 5,
						},
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal compact hook patch: %w", err)
	}

	merged, err := MergeJSON(existing, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json compact hook: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// InstallSubagentStopHook adds a SubagentStop entry pointing to
// "jarvis hook subagent-stop". It is idempotent.
// This method is on *ClaudeAgent only (not on the AgentInstaller interface).
func (a *ClaudeAgent) InstallSubagentStopHook() error {
	executable, err := osExecutable()
	if err != nil {
		return fmt.Errorf("resolve jarvis executable: %w", err)
	}
	command := shellSingleQuote(executable) + " hook subagent-stop"

	// Check for existing entry before patching (idempotency).
	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	if len(strings.TrimSpace(string(existing))) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(existing, &decoded); err == nil {
			if hooks, ok := decoded["hooks"].(map[string]any); ok {
				if subagentStop, ok := hooks["SubagentStop"].([]any); ok {
					for _, entry := range subagentStop {
						if em, ok := entry.(map[string]any); ok && em["name"] == "hive-subagent-stop" {
							return nil // already installed
						}
					}
				}
			}
		}
	}

	patch := map[string]any{
		"hooks": map[string]any{
			"SubagentStop": []any{
				map[string]any{
					"name": "hive-subagent-stop",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": command,
							"timeout": 10,
							"async":   true,
						},
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal subagent stop hook patch: %w", err)
	}

	merged, err := MergeJSON(existing, patchBytes)
	if err != nil {
		return fmt.Errorf("merge settings.json subagent stop hook: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// statuslinePath returns the path to ~/.claude/statusline-command.sh.
func (a *ClaudeAgent) statuslinePath() string {
	return filepath.Join(a.ConfigDir(), "statusline-command.sh")
}

// statusLineSettingsPatch returns the JSON patch that registers the statusline command.
func (a *ClaudeAgent) statusLineSettingsPatch() []byte {
	return []byte(`{"statusLine":{"type":"command","command":"bash ~/.claude/statusline-command.sh"}}`)
}

// InstallStatusline writes the embedded statusline script to
// ~/.claude/statusline-command.sh and merges the statusLine key into
// settings.json. When the script already exists, confirm() decides:
// true = overwrite + merge, false = atomic skip (write nothing).
// confirm is never called when the file is absent.
func (a *ClaudeAgent) InstallStatusline(hooksFS fs.FS, confirm func() bool) error {
	scriptPath := a.statuslinePath()

	_, statErr := os.Stat(scriptPath)
	if statErr == nil {
		// File exists — ask the caller.
		if !confirm() {
			return nil
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat statusline script: %w", statErr)
	}

	// Proceed: write script and merge settings.
	content, err := fs.ReadFile(hooksFS, "embed/hooks/claude/statusline-command.sh")
	if err != nil {
		return fmt.Errorf("read embedded statusline script: %w", err)
	}
	// Remove any existing script so writeFileAtomic always creates fresh,
	// guaranteeing the supplied 0755 permission is applied.
	if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing statusline script: %w", err)
	}
	if err := writeFileAtomic(scriptPath, content, 0755); err != nil {
		return fmt.Errorf("write statusline script: %w", err)
	}

	existing, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	}
	merged, err := MergeJSON(existing, a.statusLineSettingsPatch())
	if err != nil {
		return fmt.Errorf("merge statusLine into settings.json: %w", err)
	}
	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// readFileOrEmpty reads a file's contents or returns an empty byte slice if not found.
func readFileOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return data, err
}

// writeFileAtomic writes data to path via a same-directory temp file and replace.
// On Windows, os.Rename cannot reliably overwrite an existing destination, so
// the existing file is removed before the final rename when needed.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	writePerm := perm
	if info, err := os.Stat(path); err == nil {
		writePerm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat destination file: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(writePerm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove destination file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}
	cleanup = false

	return nil
}

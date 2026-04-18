package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// Ensure ClaudeAgent implements Agent at compile time.
var _ Agent = (*ClaudeAgent)(nil)

type claudeCommandRunner func(name string, args ...string) (string, error)

// ClaudeAgent implements Agent for Anthropic's Claude Code CLI.
// Config dir: ~/.claude/
// MCP registration contract: `claude mcp add --scope user ...` (persists in ~/.claude.json)
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

// MergeConfig registers MCP servers via the native Claude CLI contract:
//
//	claude mcp add --scope user <name> <command> [args...]
//
// For idempotent reruns/update behavior, it first attempts:
//
//	claude mcp remove --scope user <name>
//
// and ignores "not found" remove errors.
// settings.json remains reserved for non-MCP settings (e.g. outputStyle).
func (a *ClaudeAgent) MergeConfig(entry MCPEntry) error {
	addArgs := []string{"mcp", "add", "--scope", "user", entry.Name}

	if entry.Name == "hive" {
		if strings.TrimSpace(entry.DaemonPath) == "" {
			return fmt.Errorf("hive daemon path is required")
		}
		addArgs = append(addArgs, entry.DaemonPath)
	} else if entry.Name == "context7" {
		addArgs = append(addArgs, "npx", "-y", "@upstash/context7-mcp")
	} else {
		return fmt.Errorf("unknown MCP entry name: %s", entry.Name)
	}

	removeOut, err := a.commandRunner()("claude", "mcp", "remove", "--scope", "user", entry.Name)
	if err != nil && !isMissingClaudeMCP(removeOut, err) {
		return fmt.Errorf("remove existing claude mcp %s: %w", entry.Name, err)
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
	markers := []string{"not found", "does not exist", "no mcp server", "unknown mcp"}
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

	return writeFileAtomic(path, []byte(content), 0644)
}

// SupportsOutputStyles returns true for ClaudeAgent since Claude Code has
// native output-style support via ~/.claude/output-styles/.
func (a *ClaudeAgent) SupportsOutputStyles() bool {
	return true
}

// WriteOutputStyle writes the output-style file to ~/.claude/output-styles/{Name}.md
// and patches settings.json with {"outputStyle": "{Name}"}.
// Implements SPEC-002, SPEC-003, SPEC-004.
func (a *ClaudeAgent) WriteOutputStyle(preset *persona.Preset) error {
	// 1. Create output-styles directory
	outputStylesDir := filepath.Join(a.ConfigDir(), "output-styles")
	if err := os.MkdirAll(outputStylesDir, 0755); err != nil {
		return fmt.Errorf("create output-styles dir: %w", err)
	}

	// 2. Render output-style content
	content := persona.RenderOutputStyle(preset)

	// 3. Write output-style file atomically
	titleCaseName := toTitleCase(preset.Name)
	outputStylePath := filepath.Join(outputStylesDir, titleCaseName+".md")
	if err := writeFileAtomic(outputStylePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write output-style file: %w", err)
	}

	// 4. Patch settings.json with outputStyle key
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

// InstallOrchestrator installs sdd-orchestrator.md to ~/.claude/.
// orchestratorFS must be a sub-FS rooted at the embed/orchestrator directory.
// Idempotent: existing file is overwritten silently.
func (a *ClaudeAgent) InstallOrchestrator(orchestratorFS fs.FS) error {
	destPath := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	return installOrchestrator(destPath, orchestratorFS)
}

// readFileOrEmpty reads a file's contents or returns an empty byte slice if not found.
func readFileOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return data, err
}

// writeFileAtomic writes data to path atomically via a temp file + rename.
// This prevents partial writes on crash.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

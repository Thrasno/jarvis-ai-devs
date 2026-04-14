package agent

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// Ensure ClaudeAgent implements Agent at compile time.
var _ Agent = (*ClaudeAgent)(nil)

// ClaudeAgent implements Agent for Anthropic's Claude Code CLI.
// Config dir: ~/.claude/
// Settings file: ~/.claude/settings.json
// Instructions file: ~/.claude/CLAUDE.md
// Skills dir: ~/.claude/skills/
type ClaudeAgent struct {
	home        string
	templatesFS fs.FS
}

func newClaudeAgent(fsys fs.FS) *ClaudeAgent {
	home, _ := os.UserHomeDir()
	return &ClaudeAgent{home: home, templatesFS: fsys}
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

// MergeConfig adds the hive MCP entry to ~/.claude/settings.json.
// Claude format: command is a string, args is an array.
// Uses deep merge to preserve all existing config keys.
func (a *ClaudeAgent) MergeConfig(entry MCPEntry) error {
	// Build the hive MCP patch for Claude format
	patch := map[string]any{
		"mcpServers": map[string]any{
			"hive": map[string]any{
				"command": entry.DaemonPath,
				"args":    []string{},
				"type":    "stdio",
			},
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal hive MCP patch: %w", err)
	}

	// Read existing settings (empty object if not found)
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

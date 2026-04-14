package agent

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// Ensure OpenCodeAgent implements Agent at compile time.
var _ Agent = (*OpenCodeAgent)(nil)

// OpenCodeAgent implements Agent for the OpenCode AI coding assistant.
// Config dir: ~/.config/opencode/
// Settings file: ~/.config/opencode/opencode.json
// Instructions file: ~/.config/opencode/AGENTS.md
// Skills dir: ~/.config/opencode/skills/
type OpenCodeAgent struct {
	home        string
	templatesFS fs.FS
}

func newOpenCodeAgent(fsys fs.FS) *OpenCodeAgent {
	home, _ := os.UserHomeDir()
	return &OpenCodeAgent{home: home, templatesFS: fsys}
}

func (a *OpenCodeAgent) Name() string { return "opencode" }

func (a *OpenCodeAgent) IsInstalled() bool {
	_, err := os.Stat(a.ConfigDir())
	return err == nil
}

func (a *OpenCodeAgent) ConfigDir() string {
	return filepath.Join(a.home, ".config", "opencode")
}

func (a *OpenCodeAgent) settingsPath() string {
	return filepath.Join(a.ConfigDir(), "opencode.json")
}

func (a *OpenCodeAgent) instructionsPath() string {
	return filepath.Join(a.ConfigDir(), "AGENTS.md")
}

func (a *OpenCodeAgent) skillsDir() string {
	return filepath.Join(a.ConfigDir(), "skills")
}

// MergeConfig adds the hive MCP entry to ~/.config/opencode/opencode.json.
// OpenCode format: command is an array of strings, not a string.
// Uses deep merge to preserve all existing config keys (agents, permissions, etc).
func (a *OpenCodeAgent) MergeConfig(entry MCPEntry) error {
	// Build the hive MCP patch for OpenCode format
	// command is an array, env vars carry credentials
	hiveCfg := map[string]any{
		"command": []string{entry.DaemonPath},
		"type":    "local",
	}

	// Only add env block if credentials are provided
	if entry.APIURL != "" || entry.Email != "" || entry.Password != "" {
		hiveCfg["env"] = map[string]string{
			"HIVE_API_URL":      entry.APIURL,
			"HIVE_API_EMAIL":    entry.Email,
			"HIVE_API_PASSWORD": entry.Password,
		}
	}

	patch := map[string]any{
		"mcp": map[string]any{
			"hive": hiveCfg,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal hive MCP patch: %w", err)
	}

	existingBytes, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read opencode.json: %w", err)
	}

	merged, err := MergeJSON(existingBytes, patchBytes)
	if err != nil {
		return fmt.Errorf("merge opencode.json: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

// WriteInstructions writes ~/.config/opencode/AGENTS.md with Layer1+Layer2 sentinel blocks.
//
// Decision logic:
//   - File absent or empty → render fresh via RenderAGENTSMd ("created")
//   - File exists with Jarvis sentinels → patch in-place via PatchFile ("updated")
//   - File exists without sentinels → render fresh via RenderAGENTSMd, replacing foreign content ("replaced")
//
// After determining the final content, the Hive protocol is injected via InjectProtocol.
// Any legacy gentle-ai protocol blocks are cleaned up first via CleanupOldProtocol.
func (a *OpenCodeAgent) WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error {
	path := a.instructionsPath()

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read AGENTS.md: %w", err)
	}

	var content string
	if os.IsNotExist(err) || len(existing) == 0 {
		// Create new file from scratch using the canonical template renderer.
		content, err = config.RenderAGENTSMd(a.templatesFS, layer1, layer2, "", skills)
		if err != nil {
			return fmt.Errorf("render AGENTS.md: %w", err)
		}
	} else {
		existingStr := string(existing)
		if err := ValidateSentinels(existingStr); err == nil {
			// Sentinels present — patch in-place (preserves user content outside blocks).
			content, err = PatchFile(existingStr, layer1, layer2)
			if err != nil {
				return fmt.Errorf("patch AGENTS.md sentinels: %w", err)
			}
		} else {
			// Sentinels missing — discard foreign content and render a clean Jarvis file.
			content, err = config.RenderAGENTSMd(a.templatesFS, layer1, layer2, "", skills)
			if err != nil {
				return fmt.Errorf("render AGENTS.md (replace): %w", err)
			}
		}
	}

	// Clean up legacy gentle-ai protocol blocks and inject Hive protocol
	content = CleanupOldProtocol(content)
	content = InjectProtocol(content, getHiveProtocol())

	return writeFileAtomic(path, []byte(content), 0644)
}

// InstallSkills installs selected skills from skillsFS to ~/.config/opencode/skills/.
// skillsFS must be a sub-FS rooted at the embed/skills directory.
// The _shared/ directory is always installed regardless of the selected list.
// Idempotent: existing files are overwritten silently.
func (a *OpenCodeAgent) InstallSkills(skillsFS fs.FS, selected []string) error {
	dir := a.skillsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	return installSkillsFromFS(dir, skillsFS, selected)
}

// InstallOrchestrator installs sdd-orchestrator.md to ~/.config/opencode/.
// orchestratorFS must be a sub-FS rooted at the embed/orchestrator directory.
// Idempotent: existing file is overwritten silently.
func (a *OpenCodeAgent) InstallOrchestrator(orchestratorFS fs.FS) error {
	destPath := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	return installOrchestrator(destPath, orchestratorFS)
}

// SupportsOutputStyles returns false for OpenCodeAgent since OpenCode
// does not have native output-style support.
func (a *OpenCodeAgent) SupportsOutputStyles() bool {
	return false
}

// WriteOutputStyle is a no-op for OpenCodeAgent since OpenCode doesn't support
// output-styles. Returns nil to allow graceful handling in mixed agent environments.
func (a *OpenCodeAgent) WriteOutputStyle(preset *persona.Preset) error {
	return nil
}

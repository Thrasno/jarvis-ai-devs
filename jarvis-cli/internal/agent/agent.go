// Package agent provides the Agent interface and detection logic for
// AI coding assistants (Claude Code, OpenCode) installed on the system.
package agent

import (
	"io/fs"
	"os/exec"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// MCPEntry represents a single MCP server entry to be merged into an agent's config.
type MCPEntry struct {
	Name     string
	APIURL   string
	Email    string
	Password string
	// DaemonPath is the absolute path to the hive-daemon binary or start script.
	DaemonPath string
}

// Agent represents a configured AI coding assistant installed on the system.
type Agent interface {
	// Name returns the agent identifier: "claude" or "opencode".
	Name() string

	// IsInstalled returns true if the agent config directory exists on disk.
	IsInstalled() bool

	// ConfigDir returns the absolute path to the agent's config directory.
	ConfigDir() string

	// MergeConfig adds the hive-daemon MCP entry to the agent's config file.
	// It performs a deep JSON merge, preserving all existing keys.
	MergeConfig(entry MCPEntry) error

	// WriteInstructions writes CLAUDE.md or AGENTS.md with Layer1+Layer2 content.
	// If the file already exists, sentinel blocks are patched in-place.
	// skills lists all installed skills for the Skills section of the persona file.
	WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error

	// InstallSkills installs selected skills from skillsFS to the agent's skills directory.
	// skillsFS must be a sub-FS rooted at the embed/skills directory.
	// selected lists the skill directory names to install (e.g. ["sdd-apply", "hive"]).
	// The _shared/ directory is always installed regardless of the selected list.
	// Install is idempotent: existing files are overwritten silently.
	InstallSkills(skillsFS fs.FS, selected []string) error

	// InstallOrchestrator installs sdd-orchestrator.md to the agent's config directory.
	// orchestratorFS must be a sub-FS rooted at the embed/orchestrator directory.
	// Install is idempotent: existing file is overwritten silently.
	InstallOrchestrator(orchestratorFS fs.FS) error

	// SupportsOutputStyles returns true if the agent supports native output-styles.
	// Claude Code supports this via ~/.claude/output-styles/ and settings.json.
	// OpenCode does not have native output-style support.
	SupportsOutputStyles() bool

	// WriteOutputStyle writes the persona's output-style file and patches agent settings.
	// For agents that don't support output-styles, this is a no-op returning nil.
	// For ClaudeAgent, writes to ~/.claude/output-styles/{TitleCaseName}.md and patches
	// settings.json with {"outputStyle": "{TitleCaseName}"}.
	WriteOutputStyle(preset *persona.Preset) error

	// ClearOutputStyle removes a previously generated output-style artifact and clears
	// the settings reference when it points to the provided style name.
	ClearOutputStyle(name string) error
}

// Detect returns all agents detected as installed on the current system.
// fsys must be an fs.FS containing the template files (e.g. root-package TemplatesFS).
// It checks for ~/.claude (ClaudeAgent) and ~/.config/opencode or opencode binary
// (OpenCodeAgent).
func Detect(fsys fs.FS) []Agent {
	var agents []Agent

	if c := newClaudeAgent(fsys); c.IsInstalled() {
		agents = append(agents, c)
	}

	oc := newOpenCodeAgent(fsys)
	if oc.IsInstalled() {
		agents = append(agents, oc)
	} else if _, err := exec.LookPath("opencode"); err == nil {
		// opencode binary exists even if config dir doesn't — still configure it
		agents = append(agents, oc)
	}

	return agents
}

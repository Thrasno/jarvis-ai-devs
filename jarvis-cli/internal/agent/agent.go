// Package agent provides the Agent interface and detection logic for
// AI coding assistants (Claude Code, OpenCode) installed on the system.
package agent

import "os/exec"

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
	WriteInstructions(layer1, layer2 string) error

	// InstallSkills copies skill files to the agent's skills directory.
	// Each skill is written as {skillsDir}/{skillID}/SKILL.md.
	// Install is idempotent: existing files are overwritten silently.
	InstallSkills(skills map[string][]byte) error
}

// Detect returns all agents detected as installed on the current system.
// It checks for ~/.claude (ClaudeAgent) and ~/.config/opencode or opencode binary
// (OpenCodeAgent).
func Detect() []Agent {
	var agents []Agent

	if c := newClaudeAgent(); c.IsInstalled() {
		agents = append(agents, c)
	}

	oc := newOpenCodeAgent()
	if oc.IsInstalled() {
		agents = append(agents, oc)
	} else if _, err := exec.LookPath("opencode"); err == nil {
		// opencode binary exists even if config dir doesn't — still configure it
		agents = append(agents, oc)
	}

	return agents
}

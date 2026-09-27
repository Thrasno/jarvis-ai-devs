package sddruntime

// ResetSurfaceKind describes how a contract-owned reset surface is analyzed
// and mutated. The wizard's displayed inventory and the reset's mutation
// behavior both derive from the same ResetSurface value, so a kind must fully
// describe the semantics its apply side needs.
type ResetSurfaceKind string

const (
	// ResetSurfaceJSONPaths removes one or more dotted JSON key paths from a
	// JSON config file (e.g. "mcp.hive", "permissions.defaultMode").
	ResetSurfaceJSONPaths ResetSurfaceKind = "json_paths"
	// ResetSurfaceHooksByCommandToken filters nested hook commands out of a
	// JSON hook group by a stable command-substring token, preserving the
	// group's matcher and any unrelated nested command, and dropping the
	// group only once its nested command list becomes empty.
	ResetSurfaceHooksByCommandToken ResetSurfaceKind = "hooks_by_command_token"
	// ResetSurfacePermissionsByLiteral removes exact string entries from a
	// JSON permission allow/deny array.
	ResetSurfacePermissionsByLiteral ResetSurfaceKind = "permissions_by_literal"
	// ResetSurfaceDefaultModeByValue removes a JSON key only when its current
	// scalar value equals one of the surface's historical Jarvis-issued values.
	ResetSurfaceDefaultModeByValue ResetSurfaceKind = "default_mode_by_value"
	// ResetSurfaceMarkerBlock strips the content between a managed sentinel
	// marker pair from a text file, leaving the rest of the file untouched.
	ResetSurfaceMarkerBlock ResetSurfaceKind = "marker_block"
	// ResetSurfaceWholeFile deletes one file entirely.
	ResetSurfaceWholeFile ResetSurfaceKind = "whole_file"
	// ResetSurfaceWholeDirectory empties every file under one directory.
	ResetSurfaceWholeDirectory ResetSurfaceKind = "whole_directory"
)

// ResetSurface names one contract-owned configuration surface that a
// consented installer reset may remove. The wizard display and the reset
// mutation both derive from exactly this inventory, so they can never drift
// apart from one another.
type ResetSurface struct {
	// ID is a stable identifier apply-side code and tests address the surface
	// by, independent of slice order.
	ID string
	// Platform this surface belongs to.
	Platform Platform
	// RelativePath is relative to the platform's config directory.
	RelativePath string
	Kind         ResetSurfaceKind
	// JSONPaths lists dotted JSON key paths for ResetSurfaceJSONPaths.
	JSONPaths []string
	// Matchers lists the historical literal/token values a
	// hooks_by_command_token, permissions_by_literal, or
	// default_mode_by_value surface matches against.
	Matchers []string
	// ReplacedEntirely marks a surface where Jarvis owns the whole target
	// (a directory or a JSON key) rather than a subset of its content, so an
	// installer reset removes user-added content there too.
	ReplacedEntirely bool
	// Display is the exact English line the wizard shows for this surface.
	Display string
}

// RetiredClaudeReviewAgentBaseNames are the four legacy 4R reviewer agent
// base names (without the .md extension) retired from Jarvis-issued Claude
// configuration by issue #767 (2026-09). This is the single source both the
// Claude residue observation (internal/agent/runtime.go) and the reset
// inventory derive from.
func RetiredClaudeReviewAgentBaseNames() []string {
	return []string{
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
	}
}

// RetiredOpenCodeReviewAgentNames are the four legacy 4R reviewer agent names
// retired from Jarvis-issued OpenCode configuration by issue #767 (2026-09).
// This is the single source both the OpenCode residue observation
// (verify_opencode.go) and the reset inventory derive from.
func RetiredOpenCodeReviewAgentNames() []string {
	return append([]string(nil), RetiredClaudeReviewAgentBaseNames()...)
}

// claudeHookCommandTokens is the cumulative, versioned list of Jarvis-managed
// Claude Code hook subcommand tokens ever emitted by an install/reconfigure
// flow. Each entry names the era that introduced it. None has been retired,
// so a reset must still be able to strip every one of them from a machine
// that installed any earlier Jarvis release.
func claudeHookCommandTokens() []string {
	return []string{
		" hook prompt-submit",     // pre-#767: Hive prompt-capture hook
		" skill-registry refresh", // pre-#767: project skill-registry refresh hook
		" hook session-start",     // pre-#767: Hive session-start hook
		" hook session-stop",      // pre-#767: Hive session-stop hook
		" hook session-compact",   // pre-#767: Hive session-compact hook
		" hook subagent-stop",     // pre-#767: Hive subagent-stop hook
	}
}

// claudePermissionLiterals is the cumulative, versioned list of Jarvis-issued
// Claude Code settings.json permission allow/deny literals, including
// variants retired before issue #767 that an older install may still carry.
func claudePermissionLiterals() []string {
	return []string{
		// pre-#767: current allow guardrails
		"Bash(git status:*)",
		"Bash(git diff:*)",
		"Bash(go test:*)",
		// pre-#767: current secrets/credentials deny guardrails
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
		"Bash(git push --force*)",
		"Bash(git push --force-with-lease*)",
		"Bash(git push * --force*)",
		"Bash(git push * --force-with-lease*)",
		// pre-#767, retired: obsolete force-push deny variants
		// (see removeObsoleteClaudeForcePushDenies in internal/agent/claude.go)
		"Bash(git push --force*:*)",
		"Bash(git push --force-with-lease*:*)",
		"Bash(git push * --force*:*)",
		"Bash(git push * --force-with-lease*:*)",
	}
}

// claudeDefaultModeJarvisValues lists every value Jarvis has ever written to
// permissions.defaultMode.
func claudeDefaultModeJarvisValues() []string {
	return []string{"bypassPermissions"} // pre-#767
}

// ResetInventory derives the wizard-displayed reset inventory for one
// platform from the contract. Callers must not construct ResetSurface values
// independently: the wizard display and the reset mutation both read this
// function so they can never disagree about what a reset touches.
func ResetInventory(platform Platform) []ResetSurface {
	switch platform {
	case PlatformClaude:
		return claudeResetInventory()
	case PlatformOpenCode:
		return openCodeResetInventory()
	default:
		return nil
	}
}

func claudeResetInventory() []ResetSurface {
	return []ResetSurface{
		{
			ID:           "claude.settings.output_style",
			Platform:     PlatformClaude,
			RelativePath: "settings.json",
			Kind:         ResetSurfaceJSONPaths,
			JSONPaths:    []string{"outputStyle"},
			Display:      "settings.json: outputStyle (only when it names a Jarvis-emitted persona style)",
		},
		{
			ID:           "claude.settings.status_line",
			Platform:     PlatformClaude,
			RelativePath: "settings.json",
			Kind:         ResetSurfaceJSONPaths,
			JSONPaths:    []string{"statusLine"},
			Display:      "settings.json: statusLine (only when it matches the Jarvis statusline command)",
		},
		{
			ID:           "claude.settings.hooks",
			Platform:     PlatformClaude,
			RelativePath: "settings.json",
			Kind:         ResetSurfaceHooksByCommandToken,
			Matchers:     claudeHookCommandTokens(),
			Display:      "settings.json: hook entries by managed command token (prompt-submit, skill-registry refresh, session-start, session-stop, session-compact, subagent-stop)",
		},
		{
			ID:           "claude.settings.permissions",
			Platform:     PlatformClaude,
			RelativePath: "settings.json",
			Kind:         ResetSurfacePermissionsByLiteral,
			Matchers:     claudePermissionLiterals(),
			Display:      "settings.json: permission allow/deny entries by exact historical literal",
		},
		{
			ID:           "claude.settings.default_mode",
			Platform:     PlatformClaude,
			RelativePath: "settings.json",
			Kind:         ResetSurfaceDefaultModeByValue,
			Matchers:     claudeDefaultModeJarvisValues(),
			Display:      "settings.json: permissions.defaultMode (only when equal to a Jarvis-issued value)",
		},
		{
			ID:           "claude.instructions.marker_block",
			Platform:     PlatformClaude,
			RelativePath: "CLAUDE.md",
			Kind:         ResetSurfaceMarkerBlock,
			Display:      "CLAUDE.md: Jarvis instructions marker block content",
		},
		{
			ID:               "claude.agents.directory",
			Platform:         PlatformClaude,
			RelativePath:     "agents/",
			Kind:             ResetSurfaceWholeDirectory,
			ReplacedEntirely: true,
			Display:          "agents/: entire directory (SDD phase agents, Judgment Day agents, retired review-* agents, and any user-added agent files)",
		},
		{
			ID:           "claude.orchestrator.file",
			Platform:     PlatformClaude,
			RelativePath: "sdd-orchestrator.md",
			Kind:         ResetSurfaceWholeFile,
			Display:      "sdd-orchestrator.md: whole file",
		},
		{
			ID:           "claude.statusline.script",
			Platform:     PlatformClaude,
			RelativePath: "statusline-command.sh",
			Kind:         ResetSurfaceWholeFile,
			Display:      "statusline-command.sh: whole file",
		},
		{
			ID:           "claude.output_styles.directory",
			Platform:     PlatformClaude,
			RelativePath: "output-styles/",
			Kind:         ResetSurfaceWholeDirectory,
			Display:      "output-styles/: whole directory",
		},
		{
			ID:           "claude.skills.directory",
			Platform:     PlatformClaude,
			RelativePath: "skills/",
			Kind:         ResetSurfaceWholeDirectory,
			Display:      "skills/: whole directory",
		},
		{
			ID:           "claude.hive_hooks.directory",
			Platform:     PlatformClaude,
			RelativePath: "hive-hooks/",
			Kind:         ResetSurfaceWholeDirectory,
			Display:      "hive-hooks/: whole directory",
		},
	}
}

func openCodeResetInventory() []ResetSurface {
	return []ResetSurface{
		{
			ID:               "opencode.settings.core_keys",
			Platform:         PlatformOpenCode,
			RelativePath:     "opencode.json",
			Kind:             ResetSurfaceJSONPaths,
			JSONPaths:        []string{"default_agent", "permission", "agent"},
			ReplacedEntirely: true,
			Display:          "opencode.json: default_agent, permission, agent (entirely Jarvis-owned keys, removed whole)",
		},
		{
			ID:           "opencode.settings.mcp",
			Platform:     PlatformOpenCode,
			RelativePath: "opencode.json",
			Kind:         ResetSurfaceJSONPaths,
			JSONPaths:    []string{"mcp.hive", "mcp.context7"},
			Display:      "opencode.json: mcp.hive, mcp.context7 entries (other mcp servers preserved)",
		},
		{
			ID:           "opencode.instructions.marker_block",
			Platform:     PlatformOpenCode,
			RelativePath: "AGENTS.md",
			Kind:         ResetSurfaceMarkerBlock,
			Display:      "AGENTS.md: Jarvis instructions marker block content",
		},
		{
			ID:           "opencode.orchestrator.file",
			Platform:     PlatformOpenCode,
			RelativePath: "sdd-orchestrator.md",
			Kind:         ResetSurfaceWholeFile,
			Display:      "sdd-orchestrator.md: whole file",
		},
		{
			ID:           "opencode.skills.directory",
			Platform:     PlatformOpenCode,
			RelativePath: "skills/",
			Kind:         ResetSurfaceWholeDirectory,
			Display:      "skills/: whole directory",
		},
		{
			ID:           "opencode.plugins.hive_hook",
			Platform:     PlatformOpenCode,
			RelativePath: "plugins/hive.ts",
			Kind:         ResetSurfaceWholeFile,
			Display:      "plugins/hive.ts: whole file",
		},
		{
			ID:           "opencode.plugins.registry_hook",
			Platform:     PlatformOpenCode,
			RelativePath: "plugins/skill-registry.ts",
			Kind:         ResetSurfaceWholeFile,
			Display:      "plugins/skill-registry.ts: whole file",
		},
	}
}

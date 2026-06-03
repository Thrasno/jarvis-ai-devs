package agent

import (
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

// getHiveProtocol returns the embedded Hive protocol content.
// This content is injected into agent instruction files.
func getHiveProtocol() string {
	return jarvis.HiveProtocol
}

// Protocol injection markers for Hive memory protocol.
// These HTML comment markers are used to inject and manage protocol content
// in agent instruction files (CLAUDE.md, AGENTS.md).
const (
	// HiveProtocolStart marks the beginning of the Hive protocol section.
	HiveProtocolStart = "<!-- jarvis:hive-protocol -->"

	// HiveProtocolEnd marks the end of the Hive protocol section.
	HiveProtocolEnd = "<!-- /jarvis:hive-protocol -->"

	// OldEngramStart marks the beginning of the legacy gentle-ai engram protocol section.
	// This marker is used for cleanup during migration to the new Jarvis protocol.
	OldEngramStart = "<!-- gentle-ai:engram-protocol -->"

	// OldEngramEnd marks the end of the legacy gentle-ai engram protocol section.
	// This marker is used for cleanup during migration to the new Jarvis protocol.
	OldEngramEnd = "<!-- /gentle-ai:engram-protocol -->"

	// OrchestratorImportStart marks the beginning of the orchestrator @import block in CLAUDE.md.
	OrchestratorImportStart = "<!-- jarvis:orchestrator-import -->"

	// OrchestratorImportEnd marks the end of the orchestrator @import block in CLAUDE.md.
	OrchestratorImportEnd = "<!-- /jarvis:orchestrator-import -->"

	// OldOrchestratorLinkLine is the legacy prose line referencing sdd-orchestrator.md
	// that was injected by older versions of Jarvis into CLAUDE.md.
	OldOrchestratorLinkLine = "For detailed SDD orchestration logic, see: [sdd-orchestrator.md](./sdd-orchestrator.md)"
)

// CleanupOldProtocol removes all occurrences of the legacy gentle-ai:engram-protocol
// blocks from the content. This handles migration from the old marker format.
// Returns the cleaned content with all old protocol blocks removed (inclusive of markers).
func CleanupOldProtocol(content string) string {
	result := content

	// Keep removing occurrences until none remain (handles multiple duplicates)
	for {
		start := strings.Index(result, OldEngramStart)
		if start == -1 {
			break // No more old protocol blocks found
		}

		end := strings.Index(result, OldEngramEnd)
		if end == -1 || end <= start {
			// Malformed: start marker without matching end, or end before start
			// Remove just the start marker to avoid leaving orphaned content
			result = result[:start] + result[start+len(OldEngramStart):]
			continue
		}

		// Remove the entire block including both markers
		result = result[:start] + result[end+len(OldEngramEnd):]
	}

	return result
}

// CleanupOldOrchestratorLink removes the legacy prose line that referenced sdd-orchestrator.md
// from CLAUDE.md. The line was added by older Jarvis versions and is replaced by the
// <!-- jarvis:orchestrator-import --> @import block in newer installs.
// Surrounding blank lines are normalized so at most one blank line remains where the line was.
func CleanupOldOrchestratorLink(content string) string {
	// Split into lines for accurate line-level removal.
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == OldOrchestratorLinkLine {
			// Remove this line. Also collapse any double blank line that results:
			// if both the line before and after the removed line are blank,
			// skip one of them to avoid a double blank line.
			if len(result) > 0 && result[len(result)-1] == "" && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				// The line before was blank and the next line is also blank — skip the next line too.
				i++
			}
			continue
		}
		result = append(result, lines[i])
	}
	return strings.Join(result, "\n")
}

// InjectOrchestratorImport upserts the <!-- jarvis:orchestrator-import --> block in CLAUDE.md.
// If the markers already exist, the content between them is replaced with @./sdd-orchestrator.md.
// If the markers do not exist, the full block is appended at EOF.
// This function is idempotent: running it twice produces the same result.
func InjectOrchestratorImport(content string) string {
	start := strings.Index(content, OrchestratorImportStart)
	end := strings.Index(content, OrchestratorImportEnd)

	importBlock := OrchestratorImportStart + "\n@./sdd-orchestrator.md\n" + OrchestratorImportEnd

	// Case 1: Both markers exist — replace content between them.
	if start != -1 && end != -1 && end > start {
		before := content[:start]
		after := content[end+len(OrchestratorImportEnd):]
		return before + importBlock + after
	}

	// Case 1b: Orphaned start marker (no matching end) — strip it before appending.
	if start != -1 && (end == -1 || end <= start) {
		content = content[:start] + content[start+len(OrchestratorImportStart):]
	}

	// Case 2: No markers — append at EOF.
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + importBlock + "\n"
}

// InjectProtocol injects or replaces the Hive protocol content in agent instruction files.
// If HiveProtocol markers already exist, it replaces the content between them.
// If markers don't exist, it appends the protocol at the end of the file wrapped in markers.
// This function is idempotent: running it multiple times produces the same result.
func InjectProtocol(content, protocol string) string {
	start := strings.Index(content, HiveProtocolStart)
	end := strings.Index(content, HiveProtocolEnd)

	// Build the protocol block with markers
	protocolBlock := HiveProtocolStart + "\n" + protocol
	if !strings.HasSuffix(protocol, "\n") {
		protocolBlock += "\n"
	}
	protocolBlock += HiveProtocolEnd

	// Case 1: Markers exist — replace content between them
	if start != -1 && end != -1 && end > start {
		before := content[:start]
		after := content[end+len(HiveProtocolEnd):]
		return before + protocolBlock + after
	}

	// Case 2: Markers don't exist — append at EOF
	// Ensure there's a newline before the protocol block if content doesn't end with one
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + protocolBlock + "\n"
}

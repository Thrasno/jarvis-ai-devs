package agent

import (
	_ "embed"
	"strings"
)

// hiveProtocol contains the embedded Hive protocol markdown content.
//
//go:embed hive-protocol.md
var hiveProtocol string

// getHiveProtocol returns the embedded Hive protocol content.
// This content is injected into agent instruction files.
func getHiveProtocol() string {
	return hiveProtocol
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

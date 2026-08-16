package agent

import (
	"fmt"
	"strings"
)

// Sentinel marker constants. These are the exact byte-for-byte markers used
// in CLAUDE.md and AGENTS.md to delimit Jarvis-managed blocks.
const (
	Layer1Start = "<!-- JARVIS:LAYER1:START -->"
	Layer1End   = "<!-- JARVIS:LAYER1:END -->"
	Layer2Start = "<!-- JARVIS:LAYER2:START -->"
	Layer2End   = "<!-- JARVIS:LAYER2:END -->"
)

// ExtractLayer2 extracts the current Layer2 content from a file's string content.
// Returns the content between the Layer2 sentinel markers (exclusive).
// Returns an error if the sentinels are missing or malformed.
func ExtractLayer2(content string) (string, error) {
	start := strings.Index(content, Layer2Start)
	end := strings.Index(content, Layer2End)

	if start == -1 || end == -1 {
		return "", fmt.Errorf("Layer2 sentinel markers not found in content")
	}
	if end <= start {
		return "", fmt.Errorf("Layer2 END marker appears before START marker")
	}

	inner := content[start+len(Layer2Start) : end]
	// Trim leading/trailing newlines for clean extraction
	inner = strings.TrimPrefix(inner, "\n")
	inner = strings.TrimSuffix(inner, "\n")
	return inner, nil
}

// PatchLayer2 replaces the Layer2 block in content with newLayer2, preserving
// the Layer1 block and all content outside the sentinel markers unchanged.
//
// If both sentinel pairs are present, both blocks are updated/preserved.
// If Layer2 markers are missing, they are appended at EOF.
// Returns an error if the markers are malformed (e.g., END before START).
func PatchLayer2(content, newLayer2 string) (string, error) {
	if err := ValidateSentinels(content); err != nil {
		return "", fmt.Errorf("invalid sentinels: %w", err)
	}

	start := strings.Index(content, Layer2Start)
	end := strings.Index(content, Layer2End)

	// Built with the line endings the block already uses, for the same reason
	// PatchFile does it: a hardcoded "\n" rewrites a CRLF file into a mixed one
	// and the file stops matching itself.
	opening, closing := boundarySeparators(content, start+len(Layer2Start), end)
	newBlock := patchedBlock(Layer2Start, Layer2End, newLayer2, opening, closing)

	// Replace the entire Layer2 block (including markers)
	before := content[:start]
	after := content[end+len(Layer2End):]

	return before + newBlock + after, nil
}

// ValidateSentinels verifies that both sentinel pairs are present in content
// and in correct order (Layer1 before Layer2, START before END within each pair).
func ValidateSentinels(content string) error {
	l1Start := strings.Index(content, Layer1Start)
	l1End := strings.Index(content, Layer1End)
	l2Start := strings.Index(content, Layer2Start)
	l2End := strings.Index(content, Layer2End)

	if l1Start == -1 {
		return fmt.Errorf("missing %s", Layer1Start)
	}
	if l1End == -1 {
		return fmt.Errorf("missing %s", Layer1End)
	}
	if l2Start == -1 {
		return fmt.Errorf("missing %s", Layer2Start)
	}
	if l2End == -1 {
		return fmt.Errorf("missing %s", Layer2End)
	}

	if l1End <= l1Start {
		return fmt.Errorf("Layer1 END marker appears before START marker")
	}
	if l2End <= l2Start {
		return fmt.Errorf("Layer2 END marker appears before START marker")
	}
	if l2Start <= l1End {
		return fmt.Errorf("Layer2 block must appear after Layer1 block")
	}

	return nil
}

// The two line endings a managed instruction file can use at a sentinel
// boundary. Which one a given file uses is observed, never assumed: see
// patchedBlock.
const (
	crlfEnding = "\r\n"
	lfEnding   = "\n"
)

// patchedBlock rebuilds one sentinel block around a new payload, reproducing
// what the template renderer would have produced for it.
//
// The renderer emits marker, separator, payload, separator, marker, where the
// separator is whatever line ending the embedded template carries. A patch that
// hardcoded "\n" therefore rewrote a CRLF file into a mixed one, dropping one
// byte per boundary and four per file -- invisible on a Linux checkout, and on a
// Windows checkout enough to make an instruction file differ from itself every
// time it was rewritten. So the separators are read back out of the block being
// replaced rather than imposed, which covers both sources of CRLF: an embedded
// template checked out with CRLF, and a user's own file saved by a Windows
// editor.
//
// The payload goes in verbatim and the closing separator is unconditional,
// which is exactly what the renderer does. Both halves matter: a payload that
// already ends in a newline used to suppress the closing separator here while
// the renderer still emitted it, so the same content rendered and patched
// disagreed by one byte on every platform.
func patchedBlock(startMarker, endMarker, payload, opening, closing string) string {
	return startMarker + opening + payload + closing + endMarker
}

// boundarySeparators reports the line endings an existing block already uses:
// the one following its START marker and the one preceding its END marker. A
// boundary carrying neither is treated as LF, which is what a hand-edited file
// missing its separator would have been rebuilt with before.
func boundarySeparators(content string, afterStart, beforeEnd int) (opening, closing string) {
	opening, closing = lfEnding, lfEnding
	if strings.HasPrefix(content[afterStart:], crlfEnding) {
		opening = crlfEnding
	}
	if beforeEnd >= len(crlfEnding) && content[beforeEnd-len(crlfEnding):beforeEnd] == crlfEnding {
		closing = crlfEnding
	}
	return opening, closing
}

// PatchFile patches both Layer1 and Layer2 blocks in an existing file's content.
// Content outside the sentinel markers is preserved unchanged, and so is the
// line ending each block already uses.
func PatchFile(content, layer1, layer2 string) (string, error) {
	if err := ValidateSentinels(content); err != nil {
		return "", fmt.Errorf("validate sentinels: %w", err)
	}

	// Patch Layer1
	l1Start := strings.Index(content, Layer1Start)
	l1End := strings.Index(content, Layer1End)
	l1Opening, l1Closing := boundarySeparators(content, l1Start+len(Layer1Start), l1End)
	newL1Block := patchedBlock(Layer1Start, Layer1End, layer1, l1Opening, l1Closing)
	content = content[:l1Start] + newL1Block + content[l1End+len(Layer1End):]

	// Re-find Layer2 markers after Layer1 patch
	l2Start := strings.Index(content, Layer2Start)
	l2End := strings.Index(content, Layer2End)
	if l2Start == -1 || l2End == -1 {
		return "", fmt.Errorf("Layer2 sentinels lost after Layer1 patch")
	}

	l2Opening, l2Closing := boundarySeparators(content, l2Start+len(Layer2Start), l2End)
	newL2Block := patchedBlock(Layer2Start, Layer2End, layer2, l2Opening, l2Closing)
	content = content[:l2Start] + newL2Block + content[l2End+len(Layer2End):]

	return content, nil
}

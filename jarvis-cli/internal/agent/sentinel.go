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

	// Build the new Layer2 block
	newBlock := Layer2Start + "\n" + newLayer2 + "\n" + Layer2End

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

// PatchFile patches both Layer1 and Layer2 blocks in an existing file's content.
// Content outside the sentinel markers is preserved unchanged.
func PatchFile(content, layer1, layer2 string) (string, error) {
	if err := ValidateSentinels(content); err != nil {
		return "", fmt.Errorf("validate sentinels: %w", err)
	}

	// Patch Layer1
	l1Start := strings.Index(content, Layer1Start)
	l1End := strings.Index(content, Layer1End)
	newL1Block := Layer1Start + "\n" + layer1
	if !strings.HasSuffix(layer1, "\n") {
		newL1Block += "\n"
	}
	newL1Block += Layer1End
	content = content[:l1Start] + newL1Block + content[l1End+len(Layer1End):]

	// Re-find Layer2 markers after Layer1 patch
	l2Start := strings.Index(content, Layer2Start)
	l2End := strings.Index(content, Layer2End)
	if l2Start == -1 || l2End == -1 {
		return "", fmt.Errorf("Layer2 sentinels lost after Layer1 patch")
	}

	newL2Block := Layer2Start + "\n" + layer2
	if !strings.HasSuffix(layer2, "\n") {
		newL2Block += "\n"
	}
	newL2Block += Layer2End
	content = content[:l2Start] + newL2Block + content[l2End+len(Layer2End):]

	return content, nil
}

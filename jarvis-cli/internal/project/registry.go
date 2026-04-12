package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	suggestedHeader = "## Suggested Skills"
	customHeader    = "## Custom Skills"
	defaultCustom   = "## Custom Skills\n\n<!-- Add your project-specific skills here -->\n"
)

// WriteRegistry creates or updates .jarvis/skill-registry.md in dir.
// The Suggested Skills section is always regenerated from the provided skills list.
// The Custom Skills section is preserved as-is if it already exists.
// The write is atomic: a .tmp file is written first, then renamed into place.
func WriteRegistry(dir, projectName string, stack Stack, skills []string) error {
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")

	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return fmt.Errorf("create .jarvis dir: %w", err)
	}

	// Preserve custom skills from an existing file, if any.
	customSection := defaultCustom
	if existing, err := os.ReadFile(registryPath); err == nil {
		customSection = extractCustomSection(string(existing))
	}

	content := buildRegistryContent(projectName, stack, skills, customSection)

	// Atomic write: write to .tmp, then rename.
	tmp := registryPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("write temp registry: %w", err)
	}
	if err := os.Rename(tmp, registryPath); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("finalize registry: %w", err)
	}
	return nil
}

// extractCustomSection returns the content from the ## Custom Skills header onwards.
// If the header is absent, returns a default empty custom section.
func extractCustomSection(content string) string {
	idx := strings.Index(content, customHeader)
	if idx == -1 {
		return defaultCustom
	}
	return strings.TrimRight(content[idx:], "\n") + "\n"
}

// buildRegistryContent generates the full skill-registry.md content.
func buildRegistryContent(projectName string, stack Stack, skills []string, customSection string) string {
	var sb strings.Builder

	sb.WriteString("# Skill Registry — ")
	sb.WriteString(projectName)
	sb.WriteString("\n\n")
	sb.WriteString("**Stack**: ")
	sb.WriteString(string(stack))
	sb.WriteString("\n\n---\n\n")
	sb.WriteString(suggestedHeader)
	sb.WriteString("\n\n")
	for _, skill := range skills {
		sb.WriteString("- **")
		sb.WriteString(skill)
		sb.WriteString("**\n")
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(customSection)

	return sb.String()
}

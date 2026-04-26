package persona

import (
	"fmt"
	"strings"
)

var canonicalNotesRequiredSections = []string{
	"core principle",
	"behavior",
	"when asking questions",
}

var legacyNotesRequiredSections = []string{
	"communication style",
	"technical behavior",
}

// ValidateNotesTemplate validates the editorial contract for preset notes.
//
// Transitional behavior:
//   - Empty notes are accepted for compatibility with legacy/custom drafts.
//   - Canonical template is preferred and requires Core Principle, Behavior,
//     and When Asking Questions sections.
//   - Legacy built-in templates are still accepted when they include
//     Communication Style and Technical Behavior sections.
//   - Extra sections are always allowed.
func ValidateNotesTemplate(notes string) error {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return nil
	}

	title, sections := extractNotesHeadings(notes)
	if title == "" {
		return fmt.Errorf("notes must include a level-1 title heading (e.g. '# Custom Persona')")
	}

	if hasAllRequiredSections(sections, canonicalNotesRequiredSections) {
		return nil
	}

	if hasAllRequiredSections(sections, legacyNotesRequiredSections) {
		return nil
	}

	return fmt.Errorf("notes must include canonical sections [%s] (legacy accepted: [%s])",
		strings.Join(canonicalNotesRequiredSections, ", "),
		strings.Join(legacyNotesRequiredSections, ", "),
	)
}

func extractNotesHeadings(notes string) (string, map[string]struct{}) {
	sections := make(map[string]struct{})
	var title string

	for _, line := range strings.Split(notes, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			title = normalizeHeading(trimmed[2:])
		case strings.HasPrefix(trimmed, "## "):
			sections[normalizeHeading(trimmed[3:])] = struct{}{}
		}
	}

	return title, sections
}

func normalizeHeading(heading string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(heading)))
	return strings.Join(fields, " ")
}

func hasAllRequiredSections(actual map[string]struct{}, required []string) bool {
	for _, section := range required {
		if _, ok := actual[normalizeHeading(section)]; !ok {
			return false
		}
	}
	return true
}

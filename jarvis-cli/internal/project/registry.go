package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	suggestedHeader  = "## Suggested Skills"
	customHeader     = "## Custom Skills"
	defaultCustom    = "## Custom Skills\n\n<!-- Add your project-specific skills here -->\n"
	canonicalPath    = ".jarvis/skill-registry.md"
	legacyPathATL    = ".atl/skill-registry.md"
	projectSkillsDir = ".jarvis/skills"
)

type RegistrySource string

const (
	RegistrySourceCanonical RegistrySource = "canonical"
	RegistrySourceLegacy    RegistrySource = "legacy"
	RegistrySourceDefault   RegistrySource = "default"
)

type RegistryPaths struct {
	WritePath string
	ReadPaths []string
}

type RegistrySkill struct {
	ID           string
	Name         string
	Description  string
	Trigger      string
	Scope        string
	Path         string
	CompactRules string
	IsCore       bool
}

func CanonicalRegistryPaths() RegistryPaths {
	return RegistryPaths{
		WritePath: canonicalPath,
		ReadPaths: []string{canonicalPath, legacyPathATL},
	}
}

func ResolveRegistryReadPath(dir string) (string, RegistrySource, error) {
	paths := CanonicalRegistryPaths()
	for i, rel := range paths.ReadPaths {
		abs := filepath.Join(dir, rel)
		if _, err := os.Stat(abs); err == nil {
			if i == 0 {
				return abs, RegistrySourceCanonical, nil
			}
			return abs, RegistrySourceLegacy, nil
		}
	}

	return filepath.Join(dir, paths.WritePath), RegistrySourceDefault, nil
}

// WriteRegistry creates or updates .jarvis/skill-registry.md in dir.
// The Suggested Skills section is always regenerated from the provided skills list.
// The Custom Skills section is preserved as-is if it already exists.
// The write is atomic: a .tmp file is written first, then renamed into place.
func WriteRegistry(dir, projectName string, stack Stack, skills []string, richSkills ...[]RegistrySkill) error {
	paths := CanonicalRegistryPaths()
	registryPath := filepath.Join(dir, paths.WritePath)

	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return fmt.Errorf("create .jarvis dir: %w", err)
	}

	// Preserve custom skills from an existing canonical file, or import legacy
	// custom skills only when the canonical registry does not exist yet.
	customSection := defaultCustom
	if existing, err := os.ReadFile(registryPath); err == nil {
		customSection = extractCustomSection(string(existing))
	} else if os.IsNotExist(err) {
		legacyRegistryPath := filepath.Join(dir, legacyPathATL)
		if existing, err := os.ReadFile(legacyRegistryPath); err == nil {
			customSection = extractCustomSection(string(existing))
		}
	}

	var registrySkills []RegistrySkill
	if len(richSkills) > 0 {
		registrySkills = richSkills[0]
	}
	content := buildRegistryContent(projectName, stack, skills, registrySkills, customSection)

	// Atomic write: write to .tmp, then rename.
	tmp := registryPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("write temp registry: %w", err)
	}
	if err := os.Rename(tmp, registryPath); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
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
func buildRegistryContent(projectName string, stack Stack, skills []string, richSkills []RegistrySkill, customSection string) string {
	var sb strings.Builder

	sb.WriteString("# Skill Registry — ")
	sb.WriteString(projectName)
	sb.WriteString("\n\n")
	sb.WriteString("**Stack**: ")
	sb.WriteString(string(stack))
	sb.WriteString("\n")
	sb.WriteString("Canonical registry path: `.jarvis/skill-registry.md`")
	sb.WriteString("\n\n---\n\n")
	sb.WriteString(suggestedHeader)
	sb.WriteString("\n\n")
	for _, skill := range skills {
		sb.WriteString("- `")
		sb.WriteString(skill)
		sb.WriteString("`\n")
	}

	rows := sortedRegistrySkills(richSkills)
	if len(rows) > 0 {
		sb.WriteString("\n---\n\n")
		sb.WriteString("## Installed Skills\n\n")
		sb.WriteString("| Skill | Trigger / Description | Scope | Path |\n")
		sb.WriteString("|-------|-----------------------|-------|------|\n")
		for _, skill := range rows {
			sb.WriteString("| ")
			sb.WriteString(registryDisplayName(skill))
			sb.WriteString(" | ")
			sb.WriteString(registryTriggerDescription(skill))
			sb.WriteString(" | ")
			sb.WriteString(registryScope(skill))
			sb.WriteString(" | `")
			sb.WriteString(registryLoadablePath(skill))
			sb.WriteString("`")
			sb.WriteString(" |\n")
		}

		sb.WriteString("\n---\n\n")
		sb.WriteString("## Compact Rules (Transitional Metadata)\n\n")
		sb.WriteString("Compact rules are compatibility metadata; the skill index path rows above are the primary instruction contract.\n\n")
		for _, skill := range rows {
			if skill.CompactRules == "" {
				continue
			}
			sb.WriteString("- **")
			sb.WriteString(skill.ID)
			sb.WriteString("**: ")
			sb.WriteString(skill.CompactRules)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## Project Conventions\n\n")
	sb.WriteString("- Generated sections are deterministic; customize only from `## Custom Skills` onward.\n")
	sb.WriteString("- Keep `.jarvis/skill-registry.md` committed so the team resolves the same skills.\n")
	sb.WriteString("- Built-in skill paths point at project-local `.jarvis/skills/<skill>/SKILL.md` copies generated by `jarvis init`.\n")
	sb.WriteString("- Re-run `jarvis init` after changing stack or installed skill metadata.\n")

	sb.WriteString("\n---\n\n")
	sb.WriteString(customSection)

	return sb.String()
}

func registryTriggerDescription(skill RegistrySkill) string {
	trigger := strings.TrimSpace(skill.Trigger)
	description := strings.TrimSpace(skill.Description)
	switch {
	case trigger != "" && description != "":
		return escapeTableCell(trigger + " — " + description)
	case trigger != "":
		return escapeTableCell(trigger)
	case description != "":
		return escapeTableCell(description)
	default:
		return ""
	}
}

func registryScope(skill RegistrySkill) string {
	if skill.Scope != "" {
		return escapeTableCell(skill.Scope)
	}
	if skill.IsCore {
		return "core"
	}
	return "optional"
}

func registryLoadablePath(skill RegistrySkill) string {
	path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(skill.Path)))
	if path == "." {
		return ""
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, projectSkillsDir+"/") {
		return path
	}
	if skill.ID != "" && (path == skill.ID+"/SKILL.md" || strings.HasPrefix(path, skill.ID+"/")) {
		return projectSkillsDir + "/" + path
	}
	return path
}

func sortedRegistrySkills(skills []RegistrySkill) []RegistrySkill {
	rows := append([]RegistrySkill(nil), skills...)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ID < rows[j].ID
	})
	return rows
}

func registryDisplayName(skill RegistrySkill) string {
	if skill.Name != "" {
		return escapeTableCell(skill.Name)
	}
	return escapeTableCell(skill.ID)
}

func escapeTableCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

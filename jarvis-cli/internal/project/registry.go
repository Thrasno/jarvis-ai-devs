package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	CustomSkillsHeader     = "## Custom Skills"
	RegistryWarningsHeader = "## Registry Warnings"
	CanonicalRegistryPath  = ".jarvis/skill-registry.md"
	LegacyRegistryPathATL  = ".atl/skill-registry.md"
	ProjectSkillsDir       = ".jarvis/skills"

	defaultCustom = CustomSkillsHeader + "\n\n<!-- Add your project-specific skills here -->\n"
)

const (
	RegistryReasonCreated   = "created"
	RegistryReasonUpdated   = "updated"
	RegistryReasonUnchanged = "unchanged"
	RegistryReasonForced    = "forced"

	RegistryWarningLegacyImported = "legacy-registry-imported"
	RegistrySeverityWarning       = "warning"
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

type RegistryWarning struct {
	Code     string
	Severity string
	Path     string
	Message  string
}

type WriteRegistryOptions struct {
	Force    bool
	Warnings []RegistryWarning
}

type WriteRegistryResult struct {
	Path       string
	Changed    bool
	SkillCount int
	Reason     string
	Warnings   []RegistryWarning
}

func CanonicalRegistryPaths() RegistryPaths {
	return RegistryPaths{
		WritePath: CanonicalRegistryPath,
		ReadPaths: []string{CanonicalRegistryPath, LegacyRegistryPathATL},
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
// The installed skills table is regenerated from registrySkills.
// The Custom Skills section is preserved as-is if it already exists.
// The write is atomic: a .tmp file is written first, then renamed into place.
func WriteRegistry(dir, projectName string, registrySkills []RegistrySkill, opts WriteRegistryOptions) error {
	_, err := WriteRegistryWithResult(dir, projectName, registrySkills, opts)
	return err
}

// WriteRegistryWithResult creates or updates .jarvis/skill-registry.md in dir.
// The installed skills table is regenerated from registrySkills.
// The Custom Skills section is preserved as-is if it already exists.
// Legacy .atl custom content is imported only when the canonical registry is absent.
// Byte-equivalent content is not rewritten unless Force is set.
func WriteRegistryWithResult(dir, projectName string, registrySkills []RegistrySkill, opts WriteRegistryOptions) (WriteRegistryResult, error) {
	paths := CanonicalRegistryPaths()
	registryPath := filepath.Join(dir, paths.WritePath)
	renderedWarnings := append([]RegistryWarning(nil), opts.Warnings...)
	result := WriteRegistryResult{
		Path:       registryPath,
		SkillCount: len(registrySkills),
		Warnings:   append([]RegistryWarning(nil), renderedWarnings...),
	}

	if err := safeMkdirAllWithinRoot(dir, filepath.Dir(registryPath)); err != nil {
		return result, fmt.Errorf("create .jarvis dir: %w", err)
	}
	if err := validateRegistrySymlinkTarget(dir, registryPath); err != nil {
		return result, fmt.Errorf("validate registry path: %w", err)
	}

	// Preserve custom skills from an existing canonical file, or import legacy
	// custom skills only when the canonical registry does not exist yet.
	customSection := defaultCustom
	canonicalExists := false
	if existing, err := os.ReadFile(registryPath); err == nil {
		canonicalExists = true
		customSection = extractCustomSection(string(existing))
	} else if os.IsNotExist(err) {
		legacyRegistryPath := filepath.Join(dir, LegacyRegistryPathATL)
		if existing, err := os.ReadFile(legacyRegistryPath); err == nil {
			customSection = extractCustomSection(string(existing))
			result.Warnings = append([]RegistryWarning{{
				Code:     RegistryWarningLegacyImported,
				Severity: RegistrySeverityWarning,
				Path:     LegacyRegistryPathATL,
				Message:  "Imported legacy custom content into canonical registry.",
			}}, result.Warnings...)
		}
	} else {
		return result, fmt.Errorf("read existing registry: %w", err)
	}

	content := buildRegistryContent(projectName, registrySkills, customSection, renderedWarnings)

	if existing, err := os.ReadFile(registryPath); err == nil && string(existing) == content && !opts.Force {
		result.Changed = false
		result.Reason = RegistryReasonUnchanged
		return result, nil
	} else if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("read existing registry for comparison: %w", err)
	}

	if err := writeRegistryFileAtomically(registryPath, []byte(content), 0644); err != nil {
		return result, fmt.Errorf("finalize registry: %w", err)
	}

	result.Changed = true
	switch {
	case opts.Force:
		result.Reason = RegistryReasonForced
	case canonicalExists:
		result.Reason = RegistryReasonUpdated
	default:
		result.Reason = RegistryReasonCreated
	}
	return result, nil
}

func safeMkdirAllWithinRoot(root, dir string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absRoot = filepath.Clean(absRoot)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	absDir = filepath.Clean(absDir)
	if !pathWithinRoot(absRoot, absDir) {
		return fmt.Errorf("%s is outside project root %s", absDir, absRoot)
	}

	info, err := os.Lstat(absRoot)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to follow symlink directory %s", absRoot)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", absRoot)
	}

	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	current := absRoot
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to follow symlink directory %s", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(current, 0755); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return err
		}
	}
	return nil
}

func validateRegistrySymlinkTarget(root, registryPath string) error {
	info, err := os.Lstat(registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absRoot = filepath.Clean(absRoot)
	absRegistryPath, err := filepath.Abs(registryPath)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(absRegistryPath)
	if err != nil {
		return fmt.Errorf("resolve registry symlink %s: %w", registryPath, err)
	}
	if !pathWithinRoot(absRoot, filepath.Clean(resolved)) {
		return fmt.Errorf("registry symlink target %s is outside project root %s", resolved, absRoot)
	}
	return nil
}

func writeRegistryFileAtomically(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// extractCustomSection returns the content from the ## Custom Skills header onwards.
// If the header is absent, returns a default empty custom section.
func extractCustomSection(content string) string {
	idx := customSectionIndex(content)
	if idx < 0 {
		return defaultCustom
	}
	return strings.TrimRight(content[idx:], "\n") + "\n"
}

func customSectionIndex(content string) int {
	lineStart := 0
	for lineStart <= len(content) {
		lineEnd := strings.IndexByte(content[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(content)
		} else {
			lineEnd += lineStart
		}
		line := strings.TrimSpace(strings.TrimSuffix(content[lineStart:lineEnd], "\r"))
		if line == CustomSkillsHeader {
			return lineStart
		}
		if lineEnd == len(content) {
			break
		}
		lineStart = lineEnd + 1
	}
	return -1
}

// buildRegistryContent generates the full skill-registry.md content.
// The table uses the index-first, path-first schema:
// | Trigger | Skill | Scope | Path |
func buildRegistryContent(projectName string, richSkills []RegistrySkill, customSection string, warnings []RegistryWarning) string {
	var sb strings.Builder

	sb.WriteString("# Skill Registry — ")
	sb.WriteString(projectName)
	sb.WriteString("\n\n")
	sb.WriteString("Canonical registry path: `")
	sb.WriteString(CanonicalRegistryPath)
	sb.WriteString("`")
	sb.WriteString("\n\n---\n\n")

	rows := sortedRegistrySkills(richSkills)
	if len(rows) > 0 {
		sb.WriteString("## Installed Skills\n\n")
		sb.WriteString("| Trigger | Skill | Scope | Path |\n")
		sb.WriteString("|---------|-------|-------|------|\n")
		for _, skill := range rows {
			sb.WriteString("| ")
			sb.WriteString(registryTrigger(skill))
			sb.WriteString(" | ")
			sb.WriteString(registryDisplayName(skill))
			sb.WriteString(" | ")
			sb.WriteString(registryScope(skill))
			sb.WriteString(" | `")
			sb.WriteString(registryLoadablePath(skill))
			sb.WriteString("`")
			sb.WriteString(" |\n")
		}
		sb.WriteString("\n---\n\n")
	}

	if len(warnings) > 0 {
		sb.WriteString(RegistryWarningsHeader)
		sb.WriteString("\n\n")
		sb.WriteString("| Code | Severity | Path | Message |\n")
		sb.WriteString("|------|----------|------|---------|\n")
		for _, warning := range warnings {
			sb.WriteString("| ")
			sb.WriteString(escapeTableCell(warning.Code))
			sb.WriteString(" | ")
			sb.WriteString(escapeTableCell(warning.Severity))
			sb.WriteString(" | `")
			sb.WriteString(escapeTableCell(warning.Path))
			sb.WriteString("` | ")
			sb.WriteString(escapeTableCell(warning.Message))
			sb.WriteString(" |\n")
		}
		sb.WriteString("\n---\n\n")
	}

	sb.WriteString("## Project Conventions\n\n")
	sb.WriteString("- Generated sections are deterministic; customize only from `## Custom Skills` onward.\n")
	sb.WriteString("- Keep `")
	sb.WriteString(CanonicalRegistryPath)
	sb.WriteString("` committed so the team resolves the same skills.\n")
	sb.WriteString("- Built-in skill paths point at project-local `.jarvis/skills/<skill>/SKILL.md` copies generated by `jarvis init`.\n")
	sb.WriteString("- Re-run `jarvis init` after changing stack or installed skill metadata.\n")

	sb.WriteString("\n---\n\n")
	sb.WriteString(customSection)

	return sb.String()
}

// registryTrigger returns the trigger string for the Trigger column.
func registryTrigger(skill RegistrySkill) string {
	return escapeTableCell(strings.TrimSpace(skill.Trigger))
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
	if filepath.IsAbs(path) || strings.HasPrefix(path, ProjectSkillsDir+"/") {
		return path
	}
	if skill.ID != "" && (path == skill.ID+"/SKILL.md" || strings.HasPrefix(path, skill.ID+"/")) {
		return ProjectSkillsDir + "/" + path
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

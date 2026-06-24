package diskscan

import (
	"os"
	"path/filepath"
	"strings"
)

// userHomeDir is the function used to resolve the user's home directory.
// It is a package-level variable so tests can override it to simulate failure.
var userHomeDir = os.UserHomeDir

// ScanRow represents a single discovered skill from a SKILL.md file.
// Types are defined here (not in internal/project) to avoid a potential
// import cycle: internal/skills/catalog_contract_test.go already imports
// internal/project, and if diskscan returned project.RegistrySkill directly,
// adding production imports in the future could create a cycle.
// Callers in internal/projectregistry map ScanRow → project.RegistrySkill
// as a thin adapter.
type ScanRow struct {
	ID      string
	Name    string
	Trigger string
	Scope   string
	Path    string // absolute path to SKILL.md
}

// ScanWarning describes a problem found while scanning a skill directory.
type ScanWarning struct {
	Code string
	Path string
}

// skipIDs is the set of skill directory names that are never registered.
// sdd-* skills are skipped by prefix check (see shouldSkipSkill).
var skipIDs = map[string]bool{
	"_shared":        true,
	"skill-registry": true,
}

// ResolveScanDirs returns the ordered list of absolute directories to scan for
// project-local and global skills. Order:
//  1. <root>/.jarvis/skills  (project-local, highest priority)
//  2. ~/.claude/skills        (global Claude, omitted when home is unresolvable)
//  3. ~/.config/opencode/skills (global OpenCode, omitted when home is unresolvable)
func ResolveScanDirs(root string) []string {
	dirs := []string{
		filepath.Join(root, ".jarvis", "skills"),
	}
	home, err := userHomeDir()
	if err != nil || home == "" {
		return dirs
	}
	dirs = append(dirs,
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
	)
	return dirs
}

// Scan walks dirs in order and returns deduplicated ScanRows (project-over-global)
// and any per-skill warnings. Dirs that do not exist are silently skipped.
// Only directories containing a SKILL.md file are registered. The skip list
// and sdd-* prefix are applied before registering each skill.
func Scan(dirs []string) ([]ScanRow, []ScanWarning, error) {
	seen := make(map[string]bool)
	var rows []ScanRow
	var warns []ScanWarning

	for _, dir := range dirs {
		dirRows, dirWarns, err := scanDir(dir, seen)
		if err != nil {
			return rows, warns, err
		}
		rows = append(rows, dirRows...)
		warns = append(warns, dirWarns...)
	}

	return rows, warns, nil
}

// scanDir walks a single directory and collects ScanRows for all SKILL.md files
// found in immediate subdirectories. seen is updated for deduplication.
func scanDir(dir string, seen map[string]bool) ([]ScanRow, []ScanWarning, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var rows []ScanRow
	var warns []ScanWarning

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillID := entry.Name()
		if shouldSkipSkill(skillID) {
			continue
		}

		skillMDPath := filepath.Join(dir, skillID, "SKILL.md")
		content, err := os.ReadFile(skillMDPath)
		if err != nil {
			// No SKILL.md in this directory — not a registered skill
			continue
		}

		// Skip if already seen (project-over-global deduplication)
		if seen[skillID] {
			continue
		}

		result, warn := ParseFrontmatter(content, skillMDPath)
		if warn != nil {
			warns = append(warns, ScanWarning{Code: warn.Code, Path: warn.Path})
			if isInvalidSkillMetadata(warn.Code) {
				continue
			}
		}
		seen[skillID] = true

		rows = append(rows, ScanRow{
			ID:      skillID,
			Name:    result.Name,
			Trigger: result.Trigger,
			Scope:   result.Scope,
			Path:    skillMDPath,
		})
	}

	return rows, warns, nil
}

func isInvalidSkillMetadata(code string) bool {
	return code == "missing-name" || code == "missing-trigger"
}

// shouldSkipSkill returns true when the skill directory name matches the skip list.
// sdd-* (any prefix) and explicit IDs in skipIDs are skipped.
func shouldSkipSkill(id string) bool {
	if strings.HasPrefix(id, "sdd-") {
		return true
	}
	return skipIDs[id]
}

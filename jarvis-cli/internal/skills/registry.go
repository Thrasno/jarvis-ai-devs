// Package skills manages embedded skill Markdown files that are installed
// into AI agent skill directories.
// The embed.FS is provided by the caller (assets.SkillsFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills/diskscan"
)

// Skill represents a single embedded skill file.
type Skill struct {
	// ID is the directory name for this skill (e.g. "sdd-apply").
	ID string

	// Name is the human-readable display name sourced from frontmatter.
	Name string

	// Description is sourced from the frontmatter description field.
	Description string

	// Trigger is the context phrase that triggers this skill to load.
	Trigger string

	// IsCore marks skills that are pre-selected and cannot be deselected.
	// Derived from frontmatter scope: core.
	IsCore bool

	// Scope is the raw frontmatter scope value ("core" or "optional").
	Scope string

	// Content holds the raw Markdown content of the skill's SKILL.md file.
	Content []byte

	// Path is the relative path within embed/skills/ (e.g. "sdd-verify/SKILL.md").
	Path string
}


// ListSkills returns all available embedded skills with their metadata and content.
// fsys must be the root-package SkillsFS (embed/skills directory embedded at root).
// Only files named SKILL.md are registered. Supporting files (e.g. strict-tdd.md)
// are installed but not registered. The _shared/ directory is never registered.
// Name, Description, Trigger, Scope, and IsCore are sourced from frontmatter.
func ListSkills(fsys embed.FS) ([]Skill, error) {
	var skills []Skill

	err := fs.WalkDir(fsys, "embed/skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		// Only register SKILL.md files.
		if d.Name() != "SKILL.md" {
			return nil
		}

		// Compute relative path from embed/skills root.
		relPath := strings.TrimPrefix(path, "embed/skills/")

		// Derive skill ID from the parent directory name.
		dirName := filepath.Dir(relPath)

		// Skip _shared/ — those files are not skills.
		if dirName == "_shared" {
			return nil
		}

		skillID := dirName

		content, err := fsys.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read skill %s: %w", skillID, err)
		}

		fm, _ := diskscan.ParseFrontmatter(content, relPath)

		// Name resolution: display_name: wins if present; fall back to name:;
		// fall back to the kebab skill ID as last resort.
		name := fm.DisplayName
		if name == "" {
			name = fm.Name
		}
		if name == "" {
			name = skillID
		}

		skills = append(skills, Skill{
			ID:          skillID,
			Name:        name,
			Description: fm.Description,
			Trigger:     fm.Trigger,
			IsCore:      fm.Scope == "core",
			Scope:       fm.Scope,
			Content:     content,
			Path:        relPath,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	return skills, nil
}


// GetSkill returns a single skill by ID, or an error if not found.
// fsys must be the root-package SkillsFS (embed/skills directory embedded at root).
func GetSkill(fsys embed.FS, id string) (*Skill, error) {
	skills, err := ListSkills(fsys)
	if err != nil {
		return nil, err
	}

	for _, s := range skills {
		if s.ID == id {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("skill %q not found", id)
}

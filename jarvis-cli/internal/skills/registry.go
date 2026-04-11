// Package skills manages embedded skill Markdown files that are installed
// into AI agent skill directories.
// The embed.FS is provided by the caller (assets.SkillsFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// Skill represents a single embedded skill file.
type Skill struct {
	// ID is the filename without extension (e.g. "sdd-workflow").
	ID string

	// Name is the human-readable display name.
	Name string

	// Description is a brief explanation of what this skill teaches the AI.
	Description string

	// IsCore marks skills that are pre-selected and cannot be deselected.
	// Core skills: sdd-workflow, hive.
	IsCore bool

	// Content holds the raw Markdown content of the skill file.
	Content []byte
}

// coreSkillIDs lists the skills that are always installed.
var coreSkillIDs = map[string]bool{
	"sdd-workflow": true,
	"hive":         true,
}

// skillMeta provides human-readable metadata for each skill ID.
var skillMeta = map[string]struct{ name, description string }{
	"sdd-workflow":         {name: "SDD Workflow", description: "Spec-Driven Development lifecycle: proposal → spec → design → tasks → apply → verify → archive"},
	"hive":                 {name: "Hive Memory", description: "Persistent memory protocol: when to save, how to search, session summary triggers"},
	"zoho-deluge":          {name: "Zoho Deluge", description: "Zoho Deluge scripting conventions: no nested loops, bulk operations, null safety"},
	"laravel-architecture": {name: "Laravel Architecture", description: "Laravel conventions: thin controllers, services, repositories, FormRequest validation"},
	"phpunit-testing":      {name: "PHPUnit Testing", description: "PHPUnit patterns: AAA structure, factories, one concept per test"},
	"git-workflow":         {name: "Git Workflow", description: "Conventional commits, branch naming, no force push to main"},
}

// ListSkills returns all available embedded skills with their metadata and content.
// fsys must be the root-package SkillsFS (embed/skills directory embedded at root).
func ListSkills(fsys embed.FS) ([]Skill, error) {
	var skills []Skill

	err := fs.WalkDir(fsys, "embed/skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		id := strings.TrimSuffix(d.Name(), ".md")

		content, err := fsys.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read skill %s: %w", id, err)
		}

		meta, hasMeta := skillMeta[id]
		name := id
		description := ""
		if hasMeta {
			name = meta.name
			description = meta.description
		}

		skills = append(skills, Skill{
			ID:          id,
			Name:        name,
			Description: description,
			IsCore:      coreSkillIDs[id],
			Content:     content,
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

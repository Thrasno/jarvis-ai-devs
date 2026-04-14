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
)

// Skill represents a single embedded skill file.
type Skill struct {
	// ID is the directory name for this skill (e.g. "sdd-workflow").
	ID string

	// Name is the human-readable display name.
	Name string

	// Description is a brief explanation of what this skill teaches the AI.
	Description string

	// Trigger is the context phrase that triggers this skill to load.
	Trigger string

	// IsCore marks skills that are pre-selected and cannot be deselected.
	IsCore bool

	// Content holds the raw Markdown content of the skill's SKILL.md file.
	Content []byte

	// Path is the relative path within embed/skills/ (e.g. "sdd-qa/SKILL.md").
	Path string
}

// coreSkillIDs lists the skills that are always installed.
var coreSkillIDs = map[string]bool{
	"sdd-workflow": true,
	"hive":         true,
	"sdd-init":     true,
	"sdd-apply":    true,
	"sdd-verify":   true,
	"sdd-archive":  true,
	"sdd-qa":       true,
}

// skillMeta provides human-readable metadata for each skill ID.
var skillMeta = map[string]struct{ name, description, trigger string }{
	"sdd-workflow":         {name: "SDD Workflow", description: "Spec-Driven Development lifecycle: proposal → spec → design → tasks → apply → sdd-qa → verify → archive", trigger: "SDD workflow phase"},
	"hive":                 {name: "Hive Memory", description: "Persistent memory protocol: when to save, how to search, session summary triggers", trigger: "Using engram memory"},
	"sdd-explore":          {name: "SDD Explore", description: "Investigate ideas and compare approaches before committing to a change", trigger: "When exploring ideas"},
	"sdd-propose":          {name: "SDD Propose", description: "Create a structured change proposal with intent, scope, and success criteria", trigger: "When creating proposals"},
	"sdd-spec":             {name: "SDD Spec", description: "Write delta requirements and Given/When/Then scenarios for a change", trigger: "When writing specs"},
	"sdd-design":           {name: "SDD Design", description: "Document architecture decisions and technical approach with rationale", trigger: "When designing architecture"},
	"sdd-tasks":            {name: "SDD Tasks", description: "Break down a change into a concrete, ordered implementation checklist", trigger: "When creating task lists"},
	"sdd-apply":            {name: "SDD Apply", description: "Implement tasks following specs and design; supports Strict TDD mode", trigger: "When implementing tasks"},
	"sdd-qa":               {name: "SDD QA", description: "Run mixed [AUTO]/[MANUAL] QA checklist; mandatory before sdd-verify", trigger: "When running QA checks"},
	"sdd-verify":           {name: "SDD Verify", description: "Verify implementation against specs with structural and behavioral checks", trigger: "When verifying implementation"},
	"sdd-archive":          {name: "SDD Archive", description: "Merge delta specs to main specs and close the SDD change cycle", trigger: "When archiving changes"},
	"sdd-init":             {name: "SDD Init", description: "Detect project stack, testing capabilities, and initialize SDD context", trigger: "When initializing SDD"},
	"zoho-deluge":          {name: "Zoho Deluge", description: "Zoho Deluge scripting conventions: no nested loops, bulk operations, null safety", trigger: "When writing Zoho Deluge scripts"},
	"laravel-architecture": {name: "Laravel Architecture", description: "Laravel conventions: thin controllers, services, repositories, FormRequest validation", trigger: "When writing Laravel code"},
	"phpunit-testing":      {name: "PHPUnit Testing", description: "PHPUnit patterns: AAA structure, factories, one concept per test", trigger: "When writing PHP tests"},
	"git-workflow":         {name: "Git Workflow", description: "Conventional commits, branch naming, no force push to main", trigger: "When using git"},
	"branch-pr":            {name: "Branch & PR", description: "PR creation workflow with issue-first enforcement, branch naming, and automated checks", trigger: "When creating pull requests"},
	"issue-creation":       {name: "Issue Creation", description: "GitHub issue creation with bug report and feature request templates", trigger: "When creating GitHub issues"},
	"go-testing":           {name: "Go Testing", description: "Go testing patterns including Bubbletea TUI testing", trigger: "When writing Go tests, using teatest, or adding test coverage"},
	"judgment-day":         {name: "Judgment Day", description: "Parallel adversarial review protocol with dual blind judges", trigger: "When user says judgment day, review adversarial, dual review"},
	"sdd-onboard":          {name: "SDD Onboard", description: "Guided end-to-end walkthrough of SDD workflow", trigger: "When onboarding user through full SDD cycle"},
	"skill-creator":        {name: "Skill Creator", description: "Creates new AI agent skills following the Agent Skills spec", trigger: "When creating a new skill or documenting patterns for AI"},
	"skill-registry":       {name: "Skill Registry", description: "Create or update the skill registry for the current project", trigger: "When user says update skills, skill registry, or after installing skills"},
}

// ListSkills returns all available embedded skills with their metadata and content.
// fsys must be the root-package SkillsFS (embed/skills directory embedded at root).
// Only files named SKILL.md are registered. Supporting files (e.g. strict-tdd.md)
// are installed but not registered. The _shared/ directory is never registered.
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

		meta, hasMeta := skillMeta[skillID]
		name := skillID
		description := ""
		trigger := ""
		if hasMeta {
			name = meta.name
			description = meta.description
			trigger = meta.trigger
		}

		skills = append(skills, Skill{
			ID:          skillID,
			Name:        name,
			Description: description,
			Trigger:     trigger,
			IsCore:      coreSkillIDs[skillID],
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

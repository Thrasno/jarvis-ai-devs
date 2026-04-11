package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// InstallSelected installs the specified skills (by ID) into agentSkillsDir.
// fsys must be the root-package SkillsFS (embed/skills directory embedded at root).
// Each skill is installed as: {agentSkillsDir}/{skillID}/SKILL.md
// Install is idempotent: existing files are overwritten silently.
// Core skills (sdd-workflow, hive) are always included regardless of selected.
func InstallSelected(fsys embed.FS, agentSkillsDir string, selected []string) error {
	allSkills, err := ListSkills(fsys)
	if err != nil {
		return fmt.Errorf("list available skills: %w", err)
	}

	// Build set of selected IDs (include core skills unconditionally)
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}

	if err := os.MkdirAll(agentSkillsDir, 0755); err != nil {
		return fmt.Errorf("create skills dir %s: %w", agentSkillsDir, err)
	}

	for _, skill := range allSkills {
		if !skill.IsCore && !selectedSet[skill.ID] {
			continue
		}

		skillDir := filepath.Join(agentSkillsDir, skill.ID)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return fmt.Errorf("create skill dir %s: %w", skill.ID, err)
		}

		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, skill.Content, 0644); err != nil {
			return fmt.Errorf("write skill %s: %w", skill.ID, err)
		}
	}

	return nil
}

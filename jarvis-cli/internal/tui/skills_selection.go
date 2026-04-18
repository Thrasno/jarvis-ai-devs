package tui

import "github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"

type skillPrompt struct {
	Label       string
	Description string
	SkillIDs    []string
}

type skillSelectionPlan struct {
	Prompts  []skillPrompt
	Selected map[string]bool
}

type skillPromptTemplate struct {
	Label       string
	Description string
	SkillIDs    []string
}

var interactiveSkillPrompts = []skillPromptTemplate{
	{Label: "Zoho-Deluge", Description: "Zoho Deluge scripting conventions", SkillIDs: []string{"zoho-deluge"}},
	{Label: "PHP", Description: "PHP stack helpers (Laravel + PHPUnit)", SkillIDs: []string{"phpunit-testing", "laravel-architecture"}},
	{Label: "Go Testing", Description: "Go testing and Bubbletea testing patterns", SkillIDs: []string{"go-testing"}},
}

var interactiveSkillIDs = map[string]bool{
	"zoho-deluge":          true,
	"phpunit-testing":      true,
	"laravel-architecture": true,
	"go-testing":           true,
}

func buildSkillSelectionPlan(skillList []skills.Skill, existingSelected []string) skillSelectionPlan {
	existingSet := make(map[string]bool, len(existingSelected))
	for _, id := range existingSelected {
		existingSet[id] = true
	}

	selected := make(map[string]bool, len(skillList))
	skillByID := make(map[string]skills.Skill, len(skillList))
	for _, s := range skillList {
		skillByID[s.ID] = s
		if s.IsCore {
			selected[s.ID] = true
			continue
		}
		if !interactiveSkillIDs[s.ID] {
			// All non stack-specific skills are auto-installed.
			selected[s.ID] = true
		}
	}

	var prompts []skillPrompt
	for _, prompt := range interactiveSkillPrompts {
		presentIDs := make([]string, 0, len(prompt.SkillIDs))
		for _, id := range prompt.SkillIDs {
			if _, ok := skillByID[id]; ok {
				presentIDs = append(presentIDs, id)
			}
		}
		if len(presentIDs) == 0 {
			continue
		}

		defaultOn := false
		for _, id := range presentIDs {
			if existingSet[id] {
				defaultOn = true
				break
			}
		}

		for _, id := range presentIDs {
			selected[id] = defaultOn
		}

		prompts = append(prompts, skillPrompt{
			Label:       prompt.Label,
			Description: prompt.Description,
			SkillIDs:    presentIDs,
		})
	}

	return skillSelectionPlan{Prompts: prompts, Selected: selected}
}

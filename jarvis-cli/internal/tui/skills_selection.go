package tui

import (
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

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
	{Label: "PHP", Description: "PHP stack helpers (Laravel + PHPUnit)", SkillIDs: []string{"phpunit-testing", "laravel-architecture"}},
	{Label: "Go Testing", Description: "Go testing and Bubbletea testing patterns", SkillIDs: []string{"go-testing"}},
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
		if !skills.IsInteractive(s.ID) {
			selected[s.ID] = true
		}
	}

	pack := skills.NewZohoPack(skillList)
	prompts := make([]skillPrompt, 0, len(interactiveSkillPrompts)+1)
	if memberIDs := pack.MemberIDs(); len(memberIDs) > 0 {
		defaultOn := pack.Selected(existingSelected)
		for _, id := range memberIDs {
			selected[id] = defaultOn
		}
		prompts = append(prompts, skillPrompt{
			Label:       "Zoho Skills Pack",
			Description: "Application and language skills for supported Zoho products",
			SkillIDs:    memberIDs,
		})
	}

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
		prompts = append(prompts, skillPrompt{Label: prompt.Label, Description: prompt.Description, SkillIDs: presentIDs})
	}

	return skillSelectionPlan{Prompts: prompts, Selected: selected}
}

// selectedSkillIDs reduces either setup path's choices to concrete desired-state IDs.
func selectedSkillIDs(catalog []skills.Skill, recorded []string, selected map[string]bool) []string {
	catalogByID := make(map[string]skills.Skill, len(catalog))
	for _, skill := range catalog {
		catalogByID[skill.ID] = skill
	}

	result := make([]string, 0, len(recorded)+len(catalog))
	included := make(map[string]bool)
	appendID := func(id string) {
		if !included[id] {
			included[id] = true
			result = append(result, id)
		}
	}
	for _, id := range recorded {
		skill, current := catalogByID[id]
		if !current && !strings.HasPrefix(id, "zoho-") {
			appendID(id)
			continue
		}
		if current && !strings.HasPrefix(id, "zoho-") && (skill.IsCore || selected[id]) {
			appendID(id)
		}
	}
	for _, skill := range catalog {
		if !strings.HasPrefix(skill.ID, "zoho-") && (skill.IsCore || selected[skill.ID]) {
			appendID(skill.ID)
		}
	}

	pack := skills.NewZohoPack(catalog)
	for _, id := range recorded {
		if strings.HasPrefix(id, "zoho-") {
			result = append(result, id)
		}
	}
	return pack.ApplySelection(result, selected[skills.ZohoLegacyAnchorID])
}

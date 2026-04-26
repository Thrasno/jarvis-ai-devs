package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
)

var personaCmd = &cobra.Command{
	Use:   "persona",
	Short: "Manage AI persona preset",
}

var personaSetCmd = &cobra.Command{
	Use:   "set <preset>",
	Short: "Change active persona preset",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		presetName := args[0]

		resolved, err := persona.ResolvePreset(jarvis.PersonaFS, presetName)
		if err != nil {
			return fmt.Errorf("resolve preset %q: %w", presetName, err)
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		skillList, err := skills.ListSkills(jarvis.SkillsFS)
		if err != nil {
			return fmt.Errorf("list skills: %w", err)
		}
		selectedSkills := make(map[string]bool, len(cfg.SelectedSkills))
		for _, id := range cfg.SelectedSkills {
			selectedSkills[id] = true
		}
		var skillInfos []config.SkillInfo
		for _, s := range skillList {
			if !s.IsCore && !selectedSkills[s.ID] {
				continue
			}
			skillInfos = append(skillInfos, config.SkillInfo{
				Name:        s.Name,
				Description: s.Description,
				Trigger:     s.Trigger,
			})
		}

		agents := agent.Detect(jarvis.TemplatesFS)
		pipelineAgents := make([]persona.PresetAgent, 0, len(agents))
		for _, a := range agents {
			pipelineAgents = append(pipelineAgents, a)
		}

		if err := persona.ApplyPresetPipeline(pipelineAgents, resolved, persona.ApplyOptions{
			Layer1:               config.Layer1Content(),
			Skills:               skillInfos,
			PreviousPresetSlug:   cfg.PersonaPreset,
			PreviousPresetSource: normalizePersonaPresetSource(cfg.PersonaPresetSource),
			PersistConfig:        true,
		}); err != nil {
			return fmt.Errorf("apply persona preset %q: %w", resolved.Slug, err)
		}

		displayName := resolved.Preset.DisplayName
		if displayName == "" {
			displayName = resolved.Preset.Name
		}
		fmt.Printf("Persona set to %q (%s).\n", resolved.Slug, displayName)
		return nil
	},
}

func normalizePersonaPresetSource(value string) persona.PresetSource {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(persona.PresetSourceUser):
		return persona.PresetSourceUser
	default:
		return persona.PresetSourceBuiltin
	}
}

func init() {
	personaCmd.AddCommand(personaSetCmd)
}

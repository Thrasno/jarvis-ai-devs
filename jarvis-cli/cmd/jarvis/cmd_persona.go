package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
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

		resolved, err := resolvePersonaSetPreset(jarvis.PersonaFS, presetName)
		if err != nil {
			return fmt.Errorf("resolve preset %q: %w", presetName, err)
		}

		// ~/.jarvis/state.yaml owns the persona and the selected skills. A machine
		// that has never written one is read as an empty manifest, exactly the
		// unpopulated replay state it represents.
		manifest, err := state.Load()
		if errors.Is(err, state.ErrNotFound) {
			manifest = state.New()
		} else if err != nil {
			return fmt.Errorf("load the desired-state manifest: %w", err)
		}

		skillList, err := skills.ListSkills(jarvis.SkillsFS)
		if err != nil {
			return fmt.Errorf("list skills: %w", err)
		}
		selectedSkills := make(map[string]bool, len(manifest.Skills))
		for _, id := range manifest.Skills {
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
		if err := applyPersonaProfile(agents, resolved, persona.ApplyOptions{
			Layer1:               config.Layer1Content(),
			Skills:               skillInfos,
			PreviousPresetSlug:   manifest.Persona,
			PreviousPresetSource: normalizePersonaPresetSource(string(manifest.PersonaSource)),
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

// resolvePersonaSetPreset resolves a validated schema-v2 presentation profile
// before it reaches the canonical apply pipeline.
func resolvePersonaSetPreset(personaFS fs.FS, presetName string) (*persona.ResolvedProfile, error) {
	resolved, err := persona.ResolveProfile(personaFS, presetName)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// applyPersonaProfile applies a validated schema-v2 profile through the
// canonical profile pipeline.
func applyPersonaProfile(agents []agent.Agent, resolved *persona.ResolvedProfile, opts persona.ApplyOptions) error {
	pipelineAgents := make([]persona.ProfileAgent, 0, len(agents))
	for _, a := range agents {
		pipelineAgent, ok := persona.AdaptProfileAgent(a)
		if !ok {
			return fmt.Errorf("agent %q does not support schema v2 presentation profiles", a.Name())
		}
		pipelineAgents = append(pipelineAgents, pipelineAgent)
	}
	return persona.ApplyProfile(pipelineAgents, resolved, opts)
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

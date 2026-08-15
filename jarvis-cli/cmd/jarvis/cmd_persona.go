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

		manifest, err := loadManifestForPersona()
		if err != nil {
			return err
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

// loadManifestForPersona reads the desired state this command renders from,
// migrating first for the same reason `jarvis sync` does.
//
// A machine upgrading into this version still has its persona and skills in
// config.yaml and no manifest at all. Reading the manifest without migrating
// would see an empty one and render the Skills section of every instruction
// file from an empty selection, silently dropping every skill the user chose.
// The migration is one-way and returns early once a manifest exists, so this
// costs one stat call on every later run.
func loadManifestForPersona() (*state.State, error) {
	migration, err := state.Migrate()
	if err != nil {
		return nil, fmt.Errorf("migrate configuration into the desired-state manifest: %w", err)
	}
	if migration.Notice != "" {
		fmt.Println(migration.Notice)
	}

	manifest, err := state.Load()
	if errors.Is(err, state.ErrNotFound) {
		return state.New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("load the desired-state manifest: %w", err)
	}
	return manifest, nil
}

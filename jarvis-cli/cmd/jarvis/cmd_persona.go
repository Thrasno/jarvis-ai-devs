package main

import (
	"fmt"

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

		// 1. Load the named preset from the embedded FS.
		preset, err := persona.LoadPreset(jarvis.PersonaFS, presetName)
		if err != nil {
			return fmt.Errorf("unknown preset %q: %w", presetName, err)
		}

		// 2. Render Layer2 from the preset.
		layer2 := persona.RenderLayer2(preset)

		// 3. Build SkillInfo list from all installed skills.
		skillList, err := skills.ListSkills(jarvis.SkillsFS)
		if err != nil {
			return fmt.Errorf("list skills: %w", err)
		}
		var skillInfos []config.SkillInfo
		for _, s := range skillList {
			skillInfos = append(skillInfos, config.SkillInfo{
				Name:        s.Name,
				Description: s.Description,
				Trigger:     s.Trigger,
			})
		}

		// 4. Patch Layer2 in instructions for each configured agent.
		agents := agent.Detect(jarvis.TemplatesFS)
		for _, a := range agents {
			if patchErr := a.WriteInstructions(config.Layer1Content(), layer2, skillInfos); patchErr != nil {
				fmt.Printf("Warning: failed to update %s instructions: %v\n", a.Name(), patchErr)
			} else {
				fmt.Printf("Updated %s instructions with persona %q.\n", a.Name(), presetName)
			}
			// Write output-style file if agent supports it
			if a.SupportsOutputStyles() {
				if styleErr := a.WriteOutputStyle(preset); styleErr != nil {
					fmt.Printf("Warning: failed to update %s output-style: %v\n", a.Name(), styleErr)
				} else {
					fmt.Printf("Updated %s output-style with persona %q.\n", a.Name(), presetName)
				}
			}
		}

		// 5. Update ~/.jarvis/config.yaml preset field.
		cfg, loadErr := config.Load()
		if loadErr != nil {
			return fmt.Errorf("load config: %w", loadErr)
		}
		cfg.PersonaPreset = presetName
		cfg.Preset = presetName
		if saveErr := config.Save(cfg); saveErr != nil {
			return fmt.Errorf("save config: %w", saveErr)
		}

		// 6. Confirmation.
		displayName := preset.DisplayName
		if displayName == "" {
			displayName = preset.Name
		}
		fmt.Printf("Persona set to %q (%s).\n", presetName, displayName)
		return nil
	},
}

func init() {
	personaCmd.AddCommand(personaSetCmd)
}

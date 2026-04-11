package main

import (
	"fmt"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
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

		// 3. Patch Layer2 in instructions for each configured agent.
		agents := agent.Detect()
		for _, a := range agents {
			if patchErr := a.WriteInstructions(config.Layer1Content(), layer2); patchErr != nil {
				fmt.Printf("Warning: failed to update %s instructions: %v\n", a.Name(), patchErr)
			} else {
				fmt.Printf("Updated %s instructions with persona %q.\n", a.Name(), presetName)
			}
		}

		// 4. Update ~/.jarvis/config.yaml preset field.
		cfg, loadErr := config.Load()
		if loadErr != nil {
			return fmt.Errorf("load config: %w", loadErr)
		}
		cfg.Preset = presetName
		if saveErr := config.Save(cfg); saveErr != nil {
			return fmt.Errorf("save config: %w", saveErr)
		}

		// 5. Confirmation.
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

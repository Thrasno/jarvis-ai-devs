// jarvis is the CLI entry point for Jarvis-Dev.
// It orchestrates a 5-step Bubbletea TUI wizard for first-run setup
// and provides subcommands for ongoing lifecycle management.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "jarvis",
	Short: "Jarvis-Dev — AI coding assistant configurator",
	Long: `Jarvis-Dev configures your AI coding environment (Claude Code, OpenCode)
with persistent memory (Hive), persona selection, and embedded skills.

Run without arguments to launch the setup wizard.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		noTUI, _ := cmd.Flags().GetBool("no-tui")

		if !config.IsConfigured() {
			// First run — launch the setup wizard.
			return runWizard(noTUI)
		}
		// Already configured — show status.
		return runStatus()
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("no-tui", false, "disable TUI, use readline prompts")
	rootCmd.AddCommand(personaCmd, syncCmd, loginCmd, timelineCmd, configCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runWizard launches the 5-step wizard, using TUI or plain readline depending on flags/TTY.
func runWizard(noTUI bool) error {
	wcfg := tui.WizardConfig{
		PersonaFS:  jarvis.PersonaFS,
		SkillsFS:   jarvis.SkillsFS,
		TemplateFS: jarvis.TemplatesFS,
	}

	if noTUI || !isatty.IsTerminal(os.Stdin.Fd()) {
		return tui.RunNoTUI(wcfg)
	}

	m := tui.NewModel(wcfg, false)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// runStatus prints the current jarvis configuration summary.
func runStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	fmt.Printf("Jarvis-Dev configured\n")
	fmt.Printf("  Email:   %s\n", cfg.Email)
	fmt.Printf("  Preset:  %s\n", cfg.Preset)
	fmt.Printf("  API:     %s\n", cfg.APIURL)
	if len(cfg.ConfiguredAgents) > 0 {
		fmt.Printf("  Agents:  ")
		for i, a := range cfg.ConfiguredAgents {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(a)
		}
		fmt.Println()
	}
	return nil
}

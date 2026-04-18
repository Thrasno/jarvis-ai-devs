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
		return runWizard(noTUI)
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("no-tui", false, "disable TUI, use readline prompts")
	rootCmd.AddCommand(personaCmd, syncCmd, loginCmd, timelineCmd, configCmd, initCmd)
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

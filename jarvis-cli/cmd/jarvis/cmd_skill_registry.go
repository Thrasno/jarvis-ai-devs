package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/projectregistry"
)

var skillRegistryCmd = newSkillRegistryCmd()

func newSkillRegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill-registry",
		Short: "Manage the project-local skill registry",
	}
	cmd.AddCommand(newSkillRegistryRefreshCmd())
	return cmd
}

func newSkillRegistryRefreshCmd() *cobra.Command {
	var cwd string
	var force bool
	var quiet bool
	var noGitignore bool

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh .jarvis/skill-registry.md for a project worktree",
		RunE: func(cmd *cobra.Command, args []string) error {
			refreshCWD := cwd
			if refreshCWD == "" {
				current, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get current working directory: %w", err)
				}
				refreshCWD = current
			}

			result, err := projectregistry.Refresh(cmd.Context(), projectregistry.RefreshOptions{
				CWD:         refreshCWD,
				Force:       force,
				NoGitignore: noGitignore,
			})
			if err != nil {
				return err
			}

			if !quiet {
				fmt.Fprintln(cmd.OutOrStdout(), "Skill registry refreshed")
				fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", result.Path)
				fmt.Fprintf(cmd.OutOrStdout(), "changed: %t\n", result.Changed)
				fmt.Fprintf(cmd.OutOrStdout(), "reason: %s\n", result.Reason)
				fmt.Fprintf(cmd.OutOrStdout(), "skills: %d\n", result.SkillCount)
			}
			printSkillRegistryWarnings(cmd.ErrOrStderr(), result.Warnings)
			return nil
		},
	}

	cmd.Flags().StringVar(&cwd, "cwd", "", "project directory to refresh; subdirectories resolve to the active git worktree root")
	cmd.Flags().BoolVar(&force, "force", false, "rewrite the registry even when generated content is unchanged")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress success output; warnings remain visible")
	cmd.Flags().BoolVar(&noGitignore, "no-gitignore", false, "skip adding skill registry paths to .gitignore")
	return cmd
}

func printSkillRegistryWarnings(out io.Writer, warnings []projectregistry.Warning) {
	for _, line := range projectregistry.FormatWarningLines("Warning: ", warnings) {
		if line != "" {
			fmt.Fprintln(out, line)
		}
	}
}

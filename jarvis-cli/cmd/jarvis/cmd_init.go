package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/project"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project with Jarvis-Dev (.jarvis/skill-registry.md)",
	Long: `Scaffold the .jarvis/ directory for the current project.

Creates .jarvis/skill-registry.md with suggested skills based on the detected
technology stack. The file is safe to commit — share it with your team so
everyone gets the same skill suggestions.

Re-running jarvis init updates the Suggested Skills section while preserving
any custom skills you have added.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		return runInit(dir)
	},
}

// runInit is the testable core of the init command.
// dir is the project root (working directory in normal use).
func runInit(dir string) error {
	projectName := project.DetectProject(dir)
	stack := project.DetectStack(dir)
	skills := project.SkillsForStack(stack)

	fmt.Println("Detecting project...")
	fmt.Printf("✓ Project: %s\n", projectName)
	fmt.Printf("✓ Stack:   %s\n", stack)
	fmt.Println()
	fmt.Println("Scaffolding .jarvis/...")

	if err := project.WriteRegistry(dir, projectName, stack, skills); err != nil {
		return fmt.Errorf("write skill registry: %w", err)
	}

	fmt.Println("✓ Skill registry created: .jarvis/skill-registry.md")
	fmt.Printf("✓ Skills:  %s\n", strings.Join(skills, ", "))
	fmt.Println()
	fmt.Println("commit .jarvis/ to share with your team")
	return nil
}

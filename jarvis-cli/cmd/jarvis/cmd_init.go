package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/projectregistry"
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
	root, err := projectregistry.ResolveRoot(context.Background(), dir)
	if err != nil {
		if !projectregistry.IsNonProjectError(err) {
			return err
		}
		root = dir
	}
	projectName := project.DetectProject(root)
	stack := project.DetectStack(root)
	suggestedSkills := project.SkillsForStack(stack)

	fmt.Println("Detecting project...")
	fmt.Printf("✓ Project: %s\n", projectName)
	fmt.Printf("✓ Stack:   %s\n", stack)
	fmt.Println()
	fmt.Println("Scaffolding .jarvis/...")
	result, err := projectregistry.Refresh(context.Background(), projectregistry.RefreshOptions{CWD: dir, AllowNonGitRoot: true, SkillsFS: jarvis.SkillsFS})
	if err != nil {
		return fmt.Errorf("refresh skill registry: %w", err)
	}
	printSkillRegistryWarnings(os.Stderr, result.Warnings)

	fmt.Println("✓ Skill registry created: .jarvis/skill-registry.md")
	fmt.Printf("✓ Skills:  %s\n", strings.Join(suggestedSkills, ", "))
	fmt.Println()
	fmt.Println("commit .jarvis/ to share with your team")
	return nil
}

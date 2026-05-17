package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
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
	suggestedSkills := project.SkillsForStack(stack)
	embeddedSkills, err := skills.ListSkills(jarvis.SkillsFS)
	if err != nil {
		return fmt.Errorf("list embedded skills: %w", err)
	}
	registrySkills := toProjectRegistrySkills(skills.RegistryRows(embeddedSkills))

	fmt.Println("Detecting project...")
	fmt.Printf("✓ Project: %s\n", projectName)
	fmt.Printf("✓ Stack:   %s\n", stack)
	fmt.Println()
	fmt.Println("Scaffolding .jarvis/...")

	if err := project.WriteRegistry(dir, projectName, stack, suggestedSkills, registrySkills); err != nil {
		return fmt.Errorf("write skill registry: %w", err)
	}

	fmt.Println("✓ Skill registry created: .jarvis/skill-registry.md")
	fmt.Printf("✓ Skills:  %s\n", strings.Join(suggestedSkills, ", "))
	fmt.Println()
	fmt.Println("commit .jarvis/ to share with your team")
	return nil
}

func toProjectRegistrySkills(rows []skills.RegistryRow) []project.RegistrySkill {
	registrySkills := make([]project.RegistrySkill, 0, len(rows))
	for _, row := range rows {
		registrySkills = append(registrySkills, project.RegistrySkill{
			ID:           row.ID,
			Name:         row.Name,
			Description:  row.Description,
			Trigger:      row.Trigger,
			Path:         row.Path,
			CompactRules: row.CompactRules,
			IsCore:       row.IsCore,
		})
	}
	return registrySkills
}

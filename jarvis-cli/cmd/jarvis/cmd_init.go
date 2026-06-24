package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/projectregistry"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project with Jarvis-Dev (.jarvis/skill-registry.md)",
	Long: `Scaffold the .jarvis/ directory for the current project.

Creates .jarvis/skill-registry.md with suggested skills based on the detected
technology stack. The generated .jarvis cache is gitignored by default and can
be regenerated with jarvis init or jarvis skill-registry refresh.

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

	fmt.Println("Detecting project...")
	fmt.Printf("✓ Project: %s\n", projectName)
	fmt.Println()
	fmt.Println("Scaffolding .jarvis/...")

	// Install embedded skill copies into <root>/.jarvis/skills.
	// This is init's responsibility; Refresh only indexes disk skills.
	agentSkillsDir := filepath.Join(root, ".jarvis", "skills")
	embeddedSkills, err := skills.ListSkills(jarvis.SkillsFS)
	if err != nil {
		return fmt.Errorf("list embedded skills: %w", err)
	}
	selected := make([]string, 0, len(embeddedSkills))
	for _, s := range embeddedSkills {
		selected = append(selected, s.ID)
	}
	installResult, err := skills.InstallSelectedWithResult(jarvis.SkillsFS, agentSkillsDir, selected)
	if err != nil {
		return fmt.Errorf("install project skill copies: %w", err)
	}

	// Refresh indexes the now-installed skill copies to produce the registry.
	result, err := projectregistry.Refresh(context.Background(), projectregistry.RefreshOptions{CWD: dir, AllowNonGitRoot: true})
	if err != nil {
		return fmt.Errorf("refresh skill registry: %w", err)
	}
	printSkillRegistryWarnings(os.Stderr, result.Warnings)

	installedCount := installResult.FilesWritten
	fmt.Println("✓ Skill registry created: .jarvis/skill-registry.md")
	fmt.Printf("✓ Skills: %d skill copies installed under .jarvis/skills\n", installedCount)
	fmt.Println()
	fmt.Println(".jarvis/ generated cache is gitignored by default; regenerate it with jarvis init or jarvis skill-registry refresh")
	return nil
}

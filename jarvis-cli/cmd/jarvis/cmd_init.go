package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
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
	if err := installProjectSkillCopies(dir); err != nil {
		return fmt.Errorf("install project skill copies: %w", err)
	}

	if err := project.WriteRegistry(dir, projectName, stack, suggestedSkills, registrySkills); err != nil {
		return fmt.Errorf("write skill registry: %w", err)
	}

	fmt.Println("✓ Skill registry created: .jarvis/skill-registry.md")
	fmt.Printf("✓ Skills:  %s\n", strings.Join(suggestedSkills, ", "))
	fmt.Println()
	fmt.Println("commit .jarvis/ to share with your team")
	return nil
}

func installProjectSkillCopies(dir string) error {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		return fmt.Errorf("open embedded skills: %w", err)
	}
	destRoot := filepath.Join(dir, ".jarvis", "skills")

	return fs.WalkDir(skillsFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("read embedded skill %s: %w", path, walkErr)
		}
		if path == "." || d.IsDir() {
			return nil
		}

		content, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return fmt.Errorf("read embedded skill %s: %w", path, err)
		}

		destPath := filepath.Join(destRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create skill dir for %s: %w", path, err)
		}
		tmp := destPath + ".tmp"
		if err := os.WriteFile(tmp, content, 0644); err != nil {
			return fmt.Errorf("write skill copy %s: %w", path, err)
		}
		if err := os.Rename(tmp, destPath); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("finalize skill copy %s: %w", path, err)
		}
		return nil
	})
}

func toProjectRegistrySkills(rows []skills.RegistryRow) []project.RegistrySkill {
	registrySkills := make([]project.RegistrySkill, 0, len(rows))
	for _, row := range rows {
		registrySkills = append(registrySkills, project.RegistrySkill{
			ID:           row.ID,
			Name:         row.Name,
			Description:  row.Description,
			Trigger:      row.Trigger,
			Scope:        row.Scope,
			Path:         row.Path,
			CompactRules: row.CompactRules,
			IsCore:       row.IsCore,
		})
	}
	return registrySkills
}

package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// ObserveProjectRegistryQuality inspects the project-local canonical registry
// without mutating the worktree. Problems are warning-grade diagnostics.
func ObserveProjectRegistryQuality(projectRoot string) sddruntime.ObservedRegistryQuality {
	if worktreeRoot, ok := resolveRegistryQualityWorktreeRoot(projectRoot); ok {
		projectRoot = worktreeRoot
	}
	registryPath := filepath.Join(projectRoot, project.CanonicalRegistryPath)
	quality := sddruntime.ObservedRegistryQuality{Checked: true, Path: project.CanonicalRegistryPath}

	registryInfo, err := os.Stat(registryPath)
	if os.IsNotExist(err) {
		return quality
	}
	if err != nil || registryInfo.IsDir() {
		return quality
	}
	quality.Exists = true

	content, err := os.ReadFile(registryPath)
	if err == nil && strings.Contains(string(content), project.RegistryWarningsHeader) {
		quality.HasWarnings = true
	}

	skillsRoot := filepath.Join(projectRoot, project.ProjectSkillsDir)
	_ = filepath.WalkDir(skillsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || quality.Stale || d.IsDir() || filepath.Base(path) != "SKILL.md" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().After(registryInfo.ModTime()) {
			quality.Stale = true
		}
		return nil
	})

	return quality
}

func resolveRegistryQualityWorktreeRoot(projectRoot string) (string, bool) {
	if strings.TrimSpace(projectRoot) == "" {
		return "", false
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = abs
	output, err := cmd.Output()
	if err != nil || ctx.Err() != nil {
		return "", false
	}

	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", false
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", false
	}
	return root, true
}

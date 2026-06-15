package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type skillModelSectionClass func(skillID string) sddruntime.ModelSectionClass

func skillModelSectionClassForPlatform(platform sddruntime.Platform, cfg *config.AppConfig) (skillModelSectionClass, error) {
	assignments, err := sddruntime.ResolveAssignmentsForPlatform(platform, cfg)
	if err != nil {
		return nil, err
	}
	return func(skillID string) sddruntime.ModelSectionClass {
		model := strings.TrimSpace(assignments[skillID])
		if model == "" {
			model = assignments["default"]
		}
		return sddruntime.ModelSectionClassForModel(model)
	}, nil
}

// installSkillsFromFS walks skillsFS and installs selected skill directories
// (plus _shared/) to destDir. Files are written atomically.
// skillsFS must be a sub-FS rooted at the embed/skills directory.
// selected lists skill directory names (e.g. ["sdd-apply", "hive"]).
// The _shared/ directory is ALWAYS installed regardless of the selected list.
func installSkillsFromFS(destDir string, skillsFS fs.FS, selected []string) error {
	return installSkillsFromFSWithModelSections(destDir, skillsFS, selected, nil)
}

func installSkillsFromFSWithModelSections(destDir string, skillsFS fs.FS, selected []string, sectionClass skillModelSectionClass) error {
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}

	return fs.WalkDir(skillsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path != "." {
				return fmt.Errorf("read skill file %s: %w", path, err)
			}
			return err
		}
		if path == "." {
			return nil
		}

		// Determine the top-level directory for this path.
		topDir := strings.SplitN(path, "/", 2)[0]

		if d.IsDir() {
			// For non-_shared directories: skip subtree if not selected.
			if topDir != "_shared" && !selectedSet[topDir] {
				return fs.SkipDir
			}
			return nil
		}

		// It's a file — install it.
		destPath := filepath.Join(destDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}
		content, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return fmt.Errorf("read skill file %s: %w", path, err)
		}
		if sectionClass != nil {
			rendered, err := sddruntime.RenderModelSections(string(content), sectionClass(topDir))
			if err != nil {
				return fmt.Errorf("render model sections for skill file %s: %w", path, err)
			}
			content = []byte(rendered)
		}
		return writeFileAtomic(destPath, content, 0644)
	})
}

// installAgentsFromFS walks agentsFS and writes every file it contains into
// destDir using the same filename. It is a flat walker: subdirectories are
// skipped. Writes are atomic and idempotent (existing files are overwritten).
func installAgentsFromFS(destDir string, agentsFS fs.FS) error {
	return fs.WalkDir(agentsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path != "." {
				return fmt.Errorf("read agent file %s: %w", path, err)
			}
			return err
		}
		if d.IsDir() {
			return nil
		}

		content, err := fs.ReadFile(agentsFS, path)
		if err != nil {
			return fmt.Errorf("read agent file %s: %w", path, err)
		}

		destPath := filepath.Join(destDir, filepath.FromSlash(path))
		return writeFileAtomic(destPath, content, 0644)
	})
}

// installOrchestrator writes rendered orchestrator markdown to destPath.
// The file is written atomically.
func installOrchestrator(destPath string, content []byte) error {
	return writeFileAtomic(destPath, content, 0644)
}

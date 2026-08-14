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

// RenderedSkillFile is one file the skills installer would write, named by a
// slash-separated path relative to the agent's skills directory.
type RenderedSkillFile struct {
	RelPath string
	Bytes   []byte
}

// RenderSkillFilesForPlatform produces the exact files InstallSkillsWithConfig
// would write for a platform, without writing any of them, so a caller needing
// the desired content of an installed skill tree reuses the installer's own
// walk and model-section rendering instead of guessing at one SKILL.md per
// skill. A nil cfg renders verbatim, matching InstallSkills.
func RenderSkillFilesForPlatform(skillsFS fs.FS, selected []string, platform sddruntime.Platform, cfg *config.AppConfig) ([]RenderedSkillFile, error) {
	var sectionClass skillModelSectionClass
	if cfg != nil {
		var err error
		if sectionClass, err = skillModelSectionClassForPlatform(platform, cfg); err != nil {
			return nil, fmt.Errorf("resolve skill model sections: %w", err)
		}
	}
	files := make([]RenderedSkillFile, 0, len(selected))
	err := walkSkillFiles(skillsFS, selected, sectionClass, func(relPath string, content []byte) error {
		files = append(files, RenderedSkillFile{RelPath: relPath, Bytes: content})
		return nil
	})
	return files, err
}

func installSkillsFromFSWithModelSections(destDir string, skillsFS fs.FS, selected []string, sectionClass skillModelSectionClass) error {
	return walkSkillFiles(skillsFS, selected, sectionClass, func(relPath string, content []byte) error {
		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", relPath, err)
		}
		return writeFileAtomic(destPath, content, 0644)
	})
}

// walkSkillFiles is the single walk both the installer and the desired-state
// planner read from, so what sync compares against is what the installer writes.
func walkSkillFiles(skillsFS fs.FS, selected []string, sectionClass skillModelSectionClass, emit func(relPath string, content []byte) error) error {
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

		// It's a file — render it.
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
		return emit(path, content)
	})
}

// installAgentsFromFS walks agentsFS and writes every file it contains into
// destDir using the same filename. It is a flat walker: subdirectories are
// skipped. Writes are atomic and idempotent (existing files are overwritten).
func installAgentsFromFS(destDir string, agentsFS fs.FS) error {
	if agentsFS == nil {
		return fmt.Errorf("installAgentsFromFS: agentsFS is nil")
	}
	return fs.WalkDir(agentsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path != "." {
				return fmt.Errorf("read agent file %s: %w", path, err)
			}
			return err
		}
		if d.IsDir() {
			if path == "." {
				return nil
			}
			return fs.SkipDir
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

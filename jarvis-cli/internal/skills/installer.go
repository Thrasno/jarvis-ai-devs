package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstallSelected installs the specified skills (by ID) into agentSkillsDir.
// fsys may be the root-package SkillsFS (with files under embed/skills) or a
// sub-FS already rooted at the skills directory.
// Install is idempotent: existing files are overwritten silently.
// Core skills (sdd-workflow, hive, and the active SDD runtime helpers) are
// always included regardless of selected.
func InstallSelected(fsys fs.FS, agentSkillsDir string, selected []string) error {
	skillsFS, err := skillsSubtree(fsys)
	if err != nil {
		return err
	}

	selectedSet := make(map[string]bool, len(selected)+len(coreSkillIDs))
	for _, id := range selected {
		selectedSet[id] = true
	}
	for id := range coreSkillIDs {
		selectedSet[id] = true
	}

	if err := os.MkdirAll(agentSkillsDir, 0755); err != nil {
		return fmt.Errorf("create skills dir %s: %w", agentSkillsDir, err)
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

		topDir := strings.SplitN(path, "/", 2)[0]
		if d.IsDir() {
			if topDir != "_shared" && !selectedSet[topDir] {
				return fs.SkipDir
			}
			return nil
		}

		destPath := filepath.Join(agentSkillsDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}

		content, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return fmt.Errorf("read skill file %s: %w", path, err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("write skill file %s: %w", path, err)
		}
		return nil
	})
}

func skillsSubtree(fsys fs.FS) (fs.FS, error) {
	if _, err := fs.Stat(fsys, "embed/skills"); err == nil {
		subFS, subErr := fs.Sub(fsys, "embed/skills")
		if subErr != nil {
			return nil, fmt.Errorf("open skills subtree: %w", subErr)
		}
		return subFS, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat skills subtree: %w", err)
	}

	return fsys, nil
}

package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type InstallResult struct {
	Changed      bool
	FilesWritten int
}

// InstallSelected installs the specified skills (by ID) into agentSkillsDir.
// fsys may be the root-package SkillsFS (with files under embed/skills) or a
// sub-FS already rooted at the skills directory.
// Install is idempotent: byte-equivalent files are left untouched, changed files
// are atomically replaced without following destination symlinks.
// Core skills (sdd-workflow, hive, and the active SDD runtime helpers) are
// always included regardless of selected.
func InstallSelected(fsys fs.FS, agentSkillsDir string, selected []string) error {
	_, err := InstallSelectedWithResult(fsys, agentSkillsDir, selected)
	return err
}

func InstallSelectedWithResult(fsys fs.FS, agentSkillsDir string, selected []string) (InstallResult, error) {
	skillsFS, err := skillsSubtree(fsys)
	if err != nil {
		return InstallResult{}, err
	}
	result := InstallResult{}

	selectedSet := make(map[string]bool, len(selected)+len(coreSkillIDs))
	for _, id := range selected {
		selectedSet[id] = true
	}
	for id := range coreSkillIDs {
		selectedSet[id] = true
	}

	if err := safeMkdirAll(agentSkillsDir); err != nil {
		return result, fmt.Errorf("create skills dir %s: %w", agentSkillsDir, err)
	}

	err = fs.WalkDir(skillsFS, ".", func(path string, d fs.DirEntry, err error) error {
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
		if err := safeMkdirAll(filepath.Dir(destPath)); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}

		content, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return fmt.Errorf("read skill file %s: %w", path, err)
		}

		changed, err := writeFileNoFollow(destPath, content, 0644)
		if err != nil {
			return fmt.Errorf("write skill file %s: %w", path, err)
		}
		if changed {
			result.Changed = true
			result.FilesWritten++
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func writeFileNoFollow(destPath string, content []byte, mode fs.FileMode) (bool, error) {
	info, err := os.Lstat(destPath)
	switch {
	case err == nil:
		if info.IsDir() {
			return false, fmt.Errorf("destination is a directory")
		}
		if info.Mode()&os.ModeSymlink == 0 {
			existing, readErr := os.ReadFile(destPath)
			if readErr != nil {
				return false, readErr
			}
			if bytes.Equal(existing, content) {
				return false, nil
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// New file; write it below.
	default:
		return false, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return false, err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return false, err
	}
	cleanup = false
	return true, nil
}

func safeMkdirAll(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)

	current := filepath.VolumeName(abs)
	remainder := strings.TrimPrefix(abs, current)
	if filepath.IsAbs(abs) {
		current += string(filepath.Separator)
		remainder = strings.TrimPrefix(remainder, string(filepath.Separator))
	}
	if current == "" {
		current = "."
	}

	for _, part := range strings.Split(remainder, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to follow symlink directory %s", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(current, 0755); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return err
		}
	}
	return nil
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

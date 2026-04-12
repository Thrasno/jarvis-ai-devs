package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// installSkillsFromFS walks skillsFS and installs selected skill directories
// (plus _shared/) to destDir. Files are written atomically.
// skillsFS must be a sub-FS rooted at the embed/skills directory.
// selected lists skill directory names (e.g. ["sdd-apply", "hive"]).
// The _shared/ directory is ALWAYS installed regardless of the selected list.
func installSkillsFromFS(destDir string, skillsFS fs.FS, selected []string) error {
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}

	return fs.WalkDir(skillsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
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
		return writeFileAtomic(destPath, content, 0644)
	})
}

package project

import (
	"path/filepath"
	"strings"
)

func BlockedProjectUnsafeRootWarning(directory, homeDir string) (string, bool) {
	cleanDir := cleanPath(directory)
	if cleanDir == "" {
		return "", false
	}
	unsafeRoots := []string{cleanPath("/tmp")}
	if home := cleanPath(homeDir); home != "" {
		unsafeRoots = append(unsafeRoots,
			home,
			cleanPath(filepath.Join(home, "Documents")),
			cleanPath(filepath.Join(home, "Downloads")),
			cleanPath(filepath.Join(home, "Desktop")),
		)
	}
	for _, root := range unsafeRoots {
		if cleanDir == root {
			return "blocked project root is an unsafe broad directory; local quarantine was recorded without hard purge", true
		}
	}
	return "", false
}

func cleanPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

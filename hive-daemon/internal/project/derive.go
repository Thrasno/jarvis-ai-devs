package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// safeNamePattern matches characters allowed in a canonical project name.
// Allowed: ASCII letters, digits, dot, underscore, hyphen.
// parity anchor: jarvis-cli/internal/project/detector.go:safeNamePattern
var safeNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// DeriveFromDirectory returns the canonical project name for dir using the
// SAME resolution order as jarvis-cli DetectProject:
//  1. git remote get-url origin -> repo name (.git stripped)  [cmd.Dir = dir]
//  2. basename(dir)
//  3. "default"
//
// The directory must exist on the filesystem. If os.Stat(dir) fails for a
// non-empty dir, "default" is returned rather than deriving from the basename
// of a fabricated path.
//
// parity anchor: jarvis-cli/internal/project/detector.go:DetectProject
func DeriveFromDirectory(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	// Trust-boundary guard: only derive from directories that actually exist.
	if _, err := os.Stat(dir); err != nil {
		return "default"
	}
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		if name := extractRepoName(strings.TrimSpace(string(out))); name != "" {
			return name
		}
	}
	if base := filepath.Base(dir); base != "" && base != "." && base != "/" {
		return base
	}
	return "default"
}

// extractRepoName parses a git remote URL and returns the repository name.
// Handles both HTTPS (https://github.com/org/repo.git) and SSH (git@github.com:org/repo.git).
// The returned name is sanitized: only [A-Za-z0-9._-] characters are kept.
// This prevents prompt-injection via crafted remote URLs.
// parity anchor: jarvis-cli/internal/project/detector.go:extractRepoName
func extractRepoName(remoteURL string) string {
	remoteURL = strings.TrimSuffix(remoteURL, ".git")
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return ""
	}
	// Find last segment separator — whichever of '/' or ':' comes last.
	lastSlash := strings.LastIndex(remoteURL, "/")
	lastColon := strings.LastIndex(remoteURL, ":")
	sep := lastSlash
	if lastColon > sep {
		sep = lastColon
	}
	var name string
	if sep < 0 || sep == len(remoteURL)-1 {
		name = remoteURL
	} else {
		name = remoteURL[sep+1:]
	}
	// Sanitize: strip any character outside [A-Za-z0-9._-].
	return safeNamePattern.ReplaceAllString(name, "")
}

// ResolveEffectiveProject returns the project name to use for persistence and
// validation, together with a provenance flag indicating whether the name was
// derived from the filesystem (derived=true) or supplied by the caller
// (derived=false).
//
// Rules:
//   - Non-empty project (even whitespace-trimmed empty is treated as empty):
//     return (project, false) — caller-supplied name, no derivation.
//   - Empty project + non-empty directory: derive via DeriveFromDirectory,
//     return (derived_name, true).
//   - Both empty: return ("", false).
//
// The derived bool is an explicit return value — never inferred — so that
// callers can gate provenance-specific behaviour (e.g. the project_unknown
// bypass in memSaveHandler) without ambiguity.
func ResolveEffectiveProject(projectName, directory string) (string, bool) {
	if strings.TrimSpace(projectName) != "" {
		return projectName, false
	}
	if strings.TrimSpace(directory) != "" {
		return DeriveFromDirectory(directory), true
	}
	return "", false
}

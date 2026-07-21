// Package hivederive is the single source of truth for deriving a canonical
// project name from a directory. It is consumed by both jarvis-cli and
// hive-daemon via a relative module replace, replacing the two drifting
// implementations that previously fell back to a silent "default" string.
//
// Derivation is a pure string transform except for one filesystem stat and one
// git subprocess. The stat, runtime GOOS, and WSL-marker detection are
// injectable package vars so behavior can be exercised deterministically in
// tests without real Windows/WSL mounts.
package hivederive

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Typed errors returned by Derive. They replace the previous silent "default"
// sentinel so callers can distinguish "no directory given" from "path could not
// be resolved" from "resolved but no real name derivable".
var (
	// ErrEmptyDir is returned when the supplied directory is empty or blank.
	ErrEmptyDir = errors.New("hivederive: empty directory")
	// ErrPathUnresolvable is returned when a non-empty directory cannot be
	// stat'd, even after WSL/Windows normalization.
	ErrPathUnresolvable = errors.New("hivederive: path cannot be resolved")
	// ErrNoDerivableName is returned when the directory resolves but no real name
	// (neither git remote nor usable basename) can be derived from it.
	ErrNoDerivableName = errors.New("hivederive: no real name derivable")
)

// osStat is the injectable stat function (defaults to os.Stat).
var osStat = os.Stat

// safeNamePattern matches characters NOT allowed in a canonical project name.
// Allowed: ASCII letters, digits, dot, underscore, hyphen. Stripping the rest
// prevents prompt-injection via crafted git remote URLs.
var safeNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// Derive returns the canonical project name for dir. Resolution order:
//  1. git remote get-url origin -> repo name (.git stripped)  [cmd.Dir = dir]
//  2. basename(dir)
//
// It never returns the literal "default". An empty directory yields
// ErrEmptyDir without touching the filesystem or the process's ambient cwd. A
// non-empty directory that cannot be stat'd (after normalization) yields
// ErrPathUnresolvable. A resolvable directory with no derivable name yields
// ErrNoDerivableName.
func Derive(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", ErrEmptyDir
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		return "", err
	}
	if name := gitRemoteName(resolved); name != "" {
		return name, nil
	}
	if base := filepath.Base(resolved); base != "" && base != "." && base != "/" {
		return base, nil
	}
	return "", ErrNoDerivableName
}

// resolveDir stats dir and returns the path that actually exists. It tries the
// raw path first; only if that fails does it retry the WSL/Windows-normalized
// form, so a native path never pays for translation and a Windows-form path
// received by a WSL daemon still resolves.
func resolveDir(dir string) (string, error) {
	if _, err := osStat(dir); err == nil {
		return dir, nil
	}
	if norm, nerr := NormalizePath(dir); nerr == nil && norm != dir {
		if _, err := osStat(norm); err == nil {
			return norm, nil
		}
	}
	return "", ErrPathUnresolvable
}

// gitRemoteName returns the sanitized repo name from origin's remote URL, or ""
// when dir is not a git repo or has no origin. The directory is passed via
// cmd.Dir (never as an argument) so a crafted directory string cannot inject
// arguments into the git subprocess.
func gitRemoteName(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return extractRepoName(strings.TrimSpace(string(out)))
}

// extractRepoName parses a git remote URL and returns the repository name.
// Handles both HTTPS (https://github.com/org/repo.git) and SSH
// (git@github.com:org/repo.git). The returned name is sanitized to
// [A-Za-z0-9._-] to prevent prompt-injection via crafted remote URLs.
func extractRepoName(remoteURL string) string {
	remoteURL = strings.TrimSuffix(strings.TrimSpace(remoteURL), ".git")
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return ""
	}
	// Last segment separator — whichever of '/' or ':' comes last.
	sep := strings.LastIndex(remoteURL, "/")
	if colon := strings.LastIndex(remoteURL, ":"); colon > sep {
		sep = colon
	}
	name := remoteURL
	if sep >= 0 && sep < len(remoteURL)-1 {
		name = remoteURL[sep+1:]
	}
	return safeNamePattern.ReplaceAllString(name, "")
}

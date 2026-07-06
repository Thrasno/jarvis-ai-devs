package projectregistry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const rootResolutionTimeout = 2 * time.Second

var ErrNotGitWorktree = errors.New("not a git worktree")

func IsNonProjectError(err error) bool {
	return errors.Is(err, ErrNotGitWorktree)
}

// ResolveRoot resolves cwd to the active git worktree root.
func ResolveRoot(ctx context.Context, cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", fmt.Errorf("project root cwd is required")
	}

	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", cwd, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("project root %q is not accessible: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", abs)
	}

	ctx, cancel := context.WithTimeout(ctx, rootResolutionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = abs
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("resolve git worktree root for %q: %w", abs, ctx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("%q is %w", abs, ErrNotGitWorktree)
	}

	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("git worktree root for %q is empty", abs)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve git worktree root %q: %w", root, err)
	}
	return root, nil
}

func resolveExplicitProjectRoot(cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", fmt.Errorf("project root cwd is required")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", cwd, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("project root %q is not accessible: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", abs)
	}
	return abs, nil
}

type pathEquivalenceFunc func(a, b string) bool

type unsafeRootWarningLocation struct {
	path  string
	label string
}

func unsafeRootWarnings(root string) []Warning {
	return unsafeRootWarningsForHomes(root, homePathCandidates(), samePath)
}

func unsafeRootWarningsForHomes(root string, homes []string, same pathEquivalenceFunc) []Warning {
	var warnings []Warning
	add := func(location unsafeRootWarningLocation) {
		path := location.path
		if same(root, path) {
			warnings = append(warnings, Warning{
				Code:     "unsafe-project-root",
				Severity: SeverityWarning,
				Path:     root,
				Message:  fmt.Sprintf("unsafe project root %q points at %s; continuing with warning only", root, location.label),
			})
		}
	}
	for _, home := range homes {
		for _, location := range unsafeRootWarningPolicy(home) {
			add(location)
		}
	}
	add(unsafeRootWarningLocation{path: os.TempDir(), label: os.TempDir()})
	return warnings
}

func unsafeRootWarningPolicy(home string) []unsafeRootWarningLocation {
	return []unsafeRootWarningLocation{
		{path: home, label: "home directory"},
		{path: filepath.Join(home, "Documents"), label: "Documents"},
		{path: filepath.Join(home, "Downloads"), label: "Downloads"},
		{path: filepath.Join(home, "Desktop"), label: "Desktop"},
	}
}

func homePathCandidates() []string {
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, home)
	}
	for _, env := range []string{"HOME", "USERPROFILE"} {
		candidates = append(candidates, os.Getenv(env))
	}
	if drive, path := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"); drive != "" && path != "" {
		candidates = append(candidates, drive+path)
	}

	seen := make(map[string]bool, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		key := abs
		if runtime.GOOS == "windows" {
			key = strings.ToLower(abs)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, abs)
	}
	return unique
}

func samePath(a, b string) bool {
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	if aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo) {
		return true
	}

	aClean, aErr := cleanAbs(a)
	bClean, bErr := cleanAbs(b)
	if aErr != nil || bErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aClean, bClean)
	}
	return aClean == bClean
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

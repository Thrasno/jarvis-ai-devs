package projectregistry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const rootResolutionTimeout = 2 * time.Second

// ResolveRoot resolves cwd to the active git worktree root and rejects roots
// that could target generated home/config agent state instead of a project.
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
		return "", fmt.Errorf("%q is not inside a git worktree", abs)
	}

	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("git worktree root for %q is empty", abs)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve git worktree root %q: %w", root, err)
	}
	if err := rejectUnsafeRoot(root); err != nil {
		return "", err
	}
	return root, nil
}

func rejectUnsafeRoot(root string) error {
	return rejectUnsafeRootForHomes(root, homePathCandidates(), samePath)
}

type pathEquivalenceFunc func(a, b string) bool

func rejectUnsafeRootForHomes(root string, homes []string, same pathEquivalenceFunc) error {
	for _, home := range homes {
		if same(root, home) {
			return fmt.Errorf("unsafe project root %q: refusing to write to the home directory", root)
		}
		configRoot := filepath.Join(home, ".config")
		if same(root, configRoot) || isWithinWithSame(root, configRoot, same) {
			return fmt.Errorf("unsafe project root %q: refusing to write to home config directories", root)
		}
		claudeRoot := filepath.Join(home, ".claude")
		if same(root, claudeRoot) || isWithinWithSame(root, claudeRoot, same) {
			return fmt.Errorf("unsafe project root %q: refusing to write to home Claude generated state", root)
		}
	}
	return nil
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

func isWithin(path, parent string) bool {
	return isWithinWithSame(path, parent, samePath)
}

func isWithinWithSame(path, parent string, same pathEquivalenceFunc) bool {
	path, err := cleanAbs(path)
	if err != nil {
		return false
	}
	parent, err = cleanAbs(parent)
	if err != nil {
		return false
	}
	if same(path, parent) {
		return false
	}

	for current := path; ; current = filepath.Dir(current) {
		if same(current, parent) {
			return true
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
	}
	return false
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

package projectregistry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil
	}

	if samePath(root, home) {
		return fmt.Errorf("unsafe project root %q: refusing to write to the home directory", root)
	}
	configRoot := filepath.Join(home, ".config")
	if samePath(root, configRoot) || isWithin(root, configRoot) {
		return fmt.Errorf("unsafe project root %q: refusing to write to home config directories", root)
	}
	claudeRoot := filepath.Join(home, ".claude")
	if samePath(root, claudeRoot) || isWithin(root, claudeRoot) {
		return fmt.Errorf("unsafe project root %q: refusing to write to home Claude generated state", root)
	}
	return nil
}

func samePath(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && rel == "."
}

func isWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

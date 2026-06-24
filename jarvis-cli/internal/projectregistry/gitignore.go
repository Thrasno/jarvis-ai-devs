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

// gitignoreEntries are the paths that EnsureGitignore appends to .gitignore.
// These represent per-machine scan caches that should not be committed by default.
var gitignoreEntries = []string{
	".jarvis/skill-registry.md",
	".jarvis/skills/",
}

// EnsureGitignoreOptions configures the EnsureGitignore operation.
type EnsureGitignoreOptions struct {
	// NoGitignore disables all .gitignore mutation when true.
	NoGitignore bool
}

// EnsureGitignore ensures that .gitignore in the git worktree root contains
// the per-machine skill registry cache entries. It is idempotent: entries are
// not duplicated if already present. If root is not inside a git worktree the
// call is a non-fatal no-op.
//
// Steady-state short-circuit: when all entries are already present in
// .gitignore, no git commands are run (no worktree check, no tracking audit).
// The worktree check and the tracking audit (isGitTracked / git rm --cached)
// are only performed on the run that actually appends new entries, because
// that is the only run that may need to untrack previously committed files.
//
// When a listed path is currently tracked by git, EnsureGitignore attempts
// to untrack it with `git rm --cached`. A failure of `git rm` produces a
// non-fatal warning string rather than an error so that hook contexts are not
// broken by a missing tracking state. Filesystem errors (read/write .gitignore)
// abort with a hard error.
//
// Returns (warning, nil) on success. warning is non-empty when git rm fails.
func EnsureGitignore(ctx context.Context, root string, opts EnsureGitignoreOptions) (string, error) {
	if opts.NoGitignore {
		return "", nil
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read .gitignore: %w", err)
	}

	var toAppend []string
	for _, entry := range gitignoreEntries {
		if !containsGitignoreEntry(existing, entry) {
			toAppend = append(toAppend, entry)
		}
	}

	// Steady-state short-circuit: all entries already present — skip the git
	// worktree check and the per-path tracking audit entirely. The untrack work
	// only needs to happen on the run that first appends entries.
	if len(toAppend) == 0 {
		return "", nil
	}

	// Only do the worktree check when there is actual work to do.
	if !isInsideGitWorktree(ctx, root) {
		return "", nil
	}

	var sb strings.Builder
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		sb.WriteString(existing)
		sb.WriteString("\n")
	} else {
		sb.WriteString(existing)
	}
	for _, entry := range toAppend {
		sb.WriteString(entry)
		sb.WriteString("\n")
	}
	if err := os.WriteFile(gitignorePath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("write .gitignore: %w", err)
	}

	var warnings []string
	for _, entry := range gitignoreEntries {
		if !isGitTracked(ctx, root, entry) {
			continue
		}
		// Use -r for directories so git rm recurses.
		recursive := strings.HasSuffix(entry, "/")
		if warn := runGitRmCached(ctx, root, entry, recursive); warn != "" {
			warnings = append(warnings, warn)
		}
	}

	return strings.Join(warnings, "; "), nil
}

// isInsideGitWorktree reports whether root is inside an active git worktree.
// It uses git rev-parse --is-inside-work-tree, which returns "true" for any
// directory within a worktree (including subdirectories, not just the root).
func isInsideGitWorktree(ctx context.Context, root string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// isGitTracked reports whether path (relative to root) is currently tracked by git.
func isGitTracked(ctx context.Context, root, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// Strip trailing slash for directories; git ls-files doesn't need it.
	cleanPath := strings.TrimSuffix(path, "/")
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", cleanPath)
	cmd.Dir = root
	err := cmd.Run()
	return err == nil
}

// runGitRmCached runs `git rm --cached [-r] <path>`.
// When recursive is true, -r is added so that git rm recurses into directories.
// Returns a non-fatal warning string on failure; returns "" on success.
func runGitRmCached(ctx context.Context, root, path string, recursive bool) string {
	cleanPath := strings.TrimSuffix(path, "/")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := []string{"rm", "--cached"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, cleanPath)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		qualifier := ""
		if recursive {
			qualifier = " -r"
		}
		return fmt.Sprintf("git rm --cached%s %s: %v (%s)", qualifier, cleanPath, err, strings.TrimSpace(string(out)))
	}
	return ""
}

// containsGitignoreEntry reports whether entry is present as a complete line in content.
func containsGitignoreEntry(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

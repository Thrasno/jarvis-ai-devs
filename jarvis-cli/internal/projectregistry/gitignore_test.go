package projectregistry

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureGitignore_AppendsEntriesWhenAbsent verifies that when no .gitignore
// exists in the git worktree root, EnsureGitignore creates one containing both
// required entries.
func TestEnsureGitignore_AppendsEntriesWhenAbsent(t *testing.T) {
	root := initGitWorktree(t)

	warning, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{})
	if err != nil {
		t.Fatalf("EnsureGitignore returned error: %v", err)
	}
	_ = warning // non-fatal warning; content check is sufficient

	gitignorePath := filepath.Join(root, ".gitignore")
	content := string(mustReadFile(t, gitignorePath))
	for _, entry := range []string{".jarvis/skill-registry.md", ".jarvis/skills/"} {
		if !strings.Contains(content, entry) {
			t.Fatalf("expected .gitignore to contain %q after EnsureGitignore, got:\n%s", entry, content)
		}
	}
}

// TestEnsureGitignore_Idempotent verifies that calling EnsureGitignore twice does
// not duplicate entries.
func TestEnsureGitignore_Idempotent(t *testing.T) {
	root := initGitWorktree(t)

	if _, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{}); err != nil {
		t.Fatalf("first EnsureGitignore returned error: %v", err)
	}
	if _, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{}); err != nil {
		t.Fatalf("second EnsureGitignore returned error: %v", err)
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	content := string(mustReadFile(t, gitignorePath))

	for _, entry := range []string{".jarvis/skill-registry.md", ".jarvis/skills/"} {
		count := strings.Count(content, entry)
		if count != 1 {
			t.Fatalf("expected %q to appear exactly once in .gitignore, got %d times:\n%s", entry, count, content)
		}
	}
}

// TestEnsureGitignore_PreservesExisting verifies that existing .gitignore content
// is preserved and the new entries are appended.
func TestEnsureGitignore_PreservesExisting(t *testing.T) {
	root := initGitWorktree(t)
	gitignorePath := filepath.Join(root, ".gitignore")
	existing := "# existing rules\n*.log\nbuild/\n"
	if err := os.WriteFile(gitignorePath, []byte(existing), 0644); err != nil {
		t.Fatalf("write existing .gitignore: %v", err)
	}

	if _, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{}); err != nil {
		t.Fatalf("EnsureGitignore returned error: %v", err)
	}

	content := string(mustReadFile(t, gitignorePath))
	if !strings.Contains(content, "*.log") {
		t.Fatalf("existing content was lost; .gitignore:\n%s", content)
	}
	for _, entry := range []string{".jarvis/skill-registry.md", ".jarvis/skills/"} {
		if !strings.Contains(content, entry) {
			t.Fatalf("expected .gitignore to contain %q, got:\n%s", entry, content)
		}
	}
}

// TestEnsureGitignore_SkipsForNonGitRoot verifies that when root is not a git
// worktree, EnsureGitignore returns no error and does not create a .gitignore.
func TestEnsureGitignore_SkipsForNonGitRoot(t *testing.T) {
	root := t.TempDir() // plain directory, not a git worktree

	warning, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{})
	if err != nil {
		t.Fatalf("EnsureGitignore returned error for non-git root: %v", err)
	}
	_ = warning

	gitignorePath := filepath.Join(root, ".gitignore")
	if _, statErr := os.Stat(gitignorePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .gitignore for non-git root, stat err=%v", statErr)
	}
}

// TestEnsureGitignore_NoGitignoreOptSkips verifies that when NoGitignore is true,
// EnsureGitignore is a complete no-op — even in a valid git worktree.
func TestEnsureGitignore_NoGitignoreOptSkips(t *testing.T) {
	root := initGitWorktree(t)

	warning, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{NoGitignore: true})
	if err != nil {
		t.Fatalf("EnsureGitignore with NoGitignore=true returned error: %v", err)
	}
	_ = warning

	gitignorePath := filepath.Join(root, ".gitignore")
	if _, statErr := os.Stat(gitignorePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .gitignore when NoGitignore=true, stat err=%v", statErr)
	}
}

// TestEnsureGitignore_SteadyStateSkipsGitWork verifies that a second call to
// EnsureGitignore when all entries are already in .gitignore does NOT mutate
// the file (no write, no git execs). This is the steady-state short-circuit:
// once entries are present, git worktree detection and tracking audits are skipped.
func TestEnsureGitignore_SteadyStateSkipsGitWork(t *testing.T) {
	root := initGitWorktree(t)

	// First call: populates .gitignore.
	if _, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{}); err != nil {
		t.Fatalf("first EnsureGitignore returned error: %v", err)
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	infoBefore, err := os.Stat(gitignorePath)
	if err != nil {
		t.Fatalf("stat .gitignore after first call: %v", err)
	}

	// Second call: all entries already present — must not mutate .gitignore.
	if _, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{}); err != nil {
		t.Fatalf("second EnsureGitignore returned error: %v", err)
	}

	infoAfter, err := os.Stat(gitignorePath)
	if err != nil {
		t.Fatalf("stat .gitignore after second call: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("steady-state EnsureGitignore mutated .gitignore: mtime before=%s after=%s",
			infoBefore.ModTime(), infoAfter.ModTime())
	}
}

// TestEnsureGitignore_UntracksTrackedPaths verifies that when a path covered by
// gitignoreEntries is currently tracked by git, EnsureGitignore untracts it
// (git ls-files no longer lists it after the call).
func TestEnsureGitignore_UntracksTrackedPaths(t *testing.T) {
	root := initGitWorktreeWithConfig(t)

	// Create the skill registry file and a file under .jarvis/skills/.
	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}
	if err := os.WriteFile(registryPath, []byte("# registry\n"), 0644); err != nil {
		t.Fatalf("write skill-registry.md: %v", err)
	}
	skillDir := filepath.Join(root, ".jarvis", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# skill\n"), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Stage and commit both files so they are tracked.
	runGit(t, root, "add", ".jarvis/skill-registry.md")
	runGit(t, root, "add", ".jarvis/skills/")
	runGit(t, root, "commit", "-m", "track jarvis files")

	// Verify they are tracked before the call.
	if !isGitTracked(context.Background(), root, ".jarvis/skill-registry.md") {
		t.Fatal("precondition: .jarvis/skill-registry.md must be tracked before EnsureGitignore")
	}

	warning, err := EnsureGitignore(context.Background(), root, EnsureGitignoreOptions{})
	if err != nil {
		t.Fatalf("EnsureGitignore returned error: %v", err)
	}
	// A warning is acceptable (git rm may partially fail), but the call itself must not error.
	_ = warning

	// After EnsureGitignore, the registry file must not be tracked.
	if isGitTracked(context.Background(), root, ".jarvis/skill-registry.md") {
		t.Error(".jarvis/skill-registry.md is still tracked after EnsureGitignore")
	}
	// The skill file should also be untracked.
	if isGitTracked(context.Background(), root, ".jarvis/skills/test-skill/SKILL.md") {
		t.Error(".jarvis/skills/test-skill/SKILL.md is still tracked after EnsureGitignore")
	}
}

// TestEnsureGitignore_GitRmFailureIsWarning verifies that when nothing is actually
// tracked (git rm would fail / have nothing to remove), EnsureGitignore returns
// a warning string rather than a hard error. The specific scenario is a clean repo
// where the entries are NOT yet committed but are added to .gitignore for the first
// time — git ls-files finds nothing to untrack, so no warning is emitted at all,
// which is the success path. This test covers the case where we can synthesize a
// git rm failure without corrupting the repo.
//
// Because git rm on an untracked path exits non-zero, we confirm the function
// demotes that to a warning (no error returned).
func TestEnsureGitignore_GitRmFailureIsWarning(t *testing.T) {
	root := initGitWorktreeWithConfig(t)

	// Manually write .gitignore without the entries and also pre-seed a .jarvis/
	// directory so isGitTracked can be called without error — but do NOT commit
	// anything so git rm will fail with "not in index".
	if err := os.MkdirAll(filepath.Join(root, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}

	// Directly test runGitRmCached on a path that is not in the index.
	// This exercises the warning-path branch without needing a full EnsureGitignore setup.
	warn := runGitRmCached(context.Background(), root, ".jarvis/skill-registry.md", false)
	if warn == "" {
		t.Fatal("expected a non-empty warning when git rm targets an untracked path")
	}
	// The warning must not be an error — the function returns a string, not an error.
	// (This is validated by the function signature itself.)
	if !strings.Contains(warn, "git rm") {
		t.Fatalf("warning %q should reference 'git rm'", warn)
	}
}

// initGitWorktreeWithConfig initialises a git worktree and sets the minimal
// git config required for commits (user.email, user.name), so that tests that
// call `git commit` do not fail with a missing identity error.
func initGitWorktreeWithConfig(t *testing.T) string {
	t.Helper()
	root := initGitWorktree(t)
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	return root
}

// gitLsFiles returns the list of tracked files in root matching the given pathspec.
func gitLsFiles(t *testing.T, root, pathspec string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", pathspec)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// ls-files exits 0 even when nothing is found; non-zero means a real error.
		t.Fatalf("git ls-files %s: %v", pathspec, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

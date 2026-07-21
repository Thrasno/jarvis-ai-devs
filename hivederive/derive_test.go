package hivederive

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a git repository at dir with the given origin remote URL.
func initGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init")
	run("remote", "add", "origin", remoteURL)
}

// TestDerive covers the single derivation source-of-truth resolution order:
// git remote name -> basename -> typed error, and the no-ambient-cwd guarantee.
func TestDerive(t *testing.T) {
	t.Run("git remote name", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git not available")
		}
		dir := t.TempDir()
		initGitRepo(t, dir, "git@github.com:org/repo.git")
		name, err := Derive(dir)
		if err != nil {
			t.Fatalf("Derive(git dir) error = %v, want nil", err)
		}
		if name != "repo" {
			t.Errorf("Derive(git dir) = %q, want %q", name, "repo")
		}
	})

	t.Run("no-remote basename fallback", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "myproj")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		name, err := Derive(sub)
		if err != nil {
			t.Fatalf("Derive(non-git dir) error = %v, want nil", err)
		}
		if name != "myproj" {
			t.Errorf("Derive(non-git dir) = %q, want %q", name, "myproj")
		}
	})

	t.Run("empty directory yields ErrEmptyDir and never execs git", func(t *testing.T) {
		// A sentinel stat that fails loudly proves no filesystem/git work happens
		// for an empty directory: the guard must short-circuit before any stat.
		restore := swapStat(func(string) (os.FileInfo, error) {
			t.Fatalf("os.Stat must not be called for an empty directory")
			return nil, nil
		})
		defer restore()

		name, err := Derive("")
		if !errors.Is(err, ErrEmptyDir) {
			t.Fatalf("Derive(\"\") error = %v, want ErrEmptyDir", err)
		}
		if name != "" {
			t.Errorf("Derive(\"\") name = %q, want empty", name)
		}
		if name == "default" {
			t.Errorf("Derive must never return the literal %q", "default")
		}
	})

	t.Run("whitespace directory yields ErrEmptyDir", func(t *testing.T) {
		name, err := Derive("   ")
		if !errors.Is(err, ErrEmptyDir) {
			t.Fatalf("Derive(whitespace) error = %v, want ErrEmptyDir", err)
		}
		if name != "" {
			t.Errorf("Derive(whitespace) name = %q, want empty", name)
		}
	})

	t.Run("unresolvable path yields ErrPathUnresolvable, never default", func(t *testing.T) {
		name, err := Derive("/totally/fabricated/path/does/not/exist")
		if !errors.Is(err, ErrPathUnresolvable) {
			t.Fatalf("Derive(fabricated) error = %v, want ErrPathUnresolvable", err)
		}
		if name != "" {
			t.Errorf("Derive(fabricated) name = %q, want empty", name)
		}
		if name == "default" {
			t.Errorf("Derive must never return the literal %q", "default")
		}
	})
}

// TestExtractRepoName covers URL parsing and prompt-injection sanitization for
// the moved helper.
func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"https with .git", "https://github.com/org/repo.git", "repo"},
		{"ssh with .git", "git@github.com:org/repo.git", "repo"},
		{"https without .git", "https://github.com/org/myproj", "myproj"},
		{"empty", "", ""},
		{"injection chars stripped", "git@github.com:org/re;po`rm -rf`.git", "reporm-rf"},
		{"whitespace and newline stripped", "https://github.com/org/re\npo\r.git", "repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractRepoName(tt.url); got != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

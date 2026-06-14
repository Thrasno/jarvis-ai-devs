package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSkillRegistryRefreshCommand(t *testing.T) {
	t.Run("refreshes canonical registry from explicit subdirectory cwd", func(t *testing.T) {
		root := initCommandGitWorktree(t)
		subdir := filepath.Join(root, "cmd", "app")
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("create subdir: %v", err)
		}

		output, err := executeSkillRegistryCommand("refresh", "--cwd", subdir)

		if err != nil {
			t.Fatalf("skill-registry refresh returned error: %v\noutput:\n%s", err, output)
		}
		registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
		assertCommandFileContains(t, registryPath, "Canonical registry path: `.jarvis/skill-registry.md`")
		assertCommandFileContains(t, filepath.Join(root, ".jarvis", "skills", "sdd-apply", "SKILL.md"), "sdd-apply")
		for _, want := range []string{"Skill registry refreshed", registryPath, "changed: true", "reason: created", "skills:"} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected output to contain %q, got:\n%s", want, output)
			}
		}
	})

	t.Run("defaults cwd to current working directory", func(t *testing.T) {
		root := initCommandGitWorktree(t)
		subdir := filepath.Join(root, "packages", "api")
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("create subdir: %v", err)
		}
		previousDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("get current working directory: %v", err)
		}
		if err := os.Chdir(subdir); err != nil {
			t.Fatalf("chdir to subdir: %v", err)
		}
		t.Cleanup(func() {
			if err := os.Chdir(previousDir); err != nil {
				t.Fatalf("restore working directory: %v", err)
			}
		})

		output, err := executeSkillRegistryCommand("refresh")

		if err != nil {
			t.Fatalf("skill-registry refresh returned error: %v\noutput:\n%s", err, output)
		}
		registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
		assertCommandFileContains(t, registryPath, "Canonical registry path: `.jarvis/skill-registry.md`")
		if _, err := os.Stat(filepath.Join(subdir, ".jarvis", "skill-registry.md")); !os.IsNotExist(err) {
			t.Fatalf("expected default cwd from subdir to write at git root only, got stat err=%v", err)
		}
		if !strings.Contains(output, registryPath) {
			t.Fatalf("expected output to include git-root registry path %q, got:\n%s", registryPath, output)
		}
	})

	t.Run("rejects invalid roots without writing registry files", func(t *testing.T) {
		root := t.TempDir()

		output, err := executeSkillRegistryCommand("refresh", "--cwd", root)

		if err == nil {
			t.Fatalf("expected invalid root error, got nil output:\n%s", output)
		}
		if !strings.Contains(err.Error(), "git worktree") {
			t.Fatalf("expected git worktree error, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".jarvis", "skill-registry.md")); !os.IsNotExist(statErr) {
			t.Fatalf("expected no registry file for invalid root, got stat err=%v", statErr)
		}
	})

	t.Run("quiet suppresses unchanged success output", func(t *testing.T) {
		root := initCommandGitWorktree(t)
		if output, err := executeSkillRegistryCommand("refresh", "--cwd", root); err != nil {
			t.Fatalf("seed refresh returned error: %v\noutput:\n%s", err, output)
		}

		output, err := executeSkillRegistryCommand("refresh", "--cwd", root, "--quiet")

		if err != nil {
			t.Fatalf("quiet refresh returned error: %v\noutput:\n%s", err, output)
		}
		if strings.TrimSpace(output) != "" {
			t.Fatalf("expected quiet unchanged refresh to print no success output, got:\n%s", output)
		}
	})

	t.Run("force reports forced rewrite reason", func(t *testing.T) {
		root := initCommandGitWorktree(t)
		if output, err := executeSkillRegistryCommand("refresh", "--cwd", root); err != nil {
			t.Fatalf("seed refresh returned error: %v\noutput:\n%s", err, output)
		}

		output, err := executeSkillRegistryCommand("refresh", "--cwd", root, "--force")

		if err != nil {
			t.Fatalf("force refresh returned error: %v\noutput:\n%s", err, output)
		}
		for _, want := range []string{"changed: true", "reason: forced"} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected force output to contain %q, got:\n%s", want, output)
			}
		}
	})

	t.Run("prints concise non-fatal warnings", func(t *testing.T) {
		root := initCommandGitWorktree(t)
		legacyPath := filepath.Join(root, ".atl", "skill-registry.md")
		if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
			t.Fatalf("create legacy dir: %v", err)
		}
		if err := os.WriteFile(legacyPath, []byte("# Legacy\n\n## Custom Skills\n\n- **legacy-custom**\n"), 0644); err != nil {
			t.Fatalf("write legacy registry: %v", err)
		}

		output, err := executeSkillRegistryCommand("refresh", "--cwd", root)

		if err != nil {
			t.Fatalf("warning refresh returned error: %v\noutput:\n%s", err, output)
		}
		if !strings.Contains(output, "Warning:") || !strings.Contains(output, "legacy") {
			t.Fatalf("expected concise legacy warning output, got:\n%s", output)
		}
		assertCommandFileContains(t, filepath.Join(root, ".jarvis", "skill-registry.md"), "- **legacy-custom**")
	})
}

func executeSkillRegistryCommand(args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := &cobra.Command{Use: "jarvis"}
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.AddCommand(newSkillRegistryCmd())
	root.SetArgs(append([]string{"skill-registry"}, args...))
	err := root.Execute()
	return stdout.String() + stderr.String(), err
}

func initCommandGitWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(output))
	}
	return root
}

func assertCommandFileContains(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("expected %s to contain %q, got:\n%s", path, want, string(content))
	}
}

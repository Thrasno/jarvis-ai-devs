package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSkillRegistryRefreshCommand(t *testing.T) {
	t.Run("refreshes canonical registry from explicit subdirectory cwd", func(t *testing.T) {
		isolateTestHome(t)
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
		// skill-registry refresh indexes disk skills; it does NOT install skill copies.
		// Skill copies are installed by jarvis init, not by this command.
		for _, want := range []string{"Skill registry refreshed", "changed: true", "reason: created", "skills:"} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected output to contain %q, got:\n%s", want, output)
			}
		}
		assertCommandOutputPathSame(t, output, registryPath)
	})

	t.Run("defaults cwd to current working directory", func(t *testing.T) {
		isolateTestHome(t)
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
		assertCommandOutputPathSame(t, output, registryPath)
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
		isolateTestHome(t)
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

	t.Run("rejects no-gitignore compatibility flag", func(t *testing.T) {
		root := initCommandGitWorktree(t)

		output, err := executeSkillRegistryCommand("refresh", "--cwd", root, "--quiet", "--no-gitignore")

		if err == nil {
			t.Fatalf("expected --no-gitignore to be rejected, got nil error and output:\n%s", output)
		}
		if !strings.Contains(output+err.Error(), "unknown flag: --no-gitignore") {
			t.Fatalf("expected unknown flag error for --no-gitignore, got err=%v output:\n%s", err, output)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".jarvis", "skill-registry.md")); !os.IsNotExist(statErr) {
			t.Fatalf("expected rejected --no-gitignore refresh not to write registry, got stat err=%v", statErr)
		}
	})

	t.Run("force reports forced rewrite reason", func(t *testing.T) {
		isolateTestHome(t)
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
		isolateTestHome(t)
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

func assertCommandOutputPathSame(t *testing.T, output, wantPath string) {
	t.Helper()

	const prefix = "path: "
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		gotPath := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if commandPathsReferToSameFile(gotPath, wantPath) {
			return
		}
		t.Fatalf("expected output path %q to refer to same file as %q, got output:\n%s", gotPath, wantPath, output)
	}

	t.Fatalf("expected output to include %q line for %q, got:\n%s", prefix, wantPath, output)
}

func commandPathsReferToSameFile(a, b string) bool {
	if a == b {
		return true
	}

	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	if aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo) {
		return true
	}

	aClean := filepath.Clean(a)
	bClean := filepath.Clean(b)
	if runtime.GOOS == "windows" && strings.EqualFold(aClean, bClean) {
		return true
	}

	aEval, aErr := filepath.EvalSymlinks(aClean)
	bEval, bErr := filepath.EvalSymlinks(bClean)
	if aErr != nil || bErr != nil {
		return false
	}
	aEval = filepath.Clean(aEval)
	bEval = filepath.Clean(bEval)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aEval, bEval)
	}
	return aEval == bEval
}

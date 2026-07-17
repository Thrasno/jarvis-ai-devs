package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitPreservesFirstRunNonGitDirectory(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/nongit\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runInit(dir); err != nil {
			t.Fatalf("runInit returned error: %v", err)
		}
	})

	assertCommandFileContains(t, filepath.Join(dir, ".jarvis", "skill-registry.md"), "Canonical registry path: `.jarvis/skill-registry.md`")
	assertCommandFileContains(t, filepath.Join(dir, ".jarvis", "skills", "sdd-apply", "SKILL.md"), "sdd-apply")
	for _, want := range []string{"Scaffolding .jarvis", "Skill registry created: .jarvis/skill-registry.md", "Skills:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected init output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestRunInitNonGitRejectsSymlinkedJarvisBeforeWritingSkillCopies(t *testing.T) {
	isolateTestHome(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/nongit-symlink\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	externalJarvis := filepath.Join(t.TempDir(), "external-jarvis")
	if err := os.Mkdir(externalJarvis, 0755); err != nil {
		t.Fatalf("mkdir external jarvis: %v", err)
	}
	if err := os.Symlink(externalJarvis, filepath.Join(dir, ".jarvis")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := runInit(dir)

	if err == nil {
		t.Fatal("expected runInit to reject symlinked .jarvis")
	}
	if !strings.Contains(err.Error(), "install project skill copies") || !strings.Contains(err.Error(), "refusing to follow symlink") {
		t.Fatalf("expected symlink rejection while installing skill copies, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(externalJarvis, "skills")); !os.IsNotExist(err) {
		t.Fatalf("expected no skill copies written outside project through symlink, stat err: %v", err)
	}
}

func TestInitCmdRunEAllowsNonGitCurrentWorkingDirectory(t *testing.T) {
	isolateTestHome(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/initcmd-nongit\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to temp project: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	output := captureStdout(t, func() {
		if err := initCmd.RunE(initCmd, nil); err != nil {
			t.Fatalf("initCmd.RunE returned error: %v", err)
		}
	})

	assertCommandFileContains(t, filepath.Join(dir, ".jarvis", "skill-registry.md"), "Canonical registry path: `.jarvis/skill-registry.md`")
	if !strings.Contains(output, "✓ Skill registry created: .jarvis/skill-registry.md") {
		t.Fatalf("expected init command success output, got:\n%s", output)
	}
}

// TestRunInit_InstallsProjectSkillCopies verifies that runInit explicitly installs
// embedded skill copies under .jarvis/skills, confirming that install is init's
// responsibility (not Refresh's).
func TestRunInit_InstallsProjectSkillCopies(t *testing.T) {
	isolateTestHome(t)
	root := initCommandGitWorktree(t)

	captureStdout(t, func() {
		if err := runInit(root); err != nil {
			t.Fatalf("runInit returned error: %v", err)
		}
	})

	// Core skill copies must exist on disk after init.
	for _, id := range []string{"sdd-apply", "go-testing", "hive"} {
		skillPath := filepath.Join(root, ".jarvis", "skills", id, "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			t.Fatalf("expected skill copy at %s after runInit", skillPath)
		}
	}
	// Auxiliary skill file (strict-tdd.md) must also be installed.
	assertCommandFileContains(t, filepath.Join(root, ".jarvis", "skills", "sdd-apply", "strict-tdd.md"), "Strict TDD")
	// The registry must be written and reference the project path.
	assertCommandFileContains(t, filepath.Join(root, ".jarvis", "skill-registry.md"), "Canonical registry path:")
}

func TestRunInitUsesProjectRegistryRefreshFromGitSubdirectory(t *testing.T) {
	isolateTestHome(t)
	root := initCommandGitWorktree(t)
	subdir := filepath.Join(root, "nested", "module")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runInit(subdir); err != nil {
			t.Fatalf("runInit returned error: %v", err)
		}
	})

	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	assertCommandFileContains(t, registryPath, "Canonical registry path: `.jarvis/skill-registry.md`")
	assertCommandFileContains(t, filepath.Join(root, ".jarvis", "skills", "sdd-apply", "SKILL.md"), "sdd-apply")
	if _, err := os.Stat(filepath.Join(subdir, ".jarvis", "skill-registry.md")); !os.IsNotExist(err) {
		t.Fatalf("expected init from subdir not to write nested registry, got stat err=%v", err)
	}
	for _, want := range []string{"Scaffolding .jarvis", "Skill registry created: .jarvis/skill-registry.md", "Skills:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected init output to contain %q, got:\n%s", want, output)
		}
	}
}

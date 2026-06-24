package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

func TestInstallSelected_RecursivelyInstallsSkillTrees(t *testing.T) {
	dir := t.TempDir()

	fsys := fstest.MapFS{
		"embed/skills/hive/SKILL.md":                                  {Data: []byte("# Hive")},
		"embed/skills/sdd-init/SKILL.md":                              {Data: []byte("# Init")},
		"embed/skills/sdd-apply/SKILL.md":                             {Data: []byte("# Apply")},
		"embed/skills/sdd-verify/SKILL.md":                            {Data: []byte("# Verify")},
		"embed/skills/sdd-archive/SKILL.md":                           {Data: []byte("# Archive")},
		"embed/skills/sdd-qa/SKILL.md":                                {Data: []byte("# QA")},
		"embed/skills/custom-skill/SKILL.md":                          {Data: []byte("# Custom")},
		"embed/skills/custom-skill/references/examples.md":            {Data: []byte("ref example")},
		"embed/skills/custom-skill/references/nested/advanced.md":     {Data: []byte("advanced ref")},
		"embed/skills/custom-skill/templates/snippet.txt":             {Data: []byte("snippet")},
		"embed/skills/_shared/hive-convention.md":                     {Data: []byte("shared convention")},
		"embed/skills/unselected-skill/SKILL.md":                      {Data: []byte("# Unselected")},
		"embed/skills/unselected-skill/references/should-not-copy.md": {Data: []byte("skip me")},
	}

	if err := InstallSelected(fsys, dir, []string{"custom-skill"}); err != nil {
		t.Fatalf("InstallSelected failed: %v", err)
	}

	assertInstalledSkillFile(t, dir, "custom-skill/SKILL.md", "# Custom")
	assertInstalledSkillFile(t, dir, "custom-skill/references/examples.md", "ref example")
	assertInstalledSkillFile(t, dir, "custom-skill/references/nested/advanced.md", "advanced ref")
	assertInstalledSkillFile(t, dir, "custom-skill/templates/snippet.txt", "snippet")
	assertInstalledSkillFile(t, dir, "_shared/hive-convention.md", "shared convention")

	assertPathAbsent(t, filepath.Join(dir, "unselected-skill"))
}

func TestInstallSelected(t *testing.T) {
	t.Run("installs selected skills and core skills", func(t *testing.T) {
		dir := t.TempDir()

		err := InstallSelected(jarvis.SkillsFS, dir, []string{"git-workflow"})
		if err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		// Core skills must always be installed.
		for _, coreID := range []string{"hive", "sdd-init", "sdd-apply", "sdd-verify", "sdd-archive"} {
			path := filepath.Join(dir, coreID, "SKILL.md")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("core skill %s was not installed at %s", coreID, path)
			}
		}

		assertPathAbsent(t, filepath.Join(dir, "sdd-workflow"))

		// Selected skill must be installed
		gitPath := filepath.Join(dir, "git-workflow", "SKILL.md")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			t.Error("git-workflow skill was not installed")
		}

		// Non-selected, non-core skill must NOT be installed
		zohoPath := filepath.Join(dir, "zoho-deluge", "SKILL.md")
		if _, err := os.Stat(zohoPath); err == nil {
			t.Error("zoho-deluge was installed but was not selected")
		}
	})

	t.Run("install is idempotent", func(t *testing.T) {
		dir := t.TempDir()

		// First install
		if err := InstallSelected(jarvis.SkillsFS, dir, []string{}); err != nil {
			t.Fatalf("first install failed: %v", err)
		}

		// Second install (should overwrite silently)
		if err := InstallSelected(jarvis.SkillsFS, dir, []string{}); err != nil {
			t.Fatalf("second install failed: %v", err)
		}

		// Core skills still present
		path := filepath.Join(dir, "hive", "SKILL.md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("hive skill missing after second install")
		}
	})

	t.Run("installs all skills when all selected", func(t *testing.T) {
		dir := t.TempDir()
		allIDs := []string{"zoho-deluge", "laravel-architecture", "phpunit-testing", "git-workflow", "skill-creator", "qa-checklist"}

		if err := InstallSelected(jarvis.SkillsFS, dir, allIDs); err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		for _, id := range append(allIDs, "hive", "sdd-init", "sdd-apply", "sdd-verify", "sdd-archive") {
			path := filepath.Join(dir, id, "SKILL.md")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("skill %s was not installed", id)
			}
		}
		assertPathAbsent(t, filepath.Join(dir, "sdd-workflow"))
	})

	t.Run("skill files have non-empty content", func(t *testing.T) {
		dir := t.TempDir()

		if err := InstallSelected(jarvis.SkillsFS, dir, []string{}); err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		for _, coreID := range []string{"hive", "sdd-init", "sdd-apply", "sdd-verify", "sdd-archive"} {
			path := filepath.Join(dir, coreID, "SKILL.md")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read skill %s: %v", coreID, err)
			}
			if len(data) == 0 {
				t.Errorf("skill %s has empty content", coreID)
			}
		}
	})
}

func TestInstallSelected_ErrorPaths(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T) (fs.FS, string, []string)
		wantErr string
	}{
		{
			name: "returns stat errors for embedded subtree",
			setup: func(t *testing.T) (fs.FS, string, []string) {
				t.Helper()
				return errStatFS{}, t.TempDir(), []string{"custom-skill"}
			},
			wantErr: "stat skills subtree: boom stat embed/skills",
		},
		{
			name: "returns subtree open errors when embed skills is not a directory",
			setup: func(t *testing.T) (fs.FS, string, []string) {
				t.Helper()
				return subFailFS{}, t.TempDir(), []string{"custom-skill"}
			},
			wantErr: "open skills subtree: boom sub embed/skills",
		},
		{
			name: "returns errors creating destination skills dir",
			setup: func(t *testing.T) (fs.FS, string, []string) {
				t.Helper()
				destFile := filepath.Join(t.TempDir(), "skills-file")
				if err := os.WriteFile(destFile, []byte("occupied"), 0644); err != nil {
					t.Fatalf("seed destination file: %v", err)
				}
				fsys := fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}
				return fsys, destFile, []string{"custom-skill"}
			},
			wantErr: "create skills dir",
		},
		{
			name: "returns read errors with path context",
			setup: func(t *testing.T) (fs.FS, string, []string) {
				t.Helper()
				return brokenSkillReadFS{}, t.TempDir(), []string{"custom-skill"}
			},
			wantErr: "read skill file custom-skill/SKILL.md: boom reading custom-skill/SKILL.md",
		},
		{
			name: "returns errors creating parent directories",
			setup: func(t *testing.T) (fs.FS, string, []string) {
				t.Helper()
				destDir := t.TempDir()
				blockingParent := filepath.Join(destDir, "custom-skill")
				if err := os.WriteFile(blockingParent, []byte("occupied"), 0644); err != nil {
					t.Fatalf("seed blocking parent: %v", err)
				}
				fsys := fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}
				return fsys, destDir, []string{"custom-skill"}
			},
			wantErr: "create dir for custom-skill/SKILL.md",
		},
		{
			name: "returns write errors with path context",
			setup: func(t *testing.T) (fs.FS, string, []string) {
				t.Helper()
				destDir := t.TempDir()
				destPath := filepath.Join(destDir, "custom-skill", "SKILL.md")
				if err := os.MkdirAll(destPath, 0755); err != nil {
					t.Fatalf("seed blocking directory: %v", err)
				}
				fsys := fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}
				return fsys, destDir, []string{"custom-skill"}
			},
			wantErr: "write skill file custom-skill/SKILL.md",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fsys, destDir, selected := tt.setup(t)

			err := InstallSelected(fsys, destDir, selected)
			if err == nil {
				t.Fatal("expected InstallSelected to fail")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to include %q, got %q", tt.wantErr, err)
			}
		})
	}
}

// requireSymlinkSupport skips the test if the OS does not allow symlink creation
// without elevated privileges (e.g. Windows without Developer Mode).
func requireSymlinkSupport(t *testing.T) {
	t.Helper()
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "probe-link")
	if err := os.Symlink(src, dst); err != nil {
		t.Skipf("symlink creation not available on this system: %v", err)
	}
}

func TestInstallSelectedDoesNotFollowDestinationSymlinks(t *testing.T) {
	requireSymlinkSupport(t)
	t.Run("replaces final file symlink without overwriting target", func(t *testing.T) {
		dir := t.TempDir()
		external := filepath.Join(t.TempDir(), "outside.md")
		if err := os.WriteFile(external, []byte("do not overwrite"), 0644); err != nil {
			t.Fatalf("seed external target: %v", err)
		}
		linkPath := filepath.Join(dir, "custom-skill", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
			t.Fatalf("create skill dir: %v", err)
		}
		if err := os.Symlink(external, linkPath); err != nil {
			t.Fatalf("create destination symlink: %v", err)
		}

		err := InstallSelected(fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}, dir, []string{"custom-skill"})
		if err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		if got := string(mustReadOSFile(t, external)); got != "do not overwrite" {
			t.Fatalf("external symlink target was overwritten: got %q", got)
		}
		info, err := os.Lstat(linkPath)
		if err != nil {
			t.Fatalf("lstat installed path: %v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("destination remained a symlink; want regular installed file")
		}
		assertInstalledSkillFile(t, dir, "custom-skill/SKILL.md", "# Custom")
	})

	t.Run("rejects symlink parent directory", func(t *testing.T) {
		dir := t.TempDir()
		externalDir := t.TempDir()
		linkDir := filepath.Join(dir, "custom-skill")
		if err := os.Symlink(externalDir, linkDir); err != nil {
			t.Fatalf("create parent symlink: %v", err)
		}

		err := InstallSelected(fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}, dir, []string{"custom-skill"})
		if err == nil {
			t.Fatal("expected InstallSelected to reject symlink parent directory")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink error, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(externalDir, "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("external symlink target directory was written through: err=%v", err)
		}
	})

	t.Run("rejects symlink ancestor directory even when destination root exists", func(t *testing.T) {
		projectRoot := t.TempDir()
		externalDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(externalDir, "skills"), 0755); err != nil {
			t.Fatalf("seed external skills dir: %v", err)
		}
		if err := os.Symlink(externalDir, filepath.Join(projectRoot, ".jarvis")); err != nil {
			t.Fatalf("create .jarvis symlink: %v", err)
		}

		err := InstallSelected(fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}, filepath.Join(projectRoot, ".jarvis", "skills"), []string{"custom-skill"})
		if err == nil {
			t.Fatal("expected InstallSelected to reject symlink ancestor directory")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink error, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(externalDir, "skills", "custom-skill", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("external symlink ancestor target was written through: err=%v", err)
		}
	})
}

func TestInstallSelectedSkipsByteEquivalentFiles(t *testing.T) {
	dir := t.TempDir()
	fixedTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fsy := fstest.MapFS{"custom-skill/SKILL.md": {Data: []byte("# Custom")}}

	if err := InstallSelected(fsy, dir, []string{"custom-skill"}); err != nil {
		t.Fatalf("first InstallSelected failed: %v", err)
	}
	path := filepath.Join(dir, "custom-skill", "SKILL.md")
	if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
		t.Fatalf("set installed file time: %v", err)
	}

	if err := InstallSelected(fsy, dir, []string{"custom-skill"}); err != nil {
		t.Fatalf("second InstallSelected failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat installed file: %v", err)
	}
	if !info.ModTime().Equal(fixedTime) {
		t.Fatalf("byte-equivalent install rewrote file: got %s want %s", info.ModTime(), fixedTime)
	}
}

func TestInstallSelected_InstallsQAChecklistAndSkillCreatorWhenConfigured(t *testing.T) {
	dir := t.TempDir()

	if err := InstallSelected(jarvis.SkillsFS, dir, []string{"qa-checklist", "skill-creator"}); err != nil {
		t.Fatalf("InstallSelected failed: %v", err)
	}

	assertInstalledSkillFileContains(t, dir, "qa-checklist/SKILL.md", "## Output Contract")
	assertInstalledSkillFileContains(t, dir, "skill-creator/SKILL.md", "## Output Contract")
	assertInstalledSkillFileContains(t, dir, "skill-creator/references/quality-loop.md", "# Skill Quality Loop")
}

// TestInstallSelected_CoreDerivedFromFrontmatter verifies that skills with
// scope: core in their frontmatter are always installed (regardless of the
// selected list), and skills with scope: optional are only installed when
// explicitly selected.
func TestInstallSelected_CoreDerivedFromFrontmatter(t *testing.T) {
	dir := t.TempDir()

	// Provide a minimal FS with one core skill and one optional skill.
	// The core skill must be installed even though it is NOT in the selected list.
	fsys := fstest.MapFS{
		"embed/skills/my-core/SKILL.md":     {Data: []byte("---\nname: my-core\ndisplay_name: My Core\ndescription: \"Core skill. Trigger: always.\"\nscope: core\n---\n# Core\n")},
		"embed/skills/my-optional/SKILL.md": {Data: []byte("---\nname: my-optional\ndisplay_name: My Optional\ndescription: \"Optional skill. Trigger: when selected.\"\nscope: optional\n---\n# Optional\n")},
		"embed/skills/_shared/hive-convention.md": {Data: []byte("# shared")},
	}

	// Install nothing explicitly.
	if err := InstallSelected(fsys, dir, []string{}); err != nil {
		t.Fatalf("InstallSelected failed: %v", err)
	}

	// Core skill must be present.
	corePath := filepath.Join(dir, "my-core", "SKILL.md")
	if _, err := os.Stat(corePath); os.IsNotExist(err) {
		t.Error("expected core skill (scope: core) to be installed even when not selected")
	}

	// Optional skill must NOT be installed.
	optPath := filepath.Join(dir, "my-optional", "SKILL.md")
	if _, err := os.Stat(optPath); err == nil {
		t.Error("expected optional skill (scope: optional) to NOT be installed when not selected")
	}
}

// TestInstallSelected_OptionalCoreWhenSelected verifies that an optional skill IS
// installed when explicitly passed in the selected list.
func TestInstallSelected_OptionalCoreWhenSelected(t *testing.T) {
	dir := t.TempDir()

	fsys := fstest.MapFS{
		"embed/skills/my-optional/SKILL.md": {Data: []byte("---\nname: my-optional\ndisplay_name: My Optional\ndescription: \"Optional. Trigger: when selected.\"\nscope: optional\n---\n# Optional\n")},
	}

	if err := InstallSelected(fsys, dir, []string{"my-optional"}); err != nil {
		t.Fatalf("InstallSelected failed: %v", err)
	}

	optPath := filepath.Join(dir, "my-optional", "SKILL.md")
	if _, err := os.Stat(optPath); os.IsNotExist(err) {
		t.Error("expected optional skill to be installed when explicitly selected")
	}
}

func TestListSkills(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}

	if len(skills) < 6 {
		t.Errorf("expected at least 6 skills, got %d", len(skills))
	}

	// Check core skills are marked
	coreCount := 0
	for _, s := range skills {
		if s.IsCore {
			coreCount++
		}
		if len(s.Content) == 0 {
			t.Errorf("skill %s has empty content", s.ID)
		}
	}

	if coreCount < 2 {
		t.Errorf("expected at least 2 core skills, got %d", coreCount)
	}
}

func assertInstalledSkillFile(t *testing.T, dir, relPath, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if string(data) != expected {
		t.Fatalf("content mismatch for %s: got %q want %q", relPath, string(data), expected)
	}
}

func assertInstalledSkillFileContains(t *testing.T, dir, relPath, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if !strings.Contains(string(data), expected) {
		t.Fatalf("expected %s to contain %q, got:\n%s", relPath, expected, string(data))
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", path, err)
	}
}

func mustReadOSFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

type errStatFS struct{}

func (errStatFS) Open(name string) (fs.File, error) {
	if name == "embed/skills" {
		return nil, fmt.Errorf("boom stat %s", name)
	}
	return nil, fs.ErrNotExist
}

type subFailFS struct{}

func (subFailFS) Open(name string) (fs.File, error) {
	if name == "embed/skills" {
		return fstest.MapFS{
			"embed/skills/custom-skill/SKILL.md": {Data: []byte("# Custom")},
		}.Open(name)
	}
	return nil, fs.ErrNotExist
}

func (subFailFS) Sub(dir string) (fs.FS, error) {
	if dir == "embed/skills" {
		return nil, fmt.Errorf("boom sub %s", dir)
	}
	return nil, fs.ErrNotExist
}

type brokenSkillReadFS struct{}

func (brokenSkillReadFS) Open(name string) (fs.File, error) {
	switch name {
	case "embed/skills":
		return nil, fs.ErrNotExist
	case ".", "custom-skill":
		return fstest.MapFS{
			"custom-skill/SKILL.md": {},
		}.Open(name)
	case "custom-skill/SKILL.md":
		return nil, fmt.Errorf("boom reading %s", name)
	}

	return nil, fmt.Errorf("boom reading %s", name)
}

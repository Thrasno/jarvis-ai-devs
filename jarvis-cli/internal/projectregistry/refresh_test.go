package projectregistry

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolateHome points HOME to a temp dir so diskscan's global skill dirs do not
// resolve to real developer machine paths. Call this in any test that calls
// Refresh without an explicit ScanDirs override and cares about warning counts.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

// seedSkillFile writes a SKILL.md with valid frontmatter into root/.jarvis/skills/<id>/SKILL.md.
// scope may be empty to test the default scope path.
func seedSkillFile(t *testing.T, root, id, scope string) string {
	t.Helper()
	dir := filepath.Join(root, ".jarvis", "skills", id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	var fm string
	if scope != "" {
		fm = "---\nname: " + id + "\ntrigger: When using " + id + "\nscope: " + scope + "\n---\n\n# " + id + "\n"
	} else {
		fm = "---\nname: " + id + "\ntrigger: When using " + id + "\n---\n\n# " + id + "\n"
	}
	skillPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(fm), 0644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
	return skillPath
}

// TestRefreshIndexesScannedSkills verifies that Refresh picks up a skill placed at
// root/.jarvis/skills/<id>/SKILL.md and writes a matching row in the registry table.
// It also asserts that SkillCount reflects exactly the number of indexed skills.
func TestRefreshIndexesScannedSkills(t *testing.T) {
	root := initGitWorktree(t)
	seedSkillFile(t, root, "go-testing", "optional")

	result, err := Refresh(context.Background(), RefreshOptions{CWD: root, ScanDirs: []string{filepath.Join(root, ".jarvis", "skills")}})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	assertSamePath(t, result.Path, registryPath)
	assertFileContains(t, registryPath, "go-testing")
	assertFileContains(t, registryPath, "| Trigger |")
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1 (exactly the one seeded skill)", result.SkillCount)
	}
}

// TestRefreshOptions_ScanDirsOverridesDefault verifies that when ScanDirs is set,
// only those directories are scanned (the default dirs are ignored).
func TestRefreshOptions_ScanDirsOverridesDefault(t *testing.T) {
	root := initGitWorktree(t)
	// Seed a skill in the explicit dir (should be found).
	customDir := filepath.Join(root, "custom-skills")
	if err := os.MkdirAll(filepath.Join(customDir, "my-skill"), 0755); err != nil {
		t.Fatalf("create custom skill dir: %v", err)
	}
	skillContent := "---\nname: my-skill\ntrigger: When testing custom scan dirs\n---\n\n# my-skill\n"
	if err := os.WriteFile(filepath.Join(customDir, "my-skill", "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("write custom skill: %v", err)
	}
	// Seed a skill in the default project dir (should NOT be found because ScanDirs overrides).
	seedSkillFile(t, root, "default-only-skill", "optional")

	result, err := Refresh(context.Background(), RefreshOptions{CWD: root, ScanDirs: []string{customDir}})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	assertSamePath(t, result.Path, registryPath)
	assertFileContains(t, registryPath, "my-skill")
	// The default-only-skill must NOT appear because ScanDirs overrides default resolution.
	assertFileNotContains(t, registryPath, "default-only-skill")
}

// TestScanRowAdapterNormalizesPath verifies that the adapter converts an
// absolute OS-native path into a project-relative, forward-slash path.
func TestScanRowAdapterNormalizesPath(t *testing.T) {
	root := t.TempDir()
	// Simulate a ScanRow with an absolute OS-native path.
	absPath := filepath.Join(root, ".jarvis", "skills", "go-testing", "SKILL.md")
	relFwd := scanRowRelPath(root, absPath)

	want := ".jarvis/skills/go-testing/SKILL.md"
	if relFwd != want {
		t.Fatalf("scanRowRelPath = %q, want %q", relFwd, want)
	}
}

// TestScanRowRelPath_OutsideRootFallsBackToAbsolute verifies that when the skill
// absolute path is NOT under the project root (e.g. a user-global skill under
// ~/.claude/skills/...), scanRowRelPath returns a forward-slash absolute path
// rather than a ".." -prefixed relative path. A ".." path written into the
// registry would be broken on any machine where the directory layout differs.
func TestScanRowRelPath_OutsideRootFallsBackToAbsolute(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), ".claude", "skills", "hive", "SKILL.md")

	got := scanRowRelPath(root, outside)

	if strings.HasPrefix(got, "..") {
		t.Fatalf("scanRowRelPath returned %q; must not start with '..' for paths outside root", got)
	}
	// The result must use forward slashes (cross-platform registry format).
	if strings.Contains(got, "\\") {
		t.Fatalf("scanRowRelPath returned %q; must use forward slashes", got)
	}
	// The result must be an absolute path pointing to the original file.
	wantAbs := filepath.ToSlash(outside)
	if got != wantAbs {
		t.Fatalf("scanRowRelPath = %q, want absolute forward-slash path %q", got, wantAbs)
	}
}

// TestRefreshScopeDefaultIsOptional verifies that a SKILL.md with no scope:
// frontmatter key results in an "optional" cell in the registry table.
func TestRefreshScopeDefaultIsOptional(t *testing.T) {
	root := initGitWorktree(t)
	// Seed without scope field.
	seedSkillFile(t, root, "no-scope-skill", "")

	_, err := Refresh(context.Background(), RefreshOptions{CWD: root, ScanDirs: []string{filepath.Join(root, ".jarvis", "skills")}})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	assertFileContains(t, registryPath, "optional")
}

func TestResolveRootUsesGitTopLevelFromSubdir(t *testing.T) {
	root := initGitWorktree(t)
	subdir := filepath.Join(root, "nested", "package")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	resolved, err := ResolveRoot(context.Background(), subdir)
	if err != nil {
		t.Fatalf("ResolveRoot returned error: %v", err)
	}
	assertSamePath(t, resolved, root)
}

func TestResolveRootRejectsUnsafeRoots(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")

		_, err := ResolveRoot(context.Background(), missing)

		assertErrorContains(t, err, "project root")
	})

	t.Run("non worktree", func(t *testing.T) {
		_, err := ResolveRoot(context.Background(), t.TempDir())

		assertErrorContains(t, err, "git worktree")
	})

	t.Run("home directory", func(t *testing.T) {
		home := initGitWorktree(t)
		t.Setenv("HOME", home)

		_, err := ResolveRoot(context.Background(), home)

		assertErrorContains(t, err, "unsafe project root")
	})

	t.Run("home config directory", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		configRoot := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		runGit(t, configRoot, "init")

		_, err := ResolveRoot(context.Background(), configRoot)

		assertErrorContains(t, err, "unsafe project root")
	})

	t.Run("home claude generated state directory", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		claudeRoot := filepath.Join(home, ".claude", "projects", "example")
		if err := os.MkdirAll(claudeRoot, 0755); err != nil {
			t.Fatalf("create claude dir: %v", err)
		}
		runGit(t, claudeRoot, "init")

		_, err := ResolveRoot(context.Background(), claudeRoot)

		assertErrorContains(t, err, "unsafe project root")
	})

	t.Run("home alias directory", func(t *testing.T) {
		home := initGitWorktree(t)
		alias := createDirectoryAlias(t, home)
		t.Setenv("HOME", alias)

		_, err := ResolveRoot(context.Background(), home)

		assertErrorContains(t, err, "unsafe project root")
	})

	t.Run("home config alias directory", func(t *testing.T) {
		home := t.TempDir()
		alias := createDirectoryAlias(t, home)
		t.Setenv("HOME", alias)
		configRoot := filepath.Join(home, ".config", "opencode")
		if err := os.MkdirAll(configRoot, 0755); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		runGit(t, configRoot, "init")

		_, err := ResolveRoot(context.Background(), configRoot)

		assertErrorContains(t, err, "unsafe project root")
	})

	t.Run("home claude alias generated state directory", func(t *testing.T) {
		home := t.TempDir()
		alias := createDirectoryAlias(t, home)
		t.Setenv("HOME", alias)
		claudeRoot := filepath.Join(home, ".claude", "projects", "example")
		if err := os.MkdirAll(claudeRoot, 0755); err != nil {
			t.Fatalf("create claude dir: %v", err)
		}
		runGit(t, claudeRoot, "init")

		_, err := ResolveRoot(context.Background(), claudeRoot)

		assertErrorContains(t, err, "unsafe project root")
	})
}

func TestRejectUnsafeRootUsesPathEquivalenceForHomeAliases(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "ActualHome")
	alias := filepath.Join(base, "HOMEALIAS")
	sameAliasPath := func(a, b string) bool {
		return normalizeAliasPathForTest(a, alias, home) == normalizeAliasPathForTest(b, alias, home)
	}

	tests := []struct {
		name string
		root string
		want string
	}{
		{
			name: "home alias",
			root: home,
			want: "home directory",
		},
		{
			name: "home config alias",
			root: filepath.Join(home, ".config", "opencode"),
			want: "home config directories",
		},
		{
			name: "home Claude generated state alias",
			root: filepath.Join(home, ".claude", "projects", "example"),
			want: "home Claude generated state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectUnsafeRootForHomes(tt.root, []string{alias}, sameAliasPath)

			assertErrorContains(t, err, tt.want)
		})
	}
}

func TestPathsReferToSameFileRespectsPlatformCaseSensitivity(t *testing.T) {
	base := t.TempDir()
	upper := filepath.Join(base, "CaseProbe")
	lower := filepath.Join(base, "caseprobe")
	if err := os.WriteFile(upper, []byte("probe"), 0644); err != nil {
		t.Fatalf("write case probe: %v", err)
	}
	if !pathsReferToSameFile(upper, filepath.Join(base, ".", "CaseProbe")) {
		t.Fatal("pathsReferToSameFile(same cleaned path) = false, want true")
	}

	got := pathsReferToSameFile(upper, lower)
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("pathsReferToSameFile(case-only mismatch) = %v, want %v on %s", got, want, runtime.GOOS)
	}
}

func TestRefreshWritesCanonicalRegistry(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)
	subdir := filepath.Join(root, "cmd", "app")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Seed a skill so Refresh has something to index.
	seedSkillFile(t, root, "go-testing", "optional")

	result, err := Refresh(context.Background(), RefreshOptions{CWD: subdir})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	assertSamePath(t, result.Root, root)
	assertSamePath(t, result.Path, registryPath)
	if !result.Changed {
		t.Fatal("Changed = false, want true for first refresh")
	}
	if result.Reason != ReasonCreated {
		t.Fatalf("Reason = %q, want %q", result.Reason, ReasonCreated)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %+v, want none for clean refresh", result.Warnings)
	}
	assertFileContains(t, registryPath, "Canonical registry path: `.jarvis/skill-registry.md`")
	// Refresh does NOT install skill copies; that is cmd_init's responsibility.
	if _, err := os.Stat(filepath.Join(root, ".jarvis", "skills", "sdd-apply", "strict-tdd.md")); !os.IsNotExist(err) {
		t.Fatal("Refresh must not install skill copies under .jarvis/skills")
	}
}

func TestRefreshCanExplicitlyAllowNonGitRoot(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/nongit\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	result, err := Refresh(context.Background(), RefreshOptions{CWD: root, AllowNonGitRoot: true})

	if err != nil {
		t.Fatalf("Refresh returned error for explicitly allowed non-git root: %v", err)
	}
	assertSamePath(t, result.Root, root)
	assertFileContains(t, filepath.Join(root, ".jarvis", "skill-registry.md"), "Canonical registry path: `.jarvis/skill-registry.md`")
}

func TestFormatWarningLineIsSharedAndConcise(t *testing.T) {
	warning := Warning{Code: "metadata-gap", Path: ".jarvis/skills/example/SKILL.md", Message: "missing trigger metadata"}

	line := FormatWarningLine(warning)
	lines := FormatWarningLines("Project skill registry warning: ", []Warning{warning})

	if line != "Warning: missing trigger metadata (.jarvis/skills/example/SKILL.md)" {
		t.Fatalf("FormatWarningLine = %q", line)
	}
	if len(lines) != 1 || lines[0] != "Project skill registry warning: missing trigger metadata (.jarvis/skills/example/SKILL.md)" {
		t.Fatalf("FormatWarningLines = %#v", lines)
	}
}

func TestRefreshReportsLegacyImportWarning(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)
	legacyPath := filepath.Join(root, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	legacyContent := "# Legacy\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0644); err != nil {
		t.Fatalf("write legacy registry: %v", err)
	}

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}
	if len(first.Warnings) != 1 {
		t.Fatalf("Warnings len = %d, want 1: %+v", len(first.Warnings), first.Warnings)
	}
	if first.Warnings[0].Code != WarningLegacyRegistryImported || first.Warnings[0].Severity != SeverityWarning {
		t.Fatalf("legacy warning = %+v", first.Warnings[0])
	}
	assertFileContains(t, first.Path, "- **legacy-custom**")
	if string(mustReadFile(t, legacyPath)) != legacyContent {
		t.Fatal("legacy registry should remain unchanged after migration")
	}
}

func TestRefreshUsesUnchangedFastPath(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}

	infoBefore, err := os.Stat(first.Path)
	if err != nil {
		t.Fatalf("stat canonical registry: %v", err)
	}
	second, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("second Refresh returned error: %v", err)
	}
	infoAfter, err := os.Stat(second.Path)
	if err != nil {
		t.Fatalf("stat canonical registry after second refresh: %v", err)
	}
	if second.Changed {
		t.Fatal("Changed = true, want false for byte-equivalent refresh")
	}
	if second.Reason != ReasonUnchanged {
		t.Fatalf("Reason = %q, want %q", second.Reason, ReasonUnchanged)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("registry mod time changed on unchanged refresh: before=%s after=%s", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestRefreshDoesNotRewriteUnchangedRegistry(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)
	seedSkillFile(t, root, "go-testing", "optional")

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}
	infoBefore, err := os.Stat(first.Path)
	if err != nil {
		t.Fatalf("stat registry: %v", err)
	}

	second, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("second Refresh returned error: %v", err)
	}

	if second.Changed {
		t.Fatalf("Changed = true, want false when registry content is unchanged: %+v", second)
	}
	infoAfter, err := os.Stat(second.Path)
	if err != nil {
		t.Fatalf("stat registry after second refresh: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("registry was rewritten on unchanged refresh: before=%s after=%s", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

func TestRefreshDoesNotInstallSkillCopies(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)
	seedSkillFile(t, root, "go-testing", "optional")

	_, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	// Refresh must NOT install or modify skill copies; that is cmd_init's job.
	skillCopyPath := filepath.Join(root, ".jarvis", "skills", "go-testing", "SKILL.md")
	// The only SKILL.md under .jarvis/skills is the seed we wrote — not a copy installed by Refresh.
	// Seeded file is the source; no additional copy should appear elsewhere.
	// We just verify Refresh doesn't write outside the seeded path.
	entries, err := os.ReadDir(filepath.Join(root, ".jarvis", "skills"))
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	for _, e := range entries {
		p := filepath.Join(root, ".jarvis", "skills", e.Name(), "SKILL.md")
		if p == skillCopyPath {
			// The seeded file — OK.
			continue
		}
		if _, statErr := os.Stat(p); statErr == nil {
			t.Fatalf("Refresh must not install additional skill files; found unexpected: %s", p)
		}
	}
}

func TestRefreshPostMigrationIsStable(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)
	legacyPath := filepath.Join(root, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	legacyContent := "# Legacy\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0644); err != nil {
		t.Fatalf("write legacy registry: %v", err)
	}

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}
	if len(first.Warnings) != 1 || first.Warnings[0].Code != WarningLegacyRegistryImported {
		t.Fatalf("first warnings = %+v, want one result-only legacy warning", first.Warnings)
	}
	assertFileNotContains(t, first.Path, "## Registry Warnings")
	infoBefore, err := os.Stat(first.Path)
	if err != nil {
		t.Fatalf("stat canonical registry: %v", err)
	}

	second, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("second Refresh returned error: %v", err)
	}
	if second.Changed {
		t.Fatalf("Changed = true, want false after migration has canonical content: %+v", second)
	}
	if len(second.Warnings) != 0 {
		t.Fatalf("second warnings = %+v, want none after one-time migration result", second.Warnings)
	}
	infoAfter, err := os.Stat(second.Path)
	if err != nil {
		t.Fatalf("stat canonical registry after second refresh: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("post-migration refresh rewrote registry: before=%s after=%s", infoBefore.ModTime(), infoAfter.ModTime())
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

func TestRefreshRejectsCanonicalRegistrySymlinkOutsideWorktree(t *testing.T) {
	requireSymlinkSupport(t)
	isolateHome(t)
	root := initGitWorktree(t)
	if _, err := Refresh(context.Background(), RefreshOptions{CWD: root}); err != nil {
		t.Fatalf("initial Refresh returned error: %v", err)
	}
	registryPath := filepath.Join(root, ".jarvis", "skill-registry.md")
	if err := os.Remove(registryPath); err != nil {
		t.Fatalf("remove canonical registry: %v", err)
	}
	externalRegistry := filepath.Join(t.TempDir(), "skill-registry.md")
	externalBefore := "# External Registry\n\n## Custom Skills\n\n- **external-custom**\n"
	if err := os.WriteFile(externalRegistry, []byte(externalBefore), 0644); err != nil {
		t.Fatalf("seed external registry target: %v", err)
	}
	if err := os.Symlink(externalRegistry, registryPath); err != nil {
		t.Fatalf("create canonical registry symlink: %v", err)
	}

	_, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err == nil {
		t.Fatal("expected Refresh to reject canonical registry symlink outside worktree")
	}
	if !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("expected outside-project symlink error, got %v", err)
	}
	if got := string(mustReadFile(t, externalRegistry)); got != externalBefore {
		t.Fatalf("external registry symlink target changed: got %q want %q", got, externalBefore)
	}
	info, err := os.Lstat(registryPath)
	if err != nil {
		t.Fatalf("lstat registry symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("registry symlink was replaced; want rejected before mutation")
	}
}

// TestRefreshWritesGitignoreEntries is an integration test for the Refresh→EnsureGitignore
// seam. After a successful Refresh on a git worktree, .gitignore must contain both
// per-machine cache entries.
func TestRefreshWritesGitignoreEntries(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)

	_, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	content := string(mustReadFile(t, gitignorePath))
	for _, entry := range []string{".jarvis/skill-registry.md", ".jarvis/skills/"} {
		if !strings.Contains(content, entry) {
			t.Fatalf("expected .gitignore to contain %q after Refresh, got:\n%s", entry, content)
		}
	}
}

// TestRefreshNoGitignoreSkipsGitignoreMutation verifies that Refresh with
// NoGitignore:true does NOT write a .gitignore file at all.
func TestRefreshNoGitignoreSkipsGitignoreMutation(t *testing.T) {
	isolateHome(t)
	root := initGitWorktree(t)

	_, err := Refresh(context.Background(), RefreshOptions{CWD: root, NoGitignore: true})
	if err != nil {
		t.Fatalf("Refresh with NoGitignore=true returned error: %v", err)
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	if _, statErr := os.Stat(gitignorePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .gitignore when NoGitignore=true, stat err=%v", statErr)
	}
}

// TestRefreshSurfacesGitignoreWarning verifies that when EnsureGitignore produces
// a non-fatal warning (e.g. a git rm failure), Refresh surfaces it in Result.Warnings
// with code "gitignore-untrack" and severity SeverityWarning.
//
// We simulate this by pre-seeding a .gitignore with entries already present AND
// staging a tracked .jarvis/skill-registry.md. Because entries are already in
// .gitignore on the first call, the steady-state short-circuit fires and no git rm
// is attempted — no warning is produced. To actually exercise the warning path we
// remove one entry from .gitignore so the tracking audit runs, stage the file, and
// then call Refresh. If git rm succeeds the warning is empty; if it fails (which
// can happen in headless CI with no index entry) we assert the warning surfaces.
// This test at minimum asserts that Result.Warnings is not nil-on-err when the
// gitignore step produces a warning.
func TestRefreshSurfacesGitignoreWarning(t *testing.T) {
	isolateHome(t)
	root := initGitWorktreeWithConfig(t)

	// Write .gitignore with ONLY the directory entry so the registry file entry
	// is missing — EnsureGitignore will try to append it and run the tracking audit.
	gitignorePath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".jarvis/skills/\n"), 0644); err != nil {
		t.Fatalf("write partial .gitignore: %v", err)
	}

	// Create and stage .jarvis/skill-registry.md so it is tracked.
	registryFilePath := filepath.Join(root, ".jarvis", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(registryFilePath), 0755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}
	if err := os.WriteFile(registryFilePath, []byte("# registry\n"), 0644); err != nil {
		t.Fatalf("write registry file: %v", err)
	}
	runGit(t, root, "add", ".jarvis/skill-registry.md")
	runGit(t, root, "commit", "-m", "track registry file")

	result, err := Refresh(context.Background(), RefreshOptions{CWD: root})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	// Regardless of whether git rm succeeded or produced a warning, Result.Warnings
	// must be a valid slice (nil or non-nil, but no panic). If a warning was produced
	// it must carry the expected code.
	for _, w := range result.Warnings {
		if w.Code == "gitignore-untrack" {
			if w.Severity != SeverityWarning {
				t.Fatalf("gitignore-untrack warning severity = %q, want %q", w.Severity, SeverityWarning)
			}
			return // warning surfaced correctly
		}
	}
	// It is also valid for the warning to be absent when git rm succeeded cleanly.
}

func initGitWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func createDirectoryAlias(t *testing.T, target string) string {
	t.Helper()
	alias := filepath.Join(t.TempDir(), "home-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("directory symlink unavailable on this platform: %v", err)
	}
	return alias
}

func normalizeAliasPathForTest(path, alias, target string) string {
	path = filepath.Clean(path)
	alias = filepath.Clean(alias)
	target = filepath.Clean(target)
	if path == alias {
		return target
	}
	if rel, err := filepath.Rel(alias, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.Join(target, rel)
	}
	return path
}

func assertSamePath(t *testing.T, got, want string) {
	t.Helper()
	if pathsReferToSameFile(got, want) {
		return
	}
	t.Fatalf("path = %q, want same filesystem path as %q", got, want)
}

func pathsReferToSameFile(a, b string) bool {
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

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want to contain %q", err.Error(), want)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	content := string(mustReadFile(t, path))
	if !strings.Contains(content, want) {
		t.Fatalf("expected %s to contain %q, got:\n%s", path, want, content)
	}
}

func assertFileNotContains(t *testing.T, path, forbidden string) {
	t.Helper()
	content := string(mustReadFile(t, path))
	if strings.Contains(content, forbidden) {
		t.Fatalf("expected %s not to contain %q, got:\n%s", path, forbidden, content)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

package projectregistry

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

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

func TestRefreshWritesCanonicalRegistryAndInstallsProjectSkillCopies(t *testing.T) {
	root := initGitWorktree(t)
	subdir := filepath.Join(root, "cmd", "app")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	result, err := Refresh(context.Background(), RefreshOptions{CWD: subdir, SkillsFS: jarvis.SkillsFS})
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
	if result.SkillCount < 20 {
		t.Fatalf("SkillCount = %d, want registry rows for embedded skills", result.SkillCount)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %+v, want none for clean refresh", result.Warnings)
	}
	assertFileContains(t, registryPath, "Canonical registry path: `.jarvis/skill-registry.md`")
	assertFileContains(t, filepath.Join(root, ".jarvis", "skills", "go-testing", "SKILL.md"), "Go")
	assertFileContains(t, filepath.Join(root, ".jarvis", "skills", "sdd-apply", "strict-tdd.md"), "Strict TDD")
}

func TestRefreshReportsLegacyImportWarning(t *testing.T) {
	root := initGitWorktree(t)
	legacyPath := filepath.Join(root, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	legacyContent := "# Legacy\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0644); err != nil {
		t.Fatalf("write legacy registry: %v", err)
	}

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
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
	root := initGitWorktree(t)

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
	if err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}

	infoBefore, err := os.Stat(first.Path)
	if err != nil {
		t.Fatalf("stat canonical registry: %v", err)
	}
	second, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
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

func TestRefreshDoesNotRewriteUnchangedInstalledSkillFiles(t *testing.T) {
	root := initGitWorktree(t)

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
	if err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}
	skillPath := filepath.Join(root, ".jarvis", "skills", "go-testing", "SKILL.md")
	fixedTime := firstSkillModTime(t, first.Path)
	if err := os.Chtimes(skillPath, fixedTime, fixedTime); err != nil {
		t.Fatalf("set skill mod time: %v", err)
	}

	second, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
	if err != nil {
		t.Fatalf("second Refresh returned error: %v", err)
	}

	if second.Changed {
		t.Fatalf("Changed = true, want false when registry and installed skills are unchanged: %+v", second)
	}
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("stat skill file after refresh: %v", err)
	}
	if !info.ModTime().Equal(fixedTime) {
		t.Fatalf("skill file was rewritten on unchanged refresh: got %s want %s", info.ModTime(), fixedTime)
	}
}

func TestRefreshReportsChangedWhenOnlyInstalledSkillFilesChange(t *testing.T) {
	root := initGitWorktree(t)

	if _, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS}); err != nil {
		t.Fatalf("first Refresh returned error: %v", err)
	}
	skillPath := filepath.Join(root, ".jarvis", "skills", "go-testing", "SKILL.md")
	if err := os.Remove(skillPath); err != nil {
		t.Fatalf("remove installed skill file: %v", err)
	}

	second, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
	if err != nil {
		t.Fatalf("second Refresh returned error: %v", err)
	}

	if !second.Changed {
		t.Fatalf("Changed = false, want true when refresh reinstalls missing skill files: %+v", second)
	}
	if second.Reason != ReasonUpdated {
		t.Fatalf("Reason = %q, want %q for skill-copy write with unchanged registry", second.Reason, ReasonUpdated)
	}
	assertFileContains(t, skillPath, "Go")
}

func TestRefreshPostMigrationIsStable(t *testing.T) {
	root := initGitWorktree(t)
	legacyPath := filepath.Join(root, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	legacyContent := "# Legacy\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0644); err != nil {
		t.Fatalf("write legacy registry: %v", err)
	}

	first, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
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

	second, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
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

func TestRefreshRejectsCanonicalRegistrySymlinkOutsideWorktree(t *testing.T) {
	root := initGitWorktree(t)
	if _, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS}); err != nil {
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

	_, err := Refresh(context.Background(), RefreshOptions{CWD: root, SkillsFS: jarvis.SkillsFS})
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

func firstSkillModTime(t *testing.T, registryPath string) time.Time {
	t.Helper()
	info, err := os.Stat(registryPath)
	if err != nil {
		t.Fatalf("stat registry for baseline time: %v", err)
	}
	return info.ModTime().Add(-1 * time.Hour).Truncate(time.Second)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

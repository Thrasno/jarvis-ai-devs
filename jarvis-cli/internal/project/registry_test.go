package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestWriteRegistry_FirstRun verifies that WriteRegistry creates .jarvis/skill-registry.md
// with both the Suggested Skills and Custom Skills sections on first run.
func TestWriteRegistry_FirstRun(t *testing.T) {
	dir := t.TempDir()
	richSkills := []RegistrySkill{
		{ID: "go-testing", Name: "Go Testing", Description: "Go testing patterns", Trigger: "When writing Go tests", Path: "go-testing/SKILL.md", CompactRules: "Run gofmt and targeted go test", IsCore: false},
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", CompactRules: "Search memory before recall", IsCore: true},
	}

	if err := WriteRegistry(dir, "my-project", richSkills, WriteRegistryOptions{}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"my-project",
		"Canonical registry path: `" + CanonicalRegistryPath + "`",
		"## Installed Skills",
		"| Trigger | Skill | Scope | Path |",
		"| When writing Go tests | Go Testing | optional | `.jarvis/skills/go-testing/SKILL.md` |",
		"| Using Hive memory | Hive Memory | core | `.jarvis/skills/hive/SKILL.md` |",
		"## Project Conventions",
		"- Generated sections are deterministic; customize only from `## Custom Skills` onward.",
		"## Custom Skills",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in registry content:\n%s", want, content)
		}
	}

	if strings.Index(content, "| When writing Go tests |") > strings.Index(content, "| Using Hive memory |") {
		t.Fatalf("expected installed skills to be sorted deterministically by skill ID, got:\n%s", content)
	}

	for _, forbidden := range []string{
		"**Stack**",
		"## Suggested Skills",
		"## Compact Rules",
		"| Skill | Trigger / Description | Scope | Path |",
		"| Go Testing | When writing Go tests — Go testing patterns | optional | `go-testing/SKILL.md` |",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("registry must use new header format, but found legacy content %q in:\n%s", forbidden, content)
		}
	}
}

// TestWriteRegistry_Idempotent verifies that re-running WriteRegistry replaces
// the Suggested Skills section but leaves the Custom Skills section intact.
func TestWriteRegistry_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First run: write initial registry.
	initialRich := []RegistrySkill{
		{ID: "go-testing", Name: "Go Testing", Description: "Go testing patterns", Trigger: "When writing Go tests", Path: "go-testing/SKILL.md", CompactRules: "Run gofmt and targeted go test"},
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", CompactRules: "Search memory before recall", IsCore: true},
	}
	if err := WriteRegistry(dir, "my-project", initialRich, WriteRegistryOptions{}); err != nil {
		t.Fatalf("first WriteRegistry: %v", err)
	}

	// Manually add a custom skill to simulate user edits.
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	existing, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	withCustom := string(existing) + "- **my-custom-skill**\n"
	if err := os.WriteFile(registryPath, []byte(withCustom), 0644); err != nil {
		t.Fatal(err)
	}

	// Second run: different set of skills (simulating Laravel stack), should update
	// installed skills but keep custom.
	updatedRich := []RegistrySkill{
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", CompactRules: "Search memory before recall", IsCore: true},
		{ID: "laravel-architecture", Name: "Laravel Architecture", Description: "Laravel conventions", Trigger: "When writing Laravel code", Path: "laravel-architecture/SKILL.md", CompactRules: "Keep controllers thin"},
		{ID: "phpunit-testing", Name: "PHPUnit Testing", Description: "PHPUnit patterns", Trigger: "When writing PHP tests", Path: "phpunit-testing/SKILL.md", CompactRules: "Use AAA structure"},
	}
	if err := WriteRegistry(dir, "my-project", updatedRich, WriteRegistryOptions{}); err != nil {
		t.Fatalf("second WriteRegistry: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	content := string(data)

	// Updated skills present.
	if !strings.Contains(content, "laravel-architecture") {
		t.Error("expected updated 'laravel-architecture' in installed skills")
	}
	customIdx := strings.Index(content, "## Custom Skills")
	if customIdx < 0 {
		t.Fatal("missing ## Custom Skills section")
	}
	installedSection := content[:customIdx]
	if strings.Contains(installedSection, "go-testing") {
		t.Error("go-testing should not appear after skill set change")
	}
	// Custom skill preserved.
	if !strings.Contains(content, "my-custom-skill") {
		t.Error("expected user-added 'my-custom-skill' to be preserved in Custom Skills")
	}
	if strings.Contains(content[:customIdx], "my-custom-skill") {
		t.Error("custom content should only be preserved from ## Custom Skills onward")
	}
}

// TestWriteRegistry_CustomAbsent verifies that when ## Custom Skills section is absent
// in an existing file, WriteRegistry appends the default empty custom section.
func TestWriteRegistry_CustomAbsent(t *testing.T) {
	dir := t.TempDir()

	// Write a registry without the Custom Skills section.
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		t.Fatal(err)
	}
	noCustom := "# Skill Registry — old-project\n\n## Suggested Skills\n\n- **hive**\n"
	if err := os.WriteFile(registryPath, []byte(noCustom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRegistry(dir, "old-project", nil, WriteRegistryOptions{}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	content := string(data)

	if !strings.Contains(content, "## Custom Skills") {
		t.Error("expected ## Custom Skills section to be appended when absent")
	}
}

func TestWriteRegistry_ImportsLegacyCustomSectionWhenCanonicalAbsent(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "# Legacy Registry\n\n## Suggested Skills\n\n- `old`\n\n## Custom Skills\n\n- **legacy-custom**: keep this user-authored skill\n"
	if err := os.WriteFile(legacyPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRegistry(dir, "legacy-project", nil, WriteRegistryOptions{}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	canonicalPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	data, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("expected canonical registry to be written: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "- **legacy-custom**: keep this user-authored skill") {
		t.Fatalf("expected legacy custom skills to be imported into canonical registry, got:\n%s", content)
	}
	if strings.Contains(content, "- `old`") {
		t.Fatalf("legacy generated suggestions must not be imported, got:\n%s", content)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy registry should remain read fallback only, not be removed or rewritten: %v", err)
	}
}

func TestWriteRegistry_WritesCanonicalOnlyWhenLegacyExists(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	legacyBefore := "# Legacy Registry\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacyBefore), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRegistry(dir, "canonical-project", []RegistrySkill{{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true}}, WriteRegistryOptions{}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".jarvis", "skill-registry.md")); err != nil {
		t.Fatalf("expected canonical registry to be written: %v", err)
	}
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy registry: %v", err)
	}
	if string(legacyAfter) != legacyBefore {
		t.Fatalf("legacy registry must remain read fallback only; got %q want %q", string(legacyAfter), legacyBefore)
	}
}

func TestWriteRegistry_CanonicalCustomSectionWinsOverLegacy(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "# Legacy Registry\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	canonicalPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(canonicalPath), 0755); err != nil {
		t.Fatal(err)
	}
	canonical := "# Canonical Registry\n\n## Custom Skills\n\n- **canonical-custom**\n"
	if err := os.WriteFile(canonicalPath, []byte(canonical), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRegistry(dir, "canonical-project", nil, WriteRegistryOptions{}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	data, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical registry: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "- **canonical-custom**") {
		t.Fatalf("expected canonical custom skills to be preserved, got:\n%s", content)
	}
	if strings.Contains(content, "- **legacy-custom**") {
		t.Fatalf("canonical registry must win over legacy custom skills, got:\n%s", content)
	}
}

func TestWriteRegistryWithResultReportsLegacyImportAndExplicitWarningSection(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, ".atl", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "# Legacy Registry\n\n## Custom Skills\n\n- **legacy-custom**\n"
	if err := os.WriteFile(legacyPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := WriteRegistryWithResult(dir, "warning-project", []RegistrySkill{
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true},
	}, WriteRegistryOptions{
		Warnings: []RegistryWarning{{Code: "metadata-gap", Severity: "warning", Path: ".jarvis/skills/example/SKILL.md", Message: "missing trigger metadata"}},
	})
	if err != nil {
		t.Fatalf("WriteRegistryWithResult: %v", err)
	}

	canonicalPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	if result.Path != canonicalPath {
		t.Fatalf("Path = %q, want %q", result.Path, canonicalPath)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true for first write")
	}
	if result.Reason != RegistryReasonCreated {
		t.Fatalf("Reason = %q, want %q", result.Reason, RegistryReasonCreated)
	}
	if result.SkillCount != 1 {
		t.Fatalf("SkillCount = %d, want 1", result.SkillCount)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("Warnings = %+v, want explicit warning plus legacy import warning", result.Warnings)
	}
	if result.Warnings[0].Code != RegistryWarningLegacyImported {
		t.Fatalf("first warning = %+v, want legacy import warning", result.Warnings[0])
	}

	content := string(mustReadRegistryFile(t, canonicalPath))
	for _, want := range []string{
		"- **legacy-custom**",
		"## Registry Warnings",
		"| metadata-gap | warning | `.jarvis/skills/example/SKILL.md` | missing trigger metadata |",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected registry to contain %q, got:\n%s", want, content)
		}
	}
	if strings.Contains(content, "legacy-registry-imported") {
		t.Fatalf("legacy import warning should be result-only to keep post-migration refresh stable, got:\n%s", content)
	}
}

func TestWriteRegistryWithResultUsesUnchangedFastPathUnlessForced(t *testing.T) {
	dir := t.TempDir()
	richSkills := []RegistrySkill{{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true}}

	first, err := WriteRegistryWithResult(dir, "fast-path-project", richSkills, WriteRegistryOptions{})
	if err != nil {
		t.Fatalf("first WriteRegistryWithResult: %v", err)
	}
	if first.Reason != RegistryReasonCreated || !first.Changed {
		t.Fatalf("first result = %+v, want created change", first)
	}
	infoBefore := mustStatRegistryFile(t, first.Path)

	second, err := WriteRegistryWithResult(dir, "fast-path-project", richSkills, WriteRegistryOptions{})
	if err != nil {
		t.Fatalf("second WriteRegistryWithResult: %v", err)
	}
	if second.Changed {
		t.Fatalf("second Changed = true, want unchanged fast path: %+v", second)
	}
	if second.Reason != RegistryReasonUnchanged {
		t.Fatalf("second Reason = %q, want %q", second.Reason, RegistryReasonUnchanged)
	}
	infoAfter := mustStatRegistryFile(t, second.Path)
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("unchanged fast path rewrote registry: before=%s after=%s", infoBefore.ModTime(), infoAfter.ModTime())
	}

	forced, err := WriteRegistryWithResult(dir, "fast-path-project", richSkills, WriteRegistryOptions{Force: true})
	if err != nil {
		t.Fatalf("forced WriteRegistryWithResult: %v", err)
	}
	if !forced.Changed {
		t.Fatal("forced Changed = false, want true")
	}
	if forced.Reason != RegistryReasonForced {
		t.Fatalf("forced Reason = %q, want %q", forced.Reason, RegistryReasonForced)
	}
}

func TestWriteRegistryWithResultRejectsSymlinkedJarvisAncestor(t *testing.T) {
	requireSymlinkSupport(t)
	dir := t.TempDir()
	externalDir := t.TempDir()
	if err := os.Symlink(externalDir, filepath.Join(dir, ".jarvis")); err != nil {
		t.Fatalf("create .jarvis symlink: %v", err)
	}

	_, err := WriteRegistryWithResult(dir, "symlink-project", []RegistrySkill{{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true}}, WriteRegistryOptions{})
	if err == nil {
		t.Fatal("expected WriteRegistryWithResult to reject symlinked .jarvis ancestor")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(externalDir, "skill-registry.md")); !os.IsNotExist(err) {
		t.Fatalf("external .jarvis symlink target was written through: err=%v", err)
	}
}

func TestWriteRegistryWithResultRejectsRegistrySymlinkOutsideProjectRoot(t *testing.T) {
	requireSymlinkSupport(t)
	dir := t.TempDir()
	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		t.Fatalf("create .jarvis dir: %v", err)
	}
	externalRegistry := filepath.Join(t.TempDir(), "skill-registry.md")
	externalBefore := "# External Registry\n\n## Custom Skills\n\n- **external-custom**\n"
	if err := os.WriteFile(externalRegistry, []byte(externalBefore), 0644); err != nil {
		t.Fatalf("seed external registry target: %v", err)
	}
	if err := os.Symlink(externalRegistry, registryPath); err != nil {
		t.Fatalf("create registry symlink: %v", err)
	}

	_, err := WriteRegistryWithResult(dir, "symlink-project", []RegistrySkill{{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true}}, WriteRegistryOptions{})
	if err == nil {
		t.Fatal("expected WriteRegistryWithResult to reject registry symlink outside project root")
	}
	if !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("expected outside-project symlink error, got %v", err)
	}
	if got := string(mustReadRegistryFile(t, externalRegistry)); got != externalBefore {
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

func TestCanonicalRegistryPaths_DualReadSingleWrite(t *testing.T) {
	paths := CanonicalRegistryPaths()

	if paths.WritePath != CanonicalRegistryPath {
		t.Fatalf("write path mismatch: got %q", paths.WritePath)
	}

	wantRead := []string{CanonicalRegistryPath, LegacyRegistryPathATL}
	if len(paths.ReadPaths) != len(wantRead) {
		t.Fatalf("read paths len mismatch: got %v want %v", paths.ReadPaths, wantRead)
	}
	for i := range wantRead {
		if paths.ReadPaths[i] != wantRead[i] {
			t.Fatalf("read path mismatch at %d: got %q want %q", i, paths.ReadPaths[i], wantRead[i])
		}
	}
}

func TestResolveRegistryReadPath_PrefersCanonicalFallsBackLegacy(t *testing.T) {
	t.Run("canonical exists", func(t *testing.T) {
		dir := t.TempDir()
		canonical := filepath.Join(dir, ".jarvis", "skill-registry.md")
		if err := os.MkdirAll(filepath.Dir(canonical), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(canonical, []byte("canonical"), 0644); err != nil {
			t.Fatal(err)
		}

		resolved, source, err := ResolveRegistryReadPath(dir)
		if err != nil {
			t.Fatalf("ResolveRegistryReadPath error = %v", err)
		}
		if resolved != canonical {
			t.Fatalf("resolved path mismatch: got %q want %q", resolved, canonical)
		}
		if source != RegistrySourceCanonical {
			t.Fatalf("source mismatch: got %q want %q", source, RegistrySourceCanonical)
		}
	})

	t.Run("legacy fallback", func(t *testing.T) {
		dir := t.TempDir()
		legacy := filepath.Join(dir, ".atl", "skill-registry.md")
		if err := os.MkdirAll(filepath.Dir(legacy), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, []byte("legacy"), 0644); err != nil {
			t.Fatal(err)
		}

		resolved, source, err := ResolveRegistryReadPath(dir)
		if err != nil {
			t.Fatalf("ResolveRegistryReadPath error = %v", err)
		}
		if resolved != legacy {
			t.Fatalf("resolved path mismatch: got %q want %q", resolved, legacy)
		}
		if source != RegistrySourceLegacy {
			t.Fatalf("source mismatch: got %q want %q", source, RegistrySourceLegacy)
		}
	})
}

// TestWriteRegistry_NewHeaderFormat verifies that WriteRegistry emits the new
// table header | Trigger | Skill | Scope | Path | and does not emit the old
// Stack block, Suggested Skills section, or Compact Rules section.
func TestWriteRegistry_NewHeaderFormat(t *testing.T) {
	dir := t.TempDir()
	richSkills := []RegistrySkill{
		{ID: "go-testing", Name: "Go Testing", Description: "Go testing patterns", Trigger: "When writing Go tests", Scope: "optional", Path: "go-testing/SKILL.md"},
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Scope: "core", Path: "hive/SKILL.md"},
	}

	if err := WriteRegistry(dir, "new-format-project", richSkills, WriteRegistryOptions{}); err != nil {
		t.Fatalf("WriteRegistry: %v", err)
	}

	registryPath := filepath.Join(dir, ".jarvis", "skill-registry.md")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	content := string(data)

	// New header must be present
	if !strings.Contains(content, "| Trigger | Skill | Scope | Path |") {
		t.Errorf("expected new table header '| Trigger | Skill | Scope | Path |' in registry, got:\n%s", content)
	}

	// Old Stack block must be absent
	if strings.Contains(content, "**Stack**") {
		t.Errorf("expected no '**Stack**' in registry, got:\n%s", content)
	}

	// Suggested Skills section must be absent
	if strings.Contains(content, "## Suggested Skills") {
		t.Errorf("expected no '## Suggested Skills' in registry, got:\n%s", content)
	}

	// Compact Rules section must be absent
	if strings.Contains(content, "## Compact Rules") {
		t.Errorf("expected no '## Compact Rules' in registry, got:\n%s", content)
	}
}

func mustReadRegistryFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func mustStatRegistryFile(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info
}

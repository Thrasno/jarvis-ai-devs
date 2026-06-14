package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteRegistry_FirstRun verifies that WriteRegistry creates .jarvis/skill-registry.md
// with both the Suggested Skills and Custom Skills sections on first run.
func TestWriteRegistry_FirstRun(t *testing.T) {
	dir := t.TempDir()
	skills := []string{"sdd-workflow", "hive", "go-testing"}
	richSkills := []RegistrySkill{
		{ID: "go-testing", Name: "Go Testing", Description: "Go testing patterns", Trigger: "When writing Go tests", Path: "go-testing/SKILL.md", CompactRules: "Run gofmt and targeted go test", IsCore: false},
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", CompactRules: "Search memory before recall", IsCore: true},
		{ID: "sdd-workflow", Name: "SDD Workflow", Description: "Spec-Driven Development lifecycle", Trigger: "SDD workflow phase", Path: "sdd-workflow/SKILL.md", CompactRules: "Follow proposal/spec/design/tasks/apply/verify/archive", IsCore: true},
	}

	if err := WriteRegistry(dir, "my-project", StackGo, skills, richSkills); err != nil {
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
		"Go",
		"Canonical registry path: `.jarvis/skill-registry.md`",
		"## Suggested Skills",
		"- `sdd-workflow`",
		"- `hive`",
		"- `go-testing`",
		"## Installed Skills",
		"| Skill | Trigger / Description | Scope | Path |",
		"| Go Testing | When writing Go tests — Go testing patterns | optional | `.jarvis/skills/go-testing/SKILL.md` |",
		"| Hive Memory | Using Hive memory — Persistent memory protocol | core | `.jarvis/skills/hive/SKILL.md` |",
		"## Compact Rules (Transitional Metadata)",
		"Compact rules are compatibility metadata; the skill index path rows above are the primary instruction contract.",
		"- **hive**: Search memory before recall",
		"## Project Conventions",
		"- Generated sections are deterministic; customize only from `## Custom Skills` onward.",
		"## Custom Skills",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in registry content:\n%s", want, content)
		}
	}

	if strings.Index(content, "| Go Testing |") > strings.Index(content, "| Hive Memory |") {
		t.Fatalf("expected installed skills to be sorted deterministically by skill ID, got:\n%s", content)
	}

	for _, forbidden := range []string{
		"| Skill | Trigger | Path | Type |",
		"| Go Testing | When writing Go tests — Go testing patterns | optional | `go-testing/SKILL.md` |",
		"| Go Testing | When writing Go tests | `go-testing/SKILL.md` | optional |",
		"## Compact Rules\n",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("registry must use index-first/path-first schema, but found legacy content %q in:\n%s", forbidden, content)
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
		{ID: "sdd-workflow", Name: "SDD Workflow", Description: "Spec-Driven Development lifecycle", Trigger: "SDD workflow phase", Path: "sdd-workflow/SKILL.md", CompactRules: "Follow the SDD cycle", IsCore: true},
	}
	if err := WriteRegistry(dir, "my-project", StackGo, []string{"sdd-workflow", "hive", "go-testing"}, initialRich); err != nil {
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

	// Second run: different stack (Laravel), should update Suggested but keep custom.
	updatedRich := []RegistrySkill{
		{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", CompactRules: "Search memory before recall", IsCore: true},
		{ID: "laravel-architecture", Name: "Laravel Architecture", Description: "Laravel conventions", Trigger: "When writing Laravel code", Path: "laravel-architecture/SKILL.md", CompactRules: "Keep controllers thin"},
		{ID: "phpunit-testing", Name: "PHPUnit Testing", Description: "PHPUnit patterns", Trigger: "When writing PHP tests", Path: "phpunit-testing/SKILL.md", CompactRules: "Use AAA structure"},
		{ID: "sdd-workflow", Name: "SDD Workflow", Description: "Spec-Driven Development lifecycle", Trigger: "SDD workflow phase", Path: "sdd-workflow/SKILL.md", CompactRules: "Follow the SDD cycle", IsCore: true},
	}
	if err := WriteRegistry(dir, "my-project", StackLaravel, []string{"sdd-workflow", "hive", "laravel-architecture", "phpunit-testing"}, updatedRich); err != nil {
		t.Fatalf("second WriteRegistry: %v", err)
	}

	data, _ := os.ReadFile(registryPath)
	content := string(data)

	// Updated suggested skills present.
	if !strings.Contains(content, "laravel-architecture") {
		t.Error("expected updated 'laravel-architecture' in Suggested Skills")
	}
	// Old stack-specific skill removed from suggestions.
	suggestedIdx := strings.Index(content, "## Suggested Skills")
	customIdx := strings.Index(content, "## Custom Skills")
	if suggestedIdx < 0 || customIdx < 0 {
		t.Fatal("missing expected sections")
	}
	suggestedSection := content[suggestedIdx:customIdx]
	if strings.Contains(suggestedSection, "go-testing") {
		t.Error("go-testing should not appear in Suggested Skills after stack change to Laravel")
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
	noCustom := "# Skill Registry — old-project\n\n## Suggested Skills\n\n- **sdd-workflow**\n"
	if err := os.WriteFile(registryPath, []byte(noCustom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRegistry(dir, "old-project", StackNode, []string{"sdd-workflow", "hive"}); err != nil {
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

	if err := WriteRegistry(dir, "legacy-project", StackGo, []string{"sdd-workflow", "hive", "go-testing"}); err != nil {
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

	if err := WriteRegistry(dir, "canonical-project", StackGo, []string{"sdd-workflow"}, []RegistrySkill{{ID: "sdd-workflow", Name: "SDD Workflow", Description: "Spec-Driven Development lifecycle", Trigger: "SDD workflow phase", Path: "sdd-workflow/SKILL.md", IsCore: true}}); err != nil {
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

	if err := WriteRegistry(dir, "canonical-project", StackGo, []string{"sdd-workflow", "hive", "go-testing"}); err != nil {
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

	result, err := WriteRegistryWithResult(dir, "warning-project", StackGo, []string{"sdd-workflow"}, []RegistrySkill{
		{ID: "sdd-workflow", Name: "SDD Workflow", Description: "Spec-Driven Development lifecycle", Trigger: "SDD workflow phase", Path: "sdd-workflow/SKILL.md", IsCore: true},
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

	first, err := WriteRegistryWithResult(dir, "fast-path-project", StackGo, []string{"hive"}, richSkills, WriteRegistryOptions{})
	if err != nil {
		t.Fatalf("first WriteRegistryWithResult: %v", err)
	}
	if first.Reason != RegistryReasonCreated || !first.Changed {
		t.Fatalf("first result = %+v, want created change", first)
	}
	infoBefore := mustStatRegistryFile(t, first.Path)

	second, err := WriteRegistryWithResult(dir, "fast-path-project", StackGo, []string{"hive"}, richSkills, WriteRegistryOptions{})
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

	forced, err := WriteRegistryWithResult(dir, "fast-path-project", StackGo, []string{"hive"}, richSkills, WriteRegistryOptions{Force: true})
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
	dir := t.TempDir()
	externalDir := t.TempDir()
	if err := os.Symlink(externalDir, filepath.Join(dir, ".jarvis")); err != nil {
		t.Fatalf("create .jarvis symlink: %v", err)
	}

	_, err := WriteRegistryWithResult(dir, "symlink-project", StackGo, []string{"hive"}, []RegistrySkill{{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true}}, WriteRegistryOptions{})
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

	_, err := WriteRegistryWithResult(dir, "symlink-project", StackGo, []string{"hive"}, []RegistrySkill{{ID: "hive", Name: "Hive Memory", Description: "Persistent memory protocol", Trigger: "Using Hive memory", Path: "hive/SKILL.md", IsCore: true}}, WriteRegistryOptions{})
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

	if paths.WritePath != ".jarvis/skill-registry.md" {
		t.Fatalf("write path mismatch: got %q", paths.WritePath)
	}

	wantRead := []string{".jarvis/skill-registry.md", ".atl/skill-registry.md"}
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

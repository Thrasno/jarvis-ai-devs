package agent

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestInstallSkillsFromFS_CreatesDirectoryStructure verifies that nested skill files
// are installed at the correct paths under the destination directory.
func TestInstallSkillsFromFS_CreatesDirectoryStructure(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"test-skill-a/SKILL.md": {Data: []byte("# Skill A")},
		"test-skill-b/SKILL.md": {Data: []byte("# Skill B")},
		"test-skill-b/extra.md": {Data: []byte("# Extra")},
	}

	err := installSkillsFromFS(dest, testFS, []string{"test-skill-a", "test-skill-b"})
	if err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	// test-skill-a/SKILL.md
	assertFileContent(t, filepath.Join(dest, "test-skill-a", "SKILL.md"), "# Skill A")
	// test-skill-b/SKILL.md
	assertFileContent(t, filepath.Join(dest, "test-skill-b", "SKILL.md"), "# Skill B")
	// test-skill-b/extra.md (supporting file, still installed)
	assertFileContent(t, filepath.Join(dest, "test-skill-b", "extra.md"), "# Extra")
}

// TestInstallSkillsFromFS_SharedFilesInstalled verifies that _shared/ files are
// always installed, even when "_shared" is not in the selected list.
func TestInstallSkillsFromFS_SharedFilesInstalled(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"test-skill/SKILL.md":    {Data: []byte("# Skill")},
		"_shared/shared-file.md": {Data: []byte("# Shared")},
	}

	// Note: "_shared" is NOT in the selected list.
	err := installSkillsFromFS(dest, testFS, []string{"test-skill"})
	if err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	// _shared/shared-file.md must be installed even though not in selected.
	assertFileContent(t, filepath.Join(dest, "_shared", "shared-file.md"), "# Shared")
	// test-skill/SKILL.md must also be installed.
	assertFileContent(t, filepath.Join(dest, "test-skill", "SKILL.md"), "# Skill")
}

// TestInstallSkillsFromFS_UnselectedSkillsSkipped verifies that skills not in
// the selected list are not installed.
func TestInstallSkillsFromFS_UnselectedSkillsSkipped(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"skill-a/SKILL.md": {Data: []byte("# Skill A")},
		"skill-b/SKILL.md": {Data: []byte("# Skill B")},
		"skill-c/SKILL.md": {Data: []byte("# Skill C")},
	}

	err := installSkillsFromFS(dest, testFS, []string{"skill-a", "skill-b"})
	if err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	// skill-a and skill-b must be installed.
	assertFileExists(t, filepath.Join(dest, "skill-a", "SKILL.md"))
	assertFileExists(t, filepath.Join(dest, "skill-b", "SKILL.md"))

	// skill-c must NOT be installed.
	if _, err := os.Stat(filepath.Join(dest, "skill-c")); !os.IsNotExist(err) {
		t.Errorf("expected skill-c directory to not exist, but it does (or stat error: %v)", err)
	}
}

// TestInstallSkillsFromFS_Idempotent verifies that calling installSkillsFromFS twice
// produces no error and does not duplicate or append file contents.
func TestInstallSkillsFromFS_Idempotent(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"my-skill/SKILL.md": {Data: []byte("# My Skill")},
	}

	// First call.
	if err := installSkillsFromFS(dest, testFS, []string{"my-skill"}); err != nil {
		t.Fatalf("first installSkillsFromFS: %v", err)
	}

	// Second call (idempotency check).
	if err := installSkillsFromFS(dest, testFS, []string{"my-skill"}); err != nil {
		t.Fatalf("second installSkillsFromFS: %v", err)
	}

	// Content must be exactly what was written, not appended.
	assertFileContent(t, filepath.Join(dest, "my-skill", "SKILL.md"), "# My Skill")
}

// assertFileContent reads the file at path and asserts its content equals expected.
func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("expected file at %s, got error: %v", path, err)
		return
	}
	if string(data) != expected {
		t.Errorf("file %s content mismatch:\n  got:  %q\n  want: %q", path, string(data), expected)
	}
}

// assertFileExists checks that a file exists at path.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s to exist, got: %v", path, err)
	}
}

// TestInstallOrchestrator_CreatesFile verifies that installOrchestrator creates
// the orchestrator file at the destination path with correct content.
func TestInstallOrchestrator_CreatesFile(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	testFS := fstest.MapFS{
		"embed/orchestrator/sdd-orchestrator.md": {Data: []byte("# SDD Orchestrator\nContent here")},
	}

	err := installOrchestrator(destFile, testFS)
	if err != nil {
		t.Fatalf("installOrchestrator: %v", err)
	}

	assertFileContent(t, destFile, "# SDD Orchestrator\nContent here")
}

// TestInstallOrchestrator_ReturnsErrorOnMissingFile verifies that installOrchestrator
// returns an error when the orchestrator file is missing from the embedded FS.
func TestInstallOrchestrator_ReturnsErrorOnMissingFile(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	testFS := fstest.MapFS{
		// Empty FS - no orchestrator file
	}

	err := installOrchestrator(destFile, testFS)
	if err == nil {
		t.Error("expected error when orchestrator file is missing, got nil")
	}
}

// TestInstallOrchestrator_Idempotent verifies that calling installOrchestrator twice
// produces no error and does not duplicate content.
func TestInstallOrchestrator_Idempotent(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	testFS := fstest.MapFS{
		"embed/orchestrator/sdd-orchestrator.md": {Data: []byte("# Orchestrator")},
	}

	// First call.
	if err := installOrchestrator(destFile, testFS); err != nil {
		t.Fatalf("first installOrchestrator: %v", err)
	}

	// Second call (idempotency check).
	if err := installOrchestrator(destFile, testFS); err != nil {
		t.Fatalf("second installOrchestrator: %v", err)
	}

	// Content must be exactly what was written, not appended.
	assertFileContent(t, destFile, "# Orchestrator")
}

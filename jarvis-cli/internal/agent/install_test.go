package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestInstallSkillsFromFS(t *testing.T) {
	testCases := []struct {
		name      string
		fsys      fs.FS
		selected  []string
		wantFiles map[string]string
		wantPaths []string
		absentDir string
		wantErr   string
	}{
		{
			name: "installs selected skills shared files and nested references",
			fsys: fstest.MapFS{
				"selected-skill/SKILL.md":                       {Data: []byte("# Skill")},
				"selected-skill/references/examples.md":         {Data: []byte("reference example")},
				"selected-skill/references/nested/deep-link.md": {Data: []byte("deep link")},
				"selected-skill/templates/snippet.txt":          {Data: []byte("snippet")},
				"other-skill/SKILL.md":                          {Data: []byte("# Other")},
				"_shared/hive-convention.md":                    {Data: []byte("shared convention")},
			},
			selected: []string{"selected-skill"},
			wantFiles: map[string]string{
				"selected-skill/SKILL.md":                       "# Skill",
				"selected-skill/references/examples.md":         "reference example",
				"selected-skill/references/nested/deep-link.md": "deep link",
				"selected-skill/templates/snippet.txt":          "snippet",
				"_shared/hive-convention.md":                    "shared convention",
			},
			wantPaths: []string{
				"_shared/hive-convention.md",
				"selected-skill/SKILL.md",
				"selected-skill/references/examples.md",
				"selected-skill/references/nested/deep-link.md",
				"selected-skill/templates/snippet.txt",
			},
			absentDir: "other-skill",
		},
		{
			name: "installs qa checklist and skill creator when selected",
			fsys: fstest.MapFS{
				"qa-checklist/SKILL.md":                          {Data: []byte("# QA Checklist")},
				"skill-creator/SKILL.md":                         {Data: []byte("# Skill Creator")},
				"skill-creator/references/quality-loop.md":       {Data: []byte("quality loop")},
				"unselected-skill/SKILL.md":                      {Data: []byte("# Other")},
				"unselected-skill/references/should-not-copy.md": {Data: []byte("skip")},
			},
			selected: []string{"qa-checklist", "skill-creator"},
			wantFiles: map[string]string{
				"qa-checklist/SKILL.md":                    "# QA Checklist",
				"skill-creator/SKILL.md":                   "# Skill Creator",
				"skill-creator/references/quality-loop.md": "quality loop",
			},
			wantPaths: []string{
				"qa-checklist/SKILL.md",
				"skill-creator/SKILL.md",
				"skill-creator/references/quality-loop.md",
			},
			absentDir: "unselected-skill",
		},
		{
			name:     "returns read errors with path context",
			fsys:     brokenReadFS{},
			selected: []string{"selected-skill"},
			wantErr:  "read skill file selected-skill/SKILL.md",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()

			err := installSkillsFromFS(dest, tt.fsys, tt.selected)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected installSkillsFromFS to fail")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error to include %q, got %q", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("installSkillsFromFS: %v", err)
			}

			assertInstalledFiles(t, dest, tt.wantFiles)
			assertInstalledRelativePaths(t, dest, tt.wantPaths)
			if tt.absentDir != "" {
				assertDirectoryAbsent(t, filepath.Join(dest, tt.absentDir))
			}
		})
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

type brokenReadFS struct{}

func (brokenReadFS) Open(name string) (fs.File, error) {
	switch name {
	case ".", "selected-skill":
		return fstest.MapFS{
			"selected-skill/SKILL.md": {},
		}.Open(name)
	case "selected-skill/SKILL.md":
		return nil, fmt.Errorf("boom reading %s", name)
	}

	return nil, fmt.Errorf("boom reading %s", name)
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

func assertInstalledFiles(t *testing.T, dest string, want map[string]string) {
	t.Helper()
	for relPath, content := range want {
		assertFileContent(t, filepath.Join(dest, filepath.FromSlash(relPath)), content)
	}
}

func assertDirectoryAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", path, err)
	}
}

func assertInstalledRelativePaths(t *testing.T, dest string, want []string) {
	t.Helper()

	var got []string
	err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		t.Fatalf("walk installed files: %v", err)
	}

	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("installed files mismatch\n got: %v\nwant: %v", got, want)
	}
}

// TestInstallOrchestrator_CreatesFile verifies that installOrchestrator creates
// the orchestrator file at the destination path with correct content.
func TestInstallOrchestrator_CreatesFile(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	err := installOrchestrator(destFile, []byte("# SDD Orchestrator\nContent here"))
	if err != nil {
		t.Fatalf("installOrchestrator: %v", err)
	}

	assertFileContent(t, destFile, "# SDD Orchestrator\nContent here")
}

// TestInstallOrchestrator_ReturnsErrorOnMissingFile verifies that installOrchestrator
// returns an error when the orchestrator file is missing from the embedded FS.
func TestInstallOrchestrator_ReturnsErrorOnMissingFile(t *testing.T) {
	t.Skip("file-read behavior moved to caller; installOrchestrator now writes provided rendered content")
}

// TestInstallOrchestrator_Idempotent verifies that calling installOrchestrator twice
// produces no error and does not duplicate content.
func TestInstallOrchestrator_Idempotent(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	// First call.
	if err := installOrchestrator(destFile, []byte("# Orchestrator")); err != nil {
		t.Fatalf("first installOrchestrator: %v", err)
	}

	// Second call (idempotency check).
	if err := installOrchestrator(destFile, []byte("# Orchestrator")); err != nil {
		t.Fatalf("second installOrchestrator: %v", err)
	}

	// Content must be exactly what was written, not appended.
	assertFileContent(t, destFile, "# Orchestrator")
}

func TestInstallOrchestrator_WritesRenderedContent(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	rendered := []byte("| sdd-apply | opus |\n")
	if err := installOrchestrator(destFile, rendered); err != nil {
		t.Fatalf("installOrchestrator: %v", err)
	}

	assertFileContent(t, destFile, "| sdd-apply | opus |\n")
}

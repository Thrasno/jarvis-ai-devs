package diskscan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// makeSkillMD writes a SKILL.md with frontmatter into dir/<skillID>/SKILL.md.
func makeSkillMD(t *testing.T, dir, skillID, name, trigger, scope string) {
	t.Helper()
	skillDir := filepath.Join(dir, skillID)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	var fm string
	fm += "---\n"
	if name != "" {
		fm += "name: " + name + "\n"
	}
	if trigger != "" {
		fm += "Trigger: " + trigger + "\n"
	}
	if scope != "" {
		fm += "scope: " + scope + "\n"
	}
	fm += "---\n\n# " + skillID + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(fm), 0o644); err != nil {
		t.Fatalf("write SKILL.md for %s: %v", skillID, err)
	}
}

func TestResolveScanDirs_Order(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	dirs := ResolveScanDirs(root)

	if len(dirs) != 3 {
		t.Fatalf("expected 3 scan dirs, got %d: %v", len(dirs), dirs)
	}

	wantProject := filepath.Join(root, ".jarvis", "skills")
	wantClaudeSkills := filepath.Join(home, ".claude", "skills")
	wantOpencode := filepath.Join(home, ".config", "opencode", "skills")

	if dirs[0] != wantProject {
		t.Errorf("dirs[0] = %q, want %q", dirs[0], wantProject)
	}
	if dirs[1] != wantClaudeSkills {
		t.Errorf("dirs[1] = %q, want %q", dirs[1], wantClaudeSkills)
	}
	if dirs[2] != wantOpencode {
		t.Errorf("dirs[2] = %q, want %q", dirs[2], wantOpencode)
	}
}

func TestResolveScanDirs_Absolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	dirs := ResolveScanDirs(root)
	for _, d := range dirs {
		if !filepath.IsAbs(d) {
			t.Errorf("expected absolute path, got %q", d)
		}
	}
}

func TestScan_BasicSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectSkills := t.TempDir()

	makeSkillMD(t, projectSkills, "my-skill", "My Skill", "When my skill is needed", "optional")

	rows, warns, err := Scan([]string{projectSkills})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "my-skill" {
		t.Errorf("ID = %q, want %q", rows[0].ID, "my-skill")
	}
	if rows[0].Name != "My Skill" {
		t.Errorf("Name = %q, want %q", rows[0].Name, "My Skill")
	}
	if rows[0].Trigger != "When my skill is needed" {
		t.Errorf("Trigger = %q, want %q", rows[0].Trigger, "When my skill is needed")
	}
	if rows[0].Scope != "optional" {
		t.Errorf("Scope = %q, want %q", rows[0].Scope, "optional")
	}
	_ = warns
}

func TestScan_SkipList(t *testing.T) {
	projectSkills := t.TempDir()

	// These should be skipped
	makeSkillMD(t, projectSkills, "sdd-apply", "SDD Apply", "When applying", "core")
	makeSkillMD(t, projectSkills, "sdd-verify", "SDD Verify", "When verifying", "core")
	makeSkillMD(t, projectSkills, "skill-registry", "Skill Registry", "When updating registry", "optional")
	makeSkillMD(t, projectSkills, "_shared", "Shared", "shared", "")

	// This should NOT be skipped
	makeSkillMD(t, projectSkills, "go-testing", "Go Testing", "When writing Go tests", "optional")

	rows, _, err := Scan([]string{projectSkills})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Exactly 1 skill must survive (go-testing). Guards against both over-skip and under-skip.
	if len(rows) != 1 {
		t.Errorf("expected exactly 1 row (go-testing), got %d: %+v", len(rows), rows)
	}

	for _, row := range rows {
		if row.ID == "sdd-apply" || row.ID == "sdd-verify" || row.ID == "skill-registry" || row.ID == "_shared" {
			t.Errorf("expected %q to be skipped", row.ID)
		}
	}

	found := false
	for _, row := range rows {
		if row.ID == "go-testing" {
			found = true
		}
	}
	if !found {
		t.Error("expected go-testing to be present in scan results")
	}
}

func TestScan_SkipList_AllSDDPrefix(t *testing.T) {
	projectSkills := t.TempDir()

	// All sdd-* should be skipped
	makeSkillMD(t, projectSkills, "sdd-explore", "SDD Explore", "When exploring", "optional")
	makeSkillMD(t, projectSkills, "sdd-propose", "SDD Propose", "When proposing", "optional")
	makeSkillMD(t, projectSkills, "sdd-spec", "SDD Spec", "When writing specs", "optional")

	// Non-sdd should be present
	makeSkillMD(t, projectSkills, "hive", "Hive Memory", "Using Hive memory", "core")

	rows, _, err := Scan([]string{projectSkills})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Exactly 1 skill must survive (hive). Guards against both over-skip and under-skip.
	if len(rows) != 1 {
		t.Errorf("expected exactly 1 row (hive), got %d: %+v", len(rows), rows)
	}

	for _, row := range rows {
		if row.ID == "sdd-explore" || row.ID == "sdd-propose" || row.ID == "sdd-spec" {
			t.Errorf("expected sdd-* %q to be skipped", row.ID)
		}
	}

	found := false
	for _, row := range rows {
		if row.ID == "hive" {
			found = true
		}
	}
	if !found {
		t.Error("expected hive to be present in scan results")
	}
}

func TestScan_DeduplicatesProjectOverGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectSkills := t.TempDir()
	globalSkills := t.TempDir()

	// Same skill ID in both dirs — project should win
	makeSkillMD(t, projectSkills, "my-skill", "Project My Skill", "Project trigger", "core")
	makeSkillMD(t, globalSkills, "my-skill", "Global My Skill", "Global trigger", "optional")

	rows, _, err := Scan([]string{projectSkills, globalSkills})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	count := 0
	for _, row := range rows {
		if row.ID == "my-skill" {
			count++
			if row.Name != "Project My Skill" {
				t.Errorf("expected project skill to win deduplication, got Name=%q", row.Name)
			}
			if row.Path != filepath.Join(projectSkills, "my-skill", "SKILL.md") {
				t.Errorf("expected project path to win, got %q", row.Path)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row for deduplicated skill, got %d", count)
	}
}

func TestScan_PathIsAbsolute(t *testing.T) {
	projectSkills := t.TempDir()
	makeSkillMD(t, projectSkills, "my-skill", "My Skill", "When needed", "optional")

	rows, _, err := Scan([]string{projectSkills})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one row")
	}
	for _, row := range rows {
		if !filepath.IsAbs(row.Path) {
			t.Errorf("expected absolute Path, got %q", row.Path)
		}
	}
}

func TestScan_EmptyDirsNoError(t *testing.T) {
	emptyDir := t.TempDir()
	rows, warns, err := Scan([]string{emptyDir})
	if err != nil {
		t.Fatalf("Scan on empty dir: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no rows for empty dir, got %d", len(rows))
	}
	_ = warns
}

func TestScan_NonExistentDirNoError(t *testing.T) {
	rows, warns, err := Scan([]string{"/no/such/dir/exists/here"})
	if err != nil {
		t.Fatalf("Scan on non-existent dir: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no rows, got %d", len(rows))
	}
	_ = warns
}

func TestResolveScanDirs_HomeUnresolved_NoRelativePaths(t *testing.T) {
	// Override the home lookup to simulate failure.
	orig := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home directory not available")
	}
	t.Cleanup(func() { userHomeDir = orig })

	root := t.TempDir()
	dirs := ResolveScanDirs(root)

	// Only the project-local dir must be present — no global dirs.
	if len(dirs) != 1 {
		t.Fatalf("expected exactly 1 dir when home is unresolved, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != filepath.Join(root, ".jarvis", "skills") {
		t.Errorf("dirs[0] = %q, want project .jarvis/skills dir", dirs[0])
	}
	// Extra guard: every returned path must be absolute (no relative paths emitted).
	for _, d := range dirs {
		if !filepath.IsAbs(d) {
			t.Errorf("non-absolute path emitted when home is unresolved: %q", d)
		}
	}
}

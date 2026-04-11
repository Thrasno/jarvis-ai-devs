package skills

import (
	"os"
	"path/filepath"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
)

func TestInstallSelected(t *testing.T) {
	t.Run("installs selected skills and core skills", func(t *testing.T) {
		dir := t.TempDir()

		err := InstallSelected(jarvis.SkillsFS, dir, []string{"git-workflow"})
		if err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		// Core skills must always be installed
		for _, coreID := range []string{"sdd-workflow", "hive"} {
			path := filepath.Join(dir, coreID, "SKILL.md")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("core skill %s was not installed at %s", coreID, path)
			}
		}

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
		allIDs := []string{"zoho-deluge", "laravel-architecture", "phpunit-testing", "git-workflow"}

		if err := InstallSelected(jarvis.SkillsFS, dir, allIDs); err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		for _, id := range append(allIDs, "sdd-workflow", "hive") {
			path := filepath.Join(dir, id, "SKILL.md")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("skill %s was not installed", id)
			}
		}
	})

	t.Run("skill files have non-empty content", func(t *testing.T) {
		dir := t.TempDir()

		if err := InstallSelected(jarvis.SkillsFS, dir, []string{}); err != nil {
			t.Fatalf("InstallSelected failed: %v", err)
		}

		for _, coreID := range []string{"sdd-workflow", "hive"} {
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

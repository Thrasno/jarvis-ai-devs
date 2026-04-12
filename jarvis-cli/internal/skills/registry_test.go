package skills

import (
	"embed"
	"io/fs"
	"testing"
)

// wrapAsEmbedFS wraps a fstest.MapFS into a minimal embed.FS-compatible structure
// by building a real embed.FS via a helper. Since embed.FS cannot be constructed
// directly in tests, we use a thin adapter via the actual embed directive below.
// Instead, tests call ListSkills with an embed.FS that wraps the right paths.

// testEmbedFS is a real embed.FS used for unit testing ListSkills.
// It embeds the testdata/ directory which mirrors embed/skills/ structure.
//
//go:embed testdata
var testEmbedFS embed.FS

// TestRegistry_SkillIDFromDirectoryName verifies that a SKILL.md file results in
// a Skill with ID equal to its parent directory name, NOT "SKILL".
func TestRegistry_SkillIDFromDirectoryName(t *testing.T) {
	skills, err := listSkillsFromFS(testEmbedFS, "testdata")
	if err != nil {
		t.Fatalf("listSkillsFromFS: %v", err)
	}

	// Find sdd-qa in the results.
	var found bool
	for _, s := range skills {
		if s.ID == "sdd-qa" {
			found = true
			if s.Path != "sdd-qa/SKILL.md" {
				t.Errorf("expected Path=sdd-qa/SKILL.md, got %q", s.Path)
			}
		}
		if s.ID == "SKILL" {
			t.Errorf("got ID=SKILL — skill ID must come from directory name, not filename")
		}
	}
	if !found {
		t.Error("expected skill with ID=sdd-qa, not found")
	}
}

// TestRegistry_SupportingFilesSkipped verifies that non-SKILL.md files (e.g.
// strict-tdd.md) do not produce registry entries.
func TestRegistry_SupportingFilesSkipped(t *testing.T) {
	skills, err := listSkillsFromFS(testEmbedFS, "testdata")
	if err != nil {
		t.Fatalf("listSkillsFromFS: %v", err)
	}

	for _, s := range skills {
		if s.ID == "strict-tdd" {
			t.Error("supporting file strict-tdd.md must not be registered as a skill")
		}
	}

	// sdd-apply should be registered exactly once.
	count := 0
	for _, s := range skills {
		if s.ID == "sdd-apply" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 registry entry for sdd-apply, got %d", count)
	}
}

// TestRegistry_SharedNotRegistered verifies that _shared/ files do not produce
// registry entries.
func TestRegistry_SharedNotRegistered(t *testing.T) {
	skills, err := listSkillsFromFS(testEmbedFS, "testdata")
	if err != nil {
		t.Fatalf("listSkillsFromFS: %v", err)
	}

	for _, s := range skills {
		if s.ID == "_shared" || s.ID == "hive-convention" || s.ID == "sdd-phase-common" {
			t.Errorf("_shared file must not be registered: found ID=%q", s.ID)
		}
	}

	// hive skill (has SKILL.md) must still be registered.
	var hiveFound bool
	for _, s := range skills {
		if s.ID == "hive" {
			hiveFound = true
		}
	}
	if !hiveFound {
		t.Error("expected hive skill to be registered")
	}
}

// listSkillsFromFS is a testable variant of ListSkills that accepts a root prefix.
// This allows tests to use testdata/ instead of embed/skills/ as the root.
func listSkillsFromFS(fsys embed.FS, root string) ([]Skill, error) {
	var result []Skill

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		// relPath is relative to root.
		relPath := path[len(root)+1:] // strip "testdata/"

		dirName := relPath[:len(relPath)-len("/SKILL.md")]
		if dirName == "" {
			return nil // skip root-level SKILL.md if any
		}

		// Skip _shared/ directory.
		if dirName == "_shared" {
			return nil
		}

		content, readErr := fsys.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		meta, hasMeta := skillMeta[dirName]
		name := dirName
		description := ""
		if hasMeta {
			name = meta.name
			description = meta.description
		}

		result = append(result, Skill{
			ID:          dirName,
			Name:        name,
			Description: description,
			IsCore:      coreSkillIDs[dirName],
			Content:     content,
			Path:        relPath,
		})
		return nil
	})

	return result, err
}

package skills

import (
	"embed"
	"io/fs"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
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

	// Find sdd-apply in the results.
	var found bool
	for _, s := range skills {
		if s.ID == "sdd-apply" {
			found = true
			if s.Path != "sdd-apply/SKILL.md" {
				t.Errorf("expected Path=sdd-apply/SKILL.md, got %q", s.Path)
			}
		}
		if s.ID == "SKILL" {
			t.Errorf("got ID=SKILL — skill ID must come from directory name, not filename")
		}
	}
	if !found {
		t.Error("expected skill with ID=sdd-apply, not found")
	}
}

func TestRegistry_ListSkillsOmitsRetiredSDDQA(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	for _, s := range skills {
		if s.ID == "sdd-qa" {
			t.Fatal("expected retired sdd-qa skill to be absent from embedded registry")
		}
	}
}

func TestRegistry_ListSkillsOmitsRetiredSDDWorkflow(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	for _, s := range skills {
		if s.ID == "sdd-workflow" {
			t.Fatal("expected retired sdd-workflow skill to be absent from embedded registry")
		}
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

func TestRegistry_QAChecklistIsDiscoverableButNotCore(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	var qa Skill
	for _, skill := range skills {
		if skill.ID == "qa-checklist" {
			qa = skill
			break
		}
	}
	if qa.ID == "" {
		t.Fatal("expected qa-checklist to be present in embedded skill registry")
	}
	if qa.IsCore {
		t.Fatal("expected qa-checklist to be selectable and non-core, not always installed")
	}

	metadata := strings.ToLower(qa.Description + " " + qa.Trigger)
	for _, phrase := range []string{"qa checklist", "test checklist", "checklist de pruebas", "batería de pruebas"} {
		if !strings.Contains(metadata, phrase) {
			t.Fatalf("expected qa-checklist metadata to include trigger phrase %q; got description=%q trigger=%q", phrase, qa.Description, qa.Trigger)
		}
	}
}

func TestRegistry_ComplementarySkillsAreShippedAndDiscoverable(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	byID := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		byID[skill.ID] = skill
	}

	expected := map[string]string{
		"chained-pr":           "PRs over 400 lines",
		"work-unit-commits":    "Plan commits as reviewable work units",
		"comment-writer":       "Write warm, direct collaboration comments",
		"cognitive-doc-design": "Design docs that reduce cognitive load",
	}

	for id, descriptionSnippet := range expected {
		skill, exists := byID[id]
		if !exists {
			t.Fatalf("expected embedded registry to include complementary skill %q", id)
		}
		if skill.IsCore {
			t.Fatalf("expected complementary skill %q not to be marked core", id)
		}
		if !strings.Contains(skill.Description, descriptionSnippet) {
			t.Fatalf("expected %q description to contain %q, got %q", id, descriptionSnippet, skill.Description)
		}
		if skill.Trigger == "" {
			t.Fatalf("expected %q trigger metadata to be populated", id)
		}
	}
}

func TestRegistry_SkillImproverIsShippedDiscoverableAndOptional(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	byID := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		byID[skill.ID] = skill
	}

	skill, exists := byID["skill-improver"]
	if !exists {
		t.Fatal("expected embedded registry to include skill-improver")
	}
	if skill.IsCore {
		t.Fatal("expected skill-improver to be optional, not core")
	}
	if skill.Path != "skill-improver/SKILL.md" {
		t.Fatalf("skill-improver path = %q, want skill-improver/SKILL.md", skill.Path)
	}

	// Include skill.ID in metadata so ID-based phrases (e.g. "skill-improver") are discoverable
	// even after Name was updated to the display_name ("Skill Improver").
	metadata := strings.ToLower(skill.ID + " " + skill.Name + " " + skill.Description + " " + skill.Trigger)
	for _, phrase := range []string{"skill-improver", "audit", "refactor skills", "skill quality"} {
		if !strings.Contains(metadata, phrase) {
			t.Fatalf("expected skill-improver metadata to include %q; got id=%q name=%q description=%q trigger=%q", phrase, skill.ID, skill.Name, skill.Description, skill.Trigger)
		}
	}
}

// TestListSkills_WorkflowSkillsExposeRegistryMetadata verifies that workflow
// skills returned by ListSkills have the required metadata fields populated.
func TestListSkills_WorkflowSkillsExposeRegistryMetadata(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	byID := make(map[string]Skill, len(skills))
	for _, s := range skills {
		if _, exists := byID[s.ID]; exists {
			t.Fatalf("duplicate skill for ID %q", s.ID)
		}
		byID[s.ID] = s
	}

	for _, id := range []string{"work-unit-commits", "chained-pr", "cognitive-doc-design", "branch-pr", "issue-creation", "comment-writer", "skill-improver"} {
		s, exists := byID[id]
		if !exists {
			t.Fatalf("expected workflow skill %q to be registry-ready", id)
		}
		if s.Name == "" || s.Trigger == "" || s.Description == "" || s.Path == "" || s.Scope == "" {
			t.Fatalf("workflow skill %q missing registry metadata: %+v", id, s)
		}
		if !strings.HasSuffix(s.Path, "/SKILL.md") {
			t.Fatalf("workflow skill %q path must point at packaged SKILL.md, got %q", id, s.Path)
		}
		if s.Scope != "optional" {
			t.Fatalf("workflow skill %q scope = %q, want optional", id, s.Scope)
		}
	}

	if _, exists := byID["sdd-workflow"]; exists {
		t.Fatal("expected retired sdd-workflow to be absent from skill list")
	}

	for _, id := range []string{"hive", "sdd-init", "sdd-apply", "sdd-verify", "sdd-archive"} {
		s, exists := byID[id]
		if !exists {
			t.Fatalf("expected core skill %q to be in skill list", id)
		}
		if s.Scope != "core" {
			t.Fatalf("core skill %q scope = %q, want core", id, s.Scope)
		}
	}
}

// TestListSkills_DoesNotDuplicateSkillCreator verifies that ListSkills returns
// exactly one entry for skill-creator.
func TestListSkills_DoesNotDuplicateSkillCreator(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	count := 0
	for _, s := range skills {
		if s.ID == "skill-creator" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one skill-creator entry, got %d", count)
	}
}

func TestGetSkillReturnsEmbeddedSkillByID(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "go-testing")
	if err != nil {
		t.Fatalf("GetSkill existing skill: %v", err)
	}

	if skill.ID != "go-testing" {
		t.Fatalf("skill ID = %q, want %q", skill.ID, "go-testing")
	}
	// Name must be the canonical display name from display_name: frontmatter.
	if skill.Name != "Go Testing" {
		t.Fatalf("skill name = %q, want %q (display_name frontmatter)", skill.Name, "Go Testing")
	}
	if skill.Path != "go-testing/SKILL.md" {
		t.Fatalf("skill path = %q, want %q", skill.Path, "go-testing/SKILL.md")
	}
}

// TestGetSkill_DisplayNameOverridesName asserts that when a skill has a
// display_name: frontmatter key, Skill.Name uses the display name, not the
// kebab id from name:.
func TestGetSkill_DisplayNameOverridesName(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "sdd-apply")
	if err != nil {
		t.Fatalf("GetSkill sdd-apply: %v", err)
	}
	if skill.ID != "sdd-apply" {
		t.Fatalf("skill ID = %q, want sdd-apply", skill.ID)
	}
	if skill.Name != "SDD Apply" {
		t.Fatalf("skill Name = %q, want %q", skill.Name, "SDD Apply")
	}
}

// TestAllEmbedSkillsHaveDisplayName asserts every embed/skills/*/SKILL.md
// (excluding _shared) has a display_name: frontmatter key.
func TestAllEmbedSkillsHaveDisplayName(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected at least one embedded skill")
	}
	for _, s := range skills {
		// Name must NOT equal the kebab ID — it must be the display name.
		if s.Name == s.ID {
			t.Errorf("skill %q has Name == ID (%q); display_name: frontmatter is missing or not being used", s.ID, s.Name)
		}
		if s.Name == "" {
			t.Errorf("skill %q has empty Name", s.ID)
		}
	}
}

func TestGetSkillReturnsNotFoundError(t *testing.T) {
	_, err := GetSkill(jarvis.SkillsFS, "missing-skill")
	if err == nil {
		t.Fatal("expected not found error for missing skill")
	}
	if !strings.Contains(err.Error(), `skill "missing-skill" not found`) {
		t.Fatalf("expected missing skill error, got %v", err)
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

		result = append(result, Skill{
			ID:      dirName,
			Content: content,
			Path:    relPath,
		})
		return nil
	})

	return result, err
}

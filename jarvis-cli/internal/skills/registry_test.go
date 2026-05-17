package skills

import (
	"embed"
	"io/fs"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
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

	if _, exists := coreSkillIDs["sdd-qa"]; exists {
		t.Fatal("expected retired sdd-qa skill to be removed from core skills")
	}
	if _, exists := skillMeta["sdd-qa"]; exists {
		t.Fatal("expected retired sdd-qa skill to be removed from registry metadata")
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

// TestRegistry_SkillMetaCount verifies that skillMeta contains entries for all
// expected shipped skills after the retired sdd-qa removal.
func TestRegistry_SkillMetaCount(t *testing.T) {
	expectedSkills := []string{
		// SDD/Hive
		"sdd-workflow", "hive", "sdd-explore", "sdd-propose", "sdd-spec",
		"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify",
		"sdd-archive", "sdd-init",
		// Complementary upstream workflow helpers
		"chained-pr", "work-unit-commits", "comment-writer", "cognitive-doc-design",
		// Domain-specific
		"zoho-deluge", "laravel-architecture", "phpunit-testing", "git-workflow",
		// Workflow + product helpers already shipped
		"branch-pr", "issue-creation", "go-testing", "judgment-day",
		"sdd-onboard", "skill-creator", "skill-registry", "qa-checklist",
	}

	if len(skillMeta) != len(expectedSkills) {
		t.Errorf("expected %d skills in skillMeta, got %d", len(expectedSkills), len(skillMeta))
	}

	for _, id := range expectedSkills {
		meta, exists := skillMeta[id]
		if !exists {
			t.Errorf("expected skill %q in skillMeta, not found", id)
			continue
		}
		if meta.name == "" {
			t.Errorf("skill %q has empty name", id)
		}
		if meta.description == "" {
			t.Errorf("skill %q has empty description", id)
		}
		if meta.trigger == "" {
			t.Errorf("skill %q has empty trigger", id)
		}
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

func TestRegistryRows_WorkflowSkillsExposeRegistryMetadata(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	rows := RegistryRows(skills)
	byID := make(map[string]RegistryRow, len(rows))
	for _, row := range rows {
		if _, exists := byID[row.ID]; exists {
			t.Fatalf("duplicate registry row for skill %q", row.ID)
		}
		byID[row.ID] = row
	}

	for _, id := range []string{"work-unit-commits", "chained-pr", "cognitive-doc-design", "branch-pr", "issue-creation", "comment-writer"} {
		row, exists := byID[id]
		if !exists {
			t.Fatalf("expected workflow skill %q to be registry-ready", id)
		}
		if row.Name == "" || row.Trigger == "" || row.Path == "" || row.CompactRules == "" {
			t.Fatalf("workflow skill %q missing registry metadata: %+v", id, row)
		}
		if !strings.HasSuffix(row.Path, "/SKILL.md") {
			t.Fatalf("workflow skill %q path must point at packaged SKILL.md, got %q", id, row.Path)
		}
	}
}

func TestRegistryRows_WorkflowSkillsExposeActionableCompactRules(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	rows := RegistryRows(skills)
	byID := make(map[string]RegistryRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	wantPhrases := map[string][]string{
		"work-unit-commits":    {"Plan commits", "reviewable work units", "keep tests and docs with code"},
		"chained-pr":           {"Split work over 400 lines", "stacked PRs", "review slices"},
		"cognitive-doc-design": {"Structure docs", "reader cognitive load", "audience"},
		"branch-pr":            {"Check for an issue first", "review-focused PR", "clean diff"},
		"issue-creation":       {"Search existing issues first", "acceptance criteria", "clear scope"},
		"comment-writer":       {"warm and direct", "state the decision", "actionable next step"},
	}

	for id, phrases := range wantPhrases {
		row, exists := byID[id]
		if !exists {
			t.Fatalf("expected workflow skill %q to be registry-ready", id)
		}
		if len(row.CompactRules) < 90 {
			t.Fatalf("compact rules for %q are too thin: %q", id, row.CompactRules)
		}
		for _, phrase := range phrases {
			if !strings.Contains(row.CompactRules, phrase) {
				t.Fatalf("compact rules for %q must contain actionable phrase %q, got %q", id, phrase, row.CompactRules)
			}
		}
	}
}

func TestRegistryRows_DoesNotDuplicateSkillCreator(t *testing.T) {
	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}

	rows := RegistryRows(skills)
	count := 0
	for _, row := range rows {
		if row.ID == "skill-creator" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one skill-creator registry row, got %d", count)
	}
}

func TestRegistryRows_UsesDeterministicFallbackCompactRules(t *testing.T) {
	rows := RegistryRows([]Skill{
		{ID: "unknown-with-trigger", Name: "Unknown With Trigger", Trigger: "When a custom topic appears", Path: "unknown-with-trigger/SKILL.md"},
		{ID: "unknown-without-trigger", Name: "Unknown Without Trigger", Path: "unknown-without-trigger/SKILL.md"},
	})

	if len(rows) != 2 {
		t.Fatalf("expected two registry rows, got %d", len(rows))
	}

	byID := make(map[string]RegistryRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	if got, want := byID["unknown-with-trigger"].CompactRules, "Load when: When a custom topic appears."; got != want {
		t.Fatalf("trigger fallback compact rule = %q, want %q", got, want)
	}
	if got, want := byID["unknown-without-trigger"].CompactRules, "Read this skill when its topic matches the current task."; got != want {
		t.Fatalf("default fallback compact rule = %q, want %q", got, want)
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
	if skill.Name != "Go Testing" {
		t.Fatalf("skill name = %q, want %q", skill.Name, "Go Testing")
	}
	if skill.Path != "go-testing/SKILL.md" {
		t.Fatalf("skill path = %q, want %q", skill.Path, "go-testing/SKILL.md")
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

// TestRegistry_TriggerFieldPopulated verifies that Trigger field is populated
// for skills with metadata.
func TestRegistry_TriggerFieldPopulated(t *testing.T) {
	skills, err := listSkillsFromFS(testEmbedFS, "testdata")
	if err != nil {
		t.Fatalf("listSkillsFromFS: %v", err)
	}

	for _, s := range skills {
		// If skill has metadata, Trigger must be populated.
		if _, hasMeta := skillMeta[s.ID]; hasMeta {
			if s.Trigger == "" {
				t.Errorf("skill %q has metadata but empty Trigger field", s.ID)
			}
		}
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
		trigger := ""
		if hasMeta {
			name = meta.name
			description = meta.description
			trigger = meta.trigger
		}

		result = append(result, Skill{
			ID:          dirName,
			Name:        name,
			Description: description,
			Trigger:     trigger,
			IsCore:      coreSkillIDs[dirName],
			Content:     content,
			Path:        relPath,
		})
		return nil
	})

	return result, err
}

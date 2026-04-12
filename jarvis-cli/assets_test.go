package jarvis_test

import (
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
)

// TestPersonaFS_NotEmpty verifies that the embedded persona assets are present.
// This acts as a build-time guard: if someone moves or deletes embed/personas,
// this test fails loudly instead of silently producing an empty wizard step.
func TestPersonaFS_NotEmpty(t *testing.T) {
	entries, err := jarvis.PersonaFS.ReadDir("embed/personas")
	if err != nil {
		t.Fatalf("embed/personas not readable: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embed/personas is empty — persona assets are missing")
	}
	t.Logf("found %d persona assets", len(entries))
}

// TestSkillsFS_NotEmpty verifies that the embedded skill assets are present.
func TestSkillsFS_NotEmpty(t *testing.T) {
	entries, err := jarvis.SkillsFS.ReadDir("embed/skills")
	if err != nil {
		t.Fatalf("embed/skills not readable: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embed/skills is empty — skill assets are missing")
	}
	t.Logf("found %d skill assets", len(entries))
}

// TestTemplatesFS_NotEmpty verifies that the embedded template assets are present.
func TestTemplatesFS_NotEmpty(t *testing.T) {
	entries, err := jarvis.TemplatesFS.ReadDir("embed/templates")
	if err != nil {
		t.Fatalf("embed/templates not readable: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embed/templates is empty — template assets are missing")
	}
	t.Logf("found %d template assets", len(entries))
}

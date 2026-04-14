package config

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestRenderCLAUDEMd_SkillsSectionRendered verifies that the Skills section
// is rendered with all provided skills when calling RenderCLAUDEMd.
func TestRenderCLAUDEMd_SkillsSectionRendered(t *testing.T) {
	testFS := fstest.MapFS{
		"embed/templates/CLAUDE.md.tmpl": {Data: []byte(`# AI Agent

## Skills (Auto-load based on context)

{{range .Skills}}- **{{.Name}}**: {{.Description}}. Trigger: {{.Trigger}}
{{end}}
`)},
	}

	skills := []SkillInfo{
		{Name: "Go Testing", Description: "Go testing patterns", Trigger: "When writing Go tests"},
		{Name: "SDD Apply", Description: "Implement tasks", Trigger: "When implementing tasks"},
	}

	result, err := RenderCLAUDEMd(testFS, "layer1", "layer2", "expertise", skills)
	if err != nil {
		t.Fatalf("RenderCLAUDEMd: %v", err)
	}

	// Verify Skills section header exists.
	if !strings.Contains(result, "## Skills (Auto-load based on context)") {
		t.Error("expected Skills section header, not found")
	}

	// Verify first skill rendered.
	if !strings.Contains(result, "- **Go Testing**: Go testing patterns. Trigger: When writing Go tests") {
		t.Error("expected Go Testing skill, not found")
	}

	// Verify second skill rendered.
	if !strings.Contains(result, "- **SDD Apply**: Implement tasks. Trigger: When implementing tasks") {
		t.Error("expected SDD Apply skill, not found")
	}
}

// TestRenderCLAUDEMd_EmptySkillsList verifies that rendering works when Skills is an empty slice.
func TestRenderCLAUDEMd_EmptySkillsList(t *testing.T) {
	testFS := fstest.MapFS{
		"embed/templates/CLAUDE.md.tmpl": {Data: []byte(`# AI Agent

## Skills (Auto-load based on context)

{{range .Skills}}- **{{.Name}}**: {{.Description}}. Trigger: {{.Trigger}}
{{end}}
`)},
	}

	result, err := RenderCLAUDEMd(testFS, "layer1", "layer2", "expertise", nil)
	if err != nil {
		t.Fatalf("RenderCLAUDEMd with nil skills: %v", err)
	}

	// Skills header should still exist.
	if !strings.Contains(result, "## Skills (Auto-load based on context)") {
		t.Error("expected Skills section header even with empty skills list")
	}

	// No skill entries should be present.
	if strings.Contains(result, "- **") {
		t.Error("expected no skill entries when skills list is empty")
	}
}

// TestRenderAGENTSMd_SkillsSectionRendered verifies that the Skills section
// is rendered with all provided skills when calling RenderAGENTSMd.
func TestRenderAGENTSMd_SkillsSectionRendered(t *testing.T) {
	testFS := fstest.MapFS{
		"embed/templates/AGENTS.md.tmpl": {Data: []byte(`# Agents

## Skills (Auto-load based on context)

{{range .Skills}}- **{{.Name}}**: {{.Description}}. Trigger: {{.Trigger}}
{{end}}
`)},
	}

	skills := []SkillInfo{
		{Name: "Judgment Day", Description: "Dual review protocol", Trigger: "When user says judgment day"},
	}

	result, err := RenderAGENTSMd(testFS, "layer1", "layer2", "expertise", skills)
	if err != nil {
		t.Fatalf("RenderAGENTSMd: %v", err)
	}

	// Verify skill rendered.
	if !strings.Contains(result, "- **Judgment Day**: Dual review protocol. Trigger: When user says judgment day") {
		t.Error("expected Judgment Day skill, not found")
	}
}

// TestRenderCLAUDEMd_ReturnsErrorOnMissingTemplate verifies that RenderCLAUDEMd
// returns an error when the template file is missing.
func TestRenderCLAUDEMd_ReturnsErrorOnMissingTemplate(t *testing.T) {
	testFS := fstest.MapFS{
		// Empty FS - no template file
	}

	_, err := RenderCLAUDEMd(testFS, "layer1", "layer2", "expertise", nil)
	if err == nil {
		t.Error("expected error when template file is missing, got nil")
	}
}

// TestRenderCLAUDEMd_AllFieldsRendered verifies that all TemplateData fields
// (Layer1, Layer2, Expertise, Skills) are accessible in the template.
func TestRenderCLAUDEMd_AllFieldsRendered(t *testing.T) {
	testFS := fstest.MapFS{
		"embed/templates/CLAUDE.md.tmpl": {Data: []byte(`Layer1: {{.Layer1}}
Layer2: {{.Layer2}}
Expertise: {{.Expertise}}
Skills: {{range .Skills}}{{.Name}} {{end}}
`)},
	}

	skills := []SkillInfo{
		{Name: "SkillA", Description: "Desc A", Trigger: "Trigger A"},
	}

	result, err := RenderCLAUDEMd(testFS, "L1Content", "L2Content", "ExpertiseContent", skills)
	if err != nil {
		t.Fatalf("RenderCLAUDEMd: %v", err)
	}

	// Verify all fields rendered.
	if !strings.Contains(result, "Layer1: L1Content") {
		t.Error("expected Layer1 to be rendered")
	}
	if !strings.Contains(result, "Layer2: L2Content") {
		t.Error("expected Layer2 to be rendered")
	}
	if !strings.Contains(result, "Expertise: ExpertiseContent") {
		t.Error("expected Expertise to be rendered")
	}
	if !strings.Contains(result, "Skills: SkillA") {
		t.Error("expected Skills to be rendered")
	}
}

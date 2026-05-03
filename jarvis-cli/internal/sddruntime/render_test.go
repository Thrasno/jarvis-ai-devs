package sddruntime

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

func TestRenderOrchestrator_AssignmentsUseResolvedCrossPlatformMap(t *testing.T) {
	template := `## Model Assignments
| Phase | Default Model | Reason |
|-------|---------------|--------|
{{- range .ModelRows }}
| {{ .Phase }} | {{ .Model }} | {{ .Reason }} |
{{- end }}`

	cfg := &config.AppConfig{}
	cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{
		"sdd-apply": {OpenCode: "opus", Claude: "haiku"},
	}

	opencodeContent, err := RenderOrchestrator("opencode", cfg, template)
	if err != nil {
		t.Fatalf("RenderOrchestrator opencode error: %v", err)
	}
	claudeContent, err := RenderOrchestrator("claude", cfg, template)
	if err != nil {
		t.Fatalf("RenderOrchestrator claude error: %v", err)
	}

	if !strings.Contains(opencodeContent, "| sdd-apply | opus |") {
		t.Fatalf("expected opencode to render sdd-apply=opus, got:\n%s", opencodeContent)
	}
	if !strings.Contains(claudeContent, "| sdd-apply | haiku |") {
		t.Fatalf("expected claude to render sdd-apply=haiku, got:\n%s", claudeContent)
	}
}

func TestRenderOrchestrator_RejectsUnsupportedAgent(t *testing.T) {
	_, err := RenderOrchestrator("cursor", &config.AppConfig{}, "{{ range .ModelRows }}{{ end }}")
	if err == nil {
		t.Fatal("expected error for unsupported agent")
	}
}

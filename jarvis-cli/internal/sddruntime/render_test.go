package sddruntime

import (
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
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

func TestRenderOrchestrator_OpenCodeUsesProviderQualifiedAssignments(t *testing.T) {
	template := `## Model Assignments
| Phase | Default Model | Effort | Reason |
|-------|---------------|--------|--------|
{{- range .ModelRows }}
| {{ .Phase }} | {{ .Model }} | {{ .Effort }} | {{ .Reason }} |
{{- end }}`

	cfg := &config.AppConfig{}
	cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{
		"sdd-apply": {OpenCode: "opus", Claude: "haiku"},
	}
	cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
	}

	opencodeContent, err := RenderOrchestrator("opencode", cfg, template)
	if err != nil {
		t.Fatalf("RenderOrchestrator opencode error: %v", err)
	}
	claudeContent, err := RenderOrchestrator("claude", cfg, template)
	if err != nil {
		t.Fatalf("RenderOrchestrator claude error: %v", err)
	}

	if !strings.Contains(opencodeContent, "| sdd-apply | openai/gpt-5.1-codex-max | high |") {
		t.Fatalf("expected opencode provider-qualified assignment with effort, got:\n%s", opencodeContent)
	}
	if !strings.Contains(claudeContent, "| sdd-apply | haiku | - |") {
		t.Fatalf("expected Claude to keep Claude alias without OpenCode effort, got:\n%s", claudeContent)
	}
}

func TestRenderOrchestrator_RejectsUnsupportedAgent(t *testing.T) {
	_, err := RenderOrchestrator("cursor", &config.AppConfig{}, "{{ range .ModelRows }}{{ end }}")
	if err == nil {
		t.Fatal("expected error for unsupported agent")
	}
}

func TestRenderModelSections_SelectsMatchingClassAndRemovesMarkers(t *testing.T) {
	content := strings.Join([]string{
		"# Shared heading",
		"<!-- section:model-capable -->",
		"Capable instructions",
		"<!-- /section:model-capable -->",
		"<!-- section:model-small -->",
		"Small instructions",
		"<!-- /section:model-small -->",
		"Shared footer",
	}, "\n")

	got, err := RenderModelSections(content, ModelSectionCapable)
	if err != nil {
		t.Fatalf("RenderModelSections: %v", err)
	}

	for _, want := range []string{"# Shared heading", "Capable instructions", "Shared footer"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered content missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Small instructions", "section:model-capable", "section:model-small"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("rendered content must remove %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderModelSections_UnknownClassKeepsOnlyNeutralContent(t *testing.T) {
	content := strings.Join([]string{
		"Before",
		"<!-- section:model-capable -->",
		"Capable only",
		"<!-- /section:model-capable -->",
		"Middle",
		"<!-- section:model-small -->",
		"Small only",
		"<!-- /section:model-small -->",
		"After",
	}, "\n")

	got, err := RenderModelSections(content, ModelSectionUnknown)
	if err != nil {
		t.Fatalf("RenderModelSections: %v", err)
	}

	for _, want := range []string{"Before", "Middle", "After"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered content missing neutral text %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Capable only", "Small only", "section:model"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("unknown model class must remove model-specific content %q:\n%s", unwanted, got)
		}
	}
}

func TestModelSectionClassForModel_UsesJarvisModelAliases(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  ModelSectionClass
	}{
		{name: "opus is capable", model: "opus", want: ModelSectionCapable},
		{name: "sonnet is capable", model: "sonnet", want: ModelSectionCapable},
		{name: "haiku is small", model: "haiku", want: ModelSectionSmall},
		{name: "anthropic opus is capable", model: "anthropic/claude-opus-4-1", want: ModelSectionCapable},
		{name: "anthropic sonnet is capable", model: "anthropic/claude-sonnet-4-5", want: ModelSectionCapable},
		{name: "anthropic haiku is small", model: "anthropic/claude-haiku-4-5", want: ModelSectionSmall},
		{name: "provider qualified gpt five codex is capable", model: "openai/gpt-5.1-codex-max", want: ModelSectionCapable},
		{name: "provider qualified gpt four is capable", model: "openai/gpt-4o", want: ModelSectionCapable},
		{name: "provider qualified gpt four mini is small", model: "openai/gpt-4o-mini", want: ModelSectionSmall},
		{name: "provider qualified o3 is capable", model: "openai/o3", want: ModelSectionCapable},
		{name: "provider qualified o4 is capable", model: "openai/o4", want: ModelSectionCapable},
		{name: "provider qualified o4 mini is small", model: "openai/o4-mini", want: ModelSectionSmall},
		{name: "unknown provider model is unknown", model: "vendor/custom-model", want: ModelSectionUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModelSectionClassForModel(tt.model); got != tt.want {
				t.Fatalf("ModelSectionClassForModel(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestModelSectionClassForModel_KeepsUnknownProviderTokensUnknownAndNeutral(t *testing.T) {
	content := strings.Join([]string{
		"Neutral before",
		"<!-- section:model-capable -->",
		"Capable only",
		"<!-- /section:model-capable -->",
		"<!-- section:model-small -->",
		"Small only",
		"<!-- /section:model-small -->",
		"Neutral after",
	}, "\n")

	tests := []struct {
		name  string
		model string
	}{
		{name: "custom provider small token", model: "vendor/custom-small-model"},
		{name: "custom provider mini token", model: "vendor/custom-mini-model"},
		{name: "custom provider nano token", model: "vendor/custom-nano-model"},
		{name: "custom provider misleading gpt five token", model: "vendor/not-gpt-5-compatible"},
		{name: "custom provider misleading gpt four token", model: "vendor/not-gpt-4-compatible"},
		{name: "custom provider misleading o3 token", model: "vendor/not-o3-compatible"},
		{name: "custom provider misleading o4 token", model: "vendor/not-o4-compatible"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class := ModelSectionClassForModel(tt.model)
			if class != ModelSectionUnknown {
				t.Fatalf("ModelSectionClassForModel(%q) = %q, want %q", tt.model, class, ModelSectionUnknown)
			}

			rendered, err := RenderModelSections(content, class)
			if err != nil {
				t.Fatalf("RenderModelSections: %v", err)
			}
			for _, want := range []string{"Neutral before", "Neutral after"} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("rendered content missing neutral text %q for %q:\n%s", want, tt.model, rendered)
				}
			}
			for _, unwanted := range []string{"Capable only", "Small only", "section:model"} {
				if strings.Contains(rendered, unwanted) {
					t.Fatalf("unknown model %q must render neutral-only content; found %q in:\n%s", tt.model, unwanted, rendered)
				}
			}
		})
	}
}

func TestRenderOrchestrator_AppliesModelSectionsFromResolvedOrchestratorModel(t *testing.T) {
	template := strings.Join([]string{
		"Neutral before",
		"<!-- section:model-capable -->",
		"Capable {{ index .ModelRows 0 }}",
		"<!-- /section:model-capable -->",
		"<!-- section:model-small -->",
		"Small model instructions",
		"<!-- /section:model-small -->",
		"Neutral after",
	}, "\n")

	capableContent, err := RenderOrchestrator("opencode", &config.AppConfig{}, template)
	if err != nil {
		t.Fatalf("RenderOrchestrator capable: %v", err)
	}
	if !strings.Contains(capableContent, "Capable") || strings.Contains(capableContent, "Small model instructions") || strings.Contains(capableContent, "section:model") {
		t.Fatalf("capable orchestrator render did not select the capable section cleanly:\n%s", capableContent)
	}

	cfg := &config.AppConfig{SDD: config.SDDConfig{PhaseModels: map[string]config.PhaseModelSelection{
		"orchestrator": {OpenCode: "haiku"},
	}}}
	smallContent, err := RenderOrchestrator("opencode", cfg, template)
	if err != nil {
		t.Fatalf("RenderOrchestrator small: %v", err)
	}
	if !strings.Contains(smallContent, "Small model instructions") || strings.Contains(smallContent, "Capable") || strings.Contains(smallContent, "section:model") {
		t.Fatalf("small orchestrator render did not select the small section cleanly:\n%s", smallContent)
	}
}

func TestRenderOrchestrator_IncludesPhaseLaunchGuardrailsWithoutDuplicatingRuntimePolicy(t *testing.T) {
	templateContent, err := jarvis.OrchestratorFS.ReadFile("embed/orchestrator/sdd-orchestrator.md")
	if err != nil {
		t.Fatalf("read orchestrator template: %v", err)
	}

	content, err := RenderOrchestrator("opencode", &config.AppConfig{}, string(templateContent))
	if err != nil {
		t.Fatalf("RenderOrchestrator error: %v", err)
	}

	for _, required := range []string{
		"Runtime Activation Policy",
		"Mandatory Delegation Triggers",
		"Cost and Context Balance",
		"Sub-Agent Launch Deduplication",
		"Review Workload Guard",
		"Delivery Strategy",
		"Chain Strategy",
		"Language Domain Contract",
		"SDD Entry Routing",
		"SDD Session Preflight",
		"artifact store",
		"strict TDD",
		"review budget",
		"branch/tracker",
		"issue context",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("rendered orchestrator missing %q\n%s", required, content)
		}
	}

	if got := strings.Count(content, "## Model Assignments"); got != 1 {
		t.Fatalf("rendered orchestrator model assignment table count = %d, want 1", got)
	}
}

package sddruntime

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

func TestResolveAssignmentsForPlatform_OpenCodeProviderQualifiedOverridesAlias(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{
		"sdd-apply": {OpenCode: "opus", Claude: "haiku"},
	}
	cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
	}

	assignments, err := ResolveAssignmentsForPlatform(PlatformOpenCode, cfg)
	if err != nil {
		t.Fatalf("ResolveAssignmentsForPlatform opencode: %v", err)
	}
	if assignments["sdd-apply"] != "openai/gpt-5.1-codex-max" {
		t.Fatalf("sdd-apply assignment = %q, want provider-qualified model", assignments["sdd-apply"])
	}
}

func TestResolveAssignmentsForPlatform_IncompleteOpenCodeAssignmentFallsBackToAlias(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{
		"sdd-apply": {OpenCode: "opus", Claude: "haiku"},
	}
	cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"sdd-apply": {ProviderID: "openai"},
	}

	assignments, err := ResolveAssignmentsForPlatform(PlatformOpenCode, cfg)
	if err != nil {
		t.Fatalf("ResolveAssignmentsForPlatform opencode: %v", err)
	}
	if assignments["sdd-apply"] != "opus" {
		t.Fatalf("sdd-apply assignment = %q, want alias fallback", assignments["sdd-apply"])
	}
}

func TestResolveAssignmentsForPlatform_ClaudeIgnoresOpenCodeProviderAssignments(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{
		"sdd-apply": {OpenCode: "opus", Claude: "haiku"},
	}
	cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
	}

	assignments, err := ResolveAssignmentsForPlatform(PlatformClaude, cfg)
	if err != nil {
		t.Fatalf("ResolveAssignmentsForPlatform claude: %v", err)
	}
	if assignments["sdd-apply"] != "haiku" {
		t.Fatalf("sdd-apply assignment = %q, want Claude alias", assignments["sdd-apply"])
	}
}

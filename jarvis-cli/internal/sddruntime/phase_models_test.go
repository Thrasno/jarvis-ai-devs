package sddruntime

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

func TestResolvePhaseModels(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *config.AppConfig
		assert func(t *testing.T, got map[string]config.PhaseModelSelection)
	}{
		{
			name: "nil config returns full defaults",
			cfg:  nil,
			assert: func(t *testing.T, got map[string]config.PhaseModelSelection) {
				t.Helper()
				if len(got) != len(DefaultContract().Phases) {
					t.Fatalf("expected %d resolved phases, got %d", len(DefaultContract().Phases), len(got))
				}
				if got["orchestrator"].OpenCode == "" || got["orchestrator"].Claude == "" {
					t.Fatalf("expected defaults for orchestrator, got %+v", got["orchestrator"])
				}
			},
		},
		{
			name: "partial map fills missing entries from defaults",
			cfg: &config.AppConfig{SDD: config.SDDConfig{PhaseModels: map[string]config.PhaseModelSelection{
				"default": {OpenCode: "opus"},
			}}},
			assert: func(t *testing.T, got map[string]config.PhaseModelSelection) {
				t.Helper()
				if got["default"].OpenCode != "opus" {
					t.Fatalf("expected persisted opencode default to be preserved, got %+v", got["default"])
				}
				if got["default"].Claude == "" {
					t.Fatal("expected missing claude value to fallback from defaults")
				}
				if got["sdd-apply"].OpenCode == "" || got["sdd-apply"].Claude == "" {
					t.Fatalf("expected missing phase to fallback from defaults, got %+v", got["sdd-apply"])
				}
			},
		},
		{
			name: "unknown phase and invalid catalog values are normalized",
			cfg: &config.AppConfig{SDD: config.SDDConfig{PhaseModels: map[string]config.PhaseModelSelection{
				"unknown-phase": {OpenCode: "sonnet", Claude: "sonnet"},
				"sdd-apply":     {OpenCode: "NOT-IN-CATALOG", Claude: "ALSO-BAD"},
			}}},
			assert: func(t *testing.T, got map[string]config.PhaseModelSelection) {
				t.Helper()
				if _, ok := got["unknown-phase"]; ok {
					t.Fatal("expected unknown phase to be ignored")
				}
				defaults := DefaultContract().DefaultPhaseModels["sdd-apply"]
				if got["sdd-apply"] != defaults {
					t.Fatalf("expected invalid values to fallback to defaults, got %+v want %+v", got["sdd-apply"], defaults)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePhaseModels(tt.cfg)
			tt.assert(t, got)
		})
	}
}

func TestResolveOpenCodeProviderQualifiedAssignments_DefaultsAreProviderQualified(t *testing.T) {
	got, err := ResolveOpenCodeProviderQualifiedAssignments(nil)
	if err != nil {
		t.Fatalf("ResolveOpenCodeProviderQualifiedAssignments: %v", err)
	}

	for _, phase := range DefaultContract().Phases {
		model := got[phase]
		if !strings.Contains(model, "/") {
			t.Fatalf("phase %q OpenCode model = %q, want provider-qualified provider/model", phase, model)
		}
	}
}

func TestDefaultPhaseRoutesForPlatform_IncludesInitAndOnboard(t *testing.T) {
	routes, err := DefaultPhaseRoutesForPlatform(PlatformClaude)
	if err != nil {
		t.Fatalf("DefaultPhaseRoutesForPlatform: %v", err)
	}

	tests := []struct {
		phase     string
		wantModel string
	}{
		{phase: "sdd-init", wantModel: "sonnet"},
		{phase: "sdd-onboard", wantModel: "sonnet"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			got, ok := routes[tt.phase]
			if !ok {
				t.Fatalf("expected route for %q", tt.phase)
			}
			if got.Model != tt.wantModel || got.Effort != "" {
				t.Fatalf("route for %q = %+v, want model %q and empty effort", tt.phase, got, tt.wantModel)
			}
		})
	}
}

func TestResolvePhaseRoutesForPlatform_ClaudeReturnsModelAndEffort(t *testing.T) {
	cfg := &config.AppConfig{SDD: config.SDDConfig{
		PhaseModels: map[string]config.PhaseModelSelection{
			"sdd-design": {Claude: "haiku"},
		},
		ClaudePhaseModels: map[string]config.ClaudeModelAssignment{
			"sdd-design": {Effort: "max"},
		},
	}}

	routes, err := ResolvePhaseRoutesForPlatform(PlatformClaude, cfg)
	if err != nil {
		t.Fatalf("ResolvePhaseRoutesForPlatform: %v", err)
	}

	got := routes["sdd-design"]
	if got.Model != "haiku" || got.Effort != "max" {
		t.Fatalf("sdd-design Claude route = %+v, want model haiku and effort max", got)
	}
}

func TestDefaultAssignmentsForPlatform(t *testing.T) {
	tests := []struct {
		name      string
		platform  Platform
		wantPhase string
		wantModel string
		wantErr   bool
	}{
		{name: "opencode defaults", platform: PlatformOpenCode, wantPhase: "sdd-apply", wantModel: "sonnet"},
		{name: "claude defaults", platform: PlatformClaude, wantPhase: "orchestrator", wantModel: "opus"},
		{name: "unsupported platform errors", platform: Platform("gemini"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultAssignmentsForPlatform(tt.platform)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for unsupported platform")
				}
				return
			}
			if err != nil {
				t.Fatalf("DefaultAssignmentsForPlatform returned error: %v", err)
			}
			if len(got) != len(DefaultContract().Phases) {
				t.Fatalf("expected %d phases, got %d", len(DefaultContract().Phases), len(got))
			}
			if got[tt.wantPhase] != tt.wantModel {
				t.Fatalf("phase %q model = %q, want %q", tt.wantPhase, got[tt.wantPhase], tt.wantModel)
			}
		})
	}
}

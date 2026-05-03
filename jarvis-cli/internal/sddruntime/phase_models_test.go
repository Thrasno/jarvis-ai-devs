package sddruntime

import (
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

func TestResolvePhaseModels(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.AppConfig
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
				"sdd-apply":    {OpenCode: "NOT-IN-CATALOG", Claude: "ALSO-BAD"},
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

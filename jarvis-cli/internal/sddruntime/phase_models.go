package sddruntime

import (
	"strings"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

// ResolvePhaseModels merges persisted config values with contract defaults.
// It guarantees all known phases are returned and values are valid per platform catalog.
func ResolvePhaseModels(cfg *config.AppConfig) map[string]config.PhaseModelSelection {
	contract := DefaultContract()
	resolved := make(map[string]config.PhaseModelSelection, len(contract.Phases))

	for _, phase := range contract.Phases {
		resolved[phase] = contract.DefaultPhaseModels[phase]
	}

	if cfg == nil || cfg.SDD.PhaseModels == nil {
		return resolved
	}

	for rawPhase, persisted := range cfg.SDD.PhaseModels {
		phase := strings.ToLower(strings.TrimSpace(rawPhase))
		defaults, ok := contract.DefaultPhaseModels[phase]
		if !ok {
			continue
		}

		resolved[phase] = config.PhaseModelSelection{
			OpenCode: normalizePlatformValue(strings.ToLower(strings.TrimSpace(persisted.OpenCode)), defaults.OpenCode, contract.PlatformCatalogs[PlatformOpenCode]),
			Claude:   normalizePlatformValue(strings.ToLower(strings.TrimSpace(persisted.Claude)), defaults.Claude, contract.PlatformCatalogs[PlatformClaude]),
		}
	}

	return resolved
}

func normalizePlatformValue(value string, fallback string, catalog []string) string {
	if value == "" {
		return fallback
	}
	for _, allowed := range catalog {
		if value == allowed {
			return value
		}
	}
	return fallback
}

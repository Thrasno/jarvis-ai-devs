package sddruntime

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

// DefaultAssignmentsForPlatform returns the contract default phase->model assignments for a platform.
// It only derives from DefaultPhaseModels and returns a deterministic error for unsupported platforms.
func DefaultAssignmentsForPlatform(platform Platform) (map[string]string, error) {
	contract := DefaultContract()
	assignments := make(map[string]string, len(contract.Phases))

	for _, phase := range contract.Phases {
		selection, ok := contract.DefaultPhaseModels[phase]
		if !ok {
			continue
		}

		switch platform {
		case PlatformOpenCode:
			assignments[phase] = selection.OpenCode
		case PlatformClaude:
			assignments[phase] = selection.Claude
		default:
			return nil, fmt.Errorf("unsupported platform %q", platform)
		}
	}

	return assignments, nil
}

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

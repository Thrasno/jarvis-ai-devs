package sddruntime

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
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

// ResolveAssignmentsForPlatform returns phase->model assignments for a platform.
// OpenCode provider-qualified assignments override legacy aliases when complete.
func ResolveAssignmentsForPlatform(platform Platform, cfg *config.AppConfig) (map[string]string, error) {
	resolved := ResolvePhaseModels(cfg)
	contract := DefaultContract()
	assignments := make(map[string]string, len(contract.Phases))
	for _, phase := range contract.Phases {
		selection := resolved[phase]
		switch platform {
		case PlatformOpenCode:
			assignments[phase] = selection.OpenCode
		case PlatformClaude:
			assignments[phase] = selection.Claude
		default:
			return nil, fmt.Errorf("unsupported platform %q", platform)
		}
	}

	if platform != PlatformOpenCode || cfg == nil || cfg.SDD.OpenCodePhaseModels == nil {
		return assignments, nil
	}
	for rawPhase, assignment := range cfg.SDD.OpenCodePhaseModels {
		phase := strings.ToLower(strings.TrimSpace(rawPhase))
		if _, ok := contract.DefaultPhaseModels[phase]; !ok {
			continue
		}
		providerID := strings.TrimSpace(assignment.ProviderID)
		modelID := strings.TrimSpace(assignment.ModelID)
		if providerID == "" || modelID == "" {
			continue
		}
		assignments[phase] = providerID + "/" + modelID
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

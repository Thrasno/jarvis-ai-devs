package sddruntime

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

type PhaseRoute struct {
	Model  string
	Effort string
}

// DefaultAssignmentsForPlatform returns the contract default phase->model assignments for a platform.
// It only derives from DefaultPhaseModels and returns a deterministic error for unsupported platforms.
func DefaultAssignmentsForPlatform(platform Platform) (map[string]string, error) {
	routes, err := DefaultPhaseRoutesForPlatform(platform)
	if err != nil {
		return nil, err
	}
	assignments := make(map[string]string, len(routes))
	for phase, route := range routes {
		assignments[phase] = route.Model
	}

	return assignments, nil
}

// DefaultPhaseRoutesForPlatform returns the contract default phase routes for a platform.
func DefaultPhaseRoutesForPlatform(platform Platform) (map[string]PhaseRoute, error) {
	contract := DefaultContract()
	routes := make(map[string]PhaseRoute, len(contract.Phases))

	for _, phase := range contract.Phases {
		selection, ok := contract.DefaultPhaseModels[phase]
		if !ok {
			continue
		}

		switch platform {
		case PlatformOpenCode:
			routes[phase] = PhaseRoute{Model: selection.OpenCode}
		case PlatformClaude:
			routes[phase] = PhaseRoute{Model: selection.Claude}
		default:
			return nil, fmt.Errorf("unsupported platform %q", platform)
		}
	}

	return routes, nil
}

// ResolveAssignmentsForPlatform returns phase->model assignments for a platform.
// OpenCode provider-qualified assignments override legacy aliases when complete.
func ResolveAssignmentsForPlatform(platform Platform, models state.PhaseModels) (map[string]string, error) {
	routes, err := ResolvePhaseRoutesForPlatform(platform, models)
	if err != nil {
		return nil, err
	}
	assignments := make(map[string]string, len(routes))
	for phase, route := range routes {
		assignments[phase] = route.Model
	}
	return assignments, nil
}

// ResolvePhaseRoutesForPlatform returns phase->model/effort routes for a platform.
func ResolvePhaseRoutesForPlatform(platform Platform, models state.PhaseModels) (map[string]PhaseRoute, error) {
	resolved := ResolvePhaseModels(models)
	contract := DefaultContract()
	routes := make(map[string]PhaseRoute, len(contract.Phases))
	for _, phase := range contract.Phases {
		selection := resolved[phase]
		switch platform {
		case PlatformOpenCode:
			routes[phase] = PhaseRoute{Model: selection.OpenCode}
		case PlatformClaude:
			routes[phase] = PhaseRoute{Model: selection.Claude}
		default:
			return nil, fmt.Errorf("unsupported platform %q", platform)
		}
	}

	switch platform {
	case PlatformOpenCode:
		applyOpenCodePhaseRoutes(routes, models, contract)
	case PlatformClaude:
		applyClaudePhaseRoutes(routes, models, contract)
	}
	return routes, nil
}

func applyOpenCodePhaseRoutes(routes map[string]PhaseRoute, models state.PhaseModels, contract Contract) {
	if models.OpenCode == nil {
		return
	}
	for rawPhase, assignment := range models.OpenCode {
		phase := strings.ToLower(strings.TrimSpace(rawPhase))
		if _, ok := contract.DefaultPhaseModels[phase]; !ok {
			continue
		}
		providerID := strings.TrimSpace(assignment.ProviderID)
		modelID := strings.TrimSpace(assignment.ModelID)
		if providerID == "" || modelID == "" {
			continue
		}
		routes[phase] = PhaseRoute{Model: providerID + "/" + modelID, Effort: strings.TrimSpace(assignment.Effort)}
	}
}

func applyClaudePhaseRoutes(routes map[string]PhaseRoute, models state.PhaseModels, contract Contract) {
	if models.Claude == nil {
		return
	}
	for rawPhase, assignment := range models.Claude {
		phase := strings.ToLower(strings.TrimSpace(rawPhase))
		if _, ok := contract.DefaultPhaseModels[phase]; !ok {
			continue
		}
		route := routes[phase]
		if model := strings.TrimSpace(assignment.Model); model != "" {
			route.Model = model
		}
		route.Effort = strings.TrimSpace(assignment.Effort)
		routes[phase] = route
	}
}

// ResolveOpenCodeProviderQualifiedAssignments returns OpenCode phase assignments
// suitable for opencode.json generation, where agent model values must use the
// provider/model form required by OpenCode config schema.
func ResolveOpenCodeProviderQualifiedAssignments(models state.PhaseModels) (map[string]string, error) {
	assignments, err := ResolveAssignmentsForPlatform(PlatformOpenCode, models)
	if err != nil {
		return nil, err
	}
	for phase, model := range assignments {
		assignments[phase] = resolveOpenCodeModelID(model)
	}
	return assignments, nil
}

func resolveOpenCodeModelID(model string) string {
	model = strings.TrimSpace(model)
	if strings.Contains(model, "/") {
		return model
	}
	switch strings.ToLower(model) {
	case "opus":
		return "anthropic/claude-opus-4-1"
	case "sonnet":
		return "anthropic/claude-sonnet-4-5"
	case "haiku":
		return "anthropic/claude-haiku-4-5"
	default:
		return model
	}
}

// ResolveVariantsForPlatform returns phase->variant assignments for providers that
// support variant selection. Currently only OpenCode persists model effort as the
// agent variant; other platforms intentionally return empty variants.
func ResolveVariantsForPlatform(platform Platform, models state.PhaseModels) (map[string]string, error) {
	contract := DefaultContract()
	variants := make(map[string]string, len(contract.Phases))
	for _, phase := range contract.Phases {
		variants[phase] = ""
	}

	if platform != PlatformOpenCode {
		if platform != PlatformClaude {
			return nil, fmt.Errorf("unsupported platform %q", platform)
		}
		return variants, nil
	}
	if models.OpenCode == nil {
		return variants, nil
	}
	for rawPhase, assignment := range models.OpenCode {
		phase := strings.ToLower(strings.TrimSpace(rawPhase))
		if _, ok := contract.DefaultPhaseModels[phase]; !ok {
			continue
		}
		if strings.TrimSpace(assignment.ProviderID) == "" || strings.TrimSpace(assignment.ModelID) == "" {
			continue
		}
		variants[phase] = strings.TrimSpace(assignment.Effort)
	}
	return variants, nil
}

// ResolvePhaseModels merges persisted config values with contract defaults.
// It guarantees all known phases are returned and values are valid per platform catalog.
func ResolvePhaseModels(models state.PhaseModels) map[string]state.PhaseModelSelection {
	contract := DefaultContract()
	resolved := make(map[string]state.PhaseModelSelection, len(contract.Phases))

	for _, phase := range contract.Phases {
		resolved[phase] = state.PhaseModelSelection(contract.DefaultPhaseModels[phase])
	}

	if models.Aliases == nil {
		return resolved
	}

	for rawPhase, persisted := range models.Aliases {
		phase := strings.ToLower(strings.TrimSpace(rawPhase))
		defaults, ok := contract.DefaultPhaseModels[phase]
		if !ok {
			continue
		}

		resolved[phase] = state.PhaseModelSelection{
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

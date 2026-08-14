package config

import (
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// PhaseModelsForState projects the persisted SDD phase-model assignments onto
// the manifest shape (state.PhaseModels), which is the only part of AppConfig
// the SDD runtime, the agent adapters and the artifact pipeline actually read.
// It is the single conversion point: callers pass the result instead of
// threading a whole *AppConfig through signatures that only need these maps.
//
// A nil receiver yields the zero value, matching an AppConfig with no persisted
// assignments.
func (c *AppConfig) PhaseModelsForState() state.PhaseModels {
	if c == nil {
		return state.PhaseModels{}
	}
	return state.PhaseModels{
		Aliases:  convertMap(c.SDD.PhaseModels, func(v PhaseModelSelection) state.PhaseModelSelection { return state.PhaseModelSelection(v) }),
		OpenCode: convertMap(c.SDD.OpenCodePhaseModels, func(v OpenCodeModelAssignment) state.OpenCodeModelAssignment { return state.OpenCodeModelAssignment(v) }),
		Claude:   convertMap(c.SDD.ClaudePhaseModels, func(v ClaudeModelAssignment) state.ClaudeModelAssignment { return state.ClaudeModelAssignment(v) }),
	}
}

func convertMap[In, Out any](in map[string]In, convert func(In) Out) map[string]Out {
	out := make(map[string]Out, len(in))
	for key, value := range in {
		out[key] = convert(value)
	}
	return out
}

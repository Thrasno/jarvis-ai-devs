package config

// TEMPORARY BRIDGE — delete once every consumer reads internal/state directly.
// ~/.jarvis/state.yaml owns the replay fields (persona, selected skills,
// configured agents, per-phase models, scope), but around fifty-five files
// across internal/tui, internal/persona, internal/sddruntime, internal/agent
// and cmd/jarvis still read them off AppConfig. So AppConfig keeps the fields
// and the manifest owns them: Load projects the manifest onto AppConfig, Save
// routes them back into it and strips them from config.yaml. Scaffolding, not
// architecture — when those packages read internal/state directly, delete this
// file and remove the fields from AppConfig (the original task 1.10).

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// applyStateManifest projects the manifest's replay fields onto cfg. A missing
// manifest is the pre-migration machine: config.yaml stays authoritative.
func applyStateManifest(cfg *AppConfig) error {
	manifest, err := state.Load()
	if errors.Is(err, state.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	cfg.PersonaPreset = manifest.Persona
	cfg.PersonaPresetSource = string(manifest.PersonaSource)
	cfg.SelectedSkills = manifest.Skills
	cfg.Scope = SetupScope(manifest.Scope)

	cfg.ConfiguredAgents = make([]string, 0, len(manifest.InstalledAgents))
	cfg.Install.Agents = make(map[string]AgentState, len(manifest.InstalledAgents))
	for _, agent := range manifest.InstalledAgents {
		cfg.ConfiguredAgents = append(cfg.ConfiguredAgents, agent.ID)
		cfg.Install.Agents[agent.ID] = AgentState{
			Configured:       true,
			InstructionsPath: agent.InstructionsPath,
			ConfigPath:       agent.ConfigPath,
		}
	}

	pm := manifest.PhaseModels
	cfg.SDD.PhaseModels = convertMap(pm.Aliases, func(v state.PhaseModelSelection) PhaseModelSelection { return PhaseModelSelection(v) })
	cfg.SDD.OpenCodePhaseModels = convertMap(pm.OpenCode, func(v state.OpenCodeModelAssignment) OpenCodeModelAssignment { return OpenCodeModelAssignment(v) })
	cfg.SDD.ClaudePhaseModels = convertMap(pm.Claude, func(v state.ClaudeModelAssignment) ClaudeModelAssignment { return ClaudeModelAssignment(v) })
	return nil
}

// saveStateManifest writes cfg's replay fields into ~/.jarvis/state.yaml,
// preserving the fields config.yaml never owned (the statusline tri-state and
// the managed asset digest). The read-modify-write runs under the manifest lock
// because two jarvis processes finishing at once would otherwise each write a
// manifest the other has already replaced.
func saveStateManifest(cfg *AppConfig) error {
	return state.WithLock(func() error {
		manifest, err := state.Load()
		if errors.Is(err, state.ErrNotFound) {
			manifest = state.New()
		} else if err != nil {
			return err
		}

		manifest.Persona = cfg.PersonaPreset
		manifest.PersonaSource = state.PersonaSource(cfg.PersonaPresetSource)
		manifest.Skills = cfg.SelectedSkills
		manifest.Scope = state.Scope(cfg.Scope)

		records := make(map[string]state.AgentRecord, len(cfg.Install.Agents))
		for id, agent := range cfg.Install.Agents {
			records[id] = state.AgentRecord{
				Configured:       agent.Configured,
				InstructionsPath: agent.InstructionsPath,
				ConfigPath:       agent.ConfigPath,
			}
		}
		manifest.InstalledAgents = state.InstalledAgentsFrom(cfg.ConfiguredAgents, records)
		if len(manifest.InstalledAgents) > 0 {
			manifest.SelectionConfigured = true
		}

		manifest.PhaseModels.Aliases = convertMap(cfg.SDD.PhaseModels, func(v PhaseModelSelection) state.PhaseModelSelection { return state.PhaseModelSelection(v) })
		manifest.PhaseModels.OpenCode = convertMap(cfg.SDD.OpenCodePhaseModels, func(v OpenCodeModelAssignment) state.OpenCodeModelAssignment { return state.OpenCodeModelAssignment(v) })
		manifest.PhaseModels.Claude = convertMap(cfg.SDD.ClaudePhaseModels, func(v ClaudeModelAssignment) state.ClaudeModelAssignment { return state.ClaudeModelAssignment(v) })

		return state.Save(manifest)
	})
}

// marshalWithoutReplayFields drops every key the manifest owns, so disjointness
// holds in the written file and not just in the struct.
func marshalWithoutReplayFields(cfg *AppConfig) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	for _, key := range state.ReplayConfigKeys() {
		delete(raw, key)
	}
	if install, ok := raw["install"].(map[string]any); ok {
		delete(install, "agents")
	}
	return yaml.Marshal(raw)
}

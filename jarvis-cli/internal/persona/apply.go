package persona

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

// PresetAgent defines the minimal contract required by ApplyPresetPipeline.
// agent.Agent satisfies this interface without introducing package cycles.
type PresetAgent interface {
	Name() string
	WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error
	SupportsOutputStyles() bool
	WriteOutputStyle(preset *Preset) error
	ClearOutputStyle(name string) error
}

// PresetV2Agent is the optional adapter contract for dormant schema-v2
// presentation profiles. It deliberately remains separate from PresetAgent so
// the active V1 pipeline stays source and behavior compatible until activation.
type PresetV2Agent interface {
	Name() string
	WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error
	SupportsOutputStyles() bool
	WriteOutputStyleV2(preset *PresetV2) error
	ClearOutputStyle(name string) error
}

// PresetSelection carries exactly one resolved preset version through an
// adapter seam. Callers keep choosing V1 until the separate activation slice.
type PresetSelection struct {
	V1 *ResolvedPreset
	V2 *ResolvedPresetV2
}

// ApplyOptions controls how preset apply is executed.
type ApplyOptions struct {
	Layer1               string
	Skills               []config.SkillInfo
	PreviousPresetSlug   string
	PreviousPresetSource PresetSource
	PersistConfig        bool
}

// ApplyPresetPipeline applies a resolved preset with clean replacement semantics.
// It rewrites Layer2 instructions, clears previous output-style references/files,
// writes the new output-style, and optionally persists canonical preset identity.
func ApplyPresetPipeline(agents []PresetAgent, resolved *ResolvedPreset, opts ApplyOptions) error {
	if resolved == nil || resolved.Preset == nil {
		return fmt.Errorf("resolved preset is required")
	}

	resolvedSlug := NormalizeSlug(resolved.Slug)
	if resolvedSlug == "" {
		return fmt.Errorf("resolved preset slug cannot be empty")
	}

	layer2 := RenderLayer2(resolved.Preset)
	for _, a := range agents {
		if err := a.WriteInstructions(opts.Layer1, layer2, opts.Skills); err != nil {
			return fmt.Errorf("apply preset to %s instructions: %w", a.Name(), err)
		}

		if !a.SupportsOutputStyles() {
			continue
		}

		previousSlug := NormalizeSlug(opts.PreviousPresetSlug)
		if previousSlug != "" && previousSlug != resolvedSlug {
			if err := a.ClearOutputStyle(toTitleCase(previousSlug)); err != nil {
				return fmt.Errorf("cleanup previous output-style for %s: %w", a.Name(), err)
			}
		}

		if err := a.WriteOutputStyle(resolved.Preset); err != nil {
			return fmt.Errorf("write output-style for %s: %w", a.Name(), err)
		}
	}

	if !opts.PersistConfig {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.PersonaPreset = resolvedSlug
	cfg.Preset = resolvedSlug
	cfg.PersonaPresetSource = normalizePresetSourceForConfig(resolved.Source)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}

// ApplyPresetSelectionPipeline applies either an active V1 preset or an
// explicitly supplied dormant V2 preset. It never resolves profiles itself, so
// selecting V2 remains an opt-in adapter operation rather than V2 activation.
func ApplyPresetSelectionPipeline(agents []PresetAgent, selection PresetSelection, opts ApplyOptions) error {
	switch {
	case selection.V1 != nil && selection.V2 == nil:
		return ApplyPresetPipeline(agents, selection.V1, opts)
	case selection.V1 == nil && selection.V2 != nil:
		v2Agents := make([]PresetV2Agent, 0, len(agents))
		for _, a := range agents {
			v2Agent, ok := a.(PresetV2Agent)
			if !ok {
				return fmt.Errorf("agent %q does not support schema v2 presentation profiles", a.Name())
			}
			v2Agents = append(v2Agents, v2Agent)
		}
		return ApplyPresetV2Pipeline(v2Agents, selection.V2, opts)
	default:
		return fmt.Errorf("exactly one preset version must be selected")
	}
}

// ApplyPresetV2Pipeline applies a previously validated schema-v2 presentation
// profile through V2-capable adapters. It is intentionally not called by the
// default resolver or normal V1 CLI/TUI selection paths.
func ApplyPresetV2Pipeline(agents []PresetV2Agent, resolved *ResolvedPresetV2, opts ApplyOptions) error {
	if resolved == nil || resolved.Preset == nil {
		return fmt.Errorf("resolved schema v2 preset is required")
	}

	resolvedSlug := NormalizeSlug(resolved.Slug)
	if resolvedSlug == "" {
		return fmt.Errorf("resolved schema v2 preset slug cannot be empty")
	}

	layer2 := RenderLayer2V2(resolved.Preset)
	for _, a := range agents {
		if err := a.WriteInstructions(opts.Layer1, layer2, opts.Skills); err != nil {
			return fmt.Errorf("apply schema v2 preset to %s instructions: %w", a.Name(), err)
		}

		if !a.SupportsOutputStyles() {
			continue
		}

		previousSlug := NormalizeSlug(opts.PreviousPresetSlug)
		if previousSlug != "" && previousSlug != resolvedSlug {
			if err := a.ClearOutputStyle(toTitleCase(previousSlug)); err != nil {
				return fmt.Errorf("cleanup previous output-style for %s: %w", a.Name(), err)
			}
		}

		if err := a.WriteOutputStyleV2(resolved.Preset); err != nil {
			return fmt.Errorf("write schema v2 output-style for %s: %w", a.Name(), err)
		}
	}

	if !opts.PersistConfig {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.PersonaPreset = resolvedSlug
	cfg.Preset = resolvedSlug
	cfg.PersonaPresetSource = normalizePresetSourceForConfig(resolved.Source)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}

func normalizePresetSourceForConfig(source PresetSource) string {
	trimmed := strings.ToLower(strings.TrimSpace(string(source)))
	if trimmed == "user" {
		return "user"
	}
	return "builtin"
}

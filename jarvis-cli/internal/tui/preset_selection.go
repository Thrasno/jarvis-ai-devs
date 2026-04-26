package tui

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"gopkg.in/yaml.v3"
)

type customPresetDraft struct {
	Name        string
	DisplayName string
	YAML        string
}

func resolveWizardPresetSelection(personaFS fs.FS, requestedSlug string, custom *customPresetDraft) (*persona.ResolvedPreset, error) {
	normalized := persona.NormalizeSlug(requestedSlug)
	if normalized == "custom" {
		if custom == nil {
			return nil, fmt.Errorf("custom preset creation requires name and display name")
		}
		return createWizardCustomPreset(personaFS, *custom)
	}

	return persona.ResolvePreset(personaFS, normalized)
}

func createWizardCustomPreset(personaFS fs.FS, draft customPresetDraft) (*persona.ResolvedPreset, error) {
	name := strings.TrimSpace(draft.Name)
	if name == "" {
		return nil, fmt.Errorf("custom preset name is required")
	}
	displayName := strings.TrimSpace(draft.DisplayName)
	if displayName == "" {
		return nil, fmt.Errorf("custom preset display name is required")
	}

	slug := persona.NormalizeSlug(name)
	if slug == "" || strings.Trim(slug, "-") == "" {
		return nil, fmt.Errorf("custom preset name resolves to empty slug")
	}
	if slug == "custom" {
		return nil, fmt.Errorf("custom preset slug %q is reserved; choose a different name", slug)
	}
	builtinPath := fmt.Sprintf("embed/personas/%s.yaml", slug)
	if _, err := fs.Stat(personaFS, builtinPath); err == nil {
		return nil, fmt.Errorf("custom preset slug %q collides with built-in preset slug", slug)
	}

	content, err := buildCustomPresetContent(personaFS, slug, displayName, strings.TrimSpace(draft.YAML))
	if err != nil {
		return nil, err
	}

	if err := persona.ValidatePreset(content); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if _, err := persona.SaveUserPresetFile(slug, content); err != nil {
		return nil, fmt.Errorf("persist custom preset %q: %w", slug, err)
	}

	resolved, err := persona.ResolvePreset(personaFS, slug)
	if err != nil {
		return nil, fmt.Errorf("resolve persisted custom preset %q: %w", slug, err)
	}
	if resolved.Source != persona.PresetSourceUser {
		return nil, fmt.Errorf("custom preset %q did not resolve as user source", slug)
	}

	return resolved, nil
}

func buildCustomPresetContent(personaFS fs.FS, slug, displayName, customYAML string) ([]byte, error) {
	if customYAML == "" {
		base := defaultCustomPreset(personaFS, slug, displayName)
		content, err := yaml.Marshal(base)
		if err != nil {
			return nil, fmt.Errorf("marshal generated custom preset %q: %w", slug, err)
		}
		return content, nil
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(customYAML), &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}

	raw["name"] = slug
	raw["display_name"] = displayName
	if _, ok := raw["description"]; !ok {
		raw["description"] = fmt.Sprintf("Custom preset %s created from wizard.", displayName)
	}

	content, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal custom preset %q: %w", slug, err)
	}
	return content, nil
}

func defaultCustomPreset(personaFS fs.FS, slug, displayName string) persona.Preset {
	if neutral, err := persona.ResolvePreset(personaFS, "neutra"); err == nil && neutral != nil && neutral.Preset != nil {
		base := *neutral.Preset
		base.Name = slug
		base.DisplayName = displayName
		base.Description = fmt.Sprintf("Custom preset %s created from wizard.", displayName)
		return base
	}

	return persona.Preset{
		Name:        slug,
		DisplayName: displayName,
		Description: fmt.Sprintf("Custom preset %s created from wizard.", displayName),
		Tone: persona.Tone{
			Formality:  "neutral",
			Directness: "direct",
			Humor:      "none",
			Language:   "en-us",
		},
		CommunicationStyle: persona.CommunicationStyle{
			Verbosity:            "concise",
			ShowAlternatives:     true,
			ChallengeAssumptions: true,
		},
		CharacteristicPhrases: persona.CharacteristicPhrases{
			Greetings:     []string{"Hi"},
			Confirmations: []string{"OK"},
			Transitions:   []string{"Next"},
			SignOffs:      []string{"Done."},
		},
		Notes: `# Custom Persona

## Core Principle
Solve the technical problem first with clear reasoning.

## Behavior
Use direct language, verify assumptions, and include tradeoffs when relevant.

## When Asking Questions
Ask one specific question and stop until the user answers.
`,
	}
}

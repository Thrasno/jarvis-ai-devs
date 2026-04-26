// Package persona manages the Layer2 persona preset system.
// Presets are embedded YAML files that define tone, language, and communication style.
// The embed.FS is provided by the caller (assets.PersonaFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package persona

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"unicode"
)

// Tone describes the communication tone settings of a persona.
type Tone struct {
	Formality  string `yaml:"formality"`
	Directness string `yaml:"directness"`
	Humor      string `yaml:"humor"`
	Language   string `yaml:"language"`
}

// CommunicationStyle describes how the persona communicates.
type CommunicationStyle struct {
	Verbosity            string `yaml:"verbosity"`
	ShowAlternatives     bool   `yaml:"show_alternatives"`
	ChallengeAssumptions bool   `yaml:"challenge_assumptions"`
}

// CharacteristicPhrases holds persona-specific phrases used in responses.
type CharacteristicPhrases struct {
	Greetings     []string `yaml:"greetings"`
	Confirmations []string `yaml:"confirmations"`
	Transitions   []string `yaml:"transitions"`
	SignOffs      []string `yaml:"sign_offs"`
}

// Preset represents a complete persona configuration loaded from a YAML preset file.
type Preset struct {
	Name                  string                `yaml:"name"`
	DisplayName           string                `yaml:"display_name"`
	Description           string                `yaml:"description"`
	Tone                  Tone                  `yaml:"tone"`
	CommunicationStyle    CommunicationStyle    `yaml:"communication_style"`
	CharacteristicPhrases CharacteristicPhrases `yaml:"characteristic_phrases"`
	// Notes holds the full persona description — language rules, philosophy, speech
	// patterns, and behavior rules. Written as a freeform markdown block in the YAML
	// and appended verbatim to the Layer2 output after a horizontal rule.
	Notes string `yaml:"notes"`
}

// LoadPreset loads a named preset from the provided embed.FS.
// fs must be the root-package PersonaFS (embed/personas directory embedded at root).
// name must be one of the 7 built-in preset names (e.g. "argentino", "tony-stark").
func LoadPreset(fsys embed.FS, name string) (*Preset, error) {
	resolved, err := ResolvePreset(fsys, name)
	if err != nil {
		return nil, err
	}
	if resolved.Source != PresetSourceBuiltin {
		return nil, fmt.Errorf("preset %q is not a built-in preset", NormalizeSlug(name))
	}

	return resolved.Preset, nil
}

// ListPresets returns all built-in presets loaded from the provided embed.FS.
func ListPresets(fsys embed.FS) ([]Preset, error) {
	names := listPresetNames(fsys)
	presets := make([]Preset, 0, len(names))

	for _, name := range names {
		p, err := LoadPreset(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("load preset %q: %w", name, err)
		}
		presets = append(presets, *p)
	}

	return presets, nil
}

// listPresetNames returns the names of all built-in presets by scanning the provided embed.FS.
// Template files (*.tmpl) are excluded.
func listPresetNames(fsys fs.FS) []string {
	var names []string
	_ = fs.WalkDir(fsys, "embed/personas", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yaml.tmpl") {
			return nil
		}
		// Extract name from filename (strip directory and .yaml extension)
		base := d.Name()
		name := strings.TrimSuffix(base, ".yaml")
		names = append(names, name)
		return nil
	})
	return names
}

// ValidateCustom validates a user-provided custom persona YAML.
// Returns a descriptive error if required fields are missing or Layer1 fields
// are present (Layer1 fields must not be overridden via persona presets).
func ValidateCustom(content []byte) error {
	return ValidatePreset(content)
}

// RenderLayer2 renders a Layer2 markdown block from a preset.
// This is the content that goes between the LAYER2 sentinel markers.
func RenderLayer2(preset *Preset) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "## Persona: %s\n\n", preset.DisplayName)
	fmt.Fprintf(&sb, "%s\n\n", preset.Description)

	sb.WriteString("### Tone\n")
	fmt.Fprintf(&sb, "- **Formality**: %s\n", preset.Tone.Formality)
	fmt.Fprintf(&sb, "- **Directness**: %s\n", preset.Tone.Directness)
	fmt.Fprintf(&sb, "- **Humor**: %s\n", preset.Tone.Humor)
	fmt.Fprintf(&sb, "- **Language**: %s\n\n", preset.Tone.Language)

	sb.WriteString("### Communication Style\n")
	if preset.CommunicationStyle.ShowAlternatives {
		sb.WriteString("- Always propose alternatives with tradeoffs\n")
	}
	if preset.CommunicationStyle.ChallengeAssumptions {
		sb.WriteString("- Challenge user assumptions when incorrect\n")
	}
	fmt.Fprintf(&sb, "- Verbosity: %s\n\n", preset.CommunicationStyle.Verbosity)

	if len(preset.CharacteristicPhrases.Greetings) > 0 {
		sb.WriteString("### Characteristic Phrases\n")
		sb.WriteString("**Greetings**: " + strings.Join(preset.CharacteristicPhrases.Greetings, " / ") + "\n")
		sb.WriteString("**Confirmations**: " + strings.Join(preset.CharacteristicPhrases.Confirmations, " / ") + "\n")
		if len(preset.CharacteristicPhrases.SignOffs) > 0 {
			sb.WriteString("**Sign-off**: " + preset.CharacteristicPhrases.SignOffs[0] + "\n")
		}
	}

	if preset.Notes != "" {
		sb.WriteString("\n---\n")
		sb.WriteString(preset.Notes)
	}

	return sb.String()
}

// RenderOutputStyle renders output-style markdown with YAML frontmatter for Claude Code.
// Format: ---\nname: TitleCase\ndescription: ...\nkeep-coding-instructions: true\n---\n{Notes}
// Implements SPEC-002.
func RenderOutputStyle(preset *Preset) string {
	var sb strings.Builder

	// Convert name to TitleCase (e.g., "tony-stark" -> "TonyStark")
	titleCaseName := toTitleCase(preset.Name)

	// YAML frontmatter
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", titleCaseName)
	fmt.Fprintf(&sb, "description: %s\n", preset.Description)
	sb.WriteString("keep-coding-instructions: true\n")
	sb.WriteString("---\n")

	// Append Notes after frontmatter
	if preset.Notes != "" {
		sb.WriteString("\n")
		sb.WriteString(preset.Notes)
	}

	return sb.String()
}

// toTitleCase converts a persona name to TitleCase format.
// Examples: "argentino" -> "Argentino", "tony-stark" -> "TonyStark"
func toTitleCase(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			parts[i] = string(runes)
		}
	}
	return strings.Join(parts, "")
}

// Package persona manages the Layer2 persona preset system.
// Presets are embedded YAML files that define tone, language, and communication style.
// The embed.FS is provided by the caller (assets.PersonaFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package persona

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
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

// v1PersonaScopeGuardrail is retained only while the active V1 renderer still
// consumes legacy Notes content. Schema V2 renderers never use it.
const v1PersonaScopeGuardrail = `<!-- gentle-ai:persona-scope -->
## Persona Scope (CRITICAL)

The active persona controls ONLY direct replies to the user.

It MUST NOT control generated artifacts:
- code, identifiers, variable names, function names, comments
- UI labels, UI copy, error messages, accessibility strings
- documentation, README files, commit messages, PR descriptions
- configuration, prompts, SDD artifacts, or string literals

Generated technical artifacts default to English unless the user explicitly requests another artifact language or the existing project convention requires one. Preserve Jarvis naming: Hive, jarvis CLI, .jarvis/skill-registry.md, and .jarvis/skills/<skill>/SKILL.md. Do not introduce external assistant-memory backend wording into product/generated Jarvis artifacts.

## Response Length Contract

Default to short answers. Start with the minimum useful response, then expand only when the user asks or the task genuinely requires it.

## Language Rules

Match the user's current language in direct replies only. Do not let persona language, slang, tone, or regional voice leak into code, docs, configs, prompts, UI text, comments, identifiers, or other generated artifacts.

## When Asking Questions

Ask at most one question at a time. After asking it, STOP and wait for the user's response.
<!-- /gentle-ai:persona-scope -->`

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

// ListPresetsV2 returns all validated schema-v2 built-in presentation profiles.
func ListPresetsV2(fsys fs.FS) ([]PresetV2, error) {
	if fsys == nil {
		return nil, nil
	}

	names := listPresetV2Names(fsys)
	presets := make([]PresetV2, 0, len(names))

	for _, name := range names {
		resolved, err := ResolvePresetV2(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("load schema v2 preset %q: %w", name, err)
		}
		if resolved.Source != PresetSourceBuiltin {
			return nil, fmt.Errorf("schema v2 preset %q is not a built-in preset", NormalizeSlug(name))
		}
		presets = append(presets, *resolved.Preset)
	}

	return presets, nil
}

// listPresetNames returns the names of all built-in presets by scanning the provided embed.FS.
// Template files (*.tmpl) are excluded.
func listPresetNames(fsys fs.FS) []string {
	return listPresetNamesInDir(fsys, "embed/personas")
}

func listPresetV2Names(fsys fs.FS) []string {
	return listPresetNamesInDir(fsys, "embed/personas")
}

func listPresetNamesInDir(fsys fs.FS, directory string) []string {
	namesSet := make(map[string]struct{})
	_ = fs.WalkDir(fsys, directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.ToSlash(filepath.Dir(path)) != directory {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yaml.tmpl") {
			return nil
		}
		// Extract name from filename (strip directory and .yaml extension)
		base := d.Name()
		name := strings.TrimSuffix(base, ".yaml")
		if err := validatePresetSlug(name); err != nil {
			return nil
		}
		namesSet[name] = struct{}{}
		return nil
	})
	names := make([]string, 0, len(namesSet))
	for name := range namesSet {
		names = append(names, name)
	}
	sort.Strings(names)
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
	notes := withoutPersonaScopeGuardrail(preset.Notes)

	fmt.Fprintf(&sb, "## Persona: %s\n\n", preset.DisplayName)
	fmt.Fprintf(&sb, "%s\n\n", preset.Description)

	if !notesHasStructuredSection(notes, "Tone") {
		sb.WriteString("### Tone\n")
		fmt.Fprintf(&sb, "- **Formality**: %s\n", preset.Tone.Formality)
		fmt.Fprintf(&sb, "- **Directness**: %s\n", preset.Tone.Directness)
		fmt.Fprintf(&sb, "- **Humor**: %s\n", preset.Tone.Humor)
		fmt.Fprintf(&sb, "- **Language**: %s\n\n", preset.Tone.Language)
	}

	if !notesHasStructuredSection(notes, "Communication Style") {
		sb.WriteString("### Communication Style\n")
		if preset.CommunicationStyle.ShowAlternatives {
			sb.WriteString("- Always propose alternatives with tradeoffs\n")
		}
		if preset.CommunicationStyle.ChallengeAssumptions {
			sb.WriteString("- Challenge user assumptions when incorrect\n")
		}
		fmt.Fprintf(&sb, "- Verbosity: %s\n\n", preset.CommunicationStyle.Verbosity)
	} else if hasStructuredCommunicationBehavior(preset) {
		sb.WriteString("### Behavioral Rules\n")
		writeStructuredCommunicationBehavior(&sb, preset)
		sb.WriteString("\n")
	}

	if len(preset.CharacteristicPhrases.Greetings) > 0 && !notesHasStructuredSection(notes, "Characteristic Phrases") {
		sb.WriteString("### Characteristic Phrases\n")
		sb.WriteString("**Greetings**: " + strings.Join(preset.CharacteristicPhrases.Greetings, " / ") + "\n")
		sb.WriteString("**Confirmations**: " + strings.Join(preset.CharacteristicPhrases.Confirmations, " / ") + "\n")
		if len(preset.CharacteristicPhrases.SignOffs) > 0 {
			sb.WriteString("**Sign-off**: " + preset.CharacteristicPhrases.SignOffs[0] + "\n")
		}
	}

	if notes != "" {
		sb.WriteString("\n---\n")
		sb.WriteString(notes)
	}

	sb.WriteString("\n\n")
	sb.WriteString(v1PersonaScopeGuardrail)

	return sb.String()
}

// RenderOutputStyle renders output-style markdown with YAML frontmatter for Claude Code.
// Format: ---\nname: TitleCase\ndescription: ...\nkeep-coding-instructions: true\n---\n{Notes}
// Implements SPEC-002.
func RenderOutputStyle(preset *Preset) string {
	var sb strings.Builder
	notes := withoutPersonaScopeGuardrail(preset.Notes)

	// Convert name to TitleCase (e.g., "tony-stark" -> "TonyStark")
	titleCaseName := toTitleCase(preset.Name)

	// YAML frontmatter
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", titleCaseName)
	fmt.Fprintf(&sb, "description: %s\n", preset.Description)
	sb.WriteString("keep-coding-instructions: true\n")
	sb.WriteString("---\n")

	// Append Notes after frontmatter, then append fixed guardrails last so
	// preset-provided instructions cannot override renderer-owned scope rules.
	sb.WriteString("\n")
	if notes != "" {
		sb.WriteString(notes)
		sb.WriteString("\n\n")
	}
	sb.WriteString(v1PersonaScopeGuardrail)

	return sb.String()
}

// RenderLayer2V2 renders a schema-v2 profile as presentation only. It is
// intentionally separate from RenderLayer2 until V2 activation.
func RenderLayer2V2(preset *PresetV2) string {
	return renderPresentationV2(preset, false)
}

// RenderOutputStyleV2 renders schema-v2 presentation for Claude Code while
// retaining Claude's Layer1 coding instructions.
func RenderOutputStyleV2(preset *PresetV2) string {
	return renderPresentationV2(preset, true)
}

func renderPresentationV2(preset *PresetV2, outputStyle bool) string {
	var sb strings.Builder
	if outputStyle {
		sb.WriteString("---\n")
		fmt.Fprintf(&sb, "name: %s\n", toTitleCase(preset.Name))
		sb.WriteString("description: Jarvis presentation profile\n")
		sb.WriteString("keep-coding-instructions: true\n---\n\n")
	}

	fmt.Fprintf(&sb, "## Persona: %s\n\n", toTitleCase(preset.Name))
	sb.WriteString("### Presentation\n")
	fmt.Fprintf(&sb, "- Language: %s\n", presentationLanguageV2(preset.Presentation.Language))
	fmt.Fprintf(&sb, "- Register: %s\n", presentationRegisterV2(preset.Presentation.Register))
	fmt.Fprintf(&sb, "- Vocabulary: %s\n", preset.Presentation.Vocabulary)
	fmt.Fprintf(&sb, "- Cadence: %s\n", preset.Presentation.Cadence)
	fmt.Fprintf(&sb, "- Humor: %s\n", preset.Presentation.Humor)
	fmt.Fprintf(&sb, "- Emotional range: %s\n", preset.Presentation.EmotionalRange)
	fmt.Fprintf(&sb, "- Verbosity: %s\n", preset.Presentation.Verbosity)
	fmt.Fprintf(&sb, "- Formatting: %s\n", preset.Presentation.Formatting)
	fmt.Fprintf(&sb, "- Teaching metaphors: %s\n", preset.Presentation.TeachingMetaphors)
	fmt.Fprintf(&sb, "- Examples: %s\n", preset.Presentation.Examples)
	fmt.Fprintf(&sb, "- Address pack: %s\n", preset.Presentation.AddressPack)
	fmt.Fprintf(&sb, "- Phrase pack: %s\n", preset.Presentation.PhrasePack)
	fmt.Fprintf(&sb, "- Anti-caricature: %s\n", preset.Presentation.AntiCaricature)
	return sb.String()
}

func presentationLanguageV2(language string) string {
	if language == "es-rioplatense" {
		return "Rioplatense Spanish (voseo)"
	}
	return language
}

func presentationRegisterV2(register string) string {
	if register == "warm-direct" {
		return "warm, energetic, and direct"
	}
	return register
}

func hasStructuredCommunicationBehavior(preset *Preset) bool {
	return preset.CommunicationStyle.ShowAlternatives ||
		preset.CommunicationStyle.ChallengeAssumptions ||
		preset.CommunicationStyle.Verbosity != ""
}

func writeStructuredCommunicationBehavior(sb *strings.Builder, preset *Preset) {
	if preset.CommunicationStyle.ShowAlternatives {
		sb.WriteString("- Always propose alternatives with tradeoffs\n")
	}
	if preset.CommunicationStyle.ChallengeAssumptions {
		sb.WriteString("- Challenge user assumptions when incorrect\n")
	}
	if preset.CommunicationStyle.Verbosity != "" {
		fmt.Fprintf(sb, "- Verbosity: %s\n", preset.CommunicationStyle.Verbosity)
	}
}

func notesHasStructuredSection(notes, section string) bool {
	return strings.Contains(notes, "## "+section)
}

func withoutPersonaScopeGuardrail(notes string) string {
	notes = stripMarkedPersonaScopeBlocks(notes)
	notes = strings.ReplaceAll(notes, v1PersonaScopeGuardrail, "")
	for strings.Contains(notes, "## Persona Scope (CRITICAL)") {
		notes = stripMarkdownSection(notes, "## Persona Scope (CRITICAL)")
	}
	return strings.TrimSpace(notes)
}

func stripMarkedPersonaScopeBlocks(content string) string {
	const startMarker = "<!-- gentle-ai:persona-scope -->"
	const endMarker = "<!-- /gentle-ai:persona-scope -->"

	for {
		start := strings.Index(content, startMarker)
		if start == -1 {
			return content
		}

		endSearchStart := start + len(startMarker)
		end := strings.Index(content[endSearchStart:], endMarker)
		if end == -1 {
			return content
		}

		end = endSearchStart + end + len(endMarker)
		content = content[:start] + content[end:]
	}
}

func stripMarkdownSection(content, heading string) string {
	start := strings.Index(content, heading)
	if start == -1 {
		return content
	}

	sectionEnd := len(content)
	searchStart := start + len(heading)
	if next := strings.Index(content[searchStart:], "\n## "); next != -1 {
		sectionEnd = searchStart + next
	}

	return content[:start] + content[sectionEnd:]
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

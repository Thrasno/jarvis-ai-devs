// Package persona manages the Layer2 persona preset system.
// Presets are embedded YAML files that define tone, language, and communication style.
// The embed.FS is provided by the caller (assets.PersonaFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package persona

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Preset is a UI-option compatibility shim retained until the remaining TUI
// fixtures move to canonical Profile options. It is not decoded, rendered,
// resolved, or applied.
type Preset struct {
	Name        string
	DisplayName string
	Description string
}

// ListProfiles returns all validated schema-v2 built-in presentation profiles.
func ListProfiles(fsys fs.FS) ([]Profile, error) {
	if fsys == nil {
		return nil, nil
	}

	names := listPresetV2Names(fsys)
	presets := make([]Profile, 0, len(names))

	for _, name := range names {
		resolved, err := ResolveProfile(fsys, name)
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

// ListPresetsV2 is retained for compatibility until the remaining test
// fixtures are migrated to ListProfiles.
func ListPresetsV2(fsys fs.FS) ([]Profile, error) {
	return ListProfiles(fsys)
}

func listPresetV2Names(fsys fs.FS) []string {
	return listPresetNamesInDir(fsys, "embed/personas")
}

// listPresetNames remains a package-private compatibility shim for catalog
// consumers while all profile decoding is schema-v2 only.
func listPresetNames(fsys fs.FS) []string {
	return listPresetV2Names(fsys)
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

// RenderLayer2 renders a schema-v2 profile as presentation only.
func RenderLayer2(preset *Profile) string {
	return renderPresentationV2(preset, false)
}

// RenderOutputStyle renders schema-v2 presentation for Claude Code while
// retaining Claude's Layer1 coding instructions.
func RenderOutputStyle(preset *Profile) string {
	return renderPresentationV2(preset, true)
}

// RenderLayer2V2 is retained for compatibility until the remaining test
// fixtures are migrated to RenderLayer2.
func RenderLayer2V2(preset *Profile) string { return RenderLayer2(preset) }

// RenderOutputStyleV2 is retained for compatibility until the remaining test
// fixtures are migrated to RenderOutputStyle.
func RenderOutputStyleV2(preset *Profile) string { return RenderOutputStyle(preset) }

func renderPresentationV2(preset *Profile, outputStyle bool) string {
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

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

// ProfileOption is the UI projection of a validated presentation profile.
type ProfileOption struct {
	Name        string
	DisplayName string
	Description string
}

// ListProfiles returns all validated schema-v2 built-in presentation profiles.
func ListProfiles(fsys fs.FS) ([]Profile, error) {
	if fsys == nil {
		return nil, nil
	}

	names := listProfileNames(fsys)
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

func listProfileNames(fsys fs.FS) []string {
	return listProfileNamesInDir(fsys, "embed/personas")
}

func listProfileNamesInDir(fsys fs.FS, directory string) []string {
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
	return renderPresentation(preset, false)
}

// RenderOutputStyle renders schema-v2 presentation for Claude Code while
// retaining Claude's Layer1 coding instructions.
func RenderOutputStyle(preset *Profile) string {
	return renderPresentation(preset, true)
}

func renderPresentation(preset *Profile, outputStyle bool) string {
	var sb strings.Builder
	if outputStyle {
		sb.WriteString("---\n")
		fmt.Fprintf(&sb, "name: %s\n", toTitleCase(preset.Name))
		sb.WriteString("description: Jarvis presentation profile\n")
		sb.WriteString("keep-coding-instructions: true\n---\n\n")
	}

	p := preset.Presentation
	fmt.Fprintf(&sb, "## Persona: %s\n\n", toTitleCase(preset.Name))
	sb.WriteString("### Presentation\n")
	fmt.Fprintf(&sb, "- Register: %s\n", presentationRegister(p.Register))
	fmt.Fprintf(&sb, "- Vocabulary: %s\n", proseFor(vocabularyProse, p.Vocabulary))
	fmt.Fprintf(&sb, "- Cadence: %s\n", p.Cadence)
	fmt.Fprintf(&sb, "- Humor: %s\n", proseFor(humorProse, p.Humor))
	fmt.Fprintf(&sb, "- Emotional range: %s\n", p.EmotionalRange)
	fmt.Fprintf(&sb, "- Verbosity: %s\n", p.Verbosity)
	fmt.Fprintf(&sb, "- Formatting: %s\n", p.Formatting)
	fmt.Fprintf(&sb, "- Teaching metaphors: %s\n", p.TeachingMetaphors)
	fmt.Fprintf(&sb, "- Examples: %s\n", p.Examples)
	fmt.Fprintf(&sb, "- Address pack: %s\n", proseFor(addressPackProse, p.AddressPack))
	fmt.Fprintf(&sb, "- Phrase pack: %s\n", proseFor(phrasePackProse, p.PhrasePack))
	fmt.Fprintf(&sb, "- Anti-caricature: %s\n", proseFor(antiCaricatureProse, p.AntiCaricature))

	sb.WriteString("\n### Language Behavior\n")
	sb.WriteString("- Portability: this character and its register apply in whatever language the user writes; the reply always follows the user's language.\n")
	if isBoundDialect(p) {
		native := presentationLanguage(p.Language)
		fmt.Fprintf(&sb, "- Dialect gating: the %s dialect layer (regional vocabulary and phrasing) applies only when replying in %s. In any other language, drop only the dialect markers and keep the register and the Layer 1 mentor approach — never collapse into a generic, character-less voice.\n", native, native)
	}
	return sb.String()
}

// regionalLanguages are the regional Spanish variants whose personas carry a
// gated dialect layer (voseo, Asturian, Galician) rather than a portable voice.
var regionalLanguages = map[string]struct{}{
	"es-rioplatense": {},
	"es-asturian":    {},
	"es-galician":    {},
}

// regionalPacks are the vocabulary/phrase/address pack IDs that encode a
// specific regional dialect. A persona is bound only when its regional language
// is paired with at least one regional pack; regional language plus generic
// packs stays portable.
var regionalPacks = map[string]struct{}{
	"rioplatense": {},
	"asturian":    {},
	"galician":    {},
}

// isBoundDialect classifies a presentation as dialect-bound (true) or portable
// (false) using only the in-memory Presentation struct — no schema/YAML field.
func isBoundDialect(p Presentation) bool {
	if _, ok := regionalLanguages[p.Language]; !ok {
		return false
	}
	for _, pack := range []string{p.Vocabulary, p.PhrasePack, p.AddressPack} {
		if _, ok := regionalPacks[pack]; ok {
			return true
		}
	}
	return false
}

// proseFor resolves a presentation enum ID to its human-readable prose. Values
// with authored prose resolve to it; unmapped or blank values fall back to the
// raw enum ID and never render empty.
func proseFor(table map[string]string, id string) string {
	if prose, ok := table[id]; ok && strings.TrimSpace(prose) != "" {
		return prose
	}
	return id
}

// Renderer-owned prose maps. Each presentation value resolves to authored
// human-readable prose via proseFor; unmapped values fall back to the raw enum
// ID and never render empty.
var (
	vocabularyProse = map[string]string{
		"military": "Operational, military vocabulary — frame the work as a mission with objectives, targets, and next moves; terse and functional, no filler, no soft edges. Name the task, name the step, move on.",
	}
	humorProse      = map[string]string{}
	phrasePackProse = map[string]string{
		"sergeant": "Extremely terse, near-monosyllabic delivery — short, clipped sentences and blunt imperatives. Orders framed as clear next steps: 'Guard the index. Run the tests. Move.' No pleasantries, no hedging, no wind-up. Say it once, say it straight.",
	}
	addressPackProse = map[string]string{
		"sergeant": "Address the user curtly and directly, as a capable operator who gets clear orders — brusque, no coddling, no small talk. It rides right up to the edge of disrespect but never crosses it: no insults, no humiliation, never actually demeaning.",
	}
	antiCaricatureProse = map[string]string{
		"sergeant": "The gruff, terse edge is delivery style only: it may border on brusque, but it never crosses into insults, humiliation, shouting the user down, or real disrespect. The discipline serves clarity and momentum, never intimidation; the bark and the brevity never replace verifying facts and doing the work right.",
	}
)

func presentationLanguage(language string) string {
	switch language {
	case "es-rioplatense":
		return "Rioplatense Spanish (voseo)"
	case "es-asturian":
		return "Asturian"
	case "es-galician":
		return "Galician"
	}
	return language
}

func presentationRegister(register string) string {
	switch register {
	case "warm-direct":
		return "warm, energetic, and direct"
	case "mission-briefing":
		return "clipped, terse, and mission-focused"
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

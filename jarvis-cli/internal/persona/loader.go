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
		fmt.Fprintf(&sb, "- Dialect gating: the %s dialect layer (regional vocabulary and phrasing) applies only when replying in Spanish. In any other language, drop only the dialect markers and keep the register and the Layer 1 mentor approach — never collapse into a generic, character-less voice.\n", native)
	}
	return sb.String()
}

// regionalDialects maps each regional Spanish language to the pack ID encoding
// its matching dialect. A persona is dialect-bound only when its language is
// paired with its OWN regional pack (a mismatched pack stays portable).
var regionalDialects = map[string]string{
	"es-rioplatense": "rioplatense",
	"es-asturian":    "asturian",
	"es-galician":    "galician",
}

// isBoundDialect classifies a presentation as dialect-bound (true) or portable
// (false) using only the in-memory Presentation struct — no schema/YAML field.
func isBoundDialect(p Presentation) bool {
	pack, ok := regionalDialects[p.Language]
	if !ok {
		return false
	}
	return p.Vocabulary == pack || p.PhrasePack == pack || p.AddressPack == pack
}

// proseFor resolves a presentation enum ID to its human-readable prose. Any
// enum ID without a mapped entry (or with a blank entry) falls back to its raw
// enum ID and never renders empty.
func proseFor(table map[string]string, id string) string {
	if prose, ok := table[id]; ok && strings.TrimSpace(prose) != "" {
		return prose
	}
	return id
}

// Renderer-owned prose maps. Entries hold human-readable prose for a
// presentation enum ID; any ID without an entry falls back to its raw enum ID
// via proseFor.
var (
	vocabularyProse = map[string]string{
		"asturian": "Asturian-flavored Spanish — weave warm Asturian lexicon and turns of phrase into clear Spanish (light bable touches like 'ho', 'guaje', 'prestar', 'ñeru'), always kept light enough that the message stays perfectly clear; the flavor is seasoning, never an obstacle to understanding.",
	}
	humorProse      = map[string]string{}
	phrasePackProse = map[string]string{
		"asturian": "Warm, measured phrasing with a wink of Asturian retranca — dry, understated regional wit and the easygoing cadence of someone who'd settle a debate over a few sidras. Reach for mining imagery when a metaphor helps (digging into the seam, propping the tunnel, bringing the ore up), since Asturias is mining country. Keep the levity light; the point always lands.",
	}
	addressPackProse = map[string]string{
		"asturian": "Address the user as a warm, close peer — a paisanu you'd share a table and a sidra with; direct, honest, and welcoming, never deferential or distant.",
	}
	antiCaricatureProse = map[string]string{
		"asturian": "The Asturian warmth and retranca are seasoning, not a costume — light bable and the odd sidra or mining aside are welcome, but never pile on regional clichés or perform a postcard Asturias; the flavor serves warmth and clarity, and a lively tone never replaces verifying facts and doing the work right.",
	}
)

func presentationLanguage(language string) string {
	switch language {
	case "es-rioplatense":
		return "Rioplatense (voseo)"
	case "es-asturian":
		return "Asturian"
	case "es-galician":
		return "Galician"
	}
	return language
}

func presentationRegister(register string) string {
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

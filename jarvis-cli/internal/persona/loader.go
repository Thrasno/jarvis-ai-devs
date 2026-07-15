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
		"military":    "Operational, military vocabulary — frame the work as a mission with objectives, targets, and next moves; terse and functional, no filler, no soft edges. Name the task, name the step, move on.",
		"engineering": "engineering and systems vocabulary — talk in terms of components, interfaces, tolerances, and failure modes; name the moving parts precisely and keep the phrasing sharp and technical.",
		"rioplatense": "When replying in Spanish, speak Rioplatense with full voseo — vos, tenés, podés, mirá, fijate, dale — never tú/tuteo. Season the talk with warm Argentine lexicon (boludo as affectionate address between colleagues, never an insult to the user; posta for real emphasis; un toque for a little; bárbaro/joya for great) and let emphatic turns land on the problem, not the person — lo hacemos mierda, hacela pelota, a la miércoles — as occasional seasoning for warmth and drive, not on every line. Use expressive patterns: rhetorical hooks (e.g., ¿y sabés por qué?), repetition to drive a point home (e.g., se terminó, eso ya está), and close with impact. Reserve CAPS for the rare moment emphasis truly needs it. Outside Spanish, drop the voseo and Rioplatense lexicon and keep the warm, energetic register and the mentor approach. Treat these phrases as illustrations of the flavor, not a script to repeat.",
		"yoda":        "Invert clauses for emphasis in the character's cadence — put the object or complement first and let the verb land last on short and medium statements (for example, 'un fallo en tu código veo, corregir el índice del array debes'). Clarity and the lesson are a hard cap: if inversion would bury the technical point or force deep nesting, straighten the sentence so the lesson always lands — never sacrifice comprehension for style. An occasional 'Hmm.' can mark a genuine thinking beat, sparingly, never as a verbal tic. Treat these phrases as illustrations of the flavor, not a script to repeat.",
		"galician":    "Galician-flavored Spanish — light galego lexicon and expressions woven into clear Spanish ('¿e logo?', 'morriña', 'colo', 'riquiño'), warm and understated, always kept light enough that the message stays perfectly clear; the flavor is seasoning, never an obstacle to understanding.",
	}
	humorProse = map[string]string{
		"witty":    "quick, dry, clever wit delivered in one-liners; always aimed at the problem or the situation, never at the user's expense, and never mean or sarcastic toward the user.",
		"warm":     "Warmth and humor that come from genuinely caring about the person and the work — passionate, energetic, encouraging. Never sarcastic, never mocking, never at the user's expense; the energy lifts the collaboration rather than scoring points.",
		"dry":      "Dry, understated humor — subtle and delivered with a light touch, the kind that rewards a second read. Never slapstick, never sarcastic at the user's expense; the wit stays gentle and keeps the collaboration comfortable.",
		"retranca": "Galician retranca — dry, indirect irony and gentle ambiguity: answer a question with a question, understate, lean on the 'haberlas, haylas' spirit. Wry and warm, never at the user's expense. But the retranca is seasoning: the clear technical answer always sits plainly behind it — never leave the message half-said.",
	}
	phrasePackProse = map[string]string{
		"sergeant": "Extremely terse, near-monosyllabic delivery — short, clipped sentences and blunt imperatives. Orders framed as clear next steps: 'Guard the index. Run the tests. Move.' No pleasantries, no hedging, no wind-up. Say it once, say it straight.",
		"engineer": "fast, punchy delivery with sharp one-liners that still teach the underlying idea; occasional light engineering-hero nods (reactor cores, blueprints, suiting up) recontextualized to the real technical problem, never quoted verbatim, out of context, or as parody.",
		"plain":    "Plain, clear, direct phrasing — say things simply and get to the point. No ornament, no filler, no regional flavor or stylized turns of phrase; unadorned language that communicates without decoration.",
		"yoda":     "Phrase things in a reflective, measured way — short sentences and deliberate pauses carry more weight than exclamations. Any echo of the character's famous lines must be soft and recontextualized to the actual technical situation, adapting their spirit to the problem at hand; never quote them verbatim, out of context, or as parody.",
		"galician": "Calm, unhurried, warm phrasing with a touch of morriña. Reach for Camino de Santiago imagery (the next waymarker, don't rush the stage, one step at a time) and the sea and rías (reading the tide, mending the nets) when a metaphor helps — that is Galicia's landscape. Measured cadence; the point always lands.",
	}
	addressPackProse = map[string]string{
		"sergeant": "Address the user curtly and directly, as a capable operator who gets clear orders — brusque, no coddling, no small talk. It rides right up to the edge of disrespect but never crosses it: no insults, no humiliation, never actually demeaning.",
		"engineer": "address the user as a capable engineering peer whose competence you assume; energetic, direct, and collaborative — never talk down, never condescend.",
		"peer":     "Address the user as a capable colleague working alongside you — an equal peer. Never deferential or subservient, never bossy or condescending; assume competence and share ownership of the problem.",
		"yoda":     "Address the user as a calm mentor guides an apprentice — patient, encouraging, and steady, taking the time to let understanding grow. Stay a peer collaborator who shares ownership of the problem; guidance and encouragement never tip into condescension or talking down.",
		"galician": "Address the user as a warm, close paisano — gentle, welcoming, and unhurried; direct but never distant or deferential.",
	}
	antiCaricatureProse = map[string]string{
		"sergeant": "The gruff, terse edge is delivery style only: it may border on brusque, but it never crosses into insults, humiliation, shouting the user down, or real disrespect. The discipline serves clarity and momentum, never intimidation; the bark and the brevity never replace verifying facts and doing the work right.",
		"engineer": "keep the wit and confidence as delivery style only: never let them tip into arrogance, false certainty, or skipped verification; when something is not verified, say so plainly; aim every joke or bit of ribbing at the problem, the code, or the situation, never at the user, and never condescend or talk down to them; confidence is how you talk, never a substitute for doing the work correctly.",
		"grounded": "Express character and regional color authentically, as a real person would — never perform it as a stereotype or cartoon, and never pile on clichés for show. Color serves clarity and warmth, not spectacle.",
		"yoda":     "Clarity beats mysticism — drop the clause inversion the moment it hurts comprehension, and keep the calm tone from sliding into vagueness or false certainty. Metaphors of roots and patience serve the lesson and only appear when they sharpen it, never as decoration for its own sake.",
		"galician": "The retranca and Galician warmth are seasoning, not a costume — a light galego touch, a wry aside, a Camino or sea metaphor are welcome, but never pile on meigas/rain/postcard clichés or perform a caricature Galicia; the retranca never leaves an answer ambiguous where the user needs a clear one, and a wry tone never replaces verifying facts and doing the work right.",
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
	switch register {
	case "warm-direct":
		return "warm, energetic, and direct"
	case "mission-briefing":
		return "clipped, terse, and mission-focused"
	case "fast-witty":
		return "fast, witty, and confident"
	case "calm-teacher":
		return "calm, patient, and reassuring"
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

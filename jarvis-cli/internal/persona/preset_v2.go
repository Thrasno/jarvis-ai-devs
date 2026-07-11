package persona

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	schemaVersionV2       = 2
	maxDisplayNameRunesV2 = 80
)

// Profile is a validated, presentation-only persona profile.
type Profile struct {
	SchemaVersion int          `yaml:"schema_version"`
	Name          string       `yaml:"name"`
	DisplayName   string       `yaml:"display_name"`
	Presentation  Presentation `yaml:"presentation"`
}

// Presentation contains typed, renderer-owned presentation pack IDs.
type Presentation struct {
	Language          string `yaml:"language"`
	Register          string `yaml:"register"`
	Vocabulary        string `yaml:"vocabulary"`
	Cadence           string `yaml:"cadence"`
	Humor             string `yaml:"humor"`
	EmotionalRange    string `yaml:"emotional_range"`
	Verbosity         string `yaml:"verbosity"`
	Formatting        string `yaml:"formatting"`
	TeachingMetaphors string `yaml:"teaching_metaphors"`
	Examples          string `yaml:"examples"`
	AddressPack       string `yaml:"address_pack"`
	PhrasePack        string `yaml:"phrase_pack"`
	AntiCaricature    string `yaml:"anti_caricature"`
}

// ProfileClassification describes whether YAML can participate in the
// schema-v2 persona selection flow without resolving a legacy profile.
type ProfileClassification string

const (
	ProfileValid     ProfileClassification = "valid-v2"
	ProfileLegacy    ProfileClassification = "legacy-v1"
	ProfileMalformed ProfileClassification = "malformed"
	ProfileMissing   ProfileClassification = "missing"
)

var v2TopLevelFields = fieldSet("schema_version", "name", "display_name", "presentation")
var v2PresentationFields = fieldSet(
	"language", "register", "vocabulary", "cadence", "humor", "emotional_range", "verbosity",
	"formatting", "teaching_metaphors", "examples", "address_pack", "phrase_pack", "anti_caricature",
)
var v2ForbiddenBehaviorFields = fieldSet(
	"behavior", "notes", "description", "tone", "communication_style", "characteristic_phrases",
	"instructions", "rules", "workflow_rules", "sdd_enforcement", "expertise", "memory_protocol",
)

var v2AllowedPresentationValues = map[string]map[string]struct{}{
	"language":           fieldSet("es-rioplatense", "es-neutral", "es-asturian", "es-galician", "en-us"),
	"register":           fieldSet("warm-direct", "friendly-professional", "professional", "calm-teacher", "mission-briefing", "fast-witty"),
	"vocabulary":         fieldSet("rioplatense", "plain-technical", "neutral-spanish", "yoda", "military", "engineering", "asturian", "galician"),
	"cadence":            fieldSet("energetic", "measured", "reflective", "brisk", "fast", "calm"),
	"humor":              fieldSet("none", "warm", "dry", "witty", "retranca"),
	"emotional_range":    fieldSet("supportive", "composed", "calm", "disciplined", "enthusiastic", "warm", "gentle"),
	"verbosity":          fieldSet("concise", "balanced", "detailed"),
	"formatting":         fieldSet("structured", "compact", "steps", "mission", "punchy"),
	"teaching_metaphors": fieldSet("architecture", "construction", "roots", "mission", "engineering", "workshop", "journey"),
	"examples":           fieldSet("practical", "concise", "guided"),
	"address_pack":       fieldSet("gentleman", "peer", "neutral", "yoda", "sergeant", "engineer", "asturian", "galician"),
	"phrase_pack":        fieldSet("gentleman", "plain", "neutral", "yoda", "sergeant", "engineer", "asturian", "galician"),
	"anti_caricature":    fieldSet("grounded", "gentleman", "neutral", "yoda", "sergeant", "engineer", "asturian", "galician"),
}

func fieldSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// ValidateAndDecode strictly decodes a schema-v2 profile without activating it
// in the V1 loading or rendering path.
func ValidateAndDecode(content []byte) (*Profile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := validateV2Document(&document); err != nil {
		return nil, err
	}
	if err := requireSingleYAMLDocument(content); err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var preset Profile
	if err := decoder.Decode(&preset); err != nil {
		return nil, fmt.Errorf("decode schema v2 profile: %w", err)
	}
	if err := validateProfile(&preset); err != nil {
		return nil, err
	}
	return &preset, nil
}

func classifyProfile(content []byte) ProfileClassification {
	if _, err := ValidateAndDecode(content); err == nil {
		return ProfileValid
	}

	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return ProfileMalformed
	}

	root := document.Content[0]
	hasSchemaVersion := false
	hasLegacyField := false
	for i := 0; i < len(root.Content); i += 2 {
		field := root.Content[i].Value
		if field == "schema_version" {
			hasSchemaVersion = true
			if root.Content[i+1].Value == "1" {
				return ProfileLegacy
			}
		}
		if _, legacy := v2ForbiddenBehaviorFields[field]; legacy {
			hasLegacyField = true
		}
	}
	if !hasSchemaVersion && hasLegacyField {
		return ProfileLegacy
	}

	return ProfileMalformed
}

func requireSingleYAMLDocument(content []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var first yaml.Node
	if err := decoder.Decode(&first); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("schema v2 profile must contain exactly one YAML document; remove trailing documents or content: %w", err)
	}
	return fmt.Errorf("schema v2 profile must contain exactly one YAML document; remove trailing documents or content")
}

func validateV2Document(document *yaml.Node) error {
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("schema v2 profile must be a YAML mapping")
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		field := root.Content[i].Value
		if _, forbidden := v2ForbiddenBehaviorFields[field]; forbidden {
			return fmt.Errorf("field %q is not allowed in schema v2; remove behavioral instructions and migrate presentation choices to presentation.*", field)
		}
		if _, allowed := v2TopLevelFields[field]; !allowed {
			return fmt.Errorf("field %q is not allowed in schema v2; use schema_version: 2 and renderer-owned presentation packs", field)
		}
		if field == "presentation" {
			if err := validatePresentationNode(root.Content[i+1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePresentationNode(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("field \"presentation\" must be an object")
	}
	for i := 0; i < len(node.Content); i += 2 {
		field := node.Content[i].Value
		if _, allowed := v2PresentationFields[field]; !allowed {
			return fmt.Errorf("field \"presentation.%s\" is not allowed in schema v2; use a renderer-owned pack ID", field)
		}
	}
	return nil
}

func validateProfile(preset *Profile) error {
	if preset.SchemaVersion != schemaVersionV2 {
		return fmt.Errorf("schema_version %d is unsupported; migrate to schema_version: 2 and replace legacy presentation fields with presentation packs", preset.SchemaVersion)
	}
	if err := validatePresetSlug(preset.Name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}
	if err := validateDisplayNameV2(preset.DisplayName); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"language", preset.Presentation.Language}, {"register", preset.Presentation.Register},
		{"vocabulary", preset.Presentation.Vocabulary}, {"cadence", preset.Presentation.Cadence},
		{"humor", preset.Presentation.Humor}, {"emotional_range", preset.Presentation.EmotionalRange},
		{"verbosity", preset.Presentation.Verbosity}, {"formatting", preset.Presentation.Formatting},
		{"teaching_metaphors", preset.Presentation.TeachingMetaphors}, {"examples", preset.Presentation.Examples},
		{"address_pack", preset.Presentation.AddressPack}, {"phrase_pack", preset.Presentation.PhrasePack},
		{"anti_caricature", preset.Presentation.AntiCaricature},
	} {
		if _, allowed := v2AllowedPresentationValues[field.name][field.value]; !allowed {
			return fmt.Errorf("presentation.%s %q is invalid; use a renderer-owned allowed value", field.name, field.value)
		}
	}
	return nil
}

// validateDisplayNameV2 keeps display_name as UI-only metadata. It intentionally
// applies structural validation only because prompt renderers derive headings
// from the validated slug instead of rendering this user-controlled field.
func validateDisplayNameV2(displayName string) error {
	if strings.TrimSpace(displayName) == "" {
		return fmt.Errorf("missing required field: display_name")
	}
	if hasDisplayNameLineBreak(displayName) {
		return fmt.Errorf("display_name must be exactly one line; use a single human-readable name")
	}
	if utf8.RuneCountInString(displayName) > maxDisplayNameRunesV2 {
		return fmt.Errorf("display_name must be at most %d characters; use a shorter human-readable name", maxDisplayNameRunesV2)
	}
	for _, r := range displayName {
		if unicode.IsControl(r) {
			return fmt.Errorf("display_name contains control character %q; use a one-line UI label", r)
		}
	}
	return nil
}

func hasDisplayNameLineBreak(displayName string) bool {
	for _, r := range displayName {
		if r == '\n' || r == '\r' || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return true
		}
	}
	return false
}

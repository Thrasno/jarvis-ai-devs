package persona

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// requiredTopLevelKeys are the fields that every preset YAML must contain.
var requiredTopLevelKeys = []string{
	"name",
	"tone",
	"communication_style",
	"characteristic_phrases",
}

// layer1ProtectedFields are fields that control Layer1 behavior.
// Presets must NOT contain these — they belong to the immutable Layer1 protocol.
var layer1ProtectedFields = []string{
	"behavior",
	"sdd_enforcement",
	"workflow_rules",
	"expertise",
	"memory_protocol",
	"rules",
}

// allowedLanguages are the valid values for tone.language.
var allowedLanguages = []string{
	"es-rioplatense",
	"es-neutro",
	"es-asturian",
	"es-galician",
	"en-us",
	"en-uk",
	"mixed",
}

// validatePresetMap validates a parsed YAML map against the preset schema.
// This is used by both ValidateCustom and internal loading.
func validatePresetMap(raw map[string]any) error {
	// Check required top-level keys
	for _, key := range requiredTopLevelKeys {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing required field: %s", key)
		}
	}

	// Check for protected Layer1 fields
	for _, key := range layer1ProtectedFields {
		if _, ok := raw[key]; ok {
			return fmt.Errorf("layer 1 field %q is not allowed in presets — it belongs to the immutable Layer 1 protocol", key)
		}
	}

	// Validate tone sub-fields
	toneRaw, ok := raw["tone"].(map[string]any)
	if !ok {
		return fmt.Errorf("field 'tone' must be an object")
	}

	for _, subKey := range []string{"formality", "directness", "humor", "language"} {
		if _, ok := toneRaw[subKey]; !ok {
			return fmt.Errorf("missing required field: tone.%s", subKey)
		}
	}

	// Validate tone.language value
	lang, _ := toneRaw["language"].(string)
	if !isAllowedLanguage(lang) {
		return fmt.Errorf("tone.language %q is not allowed; valid values: %s",
			lang, strings.Join(allowedLanguages, ", "))
	}

	// Validate communication_style sub-fields
	styleRaw, ok := raw["communication_style"].(map[string]any)
	if !ok {
		return fmt.Errorf("field 'communication_style' must be an object")
	}
	if _, ok := styleRaw["verbosity"]; !ok {
		return fmt.Errorf("missing required field: communication_style.verbosity")
	}

	// Validate characteristic_phrases sub-fields
	phrasesRaw, ok := raw["characteristic_phrases"].(map[string]any)
	if !ok {
		return fmt.Errorf("field 'characteristic_phrases' must be an object")
	}
	for _, subKey := range []string{"greetings", "confirmations"} {
		if _, ok := phrasesRaw[subKey]; !ok {
			return fmt.Errorf("missing required field: characteristic_phrases.%s", subKey)
		}
	}

	return nil
}

// isAllowedLanguage checks if the given language code is in the allowed list.
func isAllowedLanguage(lang string) bool {
	for _, allowed := range allowedLanguages {
		if lang == allowed {
			return true
		}
	}
	return false
}

// ValidatePreset enforces preset double normalization:
// 1) structural YAML schema
// 2) editorial notes template
func ValidatePreset(content []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	if err := validatePresetMap(raw); err != nil {
		return err
	}

	if notesValue, exists := raw["notes"]; exists {
		notes, ok := notesValue.(string)
		if !ok {
			return fmt.Errorf("field 'notes' must be a string")
		}

		if err := ValidateNotesTemplate(notes); err != nil {
			return fmt.Errorf("invalid notes template: %w", err)
		}
	}

	return nil
}

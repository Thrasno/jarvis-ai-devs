package tui

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"gopkg.in/yaml.v3"
)

type customPresetDraft struct {
	Name        string
	DisplayName string
	YAML        string
}

const maxCustomPresetYAMLBytes = 64 * 1024

var resolveProfileForWizard = persona.ResolveProfile

func resolveWizardPresetSelection(personaFS fs.FS, requestedSlug string, custom *customPresetDraft) (*persona.ResolvedProfile, error) {
	normalized := persona.NormalizeSlug(requestedSlug)
	if normalized == "custom" {
		if custom == nil {
			return nil, fmt.Errorf("custom preset creation requires name and display name")
		}
		return createWizardCustomProfile(personaFS, *custom)
	}

	return resolveProfileForWizard(personaFS, normalized)
}

// validateConfiguredPersonaPresetForV2Selection prevents a configured V1
// profile from being silently replaced by the first schema-v2 catalog option.
func validateConfiguredPersonaPresetForV2Selection(personaFS fs.FS, configuredSlug string) error {
	normalized := persona.NormalizeSlug(configuredSlug)
	if normalized == "" {
		return nil
	}
	if _, err := resolveProfileForWizard(personaFS, normalized); err == nil {
		return nil
	}
	diagnostic, err := persona.ClassifyProfileForMigration(personaFS, normalized)
	if err != nil {
		return fmt.Errorf("classify configured persona preset %q: %w", normalized, err)
	}
	switch diagnostic.Classification {
	case persona.ProfileLegacy:
		return fmt.Errorf("configured persona preset %q is a legacy V1 profile and cannot be used by the schema v2 wizard; migrate it to a schema v2 presentation profile before reconfiguring", normalized)
	case persona.ProfileMissing:
		return fmt.Errorf("configured persona preset %q is stale or deleted and no profile file was found in the schema v2 catalog or user profile location; Recovery: explicitly select an available schema v2 preset, or restore/recreate %q before reconfiguring", normalized, normalized)
	case persona.ProfileMalformed:
		return fmt.Errorf("configured persona preset %q is malformed or unsupported for schema v2 and cannot be used by the schema v2 wizard; Recovery: repair %s as a valid schema v2 presentation profile, or explicitly select an available schema v2 preset before reconfiguring", normalized, diagnostic.FilePath)
	default:
		return fmt.Errorf("configured persona preset %q could not be resolved as a schema v2 profile; Recovery: explicitly select an available schema v2 preset before reconfiguring", normalized)
	}
}

func hasPersistedConfig() (bool, error) {
	path, err := config.ConfigPath()
	if err != nil {
		return false, fmt.Errorf("locate config: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat config: %w", err)
	}
	return true, nil
}

func createWizardCustomProfile(personaFS fs.FS, draft customPresetDraft) (*persona.ResolvedProfile, error) {
	if strings.TrimSpace(draft.YAML) != "" {
		return nil, fmt.Errorf("custom YAML overrides are legacy V1 profiles and cannot be used with schema v2; migrate presentation choices to renderer-owned presentation packs")
	}

	slug, displayName, err := normalizeWizardCustomPresetDraftForCatalog(personaFS, draft, "embed/personas")
	if err != nil {
		return nil, err
	}

	content, err := buildCustomProfileContent(personaFS, slug, displayName)
	if err != nil {
		return nil, err
	}
	if _, err := persona.ValidateAndDecode(content); err != nil {
		return nil, fmt.Errorf("schema v2 validation failed: %w", err)
	}

	savedPath, err := persona.SaveUserPresetFile(slug, content)
	if err != nil {
		return nil, fmt.Errorf("persist schema v2 custom preset %q: %w", slug, err)
	}

	resolved, err := resolveProfileForWizard(personaFS, slug)
	if err != nil {
		return nil, customPresetRecoveryError(slug, savedPath, fmt.Errorf("resolve persisted schema v2 custom preset: %w", err))
	}
	if resolved.Source != persona.PresetSourceUser {
		return nil, customPresetRecoveryError(slug, savedPath, fmt.Errorf("resolved persisted schema v2 custom preset as %q source", resolved.Source))
	}

	return resolved, nil
}

func normalizeWizardCustomPresetDraftForCatalog(personaFS fs.FS, draft customPresetDraft, catalogPath string) (string, string, error) {
	name := strings.TrimSpace(draft.Name)
	if name == "" {
		return "", "", fmt.Errorf("custom preset name is required")
	}
	displayName := strings.TrimSpace(draft.DisplayName)
	if displayName == "" {
		return "", "", fmt.Errorf("custom preset display name is required")
	}

	slug := persona.NormalizeSlug(name)
	if slug == "" || strings.Trim(slug, "-") == "" {
		return "", "", fmt.Errorf("custom preset name resolves to empty slug")
	}
	if slug == "custom" {
		return "", "", fmt.Errorf("custom preset slug %q is reserved; choose a different name", slug)
	}
	builtinPath := fmt.Sprintf("%s/%s.yaml", catalogPath, slug)
	if _, err := fs.Stat(personaFS, builtinPath); err == nil {
		return "", "", fmt.Errorf("custom preset slug %q collides with built-in preset slug", slug)
	}

	return slug, displayName, nil
}

func buildCustomProfileContent(personaFS fs.FS, slug, displayName string) ([]byte, error) {
	content, err := fs.ReadFile(personaFS, "embed/personas/custom.yaml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read schema v2 custom preset template: %w", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("parse schema v2 custom preset template: %w", err)
	}
	raw["name"] = slug
	raw["display_name"] = displayName

	generated, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal schema v2 custom preset %q: %w", slug, err)
	}
	return generated, nil
}

func customPresetRecoveryError(slug, savedPath string, cause error) error {
	return fmt.Errorf("custom preset %q was saved to %s, but it could not be loaded automatically (%v). Recovery: exit this form and select %q from the preset list, or inspect/delete that file and retry", slug, savedPath, cause, slug)
}

func validateCustomPresetYAMLSize(customYAML string) error {
	if len(customYAML) > maxCustomPresetYAMLBytes {
		return fmt.Errorf("custom YAML exceeds size limit (%d bytes maximum)", maxCustomPresetYAMLBytes)
	}
	return nil
}

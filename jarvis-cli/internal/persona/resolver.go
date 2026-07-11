package persona

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PresetSource identifies where a preset was loaded from.
type PresetSource string

const (
	PresetSourceBuiltin PresetSource = "builtin"
	PresetSourceUser    PresetSource = "user"
)

// ResolvedProfile is the schema-v2 result of profile resolution.
type ResolvedProfile struct {
	Slug     string
	Source   PresetSource
	FilePath string
	Preset   *Profile
}

// ResolvedPresetV2 is retained for compatibility until the remaining test
// fixtures are migrated to the canonical ResolvedProfile API.
type ResolvedPresetV2 = ResolvedProfile

// PresetV2MigrationDiagnostic describes a configured profile without loading
// it through a legacy resolver. It exists solely to provide safe migration
// diagnostics for unsupported configured profiles.
type PresetV2MigrationDiagnostic struct {
	Classification ProfileClassification
	Source         PresetSource
	FilePath       string
}

// NormalizeSlug canonicalizes a preset slug.
// Rules: trim outer spaces, lowercase, and replace spaces with hyphens.
func NormalizeSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

// ResolveProfile resolves and validates a schema-v2 presentation profile.
func ResolveProfile(fsys fs.FS, slug string) (*ResolvedProfile, error) {
	if fsys == nil {
		return nil, fmt.Errorf("resolve schema v2 preset %q: persona catalog is unavailable", NormalizeSlug(slug))
	}

	normalized := NormalizeSlug(slug)
	if err := validatePresetSlug(normalized); err != nil {
		return nil, err
	}

	builtinPath := filepath.ToSlash(filepath.Join("embed", "personas", normalized+".yaml"))
	if p, err := readPresetV2FromFS(fsys, builtinPath); err == nil {
		return &ResolvedProfile{
			Slug:     normalized,
			Source:   PresetSourceBuiltin,
			FilePath: builtinPath,
			Preset:   p,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load builtin schema v2 preset %q: %w", normalized, err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}

	userPath := filepath.Join(homeDir, ".jarvis", "personas", normalized+".yaml")
	if p, err := readPresetV2FromOS(userPath); err == nil {
		return &ResolvedProfile{
			Slug:     normalized,
			Source:   PresetSourceUser,
			FilePath: userPath,
			Preset:   p,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load user schema v2 preset %q: %w", normalized, err)
	}

	available := listPresetV2Names(fsys)
	return nil, fmt.Errorf("schema v2 preset %q not found (available built-ins: %s)", normalized, strings.Join(available, ", "))
}

// ResolvePresetV2 is retained for compatibility until the remaining test
// fixtures are migrated to ResolveProfile.
func ResolvePresetV2(fsys fs.FS, slug string) (*ResolvedProfile, error) {
	return ResolveProfile(fsys, slug)
}

// ClassifyPresetForV2Migration inspects the configured profile using only the
// schema-v2 validator and YAML structure. It never resolves a V1 preset.
func ClassifyPresetForV2Migration(fsys fs.FS, slug string) (*PresetV2MigrationDiagnostic, error) {
	if fsys == nil {
		return nil, fmt.Errorf("classify schema v2 preset %q: persona catalog is unavailable", NormalizeSlug(slug))
	}

	normalized := NormalizeSlug(slug)
	if err := validatePresetSlug(normalized); err != nil {
		return nil, err
	}

	builtinPath := filepath.ToSlash(filepath.Join("embed", "personas", normalized+".yaml"))
	if content, err := fs.ReadFile(fsys, builtinPath); err == nil {
		return &PresetV2MigrationDiagnostic{
			Classification: classifyPresetV2Profile(content),
			Source:         PresetSourceBuiltin,
			FilePath:       builtinPath,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read builtin schema v2 preset %q: %w", normalized, err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}
	userPath := filepath.Join(homeDir, ".jarvis", "personas", normalized+".yaml")
	if content, err := os.ReadFile(userPath); err == nil {
		return &PresetV2MigrationDiagnostic{
			Classification: classifyPresetV2Profile(content),
			Source:         PresetSourceUser,
			FilePath:       userPath,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read user schema v2 preset %q: %w", normalized, err)
	}

	return &PresetV2MigrationDiagnostic{Classification: PresetV2ProfileMissing}, nil
}

func readPresetV2FromFS(fsys fs.FS, path string) (*Profile, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	return parsePresetV2(path, data)
}

func readPresetV2FromOS(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePresetV2(path, data)
}

func parsePresetV2(path string, data []byte) (*Profile, error) {
	preset, err := ValidateAndDecode(data)
	if err != nil {
		return nil, fmt.Errorf("validate schema v2 preset at %q: %w", path, err)
	}
	return preset, nil
}

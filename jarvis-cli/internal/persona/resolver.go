package persona

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PresetSource identifies where a preset was loaded from.
type PresetSource string

const (
	PresetSourceBuiltin PresetSource = "builtin"
	PresetSourceUser    PresetSource = "user"
)

// ResolvedPreset is the canonical result of preset resolution.
type ResolvedPreset struct {
	Slug     string
	Source   PresetSource
	FilePath string
	Preset   *Preset
}

// ResolvedPresetV2 is the schema-v2 result of preset resolution.
type ResolvedPresetV2 struct {
	Slug     string
	Source   PresetSource
	FilePath string
	Preset   *PresetV2
}

// NormalizeSlug canonicalizes a preset slug.
// Rules: trim outer spaces, lowercase, and replace spaces with hyphens.
func NormalizeSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

// ResolvePreset resolves a requested preset slug against the canonical catalog:
// built-in presets first, then user-defined presets at ~/.jarvis/personas/<slug>.yaml.
func ResolvePreset(fsys fs.FS, slug string) (*ResolvedPreset, error) {
	normalized := NormalizeSlug(slug)
	if err := validatePresetSlug(normalized); err != nil {
		return nil, err
	}

	builtinPath := filepath.ToSlash(filepath.Join("embed", "personas", normalized+".yaml"))
	if p, err := readPresetFromFS(fsys, builtinPath); err == nil {
		return &ResolvedPreset{
			Slug:     normalized,
			Source:   PresetSourceBuiltin,
			FilePath: builtinPath,
			Preset:   p,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load builtin preset %q: %w", normalized, err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}

	userPath := filepath.Join(homeDir, ".jarvis", "personas", normalized+".yaml")
	if p, err := readPresetFromOS(userPath); err == nil {
		return &ResolvedPreset{
			Slug:     normalized,
			Source:   PresetSourceUser,
			FilePath: userPath,
			Preset:   p,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load user preset %q: %w", normalized, err)
	}

	available := listPresetNames(fsys)
	return nil, fmt.Errorf("preset %q not found (available built-ins: %s)", normalized, strings.Join(available, ", "))
}

// ResolvePresetV2 resolves and validates a schema-v2 presentation profile.
func ResolvePresetV2(fsys fs.FS, slug string) (*ResolvedPresetV2, error) {
	if fsys == nil {
		return nil, fmt.Errorf("resolve schema v2 preset %q: persona catalog is unavailable", NormalizeSlug(slug))
	}

	normalized := NormalizeSlug(slug)
	if err := validatePresetSlug(normalized); err != nil {
		return nil, err
	}

	builtinPath := filepath.ToSlash(filepath.Join("embed", "personas", normalized+".yaml"))
	if p, err := readPresetV2FromFS(fsys, builtinPath); err == nil {
		return &ResolvedPresetV2{
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
		return &ResolvedPresetV2{
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

func readPresetFromFS(fsys fs.FS, path string) (*Preset, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	return parsePreset(path, data)
}

func readPresetFromOS(path string) (*Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePreset(path, data)
}

func readPresetV2FromFS(fsys fs.FS, path string) (*PresetV2, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	return parsePresetV2(path, data)
}

func readPresetV2FromOS(path string) (*PresetV2, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePresetV2(path, data)
}

func parsePreset(path string, data []byte) (*Preset, error) {
	var preset Preset
	if err := yaml.Unmarshal(data, &preset); err != nil {
		return nil, fmt.Errorf("parse preset at %q: %w", path, err)
	}
	return &preset, nil
}

func parsePresetV2(path string, data []byte) (*PresetV2, error) {
	preset, err := ValidateAndDecode(data)
	if err != nil {
		return nil, fmt.Errorf("validate schema v2 preset at %q: %w", path, err)
	}
	return preset, nil
}

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

// ProfileMigrationDiagnostic describes a configured profile without loading
// it through a legacy resolver. It exists solely to provide safe migration
// diagnostics for unsupported configured profiles.
type ProfileMigrationDiagnostic struct {
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

// ResolveProfile resolves legacy manifests. A manifest predating persona replay
// records neither a slug nor a source, and therefore replays the historic default.
func ResolveProfile(fsys fs.FS, slug string) (*ResolvedProfile, error) {
	return ResolveProfileFromSource(fsys, slug, "")
}

// ResolveProfileFromSource resolves a schema-v2 presentation profile from the
// source recorded by the manifest. A user profile deliberately wins even when it
// shares a slug with a built-in; conversely, a recorded built-in is never shadowed
// by a user file. An empty source preserves the legacy built-in-first lookup.
func ResolveProfileFromSource(fsys fs.FS, slug string, source PresetSource) (*ResolvedProfile, error) {
	if fsys == nil {
		return nil, fmt.Errorf("resolve schema v2 preset %q: persona catalog is unavailable", NormalizeSlug(slug))
	}

	normalized := NormalizeSlug(slug)
	if normalized == "" {
		normalized = "argentino"
	}
	if err := validatePresetSlug(normalized); err != nil {
		return nil, err
	}
	builtinPath := filepath.ToSlash(filepath.Join("embed", "personas", normalized+".yaml"))
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}
	userPath := filepath.Join(homeDir, ".jarvis", "personas", normalized+".yaml")
	load := func(path string, fromFS bool, resolvedSource PresetSource) (*ResolvedProfile, error) {
		var p *Profile
		var readErr error
		if fromFS {
			p, readErr = readProfileFromFS(fsys, path)
		} else {
			p, readErr = readProfileFromOS(path)
		}
		if readErr == nil {
			return &ResolvedProfile{Slug: normalized, Source: resolvedSource, FilePath: path, Preset: p}, nil
		}
		return nil, readErr
	}
	if source == PresetSourceUser {
		resolved, err := load(userPath, false, PresetSourceUser)
		if err == nil {
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load user schema v2 preset %q: %w", normalized, err)
		}
		return nil, fmt.Errorf("user schema v2 preset %q not found", normalized)
	}
	if source == PresetSourceBuiltin {
		resolved, err := load(builtinPath, true, PresetSourceBuiltin)
		if err == nil {
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load builtin schema v2 preset %q: %w", normalized, err)
		}
		return nil, fmt.Errorf("builtin schema v2 preset %q not found", normalized)
	}
	for _, candidate := range []struct {
		path   string
		fromFS bool
		source PresetSource
	}{{builtinPath, true, PresetSourceBuiltin}, {userPath, false, PresetSourceUser}} {
		resolved, err := load(candidate.path, candidate.fromFS, candidate.source)
		if err == nil {
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load %s schema v2 preset %q: %w", candidate.source, normalized, err)
		}
	}
	available := listProfileNames(fsys)
	return nil, fmt.Errorf("schema v2 preset %q not found (available built-ins: %s)", normalized, strings.Join(available, ", "))
}

// ClassifyProfileForMigration inspects the configured profile using only the
// schema-v2 validator and YAML structure. It never resolves a V1 preset.
func ClassifyProfileForMigration(fsys fs.FS, slug string) (*ProfileMigrationDiagnostic, error) {
	if fsys == nil {
		return nil, fmt.Errorf("classify schema v2 preset %q: persona catalog is unavailable", NormalizeSlug(slug))
	}

	normalized := NormalizeSlug(slug)
	if err := validatePresetSlug(normalized); err != nil {
		return nil, err
	}

	builtinPath := filepath.ToSlash(filepath.Join("embed", "personas", normalized+".yaml"))
	if content, err := fs.ReadFile(fsys, builtinPath); err == nil {
		return &ProfileMigrationDiagnostic{
			Classification: classifyProfile(content),
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
		return &ProfileMigrationDiagnostic{
			Classification: classifyProfile(content),
			Source:         PresetSourceUser,
			FilePath:       userPath,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read user schema v2 preset %q: %w", normalized, err)
	}

	return &ProfileMigrationDiagnostic{Classification: ProfileMissing}, nil
}

func readProfileFromFS(fsys fs.FS, path string) (*Profile, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	return parseProfile(path, data)
}

func readProfileFromOS(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseProfile(path, data)
}

func parseProfile(path string, data []byte) (*Profile, error) {
	preset, err := ValidateAndDecode(data)
	if err != nil {
		return nil, fmt.Errorf("validate schema v2 preset at %q: %w", path, err)
	}
	return preset, nil
}

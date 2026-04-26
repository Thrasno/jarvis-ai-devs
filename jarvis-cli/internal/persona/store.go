package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type tempPresetFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
	Name() string
}

var newTempPresetFile = func(dir, pattern string) (tempPresetFile, error) {
	return os.CreateTemp(dir, pattern)
}

// UserPresetDir returns the canonical directory for user-defined persona presets.
// Path: ~/.jarvis/personas
func UserPresetDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}

	return filepath.Join(homeDir, ".jarvis", "personas"), nil
}

// LoadUserPresetFile loads a user-defined preset by slug from ~/.jarvis/personas/<slug>.yaml.
func LoadUserPresetFile(slug string) (string, []byte, error) {
	path, err := userPresetPath(slug)
	if err != nil {
		return "", nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	return path, data, nil
}

// SaveUserPresetFile saves a user-defined preset atomically at ~/.jarvis/personas/<slug>.yaml.
func SaveUserPresetFile(slug string, content []byte) (string, error) {
	path, err := userPresetPath(slug)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create user preset dir %q: %w", dir, err)
	}

	tmpFile, err := newTempPresetFile(dir, "preset-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp preset file: %w", err)
	}

	tmpPath := tmpFile.Name()
	cleanupTmp := func() {
		_ = os.Remove(tmpPath)
	}

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		cleanupTmp()
		return "", fmt.Errorf("write temp preset file %q: %w", tmpPath, err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		cleanupTmp()
		return "", fmt.Errorf("sync temp preset file %q: %w", tmpPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("close temp preset file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("rename temp preset file to %q: %w", path, err)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		return "", fmt.Errorf("set preset file permissions on %q: %w", path, err)
	}

	return path, nil
}

func userPresetPath(slug string) (string, error) {
	normalized := NormalizeSlug(slug)
	if err := validatePresetSlug(normalized); err != nil {
		return "", err
	}

	dir, err := UserPresetDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, normalized+".yaml")
	if filepath.Dir(path) != dir {
		return "", fmt.Errorf("invalid preset slug %q: path traversal is not allowed", slug)
	}

	return path, nil
}

func validatePresetSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("preset slug cannot be empty")
	}

	if strings.ContainsAny(slug, `/\`) {
		return fmt.Errorf("invalid preset slug %q: path separators are not allowed", slug)
	}

	if strings.Contains(slug, "..") || slug == "." || slug == ".." {
		return fmt.Errorf("invalid preset slug %q: path traversal is not allowed", slug)
	}

	for _, r := range slug {
		if unicode.IsLower(r) || unicode.IsDigit(r) || r == '-' {
			continue
		}
		return fmt.Errorf("invalid preset slug %q: only lowercase letters, digits, and hyphens are allowed", slug)
	}

	return nil
}

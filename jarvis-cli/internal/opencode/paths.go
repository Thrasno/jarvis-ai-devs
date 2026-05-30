package opencode

import "path/filepath"

// PathRoots contains injectable user directory roots for OpenCode discovery.
// Empty standard roots fall back to legacy home-relative dot paths.
type PathRoots struct {
	HomeDir   string
	CacheDir  string
	ConfigDir string
	DataDir   string
}

// Paths lists candidate OpenCode data files in deterministic precedence order.
type Paths struct {
	ModelsJSON   []string
	SettingsJSON []string
	AuthJSON     []string
}

// ResolvePaths returns OS-standard candidates first and legacy dot-path candidates second.
// Callers can use the first existing candidate without embedding OS-specific paths in tests.
func ResolvePaths(roots PathRoots) Paths {
	return Paths{
		ModelsJSON:   compactPaths(standardPath(roots.CacheDir, "opencode", "models.json"), legacyPath(roots.HomeDir, ".cache", "opencode", "models.json")),
		SettingsJSON: compactPaths(standardPath(roots.ConfigDir, "opencode", "opencode.json"), legacyPath(roots.HomeDir, ".config", "opencode", "opencode.json")),
		AuthJSON:     compactPaths(standardPath(roots.DataDir, "opencode", "auth.json"), legacyPath(roots.HomeDir, ".local", "share", "opencode", "auth.json")),
	}
}

func standardPath(root string, elem ...string) string {
	if root == "" {
		return ""
	}
	parts := append([]string{root}, elem...)
	return filepath.Join(parts...)
}

func legacyPath(home string, elem ...string) string {
	if home == "" {
		return ""
	}
	parts := append([]string{home}, elem...)
	return filepath.Join(parts...)
}

func compactPaths(paths ...string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

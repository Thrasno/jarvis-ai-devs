package opencode

import "path/filepath"

// PathRoots contains injectable user directory roots for OpenCode discovery.
// Home-relative documented/legacy paths are preferred over OS-standard roots.
type PathRoots struct {
	HomeDir   string
	CacheDir  string
	ConfigDir string
	DataDir   string
}

// Paths lists candidate OpenCode data files in deterministic precedence order.
type Paths struct {
	ModelsJSON    []string
	SettingsJSON  []string
	SettingsJSONC []string
	AuthJSON      []string
}

// ResolvePaths returns documented/legacy home paths first and OS-standard candidates second.
// Callers can use the first existing candidate without embedding OS-specific paths in tests.
func ResolvePaths(roots PathRoots) Paths {
	return Paths{
		ModelsJSON:    compactPaths(legacyPath(roots.HomeDir, ".cache", "opencode", "models.json"), standardPath(roots.CacheDir, "opencode", "models.json")),
		SettingsJSON:  compactPaths(legacyPath(roots.HomeDir, ".config", "opencode", "opencode.json"), standardPath(roots.ConfigDir, "opencode", "opencode.json")),
		SettingsJSONC: compactPaths(legacyPath(roots.HomeDir, ".config", "opencode", "opencode.jsonc"), standardPath(roots.ConfigDir, "opencode", "opencode.jsonc")),
		AuthJSON:      compactPaths(legacyPath(roots.HomeDir, ".local", "share", "opencode", "auth.json"), standardPath(roots.DataDir, "opencode", "auth.json")),
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

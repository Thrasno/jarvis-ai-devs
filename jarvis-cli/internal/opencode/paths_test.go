package opencode

import (
	"path/filepath"
	"testing"
)

func TestResolvePaths_UsesLegacyDotPathsBeforeOSStandardRoots(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	cache := filepath.Join(t.TempDir(), "cache")
	config := filepath.Join(t.TempDir(), "config")
	data := filepath.Join(t.TempDir(), "data")

	paths := ResolvePaths(PathRoots{
		HomeDir:   home,
		CacheDir:  cache,
		ConfigDir: config,
		DataDir:   data,
	})

	assertCandidates(t, paths.ModelsJSON, []string{
		filepath.Join(home, ".cache", "opencode", "models.json"),
		filepath.Join(cache, "opencode", "models.json"),
	})
	assertCandidates(t, paths.SettingsJSON, []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(config, "opencode", "opencode.json"),
	})
	assertCandidates(t, paths.SettingsJSONC, []string{
		filepath.Join(home, ".config", "opencode", "opencode.jsonc"),
		filepath.Join(config, "opencode", "opencode.jsonc"),
	})
	assertCandidates(t, paths.AuthJSON, []string{
		filepath.Join(home, ".local", "share", "opencode", "auth.json"),
		filepath.Join(data, "opencode", "auth.json"),
	})
}

func TestResolvePaths_FallsBackToLegacyDotPathsWhenStandardRootsMissing(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")

	paths := ResolvePaths(PathRoots{HomeDir: home})

	assertCandidates(t, paths.ModelsJSON, []string{
		filepath.Join(home, ".cache", "opencode", "models.json"),
	})
	assertCandidates(t, paths.SettingsJSON, []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
	})
	assertCandidates(t, paths.SettingsJSONC, []string{
		filepath.Join(home, ".config", "opencode", "opencode.jsonc"),
	})
	assertCandidates(t, paths.AuthJSON, []string{
		filepath.Join(home, ".local", "share", "opencode", "auth.json"),
	})
}

func assertCandidates(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

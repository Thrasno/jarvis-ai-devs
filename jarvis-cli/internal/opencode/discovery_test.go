package opencode

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverAvailableProviders_ComposesCatalogSettingsAuthAndEnv(t *testing.T) {
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "cache", "opencode", "models.json")
	settingsPath := filepath.Join(dir, "config", "opencode", "opencode.json")
	authPath := filepath.Join(dir, "data", "opencode", "auth.json")
	writeFile(t, modelsPath, `{
		"openai": {"env": ["OPENAI_API_KEY"], "models": {"gpt-5.1": {"name": "GPT 5.1"}}},
		"anthropic": {"models": {"claude-sonnet": {"name": "Claude Sonnet"}}},
		"lmstudio": {"models": {"local": {"name": "Local"}}}
	}`)
	writeFile(t, settingsPath, `{
		"provider": {"lmstudio": {"options": {"baseURL": "http://localhost:1234/v1"}}},
		"enabled_providers": ["openai", "anthropic", "lmstudio"]
	}`)
	writeFile(t, authPath, `{"anthropic": {"type": "api", "key": "secret"}}`)

	result, err := DiscoverAvailableProviders(Paths{
		ModelsJSON:   []string{filepath.Join(dir, "missing-models.json"), modelsPath},
		SettingsJSON: []string{settingsPath},
		AuthJSON:     []string{authPath},
	}, func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "set"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("DiscoverAvailableProviders: %v", err)
	}

	if result.ModelCachePath != modelsPath {
		t.Fatalf("model cache source = %q, want %q", result.ModelCachePath, modelsPath)
	}
	if result.SettingsPath != settingsPath {
		t.Fatalf("settings source = %q, want %q", result.SettingsPath, settingsPath)
	}
	if result.AuthPath != authPath {
		t.Fatalf("auth source = %q, want %q", result.AuthPath, authPath)
	}
	if got, want := providerIDs(result.Providers), []string{"anthropic", "lmstudio", "openai"}; !sameStrings(got, want) {
		t.Fatalf("provider ids = %#v, want %#v", got, want)
	}
}

func TestDiscoverAvailableProviders_SkipsMissingSettingsAndAuthCandidates(t *testing.T) {
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	writeFile(t, modelsPath, `{"openai": {"env": ["OPENAI_API_KEY"], "models": {"gpt": {}}}}`)
	t.Setenv("OPENAI_API_KEY", "set")

	result, err := DiscoverAvailableProviders(Paths{
		ModelsJSON:   []string{modelsPath},
		SettingsJSON: []string{filepath.Join(dir, "missing-settings.json")},
		AuthJSON:     []string{filepath.Join(dir, "missing-auth.json")},
	}, nil)
	if err != nil {
		t.Fatalf("DiscoverAvailableProviders missing settings/auth: %v", err)
	}
	if result.SettingsPath != "" || result.AuthPath != "" {
		t.Fatalf("unexpected settings/auth source: %+v", result)
	}
	if got, want := providerIDs(result.Providers), []string{"openai"}; !sameStrings(got, want) {
		t.Fatalf("provider ids = %#v, want %#v", got, want)
	}
}

func TestDiscoverAvailableProviders_ReturnsSettingsParseErrorFromFirstExistingCandidate(t *testing.T) {
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	settingsPath := filepath.Join(dir, "opencode.json")
	writeFile(t, modelsPath, `{"openai": {"env": ["OPENAI_API_KEY"], "models": {"gpt": {}}}}`)
	writeFile(t, settingsPath, `{not-json`)

	_, err := DiscoverAvailableProviders(Paths{
		ModelsJSON:   []string{modelsPath},
		SettingsJSON: []string{settingsPath},
	}, func(string) string { return "set" })
	if err == nil {
		t.Fatal("expected settings parse error")
	}
	if !strings.Contains(err.Error(), "parse OpenCode settings") {
		t.Fatalf("expected settings parse diagnostic, got %v", err)
	}
}

func TestDiscoverAvailableProviders_ReturnsAuthParseErrorFromFirstExistingCandidate(t *testing.T) {
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	authPath := filepath.Join(dir, "auth.json")
	writeFile(t, modelsPath, `{"openai": {"env": ["OPENAI_API_KEY"], "models": {"gpt": {}}}}`)
	writeFile(t, authPath, `{not-json`)

	_, err := DiscoverAvailableProviders(Paths{
		ModelsJSON: []string{modelsPath},
		AuthJSON:   []string{authPath},
	}, func(string) string { return "set" })
	if err == nil {
		t.Fatal("expected auth parse error")
	}
	if !strings.Contains(err.Error(), "parse OpenCode auth") {
		t.Fatalf("expected auth parse diagnostic, got %v", err)
	}
}

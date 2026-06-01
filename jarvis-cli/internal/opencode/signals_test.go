package opencode

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSettingsSignals_ReadsProviderFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	writeFile(t, path, `{
		"provider": {
			"openai": {"options": {"apiKey": "{env:OPENAI_API_KEY}"}},
			"lmstudio": {"options": {"baseURL": "http://localhost:1234/v1"}}
		},
		"enabled_providers": ["openai", " lmstudio "],
		"disabled_providers": [" anthropic "]
	}`)

	signals, err := LoadSettingsSignals(path)
	if err != nil {
		t.Fatalf("LoadSettingsSignals: %v", err)
	}
	if !signals.SettingsProviders["openai"] || !signals.SettingsProviders["lmstudio"] {
		t.Fatalf("settings providers = %#v", signals.SettingsProviders)
	}
	if !signals.EnabledProviders["openai"] || !signals.EnabledProviders["lmstudio"] {
		t.Fatalf("enabled providers = %#v", signals.EnabledProviders)
	}
	if !signals.DisabledProviders["anthropic"] {
		t.Fatalf("disabled providers = %#v", signals.DisabledProviders)
	}
}

func TestLoadSettingsSignals_MissingFileReturnsEmptySignals(t *testing.T) {
	signals, err := LoadSettingsSignals(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadSettingsSignals missing file: %v", err)
	}
	if len(signals.SettingsProviders) != 0 || len(signals.EnabledProviders) != 0 || len(signals.DisabledProviders) != 0 {
		t.Fatalf("expected empty signals, got %#v", signals)
	}
}

func TestLoadSettingsSignals_MalformedJSONReturnsDiagnosticError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	writeFile(t, path, `{not-json`)

	_, err := LoadSettingsSignals(path)
	if err == nil {
		t.Fatal("expected malformed settings error")
	}
	if !strings.Contains(err.Error(), "parse OpenCode settings") {
		t.Fatalf("expected diagnostic parse error, got %v", err)
	}
}

func TestLoadAuthSignals_ReadsProviderKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	writeFile(t, path, `{
		"openai": {"type": "api", "key": "secret"},
		"anthropic": {"type": "api", "key": "secret"}
	}`)

	signals, err := LoadAuthSignals(path)
	if err != nil {
		t.Fatalf("LoadAuthSignals: %v", err)
	}
	if !signals.AuthProviders["openai"] || !signals.AuthProviders["anthropic"] {
		t.Fatalf("auth providers = %#v", signals.AuthProviders)
	}
}

func TestLoadAuthSignals_MissingFileReturnsEmptySignals(t *testing.T) {
	signals, err := LoadAuthSignals(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadAuthSignals missing file: %v", err)
	}
	if len(signals.AuthProviders) != 0 {
		t.Fatalf("expected empty auth signals, got %#v", signals.AuthProviders)
	}
}

func TestLoadAuthSignals_MalformedJSONReturnsDiagnosticError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	writeFile(t, path, `{not-json`)

	_, err := LoadAuthSignals(path)
	if err == nil {
		t.Fatal("expected malformed auth error")
	}
	if !strings.Contains(err.Error(), "parse OpenCode auth") {
		t.Fatalf("expected diagnostic parse error, got %v", err)
	}
}

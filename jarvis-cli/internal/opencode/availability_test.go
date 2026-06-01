package opencode

import "testing"

func TestAvailableProviders_DetectsEnvAuthAndSettingsSignals(t *testing.T) {
	catalog := Catalog{Providers: map[string]Provider{
		"openai": {
			ID:  "openai",
			Env: []string{"OPENAI_API_KEY"},
			Models: map[string]Model{
				"gpt-5.1": {ID: "gpt-5.1", Name: "GPT 5.1"},
			},
		},
		"anthropic": {
			ID: "anthropic",
			Models: map[string]Model{
				"claude-sonnet": {ID: "claude-sonnet", Name: "Claude Sonnet"},
			},
		},
		"lmstudio": {
			ID: "lmstudio",
			Models: map[string]Model{
				"local-model": {ID: "local-model", Name: "Local Model"},
			},
		},
	}}

	providers := AvailableProviders(catalog, ProviderSignals{
		SettingsProviders: map[string]bool{"lmstudio": true},
		AuthProviders:     map[string]bool{"anthropic": true},
		Getenv: func(name string) string {
			if name == "OPENAI_API_KEY" {
				return "set"
			}
			return ""
		},
	})

	if got, want := providerIDs(providers), []string{"anthropic", "lmstudio", "openai"}; !sameStrings(got, want) {
		t.Fatalf("provider ids = %#v, want %#v", got, want)
	}
	assertProviderReasons(t, providers, "openai", []ProviderSignalReason{ProviderSignalEnv})
	assertProviderReasons(t, providers, "anthropic", []ProviderSignalReason{ProviderSignalAuth})
	assertProviderReasons(t, providers, "lmstudio", []ProviderSignalReason{ProviderSignalSettings})
}

func TestAvailableProviders_AppliesEnabledAndDisabledProviderFilters(t *testing.T) {
	catalog := Catalog{Providers: map[string]Provider{
		"anthropic": {ID: "anthropic", Models: map[string]Model{"claude": {ID: "claude"}}},
		"openai":    {ID: "openai", Env: []string{"OPENAI_API_KEY"}, Models: map[string]Model{"gpt": {ID: "gpt"}}},
		"opencode":  {ID: "opencode", Models: map[string]Model{"codex": {ID: "codex"}}},
	}}

	providers := AvailableProviders(catalog, ProviderSignals{
		AuthProviders:     map[string]bool{"anthropic": true},
		EnabledProviders:  map[string]bool{"anthropic": true, "openai": true},
		DisabledProviders: map[string]bool{"anthropic": true},
		SettingsProviders: map[string]bool{"opencode": true},
		Getenv:            func(string) string { return "set" },
	})

	if got, want := providerIDs(providers), []string{"openai"}; !sameStrings(got, want) {
		t.Fatalf("provider ids = %#v, want %#v", got, want)
	}
}

func TestAvailableProviders_SkipsProvidersWithoutModels(t *testing.T) {
	catalog := Catalog{Providers: map[string]Provider{
		"openai": {ID: "openai", Env: []string{"OPENAI_API_KEY"}},
	}}

	providers := AvailableProviders(catalog, ProviderSignals{
		Getenv: func(string) string { return "set" },
	})

	if len(providers) != 0 {
		t.Fatalf("expected no providers without models, got %#v", providers)
	}
}

func assertProviderReasons(t *testing.T, providers []AvailableProvider, providerID string, want []ProviderSignalReason) {
	t.Helper()
	for _, provider := range providers {
		if provider.Provider.ID == providerID {
			if !sameReasons(provider.Reasons, want) {
				t.Fatalf("%s reasons = %#v, want %#v", providerID, provider.Reasons, want)
			}
			return
		}
	}
	t.Fatalf("provider %q not found in %#v", providerID, providers)
}

func providerIDs(providers []AvailableProvider) []string {
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.Provider.ID)
	}
	return ids
}

func sameStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameReasons(got []ProviderSignalReason, want []ProviderSignalReason) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

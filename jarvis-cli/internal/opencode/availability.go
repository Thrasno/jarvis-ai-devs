package opencode

import "sort"

// ProviderSignalReason identifies why a provider is considered usable locally.
type ProviderSignalReason string

const (
	ProviderSignalEnv      ProviderSignalReason = "env"
	ProviderSignalAuth     ProviderSignalReason = "auth"
	ProviderSignalSettings ProviderSignalReason = "settings"
)

// ProviderSignals contains local OpenCode configuration/authentication signals.
type ProviderSignals struct {
	SettingsProviders map[string]bool
	AuthProviders     map[string]bool
	EnabledProviders  map[string]bool
	DisabledProviders map[string]bool
	Getenv            func(string) string
}

// AvailableProvider is a catalog provider that has at least one local usability signal.
type AvailableProvider struct {
	Provider Provider
	Reasons  []ProviderSignalReason
}

// AvailableProviders returns model-bearing providers enabled by local OpenCode signals.
func AvailableProviders(catalog Catalog, signals ProviderSignals) []AvailableProvider {
	ids := make([]string, 0, len(catalog.Providers))
	for id := range catalog.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	available := make([]AvailableProvider, 0, len(ids))
	for _, id := range ids {
		provider := catalog.Providers[id]
		if provider.ID == "" {
			provider.ID = id
		}
		if len(provider.Models) == 0 || !providerAllowed(provider.ID, signals) {
			continue
		}

		reasons := providerReasons(provider, signals)
		if len(reasons) == 0 {
			continue
		}
		available = append(available, AvailableProvider{Provider: provider, Reasons: reasons})
	}
	return available
}

func providerAllowed(providerID string, signals ProviderSignals) bool {
	if signals.DisabledProviders[providerID] {
		return false
	}
	if len(signals.EnabledProviders) > 0 && !signals.EnabledProviders[providerID] {
		return false
	}
	return true
}

func providerReasons(provider Provider, signals ProviderSignals) []ProviderSignalReason {
	reasons := []ProviderSignalReason{}
	if providerEnvConfigured(provider, signals.Getenv) {
		reasons = append(reasons, ProviderSignalEnv)
	}
	if signals.AuthProviders[provider.ID] {
		reasons = append(reasons, ProviderSignalAuth)
	}
	if signals.SettingsProviders[provider.ID] {
		reasons = append(reasons, ProviderSignalSettings)
	}
	return reasons
}

func providerEnvConfigured(provider Provider, getenv func(string) string) bool {
	if getenv == nil {
		getenv = func(name string) string { return "" }
	}
	for _, envName := range provider.Env {
		if getenv(envName) != "" {
			return true
		}
	}
	return false
}

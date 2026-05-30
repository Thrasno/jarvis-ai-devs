package opencode

import (
	"fmt"
	"os"
)

// DiscoveryResult contains available OpenCode providers and source files used to detect them.
type DiscoveryResult struct {
	Catalog        Catalog
	Providers      []AvailableProvider
	ModelCachePath string
	SettingsPath   string
	AuthPath       string
}

// DiscoverAvailableProviders combines OpenCode cache, settings, auth, and env signals.
func DiscoverAvailableProviders(paths Paths, getenv func(string) string) (DiscoveryResult, error) {
	catalog, modelCachePath, err := DiscoverModelCache(paths.ModelsJSON)
	if err != nil {
		return DiscoveryResult{Catalog: catalog, ModelCachePath: modelCachePath}, err
	}

	settingsSignals, settingsPath, err := discoverSettingsSignals(paths.SettingsJSON)
	if err != nil {
		return DiscoveryResult{Catalog: catalog, ModelCachePath: modelCachePath, SettingsPath: settingsPath}, err
	}
	authSignals, authPath, err := discoverAuthSignals(paths.AuthJSON)
	if err != nil {
		return DiscoveryResult{Catalog: catalog, ModelCachePath: modelCachePath, SettingsPath: settingsPath, AuthPath: authPath}, err
	}

	if getenv == nil {
		getenv = os.Getenv
	}
	signals := mergeSignals(settingsSignals, authSignals)
	signals.Getenv = getenv
	return DiscoveryResult{
		Catalog:        catalog,
		Providers:      AvailableProviders(catalog, signals),
		ModelCachePath: modelCachePath,
		SettingsPath:   settingsPath,
		AuthPath:       authPath,
	}, nil
}

func discoverSettingsSignals(candidates []string) (ProviderSignals, string, error) {
	empty := emptySignals()
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return empty, candidate, fmt.Errorf("inspect OpenCode settings: %w", err)
		}
		signals, err := LoadSettingsSignals(candidate)
		return signals, candidate, err
	}
	return empty, "", nil
}

func discoverAuthSignals(candidates []string) (ProviderSignals, string, error) {
	empty := emptySignals()
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return empty, candidate, fmt.Errorf("inspect OpenCode auth: %w", err)
		}
		signals, err := LoadAuthSignals(candidate)
		return signals, candidate, err
	}
	return empty, "", nil
}

func mergeSignals(all ...ProviderSignals) ProviderSignals {
	merged := emptySignals()
	for _, signals := range all {
		copyBoolMap(merged.SettingsProviders, signals.SettingsProviders)
		copyBoolMap(merged.AuthProviders, signals.AuthProviders)
		copyBoolMap(merged.EnabledProviders, signals.EnabledProviders)
		copyBoolMap(merged.DisabledProviders, signals.DisabledProviders)
	}
	return merged
}

func copyBoolMap(dst map[string]bool, src map[string]bool) {
	for key, value := range src {
		if value {
			dst[key] = true
		}
	}
}

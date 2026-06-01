package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type settingsFile struct {
	Provider          map[string]json.RawMessage `json:"provider"`
	EnabledProviders  []string                   `json:"enabled_providers"`
	DisabledProviders []string                   `json:"disabled_providers"`
}

// LoadSettingsSignals reads provider-related OpenCode settings from opencode.json.
func LoadSettingsSignals(path string) (ProviderSignals, error) {
	signals := emptySignals()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return signals, nil
		}
		return signals, fmt.Errorf("read OpenCode settings: %w", err)
	}

	var settings settingsFile
	if err := json.Unmarshal(data, &settings); err != nil {
		return signals, fmt.Errorf("parse OpenCode settings: %w", err)
	}
	for providerID := range settings.Provider {
		if providerID != "" {
			signals.SettingsProviders[providerID] = true
		}
	}
	for _, providerID := range settings.EnabledProviders {
		providerID = strings.TrimSpace(providerID)
		if providerID != "" {
			signals.EnabledProviders[providerID] = true
		}
	}
	for _, providerID := range settings.DisabledProviders {
		providerID = strings.TrimSpace(providerID)
		if providerID != "" {
			signals.DisabledProviders[providerID] = true
		}
	}
	return signals, nil
}

// LoadAuthSignals reads provider IDs present in OpenCode auth.json without exposing credential values.
func LoadAuthSignals(path string) (ProviderSignals, error) {
	signals := emptySignals()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return signals, nil
		}
		return signals, fmt.Errorf("read OpenCode auth: %w", err)
	}

	var auth map[string]json.RawMessage
	if err := json.Unmarshal(data, &auth); err != nil {
		return signals, fmt.Errorf("parse OpenCode auth: %w", err)
	}
	for providerID := range auth {
		if providerID != "" {
			signals.AuthProviders[providerID] = true
		}
	}
	return signals, nil
}

func emptySignals() ProviderSignals {
	return ProviderSignals{
		SettingsProviders: map[string]bool{},
		AuthProviders:     map[string]bool{},
		EnabledProviders:  map[string]bool{},
		DisabledProviders: map[string]bool{},
	}
}

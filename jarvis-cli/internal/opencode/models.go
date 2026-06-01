package opencode

import (
	"encoding/json"
	"fmt"
	"os"
)

// Catalog contains OpenCode providers loaded from the local models cache.
type Catalog struct {
	Providers map[string]Provider
}

// Provider describes an OpenCode model provider.
type Provider struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Env          []string         `json:"env"`
	AuthRequired bool             `json:"auth_required"`
	Models       map[string]Model `json:"models"`
}

// Model describes an OpenCode model entry relevant to SDD selection.
type Model struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolCall  bool   `json:"tool_call"`
	Reasoning bool   `json:"reasoning"`
}

// DiscoverModelCache loads the first existing OpenCode models cache candidate.
// Missing candidates are skipped so callers can pass OS-standard and legacy paths
// in deterministic precedence order. If no candidate exists, it returns an empty catalog.
func DiscoverModelCache(candidates []string) (Catalog, string, error) {
	empty := Catalog{Providers: map[string]Provider{}}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return empty, candidate, fmt.Errorf("inspect OpenCode models cache: %w", err)
		}
		catalog, err := LoadModelCache(candidate)
		if err != nil {
			return catalog, candidate, err
		}
		return catalog, candidate, nil
	}
	return empty, "", nil
}

// LoadModelCache loads OpenCode's models.json cache.
// Missing files are treated as an empty cache so installation can continue.
func LoadModelCache(path string) (Catalog, error) {
	empty := Catalog{Providers: map[string]Provider{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return empty, nil
		}
		return empty, fmt.Errorf("read OpenCode models cache: %w", err)
	}

	providers := map[string]Provider{}
	if err := json.Unmarshal(data, &providers); err != nil {
		return empty, fmt.Errorf("parse OpenCode models cache: %w", err)
	}
	for id, provider := range providers {
		if provider.ID == "" {
			provider.ID = id
		}
		if provider.Models == nil {
			provider.Models = map[string]Model{}
		}
		for modelID, model := range provider.Models {
			if model.ID == "" {
				model.ID = modelID
			}
			provider.Models[modelID] = model
		}
		providers[id] = provider
	}

	return Catalog{Providers: providers}, nil
}

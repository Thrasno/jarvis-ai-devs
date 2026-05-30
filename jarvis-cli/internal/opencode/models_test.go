package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadModelCache_LoadsProvidersAndModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	writeFile(t, path, `{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"env": ["OPENAI_API_KEY"],
			"models": {
				"gpt-5.1": {
					"id": "gpt-5.1",
					"name": "GPT 5.1",
					"tool_call": true,
					"reasoning": true
				},
				"gpt-4o-mini": {
					"id": "gpt-4o-mini",
					"name": "GPT 4o mini",
					"tool_call": false,
					"reasoning": false
				}
			}
		}
	}`)

	catalog, err := LoadModelCache(path)
	if err != nil {
		t.Fatalf("LoadModelCache: %v", err)
	}

	provider, ok := catalog.Providers["openai"]
	if !ok {
		t.Fatalf("expected openai provider, got %#v", catalog.Providers)
	}
	if provider.Name != "OpenAI" || len(provider.Env) != 1 || provider.Env[0] != "OPENAI_API_KEY" {
		t.Fatalf("unexpected provider: %+v", provider)
	}
	model := provider.Models["gpt-5.1"]
	if model.Name != "GPT 5.1" || !model.ToolCall || !model.Reasoning {
		t.Fatalf("unexpected model: %+v", model)
	}
}

func TestLoadModelCache_MissingFileReturnsEmptyCatalog(t *testing.T) {
	catalog, err := LoadModelCache(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadModelCache missing file: %v", err)
	}
	if len(catalog.Providers) != 0 {
		t.Fatalf("expected empty catalog, got %#v", catalog.Providers)
	}
}

func TestLoadModelCache_MalformedJSONReturnsDiagnosticError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	writeFile(t, path, `{not-json`)

	catalog, err := LoadModelCache(path)
	if err == nil {
		t.Fatal("expected malformed cache error")
	}
	if !strings.Contains(err.Error(), "parse OpenCode models cache") {
		t.Fatalf("expected diagnostic parse error, got %v", err)
	}
	if len(catalog.Providers) != 0 {
		t.Fatalf("expected empty catalog on malformed cache, got %#v", catalog.Providers)
	}
}

func TestDiscoverModelCache_UsesFirstExistingCandidate(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "standard", "models.json")
	legacy := filepath.Join(dir, "legacy", "models.json")
	writeFile(t, legacy, `{"openai":{"name":"OpenAI","models":{"gpt-5.1":{"name":"GPT 5.1"}}}}`)

	catalog, source, err := DiscoverModelCache([]string{missing, legacy})
	if err != nil {
		t.Fatalf("DiscoverModelCache: %v", err)
	}
	if source != legacy {
		t.Fatalf("source = %q, want %q", source, legacy)
	}
	provider := catalog.Providers["openai"]
	if provider.ID != "openai" || provider.Models["gpt-5.1"].ID != "gpt-5.1" {
		t.Fatalf("unexpected catalog: %#v", catalog.Providers)
	}
}

func TestDiscoverModelCache_ReturnsEmptyWhenNoCandidatesExist(t *testing.T) {
	dir := t.TempDir()

	catalog, source, err := DiscoverModelCache([]string{
		filepath.Join(dir, "standard", "models.json"),
		filepath.Join(dir, "legacy", "models.json"),
	})
	if err != nil {
		t.Fatalf("DiscoverModelCache missing candidates: %v", err)
	}
	if source != "" {
		t.Fatalf("source = %q, want empty", source)
	}
	if len(catalog.Providers) != 0 {
		t.Fatalf("expected empty catalog, got %#v", catalog.Providers)
	}
}

func TestDiscoverModelCache_ReturnsMalformedErrorForFirstExistingCandidate(t *testing.T) {
	dir := t.TempDir()
	standard := filepath.Join(dir, "standard", "models.json")
	legacy := filepath.Join(dir, "legacy", "models.json")
	writeFile(t, standard, `{not-json`)
	writeFile(t, legacy, `{"openai":{"models":{}}}`)

	catalog, source, err := DiscoverModelCache([]string{standard, legacy})
	if err == nil {
		t.Fatal("expected malformed first candidate error")
	}
	if source != standard {
		t.Fatalf("source = %q, want %q", source, standard)
	}
	if !strings.Contains(err.Error(), "parse OpenCode models cache") {
		t.Fatalf("expected diagnostic parse error, got %v", err)
	}
	if len(catalog.Providers) != 0 {
		t.Fatalf("expected empty catalog on malformed cache, got %#v", catalog.Providers)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

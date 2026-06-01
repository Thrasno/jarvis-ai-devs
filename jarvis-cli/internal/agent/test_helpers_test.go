package agent

import (
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

func isolateTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	setTestHome(t, home)
	return home
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}

func stubRuntimeConfig(t *testing.T, cfg *config.AppConfig) {
	t.Helper()
	previous := loadAppConfig
	loadAppConfig = func() (*config.AppConfig, error) { return cfg, nil }
	t.Cleanup(func() { loadAppConfig = previous })
}

func defaultRuntimeConfig() *config.AppConfig {
	return &config.AppConfig{
		SchemaVersion:    2,
		APIURL:           config.DefaultAPIURL,
		PersonaPreset:    "argentino",
		SelectedSkills:   []string{},
		ConfiguredAgents: []string{},
		Scope:            config.ScopeLocalOnly,
		Install: config.InstallState{
			Mode:   string(config.ConfigStatusSetup),
			Agents: map[string]config.AgentState{},
		},
	}
}

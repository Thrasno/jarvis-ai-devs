// Package config manages the Jarvis-CLI configuration stored at ~/.jarvis/config.yaml.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultAPIURL is the production Hive Cloud URL.
	DefaultAPIURL = "https://hivemem.dev"

	// configFileName is the config file name (without extension).
	configFileName = "config"

	// configFileExt is the config file extension (without dot).
	configFileExt = "yaml"
)

const currentSchemaVersion = 2

type ConfigStatus string

type SetupScope string

const (
	ScopeLocalOnly  SetupScope = "local-only"
	ScopeLocalCloud SetupScope = "local+cloud"
)

const (
	ConfigStatusSetup       ConfigStatus = "setup"
	ConfigStatusReconfigure ConfigStatus = "reconfigure"
	ConfigStatusRecover     ConfigStatus = "recover"
)

// CloudConfig stores optional Hive Cloud state.
type CloudConfig struct {
	Email          string `mapstructure:"email" yaml:"email,omitempty"`
	SyncConfigured bool   `mapstructure:"sync_configured" yaml:"sync_configured"`
}

// AgentState stores setup status per detected agent.
type AgentState struct {
	Configured       bool   `mapstructure:"configured" yaml:"configured"`
	InstructionsPath string `mapstructure:"instructions_path" yaml:"instructions_path,omitempty"`
	ConfigPath       string `mapstructure:"config_path" yaml:"config_path,omitempty"`
}

// InstallState stores machine-scoped setup completion metadata.
type InstallState struct {
	Mode      string                `mapstructure:"mode" yaml:"mode,omitempty"`
	Completed bool                  `mapstructure:"completed" yaml:"completed"`
	Agents    map[string]AgentState `mapstructure:"agents" yaml:"agents,omitempty"`
}

// AppConfig holds all Jarvis-CLI configuration.
type AppConfig struct {
	SchemaVersion int `mapstructure:"schema_version" yaml:"schema_version"`

	// APIURL is the Hive Cloud API base URL.
	APIURL string `mapstructure:"api_url" yaml:"api_url"`
	// PersonaPreset is the active persona preset name.
	PersonaPreset string `mapstructure:"persona_preset" yaml:"persona_preset,omitempty"`
	// PersonaPresetSource identifies where PersonaPreset was resolved from.
	// Allowed values: "builtin" or "user".
	PersonaPresetSource string `mapstructure:"persona_preset_source" yaml:"persona_preset_source,omitempty"`
	// SelectedSkills stores selected skill IDs.
	SelectedSkills []string `mapstructure:"selected_skills" yaml:"selected_skills,omitempty"`

	// ConfiguredAgents lists agents that have been fully configured by the wizard.
	ConfiguredAgents []string     `mapstructure:"configured_agents" yaml:"configured_agents"`
	Cloud            *CloudConfig `mapstructure:"cloud" yaml:"cloud,omitempty"`
	Scope            SetupScope   `mapstructure:"scope" yaml:"scope,omitempty"`
	Install          InstallState `mapstructure:"install" yaml:"install,omitempty"`

	// Legacy compatibility fields (v1 schema). These are normalized on load.
	Email  string `mapstructure:"email" yaml:"email,omitempty"`
	Preset string `mapstructure:"preset" yaml:"preset,omitempty"`

	// Version is the jarvis-cli version that wrote this config (for future migrations).
	Version string `mapstructure:"version" yaml:"version"`
}

// ConfigPath returns the expanded path to the Jarvis config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".jarvis", configFileName+"."+configFileExt), nil
}

func defaultConfig() *AppConfig {
	return &AppConfig{
		SchemaVersion:       currentSchemaVersion,
		APIURL:              DefaultAPIURL,
		PersonaPreset:       "argentino",
		PersonaPresetSource: "builtin",
		SelectedSkills:      []string{},
		Scope:               ScopeLocalOnly,
		ConfiguredAgents:    []string{},
		Install: InstallState{
			Mode:   string(ConfigStatusSetup),
			Agents: map[string]AgentState{},
		},
		Version: "",
	}
}

// Load reads the config from ~/.jarvis/config.yaml.
// Returns default AppConfig if the file doesn't exist yet.
func Load() (*AppConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return applyEnvOverrides(defaultConfig()), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &AppConfig{}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}

	normalizeAndMigrate(cfg)
	applyEnvOverrides(cfg)
	return cfg, nil
}

// Save writes the config to ~/.jarvis/config.yaml atomically.
func Save(cfg *AppConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	normalizeAndMigrate(cfg)

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create jarvis dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := atomicWriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (c *AppConfig) ConfigStatus() ConfigStatus {
	if c == nil {
		return ConfigStatusSetup
	}
	if c.IsReadyForReconfigure() {
		return ConfigStatusReconfigure
	}
	if c.hasAnyState() {
		return ConfigStatusRecover
	}
	return ConfigStatusSetup
}

func (c *AppConfig) IsReadyForReconfigure() bool {
	if c == nil {
		return false
	}
	if c.SchemaVersion < currentSchemaVersion {
		return false
	}
	if strings.TrimSpace(c.APIURL) == "" {
		return false
	}
	if strings.TrimSpace(c.PersonaPreset) == "" {
		return false
	}
	if len(c.SelectedSkills) == 0 {
		return false
	}
	if !c.Install.Completed {
		return false
	}
	for _, name := range c.ConfiguredAgents {
		st, ok := c.Install.Agents[name]
		if !ok || !st.Configured {
			return false
		}
	}
	return true
}

// IsConfigured reports if machine state is ready for reconfiguration.
func IsConfigured() bool {
	cfg, err := Load()
	if err != nil {
		return false
	}
	return cfg.IsReadyForReconfigure()
}

func normalizeAndMigrate(cfg *AppConfig) {
	if cfg.SchemaVersion < currentSchemaVersion {
		cfg.SchemaVersion = currentSchemaVersion
	}
	if strings.TrimSpace(cfg.APIURL) == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.Install.Agents == nil {
		cfg.Install.Agents = map[string]AgentState{}
	}
	if cfg.ConfiguredAgents == nil {
		cfg.ConfiguredAgents = []string{}
	}
	if cfg.SelectedSkills == nil {
		cfg.SelectedSkills = []string{}
	}

	if strings.TrimSpace(cfg.PersonaPreset) == "" {
		cfg.PersonaPreset = strings.TrimSpace(cfg.Preset)
	}
	if strings.TrimSpace(cfg.PersonaPreset) == "" {
		cfg.PersonaPreset = "argentino"
	}

	source := strings.ToLower(strings.TrimSpace(cfg.PersonaPresetSource))
	switch source {
	case "builtin", "user":
		cfg.PersonaPresetSource = source
	default:
		cfg.PersonaPresetSource = "builtin"
	}

	if cfg.Cloud == nil && strings.TrimSpace(cfg.Email) != "" {
		cfg.Cloud = &CloudConfig{Email: strings.TrimSpace(cfg.Email)}
	}
	if cfg.Cloud != nil {
		cfg.Cloud.Email = strings.TrimSpace(cfg.Cloud.Email)
		cfg.Email = cfg.Cloud.Email
	} else {
		cfg.Email = ""
	}
	cfg.Preset = cfg.PersonaPreset

	switch cfg.Scope {
	case ScopeLocalOnly, ScopeLocalCloud:
		// valid value
	default:
		if hasStoredCloudLink(cfg) {
			cfg.Scope = ScopeLocalCloud
		} else {
			cfg.Scope = ScopeLocalOnly
		}
	}

	if cfg.Install.Mode == "" {
		cfg.Install.Mode = string(cfg.ConfigStatus())
	}
	cfg.Install.Completed = cfg.IsReadyForReconfigure()
}

func hasStoredCloudLink(cfg *AppConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.Cloud == nil {
		return false
	}
	return strings.TrimSpace(cfg.Cloud.Email) != "" || cfg.Cloud.SyncConfigured
}

func applyEnvOverrides(cfg *AppConfig) *AppConfig {
	if cfg == nil {
		return cfg
	}
	if v := strings.TrimSpace(os.Getenv("JARVIS_API_URL")); v != "" {
		cfg.APIURL = v
	}
	return cfg
}

func (c *AppConfig) hasAnyState() bool {
	if c == nil {
		return false
	}
	if strings.TrimSpace(c.APIURL) != "" && c.APIURL != DefaultAPIURL {
		return true
	}
	if strings.TrimSpace(c.PersonaPreset) != "" && c.PersonaPreset != "argentino" {
		return true
	}
	if len(c.SelectedSkills) > 0 || len(c.ConfiguredAgents) > 0 {
		return true
	}
	if c.Cloud != nil && strings.TrimSpace(c.Cloud.Email) != "" {
		return true
	}
	if c.Install.Completed || len(c.Install.Agents) > 0 {
		return true
	}
	return false
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := bytes.NewReader(data).WriteTo(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent dir: %w", err)
	}
	defer func() {
		_ = d.Close()
	}()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("fsync parent dir: %w", err)
	}

	cleanup = false
	return nil
}

// Package config manages the Jarvis-CLI configuration stored at ~/.jarvis/config.yaml.
// It uses Viper for YAML reading/writing with env var override support.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	// DefaultAPIURL is the production Hive Cloud URL.
	DefaultAPIURL = "https://hivemem.dev"

	// configFileName is the config file name (without extension).
	configFileName = "config"

	// configFileExt is the config file extension (without dot).
	configFileExt = "yaml"
)

// AppConfig holds all Jarvis-CLI configuration.
type AppConfig struct {
	// APIURL is the Hive Cloud API base URL.
	APIURL string `mapstructure:"api_url" yaml:"api_url"`

	// Email is the Hive Cloud account email.
	Email string `mapstructure:"email" yaml:"email"`

	// Preset is the active persona preset name (e.g. "argentino", "tony-stark").
	Preset string `mapstructure:"preset" yaml:"preset"`

	// ConfiguredAgents lists agents that have been fully configured by the wizard.
	ConfiguredAgents []string `mapstructure:"configured_agents" yaml:"configured_agents"`

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

// Load reads the config from ~/.jarvis/config.yaml.
// Returns a default AppConfig if the file doesn't exist yet (first-run scenario).
func Load() (*AppConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType(configFileExt)

	// Defaults
	v.SetDefault("api_url", DefaultAPIURL)
	v.SetDefault("preset", "argentino")
	v.SetDefault("version", "")
	v.SetDefault("configured_agents", []string{})

	// Allow env var override (e.g. JARVIS_API_URL)
	v.SetEnvPrefix("JARVIS")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) || isViperNotFound(err) {
			// First run — return defaults
			cfg := &AppConfig{}
			if err := v.Unmarshal(cfg); err != nil {
				return nil, fmt.Errorf("unmarshal defaults: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &AppConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}

// Save writes the config to ~/.jarvis/config.yaml atomically.
func Save(cfg *AppConfig) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	// Ensure ~/.jarvis directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create jarvis dir: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType(configFileExt)

	v.Set("api_url", cfg.APIURL)
	v.Set("email", cfg.Email)
	v.Set("preset", cfg.Preset)
	v.Set("configured_agents", cfg.ConfiguredAgents)
	v.Set("version", cfg.Version)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// IsConfigured returns true if ~/.jarvis/config.yaml exists and contains
// both api_url and email (indicating the wizard has been completed).
func IsConfigured() bool {
	cfg, err := Load()
	if err != nil {
		return false
	}
	return cfg.APIURL != "" && cfg.Email != ""
}

// isViperNotFound checks if a Viper error is a "config file not found" error.
func isViperNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Viper wraps os.ErrNotExist in its own type; check the message as fallback.
	_, isNotFound := err.(viper.ConfigFileNotFoundError)
	return isNotFound || os.IsNotExist(err)
}

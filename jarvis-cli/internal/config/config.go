// Package config manages the Jarvis-CLI configuration stored at ~/.jarvis/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/atomicfile"
)

const (
	// DefaultAPIURL is the production Hive Cloud URL.
	DefaultAPIURL = "https://hivemem.dev"

	// configFileName is the config file name (without extension).
	configFileName = "config"

	// configFileExt is the config file extension (without dot).
	configFileExt = "yaml"
)

// currentSchemaVersion is 3: the version at which the replay fields left
// config.yaml for ~/.jarvis/state.yaml. state.Migrate is what moves them and
// stamps an existing file; this is what a file written from scratch carries.
const currentSchemaVersion = 3

type ConfigStatus string

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

// InstallState stores machine-scoped setup completion metadata.
//
// The per-agent records this used to carry moved to ~/.jarvis/state.yaml, which
// owns what the last installation configured. Mode and Completed stay because
// they describe config.yaml's own history, not the desired state to replay.
type InstallState struct {
	Mode      string `mapstructure:"mode" yaml:"mode,omitempty"`
	Completed bool   `mapstructure:"completed" yaml:"completed"`
}

// AppConfig holds the Jarvis-CLI configuration stored in ~/.jarvis/config.yaml.
//
// ~/.jarvis/config.yaml and ~/.jarvis/state.yaml are disjoint stores. The
// persona, the selected skills, the configured agents, the scope and the
// per-phase SDD models are desired state to replay, so internal/state owns them
// and this struct deliberately has no field for any of them: a value that cannot
// be spelled here cannot drift from the store that owns it.
type AppConfig struct {
	SchemaVersion int `mapstructure:"schema_version" yaml:"schema_version"`

	// APIURL is the Hive Cloud API base URL.
	APIURL string       `mapstructure:"api_url" yaml:"api_url"`
	Cloud  *CloudConfig `mapstructure:"cloud" yaml:"cloud,omitempty"`

	Install InstallState `mapstructure:"install" yaml:"install,omitempty"`

	// Email is a legacy compatibility field (v1 schema), normalized on load
	// against Cloud.
	Email string `mapstructure:"email" yaml:"email,omitempty"`

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
		SchemaVersion: currentSchemaVersion,
		APIURL:        DefaultAPIURL,
		Install: InstallState{
			Mode: string(ConfigStatusSetup),
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

	data, err := marshalPreservingUnknownKeys(cfg, path)
	if err != nil {
		return err
	}

	if err := atomicfile.Write(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// RecordedInstall is the half of a recorded installation that config.yaml does
// not own.
//
// ~/.jarvis/state.yaml owns the persona, the selected skills and the configured
// agents, so "is this machine installed" is a question neither store can answer
// alone. Passing that half in as a plain value is what lets this package stay
// independent of the manifest package: the two stores are disjoint in the import
// graph as well as on disk, and every caller can see at the call site that the
// answer needs both.
//
// Build it from the manifest with State.RecordsCompleteInstall and
// State.RecordsAnyState.
type RecordedInstall struct {
	// Complete reports that the manifest records a persona, at least one skill
	// and at least one configured agent.
	Complete bool
	// Populated reports that the manifest records any choice at all, which is
	// what tells a damaged installation apart from a machine never set up.
	Populated bool
}

// ConfigStatus reports which flow this machine is in, given what the manifest
// recorded.
func (c *AppConfig) ConfigStatus(recorded RecordedInstall) ConfigStatus {
	if c == nil {
		return ConfigStatusSetup
	}
	if c.IsReadyForReconfigure(recorded) {
		return ConfigStatusReconfigure
	}
	if recorded.Populated || c.hasAnyState() {
		return ConfigStatusRecover
	}
	return ConfigStatusSetup
}

// IsReadyForReconfigure reports whether this machine carries a complete recorded
// installation, combining what config.yaml owns with what the manifest recorded.
//
// Install.Completed is read here and never written back. It used to be
// recomputed from this same predicate on every load and save, which made it
// self-referential: once a save persisted false the machine could never report
// ready again. It is now simply what the installer recorded when it finished.
func (c *AppConfig) IsReadyForReconfigure(recorded RecordedInstall) bool {
	if c == nil {
		return false
	}
	if c.SchemaVersion < currentSchemaVersion {
		return false
	}
	if strings.TrimSpace(c.APIURL) == "" {
		return false
	}
	if !c.Install.Completed {
		return false
	}
	return recorded.Complete
}

func normalizeAndMigrate(cfg *AppConfig) {
	if cfg.SchemaVersion < currentSchemaVersion {
		cfg.SchemaVersion = currentSchemaVersion
	}
	if strings.TrimSpace(cfg.APIURL) == "" {
		cfg.APIURL = DefaultAPIURL
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

	// install.mode records which flow last touched this file. It cannot be
	// derived from the full status any more, because that needs the manifest and
	// this store does not read it; the config-owned signals still separate a
	// machine carrying state from a fresh one.
	if cfg.Install.Mode == "" {
		if cfg.hasAnyState() {
			cfg.Install.Mode = string(ConfigStatusRecover)
		} else {
			cfg.Install.Mode = string(ConfigStatusSetup)
		}
	}
}

// HasStoredCloudLink reports whether this machine already has a Hive Cloud link
// recorded in config.yaml.
//
// It is the seam the scope default needs. ~/.jarvis/state.yaml owns the scope
// and decides what an unrecorded one means, but the evidence it decides on lives
// here, so callers read it from the config and hand it to State.ResolvedScope.
func (c *AppConfig) HasStoredCloudLink() bool {
	if c == nil || c.Cloud == nil {
		return false
	}
	return strings.TrimSpace(c.Cloud.Email) != "" || c.Cloud.SyncConfigured
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

// hasAnyState reports whether config.yaml carries anything beyond a fresh
// machine's defaults. It covers only the keys this store owns; the manifest
// answers for its own half through State.RecordsAnyState, and ConfigStatus
// combines them.
func (c *AppConfig) hasAnyState() bool {
	if c == nil {
		return false
	}
	if strings.TrimSpace(c.APIURL) != "" && c.APIURL != DefaultAPIURL {
		return true
	}
	if c.Cloud != nil && strings.TrimSpace(c.Cloud.Email) != "" {
		return true
	}
	return c.Install.Completed
}

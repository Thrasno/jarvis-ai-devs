package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

// settableKeys lists the config keys that users are allowed to change.
// configured_agents and version are managed by the wizard and are read-only.
var settableKeys = []string{"preset", "api_url", "email"}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View current Jarvis configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigView()
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Update a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigSet(args[0], args[1])
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
}

// runConfigView prints all configuration values to stdout.
func runConfigView() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	agents := strings.Join(cfg.ConfiguredAgents, ", ")
	if agents == "" {
		agents = "(none)"
	}
	version := cfg.Version
	if version == "" {
		version = "(unset)"
	}

	fmt.Println("Current configuration:")
	fmt.Printf("  %-20s %s\n", "preset:", cfg.Preset)
	fmt.Printf("  %-20s %s\n", "api_url:", cfg.APIURL)
	fmt.Printf("  %-20s %s\n", "email:", cfg.Email)
	fmt.Printf("  %-20s %s\n", "configured_agents:", agents)
	fmt.Printf("  %-20s %s\n", "version:", version)
	return nil
}

// runConfigSet updates a single settable key in the config and saves it.
func runConfigSet(key, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch key {
	case "preset":
		cfg.Preset = value
	case "api_url":
		cfg.APIURL = value
	case "email":
		cfg.Email = value
	default:
		return fmt.Errorf("unknown key %q — settable keys: %s", key, strings.Join(settableKeys, ", "))
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("✓ %s updated to: %s\n", key, value)
	return nil
}

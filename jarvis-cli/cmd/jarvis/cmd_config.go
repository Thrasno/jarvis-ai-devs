package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
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

var configForgetAgentCmd = &cobra.Command{
	Use:   "forget-agent <agent>",
	Short: "Stop managing an agent this machine no longer has",
	Long: "Removes an agent from the desired-state manifest so jarvis stops replaying its\n" +
		"configuration. Nothing is deleted from disk; only the record is removed.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigForgetAgent(args[0])
	},
}

func init() {
	configCmd.AddCommand(configSetCmd, configForgetAgentCmd)
}

// runConfigForgetAgent removes one agent from the desired-state manifest.
//
// This is the only way an agent record leaves ~/.jarvis/state.yaml, and it is
// deliberately a command the user runs rather than a side effect of a run that
// did not detect the agent. Detection is presence-based: a config directory that
// moved makes an agent invisible without saying anything about ownership, so the
// installer keeps the record. Without this exit the record was permanent, and
// `jarvis sync` kept rebuilding the managed files it names on every run.
func runConfigForgetAgent(agentID string) error {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return fmt.Errorf("name the agent to forget — recorded agents: %s", recordedAgentsForMessage(nil))
	}

	// Migrate before touching the manifest, for the same reason `config set`
	// does: a machine upgrading into this version still holds its agents in
	// config.yaml, and writing a manifest here first would strand them.
	manifest, err := loadManifestForPersona()
	if err != nil {
		return err
	}
	if !recordsAgent(manifest, id) {
		return fmt.Errorf("%s is not a recorded agent — recorded agents: %s", id, recordedAgentsForMessage(manifest))
	}

	if err := state.Update(func(st *state.State) {
		st.InstalledAgents = withoutAgent(st.InstalledAgents, id)
	}); err != nil {
		return fmt.Errorf("remove %s from the desired-state manifest: %w", id, err)
	}

	fmt.Printf("✓ %s removed from the desired-state manifest; jarvis no longer manages its files.\n", id)
	fmt.Println("  Its files were left on disk exactly as they are.")
	return nil
}

// recordsAgent reports whether the manifest holds a record for this agent ID.
func recordsAgent(manifest *state.State, id string) bool {
	for _, recorded := range manifestAgentIDs(manifest) {
		if recorded == id {
			return true
		}
	}
	return false
}

// withoutAgent returns the recorded agents with one ID removed.
func withoutAgent(agents []state.Agent, id string) []state.Agent {
	kept := make([]state.Agent, 0, len(agents))
	for _, agent := range agents {
		if strings.TrimSpace(agent.ID) == id {
			continue
		}
		kept = append(kept, agent)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// recordedAgentsForMessage lists the recorded agent IDs for an error message.
func recordedAgentsForMessage(manifest *state.State) string {
	ids := manifestAgentIDs(manifest)
	if len(ids) == 0 {
		return "(none)"
	}
	return strings.Join(ids, ", ")
}

// runConfigView prints all configuration values to stdout.
func runConfigView() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ~/.jarvis/state.yaml owns the persona and the configured agents.
	manifest, err := loadManifestForPersona()
	if err != nil {
		return err
	}

	agents := strings.Join(manifestAgentIDs(manifest), ", ")
	if agents == "" {
		agents = "(none)"
	}
	version := cfg.Version
	if version == "" {
		version = "(unset)"
	}

	fmt.Println("Current configuration:")
	persona, _ := manifest.ResolvedPersona()
	fmt.Printf("  %-20s %s\n", "preset:", persona)
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

	// recordInManifest carries the fields ~/.jarvis/state.yaml owns.
	var recordInManifest func(*state.State)

	switch key {
	case "preset":
		resolved, err := persona.ResolveProfile(jarvis.PersonaFS, value)
		if err != nil {
			return fmt.Errorf("invalid preset %q: %w", value, err)
		}
		value = resolved.Slug
		recordInManifest = func(st *state.State) {
			st.Persona = resolved.Slug
			st.PersonaSource = state.PersonaSource(resolved.Source)
		}
	case "api_url":
		cfg.APIURL = value
	case "email":
		if cfg.Cloud == nil {
			cfg.Cloud = &config.CloudConfig{}
		}
		cfg.Cloud.Email = value
		cfg.Email = value
	default:
		return fmt.Errorf("unknown key %q — settable keys: %s", key, strings.Join(settableKeys, ", "))
	}

	if recordInManifest != nil {
		// Migrate before touching the manifest. A machine upgrading into this
		// version still has its persona, skills and agents in config.yaml and no
		// manifest at all; writing one here without migrating first would create
		// a manifest carrying only this persona, and the migration would then
		// find one already there and never carry the rest across.
		if _, err := loadManifestForPersona(); err != nil {
			return err
		}
		if err := state.Update(recordInManifest); err != nil {
			return fmt.Errorf("record %s in the desired-state manifest: %w", key, err)
		}
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("✓ %s updated to: %s\n", key, value)
	return nil
}

// manifestAgentIDs returns the configured agent IDs in the order the manifest
// records them, which is the order the last installation configured them in.
func manifestAgentIDs(manifest *state.State) []string {
	if manifest == nil {
		return nil
	}
	ids := make([]string, 0, len(manifest.InstalledAgents))
	for _, agent := range manifest.InstalledAgents {
		ids = append(ids, agent.ID)
	}
	return ids
}

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/atomicfile"
)

// configSchemaVersionAfterMigration is the config.yaml schema version that
// records the replay fields as moved out.
const configSchemaVersionAfterMigration = 3

const configFileName = "config.yaml"

// replayConfigKeys are the top-level config.yaml keys the manifest takes over.
// They are deleted from config.yaml so no value is readable from both stores.
var replayConfigKeys = []string{
	"persona_preset",
	"persona_preset_source",
	"preset",
	"selected_skills",
	"configured_agents",
	"scope",
	"sdd",
}

// ReplayConfigKeys copies the keys above for other writers keeping them disjoint.
func ReplayConfigKeys() []string {
	out := make([]string, len(replayConfigKeys))
	copy(out, replayConfigKeys)
	return out
}

// Result reports the outcome of a migration attempt.
//
// Notice is populated only after the manifest and the rewritten config.yaml are
// both durably on disk. Callers print it verbatim and must never announce a
// migration from any other signal.
type Result struct {
	Migrated bool
	Notice   string
}

// legacyConfig mirrors only the config.yaml fields the manifest takes over.
// It deliberately ignores everything else so unrelated config keys are never
// decoded, reshaped, or lost.
type legacyConfig struct {
	SchemaVersion       int      `yaml:"schema_version"`
	PersonaPreset       string   `yaml:"persona_preset"`
	Preset              string   `yaml:"preset"`
	PersonaPresetSource string   `yaml:"persona_preset_source"`
	SelectedSkills      []string `yaml:"selected_skills"`
	ConfiguredAgents    []string `yaml:"configured_agents"`
	Scope               string   `yaml:"scope"`

	Install struct {
		Agents map[string]AgentRecord `yaml:"agents"`
	} `yaml:"install"`

	SDD struct {
		PhaseModels         map[string]PhaseModelSelection     `yaml:"phase_models"`
		OpenCodePhaseModels map[string]OpenCodeModelAssignment `yaml:"opencode_phase_models"`
		ClaudePhaseModels   map[string]ClaudeModelAssignment   `yaml:"claude_phase_models"`
	} `yaml:"sdd"`
}

// Migrate moves the replay fields out of ~/.jarvis/config.yaml into
// ~/.jarvis/state.yaml exactly once and advances config.yaml to schema 3.
//
// The move is one-way: after a successful migration no replay field is readable
// from config.yaml. Migration is never gated on replay-readiness, so a config
// whose replay fields were never populated still migrates; replay blocks
// afterwards through State.ValidateForReplay.
//
// The manifest is written and fsynced before config.yaml is rewritten. If the
// manifest write fails, config.yaml is left untouched at its pre-migration
// schema version and no notice is produced.
func Migrate() (Result, error) {
	statePath, err := Path()
	if err != nil {
		return Result{}, err
	}
	// A manifest on disk already owns the replay fields; re-deriving them from a
	// config.yaml they have already left would erase them.
	if info, err := os.Stat(statePath); err == nil {
		if info.Mode().IsRegular() {
			return Result{}, nil
		}
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("stat %s: %w", stateFileName, err)
	}

	configPath, err := configFilePath()
	if err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to migrate on a fresh machine.
			return Result{}, nil
		}
		return Result{}, fmt.Errorf("read %s: %w", configFileName, err)
	}

	var legacy legacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", configFileName, err)
	}
	if legacy.SchemaVersion >= configSchemaVersionAfterMigration {
		// Already migrated; the manifest owns these fields.
		return Result{}, nil
	}

	raw := map[string]any{}
	if strings.TrimSpace(string(data)) != "" {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", configFileName, err)
		}
	}

	manifest := manifestFromLegacyConfig(legacy)
	if err := manifest.Validate(); err != nil {
		return Result{}, fmt.Errorf("migrate %s: %w", configFileName, err)
	}

	// Durability gate: the manifest lands first. Everything after this point is
	// reachable only because the replay fields are already safe on disk.
	if err := Save(manifest); err != nil {
		return Result{}, fmt.Errorf("write %s during migration: %w", stateFileName, err)
	}

	stripReplayFields(raw)
	raw["schema_version"] = configSchemaVersionAfterMigration

	rewritten, err := yaml.Marshal(raw)
	if err != nil {
		return Result{}, fmt.Errorf("marshal %s: %w", configFileName, err)
	}
	if err := atomicfile.WriteYAML(configPath, rewritten); err != nil {
		return Result{}, fmt.Errorf("write %s during migration: %w", configFileName, err)
	}

	return Result{
		Migrated: true,
		Notice: fmt.Sprintf(
			"Moved installation state out of %s into %s; %s is now at schema version %d.",
			configFileName, stateFileName, configFileName, configSchemaVersionAfterMigration,
		),
	}, nil
}

// manifestFromLegacyConfig builds the manifest from the legacy config's replay
// fields. Values are carried over verbatim; nothing is filtered or defaulted.
func manifestFromLegacyConfig(legacy legacyConfig) *State {
	manifest := New()

	manifest.Persona = firstNonEmpty(legacy.PersonaPreset, legacy.Preset)
	manifest.PersonaSource = PersonaSource(legacy.PersonaPresetSource)

	if legacy.SelectedSkills != nil {
		manifest.Skills = legacy.SelectedSkills
	}

	manifest.InstalledAgents = InstalledAgentsFrom(legacy.ConfiguredAgents, legacy.Install.Agents)
	manifest.SelectionConfigured = selectionWasMade(legacy)

	if legacy.SDD.PhaseModels != nil {
		manifest.PhaseModels.Aliases = legacy.SDD.PhaseModels
	}
	if legacy.SDD.OpenCodePhaseModels != nil {
		manifest.PhaseModels.OpenCode = legacy.SDD.OpenCodePhaseModels
	}
	if legacy.SDD.ClaudePhaseModels != nil {
		manifest.PhaseModels.Claude = legacy.SDD.ClaudePhaseModels
	}

	manifest.Scope = Scope(legacy.Scope)
	return manifest
}

// AgentRecord mirrors one config.yaml `install.agents` entry.
// selectionWasMade reports whether the legacy config carries evidence that the
// user was actually asked to choose agents. That is what SelectionConfigured
// records, and the field exists to tell "selected nothing" apart from "never
// asked", so counting how many agents happen to be mentioned answers a
// different question than the one being asked.
//
// Two kinds of evidence count, and one does not:
//
//   - A present configured_agents key counts even when the list is empty. The
//     wizard writes that key only after asking, so an empty list is a recorded
//     answer of "none" rather than an absent answer.
//   - A record marked configured counts, for the same reason it is the thing
//     InstalledAgentsFrom carries over.
//   - install.agents entries that are not configured do not count. The
//     installer writes them from what it detected on the machine, before the
//     user answers anything, so they are evidence of detection and nothing else.
//
// This runs inside a one-way migration that executes once per machine, so a
// value derived from the wrong evidence is not a bug a later release can fix by
// changing this function; it would need a second migration to undo.
func selectionWasMade(legacy legacyConfig) bool {
	if legacy.ConfiguredAgents != nil {
		return true
	}
	for _, record := range legacy.Install.Agents {
		if record.Configured {
			return true
		}
	}
	return false
}

type AgentRecord struct {
	Configured       bool   `yaml:"configured"`
	InstructionsPath string `yaml:"instructions_path"`
	ConfigPath       string `yaml:"config_path"`
}

// InstalledAgentsFrom merges the ordered configured_agents list with the
// per-agent paths recorded under install.agents. An agent that was never
// configured is not installed and is therefore not carried over. Migration and
// the config bridge both go through here, so they cannot disagree.
func InstalledAgentsFrom(order []string, records map[string]AgentRecord) []Agent {
	seen := map[string]bool{}
	agents := make([]Agent, 0, len(order))

	appendAgent := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		details := records[id]
		agents = append(agents, Agent{
			ID:               id,
			InstructionsPath: details.InstructionsPath,
			ConfigPath:       details.ConfigPath,
		})
	}

	for _, id := range order {
		appendAgent(id)
	}

	// install.agents may record a configured agent the list forgot. Sorting keeps
	// the migration deterministic across runs.
	extra := make([]string, 0, len(records))
	for id, details := range records {
		if details.Configured && !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		appendAgent(id)
	}

	if len(agents) == 0 {
		return nil
	}
	return agents
}

// stripReplayFields deletes every migrated key from the decoded config.yaml,
// leaving all other keys exactly as they were.
func stripReplayFields(raw map[string]any) {
	for _, key := range replayConfigKeys {
		delete(raw, key)
	}
	if install, ok := raw["install"].(map[string]any); ok {
		delete(install, "agents")
	}
}

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, jarvisDirName, configFileName), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

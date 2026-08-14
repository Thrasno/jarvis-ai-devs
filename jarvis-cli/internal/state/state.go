// Package state manages ~/.jarvis/state.yaml, the desired-state manifest that
// records what the last installation configured on this machine.
//
// The manifest is versioned independently of ~/.jarvis/config.yaml and their
// field sets are disjoint: every replay-relevant field lives here and nowhere
// else, so the two stores can never disagree and no tie-breaking rule is needed.
//
// Loading is fail-closed. A missing manifest on a fresh machine is acceptable
// and reported as ErrNotFound; a read error, a corrupt file, an incompatible
// schema version, a whitespace-only value, or an unrecognized value all abort
// before any mutation happens.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/atomicfile"
)

// currentSchemaVersion is the state.yaml schema version. It advances
// independently of config.yaml's schema version.
const currentSchemaVersion = 1

const (
	stateFileName = "state.yaml"
	jarvisDirName = ".jarvis"
)

// ErrNotFound reports that no manifest exists yet. On a fresh machine this is
// an expected outcome, not a failure.
var ErrNotFound = errors.New("state manifest not found")

// Scope records how far the installation reaches.
type Scope string

const (
	ScopeLocalOnly  Scope = "local-only"
	ScopeLocalCloud Scope = "local+cloud"
)

// PersonaSource records where the active persona was resolved from.
type PersonaSource string

const (
	PersonaSourceBuiltin PersonaSource = "builtin"
	PersonaSourceUser    PersonaSource = "user"
)

// Agent records one configured agent and where its managed files live.
type Agent struct {
	ID               string `yaml:"id"`
	InstructionsPath string `yaml:"instructions_path,omitempty"`
	ConfigPath       string `yaml:"config_path,omitempty"`
}

// StatuslineState records statusline consent as a tri-state built from two
// plain booleans:
//
//	Decided=false            -> never asked; leave the statusline untouched
//	Decided=true,  Enabled=false -> asked and declined; leave it untouched
//	Decided=true,  Enabled=true  -> asked and accepted; install or maintain it
//
// The zero value is "never asked", which is already the safe outcome, so no
// field-presence detection is required when decoding.
type StatuslineState struct {
	Decided bool `yaml:"statusline_decided"`
	Enabled bool `yaml:"statusline_enabled"`
}

// ShouldManage reports whether replay is authorized to install or maintain the
// statusline. Only a recorded, enabled decision authorizes it: "never asked"
// and "decided against" both mean "do not touch".
func (s StatuslineState) ShouldManage() bool {
	return s.Decided && s.Enabled
}

// PhaseModelSelection stores per-platform model aliases for a single SDD phase.
type PhaseModelSelection struct {
	OpenCode string `yaml:"opencode,omitempty"`
	Claude   string `yaml:"claude,omitempty"`
}

// OpenCodeModelAssignment stores a provider-qualified OpenCode model assignment
// for a single SDD phase.
type OpenCodeModelAssignment struct {
	ProviderID string `yaml:"provider_id,omitempty"`
	ModelID    string `yaml:"model_id,omitempty"`
	Effort     string `yaml:"effort,omitempty"`
}

// ClaudeModelAssignment stores Claude-specific model routing for a single SDD phase.
type ClaudeModelAssignment struct {
	Model  string `yaml:"model,omitempty"`
	Effort string `yaml:"effort,omitempty"`
}

// PhaseModels groups every per-phase model assignment the installation made.
type PhaseModels struct {
	Aliases  map[string]PhaseModelSelection     `yaml:"aliases"`
	OpenCode map[string]OpenCodeModelAssignment `yaml:"opencode"`
	Claude   map[string]ClaudeModelAssignment   `yaml:"claude"`
}

// State is the desired-state manifest persisted at ~/.jarvis/state.yaml.
type State struct {
	SchemaVersion int `yaml:"schema_version"`

	// InstalledAgents lists the agents the last installation configured, in the
	// order they were configured.
	InstalledAgents []Agent `yaml:"installed_agents"`
	// SelectionConfigured records that an explicit agent selection was made,
	// distinguishing "selected nothing" from "never asked".
	SelectionConfigured bool `yaml:"selection_configured"`
	// Skills lists the skill IDs this machine owns. It is never filtered against
	// the current embedded catalog on write: an ID the catalog no longer offers
	// is the only ownership proof that authorizes deleting that skill later.
	Skills []string `yaml:"skills"`

	Persona       string        `yaml:"persona"`
	PersonaSource PersonaSource `yaml:"persona_source"`

	Statusline StatuslineState `yaml:",inline"`

	PhaseModels PhaseModels `yaml:"phase_models"`
	Scope       Scope       `yaml:"scope"`

	// ManagedAssetDigest identifies the embedded asset set that produced the
	// last installation.
	ManagedAssetDigest string `yaml:"managed_asset_digest"`
}

// New returns an empty manifest stamped with the current schema version.
func New() *State {
	return &State{
		SchemaVersion: currentSchemaVersion,
		Skills:        []string{},
		PhaseModels: PhaseModels{
			Aliases:  map[string]PhaseModelSelection{},
			OpenCode: map[string]OpenCodeModelAssignment{},
			Claude:   map[string]ClaudeModelAssignment{},
		},
	}
}

// Path returns the expanded path to the desired-state manifest.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, jarvisDirName, stateFileName), nil
}

// Load reads and validates ~/.jarvis/state.yaml.
//
// It returns ErrNotFound when no manifest exists yet. Every other problem is a
// hard failure: the returned state is nil and the caller must abort before
// mutating anything.
func Load() (*State, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: %w", path, ErrNotFound)
		}
		return nil, fmt.Errorf("read %s: %w", stateFileName, err)
	}

	return decode(data)
}

// decode parses and validates raw manifest bytes without touching the filesystem.
func decode(data []byte) (*State, error) {
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("%s is empty", stateFileName)
	}

	st := &State{}
	if err := yaml.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", stateFileName, err)
	}
	if err := st.Validate(); err != nil {
		return nil, err
	}
	return st, nil
}

// Save validates the manifest and writes it atomically to ~/.jarvis/state.yaml.
//
// The skills list is written exactly as given. Filtering it against the current
// embedded catalog would discard the ownership proof for skills the catalog has
// dropped, so the writer never filters.
func Save(st *State) error {
	if st == nil {
		return errors.New("state is nil")
	}
	if err := st.Validate(); err != nil {
		return err
	}

	path, err := Path()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", stateFileName, err)
	}
	return atomicfile.WriteYAML(path, data)
}

// Update applies mutate to the manifest and writes the result back. A machine
// with no manifest yet gets a fresh one, so a first writer never has to special-
// case the pre-migration state.
//
// It deliberately does not take WithLock. config.Save's temporary bridge takes
// that lock internally, and it is fail-fast and non-reentrant, so a caller that
// writes both stores would deadlock on the first run if this acquired it too.
// Callers therefore sequence the two writes rather than nesting them. Once the
// bridge is gone and config.Save no longer holds the lock, this can take it.
func Update(mutate func(*State)) error {
	st, err := Load()
	if errors.Is(err, ErrNotFound) {
		st = New()
	} else if err != nil {
		return err
	}
	mutate(st)
	return Save(st)
}

// Validate reports structural problems that make the manifest unsafe to trust.
// It deliberately does not require any field to be populated: a manifest
// migrated from a config.yaml with unpopulated replay fields is structurally
// valid, and replay blocks on it later through ValidateForReplay.
func (s *State) Validate() error {
	if s == nil {
		return errors.New("state is nil")
	}
	if s.SchemaVersion != currentSchemaVersion {
		return fmt.Errorf(
			"%s has incompatible schema_version %d, want %d",
			stateFileName, s.SchemaVersion, currentSchemaVersion,
		)
	}

	for i, agent := range s.InstalledAgents {
		if isBlank(agent.ID) {
			return fmt.Errorf("%s installed_agents[%d] has a blank id", stateFileName, i)
		}
		if isBlank(agent.InstructionsPath) {
			return fmt.Errorf("%s installed_agents[%d] (%s) has a blank instructions_path", stateFileName, i, agent.ID)
		}
		if isBlank(agent.ConfigPath) {
			return fmt.Errorf("%s installed_agents[%d] (%s) has a blank config_path", stateFileName, i, agent.ID)
		}
	}

	for i, skill := range s.Skills {
		if isBlank(skill) {
			return fmt.Errorf("%s skills[%d] is blank", stateFileName, i)
		}
	}

	if isBlank(s.Persona) {
		return fmt.Errorf("%s persona is blank", stateFileName)
	}
	switch s.PersonaSource {
	case "", PersonaSourceBuiltin, PersonaSourceUser:
	default:
		return fmt.Errorf("%s has unrecognized persona_source %q", stateFileName, s.PersonaSource)
	}

	if err := validatePhaseModels(s.PhaseModels); err != nil {
		return err
	}

	switch s.Scope {
	case "", ScopeLocalOnly, ScopeLocalCloud:
	default:
		return fmt.Errorf("%s has unrecognized scope %q", stateFileName, s.Scope)
	}

	if isBlank(s.ManagedAssetDigest) {
		return fmt.Errorf("%s managed_asset_digest is blank", stateFileName)
	}
	return nil
}

// ValidateForReplay reports whether the manifest carries enough desired state to
// replay an installation. It is separate from Validate because migration must
// succeed on a config.yaml whose replay fields were never populated; replay then
// blocks on the missing agents list rather than the migration failing.
func (s *State) ValidateForReplay() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if len(s.InstalledAgents) == 0 {
		return fmt.Errorf("%s installed_agents is empty; nothing to replay", stateFileName)
	}
	return nil
}

func validatePhaseModels(pm PhaseModels) error {
	for phase := range pm.Aliases {
		if isBlank(phase) || phase == "" {
			return fmt.Errorf("%s phase_models.aliases has a blank phase name", stateFileName)
		}
	}
	for phase := range pm.OpenCode {
		if isBlank(phase) || phase == "" {
			return fmt.Errorf("%s phase_models.opencode has a blank phase name", stateFileName)
		}
	}
	for phase := range pm.Claude {
		if isBlank(phase) || phase == "" {
			return fmt.Errorf("%s phase_models.claude has a blank phase name", stateFileName)
		}
	}
	return nil
}

// isBlank reports whether a value was written but carries no content. An absent
// value decodes to the empty string and is not blank; a whitespace-only value is.
func isBlank(v string) bool {
	return v != "" && strings.TrimSpace(v) == ""
}

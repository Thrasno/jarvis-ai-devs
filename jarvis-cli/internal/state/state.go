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
// The read, the mutation and the write are one critical section under WithLock:
// without it, this writer and a concurrent one each save a manifest the other
// has already replaced, and the later save silently discards the earlier one.
// The lock is fail-fast and non-reentrant, so no caller may reach this from
// inside a WithLock block of its own.
//
// The fresh manifest is only correct for a genuinely fresh machine. An absent
// manifest also describes a machine whose migration failed, and there the fresh
// manifest would carry nothing but the field this writer touched: Migrate's
// regular-file gate then early-returns forever and the persona, skills, agents,
// scope and phase models still in config.yaml become unreachable. The two are
// told apart by config.yaml itself, so every writer refuses before writing
// anything while it still carries state that has not moved.
func Update(mutate func(*State)) error {
	return WithLock(func() error {
		st, err := Load()
		if errors.Is(err, ErrNotFound) {
			if pendingErr := errIfMigrationPending(); pendingErr != nil {
				return pendingErr
			}
			st = New()
		} else if err != nil {
			return err
		}
		mutate(st)
		return Save(st)
	})
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

	// An instruction path is what proves an agent may write a file, and that
	// proof is looked up by path: two agents recording the same one collapse into
	// a single entry there, so one of them loses the file the manifest recorded
	// for it and the other gains permission over its sibling's. Neither is
	// detectable once the manifest is loaded, so the collision is refused here,
	// at the boundary, rather than by making every ownership constructor fallible.
	agentByPath := make(map[string]string, len(s.InstalledAgents))
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
		// Normalized exactly the way the ownership map keys these paths (trim,
		// then Clean, and never a filesystem lookup), because a collision this
		// misses is a collision that map still makes. The rule is deliberately
		// restated here rather than imported: the ownership package depends on
		// this one, so the dependency cannot run the other way.
		//
		// An unrecorded path is skipped rather than normalized: Clean turns the
		// empty string into ".", which would make two agents that record no path
		// at all collide with each other.
		if trimmed := strings.TrimSpace(agent.InstructionsPath); trimmed != "" {
			path := filepath.Clean(trimmed)
			if owner, taken := agentByPath[path]; taken {
				return fmt.Errorf(
					"%s installed_agents[%d] (%s) records instructions_path %q, already recorded for agent %s",
					stateFileName, i, agent.ID, agent.InstructionsPath, owner,
				)
			}
			agentByPath[path] = agent.ID
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

// ErrNoInstalledAgents reports the one replay precondition a caller can repair,
// so a command can recognize it and name the recovery instead of surfacing a
// bare field check the user cannot act on.
var ErrNoInstalledAgents = errors.New(stateFileName + " installed_agents is empty; nothing to replay")

// ValidateForReplay reports whether the manifest carries enough desired state to
// replay an installation. It is separate from Validate because migration must
// succeed on a config.yaml whose replay fields were never populated; replay then
// blocks on the missing agents list rather than the migration failing.
func (s *State) ValidateForReplay() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if len(s.InstalledAgents) == 0 {
		return ErrNoInstalledAgents
	}
	return nil
}

// DefaultPersona is the persona a machine replays when the manifest records
// none. It is the product default, not a placeholder: an installation that was
// never asked which persona to use still renders instruction files, and this is
// the one it renders.
const DefaultPersona = "argentino"

// ResolvedPersona returns the persona slug and source this machine replays,
// applying the defaults an unpopulated manifest falls back to: the built-in
// default persona, and a builtin source for anything that is not exactly "user".
//
// Validate only rejects an unrecognized persona_source; it does not default one,
// because a manifest is allowed to record nothing. Every reader needs the same
// answer for "nothing recorded", so it is resolved here rather than at each
// call site.
func (s *State) ResolvedPersona() (string, PersonaSource) {
	if s == nil {
		return DefaultPersona, PersonaSourceBuiltin
	}
	slug := strings.TrimSpace(s.Persona)
	if slug == "" {
		slug = DefaultPersona
	}
	source := PersonaSourceBuiltin
	if strings.ToLower(strings.TrimSpace(string(s.PersonaSource))) == string(PersonaSourceUser) {
		source = PersonaSourceUser
	}
	return slug, source
}

// ResolvedScope returns the scope this machine replays.
//
// A manifest that records no scope, or one Validate tolerates but does not
// recognize, falls back to local+cloud when the machine already has a stored
// cloud link and to local-only otherwise. That link lives in ~/.jarvis/config.yaml,
// which this package deliberately does not read, so it arrives as an argument:
// the caller that owns the config answers "is there a cloud link" and this owns
// what the answer means for the scope.
func (s *State) ResolvedScope(hasStoredCloudLink bool) Scope {
	if s != nil {
		switch s.Scope {
		case ScopeLocalOnly, ScopeLocalCloud:
			return s.Scope
		}
	}
	if hasStoredCloudLink {
		return ScopeLocalCloud
	}
	return ScopeLocalOnly
}

// RecordsCompleteInstall reports whether the manifest carries every part of a
// recorded installation that it owns: a persona, at least one selected skill and
// at least one configured agent.
//
// It answers only half the question. config.yaml owns the API URL, the schema
// version and the install-completion flag, so a caller deciding whether a
// machine can be reconfigured has to combine this with those.
func (s *State) RecordsCompleteInstall() bool {
	if s == nil {
		return false
	}
	if strings.TrimSpace(s.Persona) == "" {
		return false
	}
	if len(s.Skills) == 0 {
		return false
	}
	return len(s.InstalledAgents) > 0
}

// RecordsAnyState reports whether the manifest carries any recorded choice at
// all. It is what tells a damaged installation apart from a machine that was
// never set up.
//
// A persona equal to DefaultPersona does not count: that is what an unpopulated
// manifest reads as, so treating it as a choice would report every fresh machine
// as damaged.
func (s *State) RecordsAnyState() bool {
	if s == nil {
		return false
	}
	if persona := strings.TrimSpace(s.Persona); persona != "" && persona != DefaultPersona {
		return true
	}
	if len(s.Skills) > 0 || len(s.InstalledAgents) > 0 {
		return true
	}
	return s.SelectionConfigured
}

// NormalizedPhaseModels returns the manifest's per-phase model assignments with
// phase keys lowercased and trimmed, platform aliases lowercased and trimmed,
// provider and model identifiers trimmed, and unnamed phases dropped.
//
// The manifest stores what was written: migration copies config.yaml verbatim,
// so a hand-edited `Apply:` key would otherwise stop matching the SDD contract's
// `apply`. config.Load applied exactly this normalization to the AppConfig
// fields these values used to be read from, so reading them here must not change
// the effective value.
func (s *State) NormalizedPhaseModels() PhaseModels {
	out := PhaseModels{
		Aliases:  map[string]PhaseModelSelection{},
		OpenCode: map[string]OpenCodeModelAssignment{},
		Claude:   map[string]ClaudeModelAssignment{},
	}
	if s == nil {
		return out
	}
	for rawPhase, sel := range s.PhaseModels.Aliases {
		phase := normalizePhaseKey(rawPhase)
		if phase == "" {
			continue
		}
		sel.OpenCode = strings.ToLower(strings.TrimSpace(sel.OpenCode))
		sel.Claude = strings.ToLower(strings.TrimSpace(sel.Claude))
		out.Aliases[phase] = sel
	}
	for rawPhase, assignment := range s.PhaseModels.OpenCode {
		phase := normalizePhaseKey(rawPhase)
		if phase == "" {
			continue
		}
		assignment.ProviderID = strings.TrimSpace(assignment.ProviderID)
		assignment.ModelID = strings.TrimSpace(assignment.ModelID)
		assignment.Effort = strings.TrimSpace(assignment.Effort)
		out.OpenCode[phase] = assignment
	}
	for rawPhase, assignment := range s.PhaseModels.Claude {
		phase := normalizePhaseKey(rawPhase)
		if phase == "" {
			continue
		}
		assignment.Model = strings.TrimSpace(assignment.Model)
		assignment.Effort = strings.TrimSpace(assignment.Effort)
		out.Claude[phase] = assignment
	}
	return out
}

func normalizePhaseKey(phase string) string {
	return strings.ToLower(strings.TrimSpace(phase))
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

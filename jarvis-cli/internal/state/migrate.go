package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

	// Email and Cloud are not replay fields and never move into the manifest.
	// They are decoded only as the stored-cloud-link evidence the scope fallback
	// needs, exactly as the release this replaces read it.
	Email string       `yaml:"email"`
	Cloud *legacyCloud `yaml:"cloud"`

	Install struct {
		Agents map[string]AgentRecord `yaml:"agents"`
	} `yaml:"install"`

	SDD struct {
		PhaseModels         map[string]PhaseModelSelection     `yaml:"phase_models"`
		OpenCodePhaseModels map[string]OpenCodeModelAssignment `yaml:"opencode_phase_models"`
		ClaudePhaseModels   map[string]ClaudeModelAssignment   `yaml:"claude_phase_models"`
	} `yaml:"sdd"`
}

// legacyCloud mirrors the config.yaml cloud block, which stays in config.yaml.
type legacyCloud struct {
	Email          string `yaml:"email"`
	SyncConfigured bool   `yaml:"sync_configured"`
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
		return Result{}, fmt.Errorf("%w: read %s: %w", ErrConfigUnreadable, configFileName, err)
	}

	// The lenient decode runs first because it is what decides between the two
	// failure modes. A document no parser accepts is quarantined; a document that
	// parses is never moved, whatever shape its values have.
	raw := map[string]any{}
	if strings.TrimSpace(string(data)) != "" {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			if isShapeMismatch(err) {
				return Result{}, errConfigShapeMismatch(nil, err)
			}
			return quarantineUnparsableConfig(configPath, err)
		}
	}

	var legacy legacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		if isShapeMismatch(err) {
			return Result{}, errConfigShapeMismatch(raw, err)
		}
		return quarantineUnparsableConfig(configPath, err)
	}

	// The schema version alone is not allowed to declare the move done. Once the
	// replay fields left the AppConfig struct, a plain config.Save on a machine
	// that has not migrated yet advances config.yaml to the current schema
	// version while those keys are still in the file. Trusting the version there
	// would strand the user's persona, skills, agents, scope and phase models in
	// a file nothing reads any more. The keys themselves are the honest signal.
	if legacy.SchemaVersion >= configSchemaVersionAfterMigration && !carriesReplayFields(raw) {
		// Already migrated; the manifest owns these fields.
		return Result{}, nil
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

// ErrConfigUnreadable marks a failure whose cause is ~/.jarvis/config.yaml
// rather than the manifest, so a caller can tell the user which of the two
// files to look at. On a machine that never migrated, config.yaml is often the
// only one of them that exists.
var ErrConfigUnreadable = errors.New("config.yaml could not be read")

// corruptConfigSuffix prefixes the timestamp of a preserved config.yaml.
const corruptConfigSuffix = ".corrupt-"

// isShapeMismatch reports whether a decode failure is a value whose Go type does
// not match, rather than a document the parser could not read.
//
// gopkg.in/yaml.v3 reports the first as *yaml.TypeError, which it returns only
// after parsing the whole document successfully and decoding every other field.
// So a TypeError is proof that the file parses, and a file that parses is never
// moved aside: legacyConfig types far more structure than config.AppConfig does,
// which means a key nothing else in the CLI even looks at could otherwise
// quarantine a config every other reader loads without complaint.
func isShapeMismatch(err error) bool {
	var typeErr *yaml.TypeError
	return errors.As(err, &typeErr)
}

// errConfigShapeMismatch reports a config.yaml that parses but holds a value the
// migration cannot decode, naming the offending key.
//
// Nothing is written and nothing is moved. Carrying the document across with the
// offending value dropped would strip a setting from config.yaml that the
// manifest never captured, and stripping a value the user typed is worse than
// stopping: they can fix one line, but they cannot recover a value no file holds
// any more. The message says that plainly rather than implying the file is
// broken beyond repair.
//
// raw is the leniently decoded document, or nil when even that decode failed on
// shape -- a parsable document that is not a mapping at all.
func errConfigShapeMismatch(raw map[string]any, cause error) error {
	offending := "a value"
	if fields := offendingConfigFields(raw); len(fields) > 0 {
		offending = strings.Join(fields, ", ")
	} else if raw == nil {
		offending = "its top level"
	}

	return fmt.Errorf(
		"%w: %s holds %s with a shape this version cannot read (%w). "+
			"Nothing was moved and nothing was changed: your settings are still in %s exactly as you wrote them. "+
			"Fix that key and re-run jarvis. Ver docs/setup-recovery.md",
		ErrConfigUnreadable, configFileName, offending, cause, configFileName,
	)
}

// offendingConfigFields names the top-level keys whose value the migration
// cannot decode, sorted so the message is stable across runs.
//
// Each key is probed on its own through the same struct the migration decodes
// into, because the decoder reports a line and a type but never the field name,
// and a line number is not what the user has to edit.
func offendingConfigFields(raw map[string]any) []string {
	fields := make([]string, 0, 1)
	for key, value := range raw {
		encoded, err := yaml.Marshal(map[string]any{key: value})
		if err != nil {
			fields = append(fields, key)
			continue
		}
		var probe legacyConfig
		if err := yaml.Unmarshal(encoded, &probe); err != nil {
			fields = append(fields, key)
		}
	}
	sort.Strings(fields)
	return fields
}

// quarantineUnparsableConfig moves a config.yaml no parser accepts aside and
// reports what happened, so the machine stops being an unrecoverable dead end.
//
// A config.yaml that does not parse blocks the migration, and the migration
// blocks every command that reads either store, with no exit that does not
// involve hand-editing YAML. The recovery is deliberately non-destructive: the
// file is renamed, never rewritten, so every byte survives at a sibling path
// the caller names to the user. Overwriting a file whose contents could not be
// read would destroy state nobody has seen, which is worse than the dead end it
// resolves.
//
// The returned Result carries the notice and Migrated stays false: nothing was
// migrated, because nothing could be read. The caller continues as if this
// machine had no config.yaml, which is now true.
func quarantineUnparsableConfig(configPath string, cause error) (Result, error) {
	preserved, err := reserveQuarantinePath(configPath)
	if err != nil {
		return Result{}, fmt.Errorf("%w: parse %s: %w", ErrConfigUnreadable, configFileName, cause)
	}
	if err := os.Rename(configPath, preserved); err != nil {
		return Result{}, fmt.Errorf("%w: preserve the unparsable %s: %w", ErrConfigUnreadable, configFileName, err)
	}

	return Result{
		Notice: fmt.Sprintf(
			"%s could not be parsed (%v). It was preserved unchanged at %s and moved aside so jarvis can run again. "+
				"No setting it held was recovered; re-run jarvis to record them again. Ver docs/setup-recovery.md",
			configFileName, cause, preserved,
		),
	}, nil
}

// reserveQuarantinePath returns a sibling path that does not exist yet. A
// machine that corrupts its config twice in the same second must not overwrite
// the copy the first failure preserved.
func reserveQuarantinePath(configPath string) (string, error) {
	base := configPath + corruptConfigSuffix + time.Now().UTC().Format("20060102T150405Z")
	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-%d", base, attempt)
		}
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no free path to preserve %s next to %s", configFileName, configPath)
}

// errIfMigrationPending reports why no manifest may be written yet.
//
// It answers the one question an absent manifest leaves open: is this a machine
// that was never set up, or one whose migration did not complete? config.yaml is
// the evidence. While it still carries a key the manifest owns, the replay
// fields have not moved, and writing a manifest now would both lose them and
// stop Migrate from ever looking at that file again.
//
// A config.yaml that cannot be read or parsed is treated the same way: the
// question cannot be answered, and answering it wrongly is unrecoverable.
func errIfMigrationPending() error {
	configPath, err := configFilePath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing was ever recorded on this machine; it is genuinely fresh.
			return nil
		}
		return fmt.Errorf("%w: read %s before writing %s: %w", ErrConfigUnreadable, configFileName, stateFileName, err)
	}

	raw := map[string]any{}
	if strings.TrimSpace(string(data)) != "" {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("%w: parse %s before writing %s: %w", ErrConfigUnreadable, configFileName, stateFileName, err)
		}
	}
	if !carriesReplayFields(raw) {
		return nil
	}

	return fmt.Errorf(
		"refusing to write %s: %s still holds installation state that has not migrated into it. "+
			"Nothing was written; fix the migration failure and re-run jarvis. Ver docs/setup-recovery.md",
		stateFileName, configFileName,
	)
}

// LoadWithoutMigrating returns the desired state this machine would replay,
// without writing anything.
//
// It exists for the read-only callers -- `jarvis doctor`, `jarvis verify` and a
// dry-run reconcile -- that observe the machine and must not change it.
// Migrating first would answer correctly but create state.yaml and rewrite
// config.yaml as a side effect of looking, and would turn a migration failure
// into a failed diagnostic.
//
// Reading the manifest alone is not an option either: a machine that has not
// migrated yet still holds every replay field in config.yaml, and an empty
// manifest would report the contract defaults instead of the user's choices.
// So when no manifest exists the same values a migration would record are
// derived from config.yaml in memory, through the very function that records
// them. It returns ErrNotFound only when neither store exists.
func LoadWithoutMigrating() (*State, error) {
	st, err := Load()
	if err == nil {
		return st, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	configPath, cfgErr := configFilePath()
	if cfgErr != nil {
		return nil, cfgErr
	}
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// Neither store exists: nothing has been recorded on this machine.
			return nil, err
		}
		return nil, fmt.Errorf("read %s: %w", configFileName, readErr)
	}

	var legacy legacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse %s: %w", configFileName, err)
	}
	// The derived manifest is deliberately not validated. Validation is what
	// gates the write in Migrate; making it gate a read as well would give the
	// diagnostics the hard-failure mode this function exists to remove, and
	// every value the manifest constrains is already coerced above.
	return manifestFromLegacyConfig(legacy), nil
}

// manifestFromLegacyConfig builds the manifest from the legacy config's replay
// fields.
//
// Values are carried over as they are, except the two the manifest constrains:
// the persona source and the scope are coerced into a value it recognizes.
// Migration is a compatibility boundary and must accept whatever an older
// release or a hand-edited file holds. The release this replaces normalized both
// on every config load, so a non-canonical value reached nobody; rejecting one
// here would fail the migration and strand every replay field in a file nothing
// reads any more.
func manifestFromLegacyConfig(legacy legacyConfig) *State {
	manifest := New()

	manifest.Persona = firstNonBlank(legacy.PersonaPreset, legacy.Preset)
	manifest.PersonaSource = normalizePersonaSource(legacy.PersonaPresetSource)

	if legacy.SelectedSkills != nil {
		manifest.Skills = migratedSkills(legacy.SelectedSkills)
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
	// A phase name is a map key, so it reaches Validate the same way the persona
	// and the skill ids do. The previous release dropped an unnamed phase on
	// every config.Load; carrying one over instead would fail Validate, abort the
	// migration, and leave the machine unable to write its manifest at all.
	// NormalizedPhaseModels is that same normalization, so the migration and the
	// readers cannot drift apart.
	manifest.PhaseModels = manifest.NormalizedPhaseModels()

	manifest.Scope = normalizeLegacyScope(legacy)
	return manifest
}

// normalizePersonaSource coerces a recorded persona source into a value the
// manifest recognizes. Case and padding do not make a different value, and
// anything else falls back to the built-in source, which is what the previous
// release resolved it to.
func normalizePersonaSource(raw string) PersonaSource {
	switch source := PersonaSource(strings.ToLower(strings.TrimSpace(raw))); source {
	case PersonaSourceBuiltin, PersonaSourceUser:
		return source
	default:
		return PersonaSourceBuiltin
	}
}

// normalizeLegacyScope coerces a recorded scope into a value the manifest
// recognizes. A scope it does not recognize, including an absent one, falls back
// to local+cloud when config.yaml already stores a cloud link and to local-only
// otherwise -- the fallback the previous release applied on every load, and the
// one ResolvedScope still applies at read time.
func normalizeLegacyScope(legacy legacyConfig) Scope {
	switch scope := Scope(strings.ToLower(strings.TrimSpace(legacy.Scope))); scope {
	case ScopeLocalOnly, ScopeLocalCloud:
		return scope
	}
	if hasStoredCloudLink(legacy) {
		return ScopeLocalCloud
	}
	return ScopeLocalOnly
}

// hasStoredCloudLink reports whether config.yaml records a cloud link. The
// previous release promoted a top-level email into the cloud block before
// checking it, so a config that carries only that key counts too.
func hasStoredCloudLink(legacy legacyConfig) bool {
	if legacy.Cloud == nil {
		return strings.TrimSpace(legacy.Email) != ""
	}
	return strings.TrimSpace(legacy.Cloud.Email) != "" || legacy.Cloud.SyncConfigured
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

	// Padding is not a recorded value here either: Validate rejects a
	// whitespace-only id or path, so an agent whose id is nothing but padding is
	// not an agent, and a path that is nothing but padding migrates as the
	// unrecorded empty path rather than failing the migration.
	appendAgent := func(rawID string) {
		id := strings.TrimSpace(rawID)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		details, ok := records[rawID]
		if !ok {
			details = records[id]
		}
		agents = append(agents, Agent{
			ID:               id,
			InstructionsPath: strings.TrimSpace(details.InstructionsPath),
			ConfigPath:       strings.TrimSpace(details.ConfigPath),
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
// carriesReplayFields reports whether a decoded config.yaml still holds any key
// the manifest took over. An empty list value counts: `configured_agents: []` is
// a recorded answer of "none", not an absent one, and migration is what carries
// that answer across.
func carriesReplayFields(raw map[string]any) bool {
	for _, key := range replayConfigKeys {
		if _, present := raw[key]; present {
			return true
		}
	}
	install, ok := raw["install"].(map[string]any)
	if !ok {
		return false
	}
	_, present := install["agents"]
	return present
}

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

// firstNonBlank returns the first value that carries something other than
// padding, trimmed.
//
// Padding is not a recorded value: Validate rejects a whitespace-only string, so
// carrying one over would fail the whole migration, and the release this
// replaces trimmed these fields on every load. A value that is blank everywhere
// migrates as unrecorded, exactly as an absent key does, and the readers apply
// the same default to both.
func firstNonBlank(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// migratedSkills carries the recorded skill IDs over, dropping only entries that
// are nothing but padding.
//
// The list is never filtered against the current embedded catalog: an ID the
// catalog no longer offers is the only ownership proof that authorizes deleting
// that skill later, so every non-blank entry is carried over exactly as written.
// A list that had entries and is left with none stays present and empty, which
// is the recorded answer of "none" rather than an absent answer.
func migratedSkills(recorded []string) []string {
	skills := make([]string, 0, len(recorded))
	for _, id := range recorded {
		if strings.TrimSpace(id) == "" {
			continue
		}
		skills = append(skills, id)
	}
	return skills
}

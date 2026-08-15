// Package tui provides the Bubbletea-based wizard for jarvis-cli first-run setup
// and auxiliary TUI components (timeline stub).
package tui

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/opencode"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// Step identifies the current wizard step.
type Step int

const (
	StepScope Step = iota
	StepHiveCloud
	StepPersona
	StepSkills
	StepPhaseModels
	StepReview
	StepMCPDisclosure
	StepStatuslineConfirm
	StepApply
	StepDone
)

// Backwards-compatible aliases for legacy tests.
const (
	StepHiveLocal   = StepScope
	StepAgentConfig = StepReview
)

// StepExtraSkills aliases StepSkills to keep explicit naming in tests/UX.
const StepExtraSkills = StepSkills

// Screen identifies the root TUI surface currently receiving input.
type Screen int

const (
	ScreenWizard Screen = iota
	ScreenCockpit
)

// Model is the root Bubbletea model for the jarvis-cli wizard.
type Model struct {
	Screen Screen
	Step   Step
	Mode   string
	Scope  state.Scope

	PersonaFS  fs.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS
	ProjectCWD string

	Email       string
	Password    string
	APIToken    string
	activeField int

	Presets              []persona.ProfileOption
	presetCur            int
	CustomYAML           string
	customEdit           bool
	customField          int
	customPresetName     string
	customDisplayName    string
	selectedProfile      *persona.ResolvedProfile
	previousPresetSlug   string
	previousPresetSource persona.PresetSource
	personaSelectionErr  error

	SkillList    []skills.Skill
	SkillPrompts []skillPrompt
	Selected     map[string]bool

	Agents                   []agent.Agent
	agentProgress            []string
	agentDone                bool
	reviewChoice             int
	statuslineOverwriteReady bool // true once the overwrite/skip decision has been captured
	statuslineOverwrite      bool // the captured decision: true = overwrite, false = skip
	mcpAcknowledgement       string
	mcpAcknowledged          bool

	phaseModelRows                []phaseModelRow
	phaseModelActiveRow           int
	phaseModelActiveCol           int
	phaseModelOpenCode            []string
	phaseModelClaude              []string
	phaseModelOpenCodeAssignments []state.OpenCodeModelAssignment
	phaseModelOpenCodeDiagnostics []string
	phaseModelHasOpenCode         bool
	phaseModelHasClaude           bool
	phaseModelMode                phaseModelMode
	phaseModelProviderCursor      int
	phaseModelModelCursor         int
	phaseModelEffortCursor        int
	phaseModelModelSearch         string
	phaseModelOpenCodeProviders   []openCodeProviderOption
	phaseModelPendingOpenCode     state.OpenCodeModelAssignment
	phaseModelPendingClaude       state.ClaudeModelAssignment

	cfg *config.AppConfig
	// manifest is the desired state the wizard prefills from. ~/.jarvis/state.yaml
	// owns persona, selected skills and the per-phase models; cfg only still
	// carries them because config.Load's temporary bridge projects them back.
	// It is never nil: a machine with no manifest yet reads as an empty one.
	manifest *state.State
	// manifestErr records that the manifest could not be read. A manifest the
	// wizard failed to read is not an empty manifest: the empty one it falls back
	// to prefills the built-in defaults and an empty skill list, and applying
	// would record those defaults over desired state the wizard never saw, after
	// which the migration early-returns forever because a manifest now exists.
	// The apply step refuses while this is set. It is a field rather than a
	// returned error because a Bubbletea constructor has no error channel.
	manifestErr error
	// migrationNotice carries what the migration had to report about this
	// machine, most importantly that an unreadable config.yaml was preserved and
	// moved aside. The plain-text run prints it; the wizard shows it above every
	// step so no recovery happens without the user seeing it.
	migrationNotice string

	Err  error
	Done bool

	width  int
	height int

	noTUI bool

	cockpitCursor         int
	cockpitMessage        string
	cockpitRunner         cockpitRunner
	cockpitMode           cockpitMode
	cockpitAction         CockpitActionID
	cockpitProviderCursor int
	cockpitProvider       string
	cockpitInput          string
	cockpitSnapshot       string
	cockpitPlan           string
}

// manifestWriteGuard reports why no action may write anything yet.
//
// A manifest the run could not read is not an empty manifest, so any action
// that renders managed files or records desired state has to refuse while the
// failure stands. It must be consulted before the first file is written: the
// state.Update inside those actions refuses too, but only after the agent
// instructions, output styles or sync credentials are already on disk, which
// leaves the machine's rendered files disagreeing with the manifest jarvis sync
// replays from.
func (m Model) manifestWriteGuard() error {
	if m.manifestErr == nil {
		return nil
	}
	return fmt.Errorf(
		"%w. Nothing was written; %s and re-run jarvis. Ver docs/setup-recovery.md",
		m.manifestErr, manifestFailureGuidance(m.manifestErr),
	)
}

// manifestFailureGuidance names the file the user actually has to look at. A
// manifest read that failed because ~/.jarvis/config.yaml could not be read
// points at config.yaml, which on such a machine is often the only file of the
// two that exists.
func manifestFailureGuidance(err error) string {
	if errors.Is(err, state.ErrConfigUnreadable) {
		return "fix ~/.jarvis/config.yaml"
	}
	return "fix ~/.jarvis/state.yaml"
}

// wizardProfile returns the validated schema-v2 profile selected by the wizard.
func (m Model) wizardProfile() (*persona.ResolvedProfile, bool) {
	if m.selectedProfile != nil {
		return m.selectedProfile, true
	}
	return nil, false
}

type phaseModelMode int

const (
	phaseModelModeList phaseModelMode = iota
	phaseModelModeOpenCodeProvider
	phaseModelModeOpenCodeModel
	phaseModelModeOpenCodeEffort
	phaseModelModeClaudeModel
	phaseModelModeClaudeEffort
)

const (
	phaseModelNoColumn       = 0
	phaseModelOpenCodeColumn = 1
	phaseModelClaudeColumn   = 2
)

type openCodeProviderOption struct {
	Provider opencode.Provider
	Models   []openCodeModelOption
}

func (o openCodeProviderOption) DisplayName() string {
	if strings.TrimSpace(o.Provider.Name) != "" {
		return strings.TrimSpace(o.Provider.Name)
	}
	return o.Provider.ID
}

type openCodeModelOption struct {
	ProviderID string
	Model      opencode.Model
}

// WizardConfig carries FSes needed to run the wizard, injected by main.
type WizardConfig struct {
	PersonaFS  fs.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS
	ProjectCWD string
}

// NewModel creates a fresh wizard model at StepScope.
func NewModel(wcfg WizardConfig, noTUI bool) Model {
	m := Model{
		Screen:     ScreenWizard,
		Step:       StepScope,
		Mode:       string(config.ConfigStatusSetup),
		Scope:      state.ScopeLocalOnly,
		PersonaFS:  wcfg.PersonaFS,
		SkillsFS:   wcfg.SkillsFS,
		TemplateFS: wcfg.TemplateFS,
		ProjectCWD: wcfg.ProjectCWD,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL},
		manifest:   state.New(),
		noTUI:      noTUI,
	}

	// The manifest is read before config.Load for two reasons: the migration it
	// performs must have run by the time the bridge projects the manifest back
	// onto AppConfig, and reading it separately means a config.yaml that fails to
	// load no longer takes the user's recorded persona, skills and phase models
	// down with it.
	//
	// A failed read is recorded rather than swallowed. The wizard still opens on
	// the empty manifest above so the user can see and diagnose the machine, but
	// the apply step refuses while the failure stands: writing the built-in
	// defaults into a new manifest would strand the persona, skills and agents
	// this read could not reach in a config.yaml the migration will never look at
	// again.
	manifest, migration, err := loadWizardManifest()
	if err == nil {
		m.manifest = manifest
	} else {
		m.manifestErr = err
	}
	// The migration reports what it had to do to this machine -- notably that an
	// unreadable config.yaml was preserved and moved aside. Recording it here is
	// what keeps that recovery from happening silently in a TTY run, which has no
	// stdout to print to.
	m.migrationNotice = migration.Notice

	if loaded, err := config.Load(); err == nil {
		m.cfg = loaded
		m.Mode = string(loaded.ConfigStatus(recordedInstallFrom(m.manifest)))
		if loaded.Cloud != nil {
			m.Email = loaded.Cloud.Email
		}
	}
	m.Scope = wizardScope(m.manifest, m.cfg)

	presets, err := persona.ListProfiles(m.PersonaFS)
	if err == nil {
		m.Presets = append(m.Presets, profileOptions(presets)...)
		m.Presets = append(m.Presets, persona.ProfileOption{
			Name:        "custom",
			DisplayName: "Custom (crear nuevo)",
			Description: "Creá un preset propio con slug y display name, validado y persistido en ~/.jarvis/personas/<slug>.yaml.",
		})
		configuredPersona, _ := wizardPersonaSelection(m.manifest)
		configExists, configErr := hasPersistedConfig()
		if configErr != nil {
			m.personaSelectionErr = configErr
			m.presetCur = -1
		} else if configExists {
			if err := validateConfiguredPersonaPresetForV2Selection(m.PersonaFS, configuredPersona); err != nil {
				m.personaSelectionErr = err
				m.presetCur = -1
			}
		}
		if m.personaSelectionErr == nil {
			for i, p := range presets {
				if p.Name == configuredPersona {
					m.presetCur = i
					break
				}
			}
		}
	}

	skillList, err := skills.ListSkills(m.SkillsFS)
	if err == nil {
		m.SkillList = skillList
		plan := buildSkillSelectionPlan(skillList, wizardSelectedSkills(m.manifest))
		m.SkillPrompts = plan.Prompts
		m.Selected = plan.Selected
	}

	if m.cfg == nil {
		m.cfg = &config.AppConfig{APIURL: config.DefaultAPIURL}
	}
	// The persona being replaced is whatever the manifest recorded, so the apply
	// step cleans up the right previous profile even when config.yaml failed to
	// load or never carried one.
	m.previousPresetSlug, m.previousPresetSource = wizardPersonaSelection(m.manifest)

	m.Agents = agent.Detect(wcfg.TemplateFS)
	m = initializePhaseModelEditor(m)

	return m
}

// loadWizardManifest reads the desired state the wizard prefills from and
// records into.
//
// It migrates before it reads, for the same reason `jarvis sync` and
// `jarvis persona set` do. A machine upgrading into this version still has its
// persona, skills and phase models in ~/.jarvis/config.yaml and no manifest at
// all; reading the manifest without migrating first would see an empty one and
// prefill the wizard from an empty selection, silently dropping every choice the
// user had already made and then writing that emptiness back on apply. The
// migration is one-way and returns early once a manifest exists, so it costs one
// stat call on every later run.
//
// The migration Result is returned rather than printed: only the plain-text
// caller has somewhere to print it.
func loadWizardManifest() (*state.State, state.Result, error) {
	migration, err := state.Migrate()
	if err != nil {
		return nil, migration, fmt.Errorf("migrate configuration into the desired-state manifest: %w", err)
	}

	manifest, err := state.Load()
	if errors.Is(err, state.ErrNotFound) {
		return state.New(), migration, nil
	}
	if err != nil {
		return nil, migration, fmt.Errorf("load the desired-state manifest: %w", err)
	}
	return manifest, migration, nil
}

// wizardPersonaSelection returns the persona slug and source the wizard prefills
// from, in the persona package's vocabulary.
//
// ~/.jarvis/state.yaml owns the persona and the defaults an unrecorded one falls
// back to, so this only translates its answer.
func wizardPersonaSelection(manifest *state.State) (string, persona.PresetSource) {
	slug, source := manifest.ResolvedPersona()
	if source == state.PersonaSourceUser {
		return slug, persona.PresetSourceUser
	}
	return slug, persona.PresetSourceBuiltin
}

// recordWizardPersona stores a persona choice on the wizard's working copy of
// the desired-state manifest, which owns it. A model built outside NewModel
// carries no working manifest, so one is created rather than dropping the
// choice; an empty manifest is the same "nothing recorded yet" state a fresh
// machine starts from.
func recordWizardPersona(desired *state.State, slug string, source persona.PresetSource) *state.State {
	if desired == nil {
		desired = state.New()
	}
	desired.Persona = slug
	desired.PersonaSource = state.PersonaSource(source)
	return desired
}

// wizardScope returns the scope the wizard starts from.
//
// ~/.jarvis/state.yaml owns the scope and what an unrecorded one falls back to;
// the stored cloud link that fallback turns on lives in config.yaml. This joins
// the two halves and owns neither.
func wizardScope(manifest *state.State, cfg *config.AppConfig) state.Scope {
	return manifest.ResolvedScope(cfg.HasStoredCloudLink())
}

// recordedInstallFrom answers the half of the install-status question config.yaml
// cannot: whether the manifest records a complete installation, and whether it
// records anything at all.
func recordedInstallFrom(manifest *state.State) config.RecordedInstall {
	return config.RecordedInstall{
		Complete:  manifest.RecordsCompleteInstall(),
		Populated: manifest.RecordsAnyState(),
	}
}

// wizardAgentIDs returns the configured agent IDs in the order the manifest
// records them.
func wizardAgentIDs(manifest *state.State) []string {
	if manifest == nil {
		return nil
	}
	ids := make([]string, 0, len(manifest.InstalledAgents))
	for _, agent := range manifest.InstalledAgents {
		ids = append(ids, agent.ID)
	}
	return ids
}

// wizardSelectedSkills returns the skill IDs this machine already owns.
func wizardSelectedSkills(manifest *state.State) []string {
	if manifest == nil {
		return nil
	}
	return manifest.Skills
}

// wizardPhaseModels returns the manifest's per-phase model assignments keyed the
// way the SDD contract names its phases.
func wizardPhaseModels(manifest *state.State) state.PhaseModels {
	return manifest.NormalizedPhaseModels()
}

// recordWizardDesiredState writes the choices the wizard just applied into
// ~/.jarvis/state.yaml, which owns them. desired is the wizard's working copy of
// the manifest: every step edits it, and this commits it.
//
// The write is sequenced with config.Save and never wrapped around it:
// state.Update takes the fail-fast, non-reentrant manifest lock internally, so
// nesting it inside another holder of that lock would fail the write. It runs
// before config.Save so that the install.mode and install.completed keys
// config.yaml still owns are derived from the choices recorded here.
func recordWizardDesiredState(desired *state.State) error {
	if desired == nil {
		return nil
	}
	slug, source := desired.Persona, desired.PersonaSource
	skillIDs := append([]string(nil), desired.Skills...)
	scope := desired.Scope
	agents := append([]state.Agent(nil), desired.InstalledAgents...)
	phaseModels := desired.PhaseModels

	return state.Update(func(st *state.State) {
		st.Persona = slug
		st.PersonaSource = source
		st.Skills = skillIDs
		st.Scope = scope
		st.InstalledAgents = mergeAgentRecords(agents, st.InstalledAgents)
		if len(st.InstalledAgents) > 0 {
			st.SelectionConfigured = true
		}
		st.PhaseModels = phaseModels
	})
}

// recordWizardAgents stores the agents an apply just configured on the wizard's
// working manifest, in the order they were configured, merged over the agents
// the manifest already records.
//
// The merge is the point. The wizard configures whatever agent.Detect found on
// this machine, and detection is presence-based: a config directory that moved
// or an executable that left $PATH makes an agent invisible to this run without
// saying anything about whether the user still owns it. Replacing the record
// with what this run happened to see would delete the only ownership proof that
// authorizes cleaning that agent's managed files up later -- the same reason the
// manifest never filters its skill list on write.
//
// So an agent leaves the record only through an explicit removal, never as a
// side effect of a run failing to detect it. That removal is
// `jarvis config forget-agent <agent>`: the user says the agent is gone, and
// nothing on this path infers it.
func recordWizardAgents(desired *state.State, results []AgentApplyResult) {
	if desired == nil {
		return
	}
	order := make([]string, 0, len(results))
	records := make(map[string]state.AgentRecord, len(results))
	for _, res := range results {
		order = append(order, res.AgentName)
		records[res.AgentName] = state.AgentRecord{
			Configured:       res.State.Configured,
			InstructionsPath: res.State.InstructionsPath,
			ConfigPath:       res.State.ConfigPath,
		}
	}
	desired.InstalledAgents = mergeAgentRecords(
		state.InstalledAgentsFrom(order, records),
		desired.InstalledAgents,
	)
}

// mergeAgentRecords returns the agents observed by the current run followed by
// the previously recorded agents it did not observe, each ID appearing once.
//
// Observed wins on collision so a reconfigured agent carries the paths this run
// wrote rather than the stale ones it replaced.
func mergeAgentRecords(observed, recorded []state.Agent) []state.Agent {
	merged := make([]state.Agent, 0, len(observed)+len(recorded))
	seen := make(map[string]bool, len(observed)+len(recorded))

	appendAgents := func(agents []state.Agent) {
		for _, agent := range agents {
			id := strings.TrimSpace(agent.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			agent.ID = id
			merged = append(merged, agent)
		}
	}
	appendAgents(observed)
	appendAgents(recorded)

	if len(merged) == 0 {
		return nil
	}
	return merged
}

func profileOptions(presets []persona.Profile) []persona.ProfileOption {
	options := make([]persona.ProfileOption, 0, len(presets))
	for _, preset := range presets {
		options = append(options, persona.ProfileOption{
			Name:        preset.Name,
			DisplayName: preset.DisplayName,
			Description: schemaV2PresetDescription(preset.Name),
		})
	}
	return options
}

// NewCockpitModel creates the cockpit-first root model used by bare TTY runs.
func NewCockpitModel(wcfg WizardConfig) Model {
	m := NewModel(wcfg, false)
	m.Screen = ScreenCockpit
	m.Step = StepScope
	m.cockpitCursor = 0
	m.cockpitMessage = ""
	m.cockpitRunner = defaultCockpitRunner{}
	m.cockpitMode = cockpitModeMenu
	return m
}

func (m Model) Init() tea.Cmd { return nil }

// Update routes incoming messages to the per-step handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.Screen == ScreenCockpit {
			return updateCockpit(m, msg)
		}
		return m.updateStep(msg)
	case errMsg:
		m.Err = msg.err
		return m, nil
	default:
		updated, handled, cmd := handleStepMsg(m, msg)
		if handled {
			if m.Step == StepApply && len(m.agentProgress) == 0 && len(updated.agentProgress) > 0 {
				return updated, tea.Batch(cmd, runAgentConfigSequence(updated))
			}
			return updated, cmd
		}
	}

	return m, nil
}

type errMsg struct{ err error }

func (m Model) updateStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.Step {
	case StepScope:
		return updateScope(m, msg)
	case StepHiveCloud:
		return updateHiveCloud(m, msg)
	case StepPersona:
		return updatePersona(m, msg)
	case StepSkills:
		return updateSkills(m, msg)
	case StepPhaseModels:
		return updatePhaseModels(m, msg)
	case StepReview:
		return updateReview(m, msg)
	case StepMCPDisclosure:
		return updateMCPDisclosure(m, msg)
	case StepStatuslineConfirm:
		return updateStatuslineConfirm(m, msg)
	case StepApply:
		return updateApply(m, msg)
	case StepDone:
		return updateDone(m, msg)
	}
	return m, nil
}

func (m Model) View() string {
	return m.migrationNoticeBanner() + m.stepView()
}

// migrationNoticeBanner renders what the migration had to report, above every
// screen. A recovery that moved the user's config.yaml aside must be visible,
// not inferred from a file that quietly stopped existing.
func (m Model) migrationNoticeBanner() string {
	if strings.TrimSpace(m.migrationNotice) == "" {
		return ""
	}
	return m.migrationNotice + "\n\n"
}

func (m Model) stepView() string {
	if m.Screen == ScreenCockpit {
		return viewCockpit(m)
	}

	switch m.Step {
	case StepScope:
		return viewScope(m)
	case StepHiveCloud:
		return viewHiveCloud(m)
	case StepPersona:
		return viewPersona(m)
	case StepSkills:
		return viewSkills(m)
	case StepPhaseModels:
		return viewPhaseModels(m)
	case StepReview:
		return viewReview(m)
	case StepMCPDisclosure:
		return viewMCPDisclosure(m)
	case StepStatuslineConfirm:
		return viewStatuslineConfirm(m)
	case StepApply:
		return viewApply(m)
	case StepDone:
		return viewDone(m)
	}
	return ""
}

func initializePhaseModelEditor(m Model) Model {
	// A model built outside NewModel carries no working manifest. An empty one is
	// the same "nothing recorded yet" state a fresh machine starts from, and the
	// editor needs somewhere to record its rows.
	if m.manifest == nil {
		m.manifest = state.New()
	}
	contract := sddruntime.DefaultContract()
	// The manifest owns the per-phase models; the editor seeds its rows from it.
	phaseModels := wizardPhaseModels(m.manifest)
	resolved := sddruntime.ResolvePhaseModels(phaseModels)
	openCodePhaseModelDiscoveryDiagnostics = nil
	openCodePhaseModelProviderOptions = nil
	m.phaseModelHasOpenCode, m.phaseModelHasClaude = detectPhaseModelAgentTargets(m.Agents)
	if m.phaseModelHasOpenCode {
		m.phaseModelOpenCodeAssignments = ensureOpenCodeLegacyOption(discoverOpenCodePhaseModelOptions())
		m.phaseModelOpenCodeProviders = append([]openCodeProviderOption(nil), openCodePhaseModelProviderOptions...)
		m.phaseModelOpenCodeDiagnostics = append([]string(nil), openCodePhaseModelDiscoveryDiagnostics...)
	} else {
		m.phaseModelOpenCodeAssignments = nil
		m.phaseModelOpenCodeProviders = nil
		m.phaseModelOpenCodeDiagnostics = nil
	}
	m.phaseModelRows = make([]phaseModelRow, 0, len(contract.Phases))
	for _, phase := range contract.Phases {
		sel := resolved[phase]
		row := phaseModelRow{Phase: phase, OpenCode: sel.OpenCode, Claude: sel.Claude}
		row.OpenCodeAssignment = state.OpenCodeModelAssignment(phaseModels.OpenCode[phase])
		assignment := phaseModels.Claude[phase]
		if strings.TrimSpace(assignment.Model) != "" {
			row.Claude = strings.TrimSpace(assignment.Model)
		}
		row.ClaudeEffort = strings.TrimSpace(assignment.Effort)
		m.phaseModelRows = append(m.phaseModelRows, row)
	}
	m.phaseModelOpenCode = append([]string(nil), contract.PlatformCatalogs[sddruntime.PlatformOpenCode]...)
	m.phaseModelClaude = append([]string(nil), contract.PlatformCatalogs[sddruntime.PlatformClaude]...)
	if m.phaseModelActiveCol == phaseModelNoColumn || !m.phaseModelColumnEnabled(m.phaseModelActiveCol) {
		m.phaseModelActiveCol = m.firstPhaseModelColumn()
	}
	// The resolved rows are seeded back onto the working manifest so a run that
	// never opens the phase-model step still records the selection it displayed.
	if m.manifest != nil {
		if m.manifest.PhaseModels.Aliases == nil {
			m.manifest.PhaseModels.Aliases = map[string]state.PhaseModelSelection{}
		}
		if m.manifest.PhaseModels.OpenCode == nil {
			m.manifest.PhaseModels.OpenCode = map[string]state.OpenCodeModelAssignment{}
		}
		if m.manifest.PhaseModels.Claude == nil {
			m.manifest.PhaseModels.Claude = map[string]state.ClaudeModelAssignment{}
		}
		for _, row := range m.phaseModelRows {
			m.manifest.PhaseModels.Aliases[row.Phase] = state.PhaseModelSelection{OpenCode: row.OpenCode, Claude: row.Claude}
		}
	}
	return m
}

func hasPhaseModelRuntimeTarget(agents []agent.Agent) bool {
	hasOpenCode, hasClaude := detectPhaseModelAgentTargets(agents)
	return hasOpenCode || hasClaude
}

func detectPhaseModelAgentTargets(agents []agent.Agent) (bool, bool) {
	hasOpenCode := false
	hasClaude := false
	for _, a := range agents {
		switch strings.ToLower(strings.TrimSpace(a.Name())) {
		case "opencode":
			hasOpenCode = true
		case "claude":
			hasClaude = true
		}
	}
	return hasOpenCode, hasClaude
}

func (m Model) firstPhaseModelColumn() int {
	if m.phaseModelHasOpenCode {
		return phaseModelOpenCodeColumn
	}
	if m.phaseModelHasClaude {
		return phaseModelClaudeColumn
	}
	return phaseModelNoColumn
}

func (m Model) phaseModelColumnEnabled(col int) bool {
	switch col {
	case phaseModelOpenCodeColumn:
		return m.phaseModelHasOpenCode
	case phaseModelClaudeColumn:
		return m.phaseModelHasClaude
	default:
		return false
	}
}

func ensureOpenCodeLegacyOption(options []state.OpenCodeModelAssignment) []state.OpenCodeModelAssignment {
	if len(options) == 0 || options[0] == (state.OpenCodeModelAssignment{}) {
		return options
	}
	out := make([]state.OpenCodeModelAssignment, 0, len(options)+1)
	out = append(out, state.OpenCodeModelAssignment{})
	out = append(out, options...)
	return out
}

var openCodePhaseModelDiscoveryDiagnostics []string
var openCodePhaseModelProviderOptions []openCodeProviderOption

var discoverOpenCodePhaseModelOptions = func() []state.OpenCodeModelAssignment {
	openCodePhaseModelDiscoveryDiagnostics = nil
	result, err := opencode.DiscoverAvailableProviders(opencode.ResolvePaths(defaultOpenCodePathRoots()), nil)
	if err != nil {
		openCodePhaseModelDiscoveryDiagnostics = []string{"OpenCode provider/model discovery unavailable: " + err.Error()}
		return nil
	}
	openCodePhaseModelDiscoveryDiagnostics = append([]string(nil), result.Diagnostics...)
	openCodePhaseModelProviderOptions = openCodeProviderOptionsFromDiscovery(result)
	return openCodePhaseModelOptionsFromDiscovery(result)
}

func openCodeProviderOptionsFromDiscovery(result opencode.DiscoveryResult) []openCodeProviderOption {
	providers := append([]opencode.AvailableProvider(nil), result.Providers...)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Provider.ID < providers[j].Provider.ID
	})
	options := make([]openCodeProviderOption, 0, len(providers))
	for _, available := range providers {
		providerID := available.Provider.ID
		modelIDs := make([]string, 0, len(available.Provider.Models))
		modelsByID := map[string]opencode.Model{}
		for modelID, model := range available.Provider.Models {
			if model.ID != "" {
				modelID = model.ID
			}
			if modelID == "" {
				continue
			}
			model.ID = modelID
			modelsByID[modelID] = model
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)
		models := make([]openCodeModelOption, 0, len(modelIDs))
		for _, modelID := range modelIDs {
			models = append(models, openCodeModelOption{ProviderID: providerID, Model: modelsByID[modelID]})
		}
		options = append(options, openCodeProviderOption{Provider: available.Provider, Models: models})
	}
	return options
}

func filterOpenCodeModelOptions(models []openCodeModelOption, query string) []openCodeModelOption {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]openCodeModelOption(nil), models...)
	}
	filtered := []openCodeModelOption{}
	for _, model := range models {
		id := strings.ToLower(model.Model.ID)
		name := strings.ToLower(model.Model.Name)
		if strings.Contains(id, query) || strings.Contains(name, query) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func phaseModelEffortOptions(providerID string, model opencode.Model) []string {
	return opencode.EffortOptions(providerID, model)
}

func defaultOpenCodePathRoots() opencode.PathRoots {
	home, _ := os.UserHomeDir()
	cache, _ := os.UserCacheDir()
	configDir, _ := os.UserConfigDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" && home != "" {
		data = filepath.Join(home, ".local", "share")
	}
	return opencode.PathRoots{HomeDir: home, CacheDir: cache, ConfigDir: configDir, DataDir: data}
}

func openCodePhaseModelOptionsFromDiscovery(result opencode.DiscoveryResult) []state.OpenCodeModelAssignment {
	providers := append([]opencode.AvailableProvider(nil), result.Providers...)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Provider.ID < providers[j].Provider.ID
	})
	options := []state.OpenCodeModelAssignment{}
	for _, available := range providers {
		providerID := available.Provider.ID
		modelsByID := map[string]opencode.Model{}
		modelIDs := make([]string, 0, len(available.Provider.Models))
		for modelID, model := range available.Provider.Models {
			if model.ID != "" {
				modelID = model.ID
			}
			if modelID != "" {
				model.ID = modelID
				modelsByID[modelID] = model
				modelIDs = append(modelIDs, modelID)
			}
		}
		sort.Strings(modelIDs)
		for _, modelID := range modelIDs {
			for _, effort := range opencode.EffortOptions(providerID, modelsByID[modelID]) {
				options = append(options, state.OpenCodeModelAssignment{ProviderID: providerID, ModelID: modelID, Effort: effort})
			}
		}
	}
	return ensureOpenCodeLegacyOption(options)
}

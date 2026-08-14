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
	Scope  config.SetupScope

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
	phaseModelOpenCodeAssignments []config.OpenCodeModelAssignment
	phaseModelOpenCodeDiagnostics []string
	phaseModelHasOpenCode         bool
	phaseModelHasClaude           bool
	phaseModelMode                phaseModelMode
	phaseModelProviderCursor      int
	phaseModelModelCursor         int
	phaseModelEffortCursor        int
	phaseModelModelSearch         string
	phaseModelOpenCodeProviders   []openCodeProviderOption
	phaseModelPendingOpenCode     config.OpenCodeModelAssignment
	phaseModelPendingClaude       config.ClaudeModelAssignment

	cfg *config.AppConfig
	// manifest is the desired state the wizard prefills from. ~/.jarvis/state.yaml
	// owns persona, selected skills and the per-phase models; cfg only still
	// carries them because config.Load's temporary bridge projects them back.
	// It is never nil: a machine with no manifest yet reads as an empty one.
	manifest *state.State

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
		Scope:      config.ScopeLocalOnly,
		PersonaFS:  wcfg.PersonaFS,
		SkillsFS:   wcfg.SkillsFS,
		TemplateFS: wcfg.TemplateFS,
		ProjectCWD: wcfg.ProjectCWD,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalOnly},
		manifest:   state.New(),
		noTUI:      noTUI,
	}

	// The manifest is read before config.Load for two reasons: the migration it
	// performs must have run by the time the bridge projects the manifest back
	// onto AppConfig, and reading it separately means a config.yaml that fails to
	// load no longer takes the user's recorded persona, skills and phase models
	// down with it. A failed manifest read leaves the empty manifest above, which
	// is the same "nothing recorded" state a fresh machine starts from; this
	// constructor has always continued past a failed load rather than aborting
	// the wizard, and a TUI constructor has no channel to report on.
	if manifest, _, err := loadWizardManifest(); err == nil {
		m.manifest = manifest
	}

	if loaded, err := config.Load(); err == nil {
		m.cfg = loaded
		m.Mode = string(loaded.ConfigStatus())
		m.Scope = loaded.Scope
		if loaded.Cloud != nil {
			m.Email = loaded.Cloud.Email
		}
	}
	if m.Scope == "" {
		m.Scope = config.ScopeLocalOnly
	}

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
		m.cfg = &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalOnly}
	}
	if m.cfg.SelectedSkills == nil {
		m.cfg.SelectedSkills = []string{}
	}
	if m.cfg.Install.Agents == nil {
		m.cfg.Install.Agents = map[string]config.AgentState{}
	}
	m.cfg.Scope = m.Scope
	// The persona being replaced is whatever the manifest recorded, so the apply
	// step cleans up the right previous profile even when config.yaml failed to
	// load or never carried one.
	m.previousPresetSlug, m.previousPresetSource = wizardPersonaSelection(m.manifest)

	m.Agents = agent.Detect(wcfg.TemplateFS)
	m = initializePhaseModelEditor(m)

	return m
}

// defaultWizardPersona is the persona slug config.Load falls back to when none
// is recorded. The wizard has always prefilled that value, so reading the
// persona out of the manifest has to keep falling back to it.
const defaultWizardPersona = "argentino"

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
// from.
//
// The defaults it applies -- a blank slug becomes the default persona, and any
// source that is not "user" becomes "builtin" -- are the ones config.Load
// applied to these fields while AppConfig still owned them. Reproducing them
// here is deliberate and temporary: the wizard must not start prefilling a
// different persona than it did yesterday, and the duplication disappears with
// the AppConfig bridge, not before.
func wizardPersonaSelection(manifest *state.State) (string, persona.PresetSource) {
	slug := ""
	source := persona.PresetSourceBuiltin
	if manifest != nil {
		slug = strings.TrimSpace(manifest.Persona)
		if strings.ToLower(strings.TrimSpace(string(manifest.PersonaSource))) == string(persona.PresetSourceUser) {
			source = persona.PresetSourceUser
		}
	}
	if slug == "" {
		slug = defaultWizardPersona
	}
	return slug, source
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
//
// config.Load lowercased and trimmed these keys and values on the way in, and
// the wizard looks them up by contract phase name, so the same normalization has
// to happen at this read. The manifest stores what was written: migration copies
// config.yaml verbatim, so a hand-edited `Apply:` key would otherwise stop
// matching the contract's `apply`.
func wizardPhaseModels(manifest *state.State) state.PhaseModels {
	out := state.PhaseModels{
		Aliases:  map[string]state.PhaseModelSelection{},
		OpenCode: map[string]state.OpenCodeModelAssignment{},
		Claude:   map[string]state.ClaudeModelAssignment{},
	}
	if manifest == nil {
		return out
	}
	for rawPhase, sel := range manifest.PhaseModels.Aliases {
		phase := normalizeWizardPhaseKey(rawPhase)
		if phase == "" {
			continue
		}
		sel.OpenCode = strings.ToLower(strings.TrimSpace(sel.OpenCode))
		sel.Claude = strings.ToLower(strings.TrimSpace(sel.Claude))
		out.Aliases[phase] = sel
	}
	for rawPhase, assignment := range manifest.PhaseModels.OpenCode {
		phase := normalizeWizardPhaseKey(rawPhase)
		if phase == "" {
			continue
		}
		assignment.ProviderID = strings.TrimSpace(assignment.ProviderID)
		assignment.ModelID = strings.TrimSpace(assignment.ModelID)
		assignment.Effort = strings.TrimSpace(assignment.Effort)
		out.OpenCode[phase] = assignment
	}
	for rawPhase, assignment := range manifest.PhaseModels.Claude {
		phase := normalizeWizardPhaseKey(rawPhase)
		if phase == "" {
			continue
		}
		assignment.Model = strings.TrimSpace(assignment.Model)
		assignment.Effort = strings.TrimSpace(assignment.Effort)
		out.Claude[phase] = assignment
	}
	return out
}

func normalizeWizardPhaseKey(phase string) string {
	return strings.ToLower(strings.TrimSpace(phase))
}

// recordWizardDesiredState writes the choices the wizard just applied into
// ~/.jarvis/state.yaml, which owns them.
//
// The write is sequenced strictly after config.Save and never wrapped around it.
// config.Save's temporary bridge takes the fail-fast, non-reentrant manifest
// lock internally, so nesting would deadlock the process on the first setup run,
// and going last keeps the bridge's own re-derivation from overwriting these
// values. state.Update is unlocked for the same reason and says so.
func recordWizardDesiredState(cfg *config.AppConfig) error {
	if cfg == nil {
		return nil
	}
	slug, source := cfg.PersonaPreset, cfg.PersonaPresetSource
	skillIDs := append([]string(nil), cfg.SelectedSkills...)
	scope := cfg.Scope
	order := append([]string(nil), cfg.ConfiguredAgents...)

	records := make(map[string]state.AgentRecord, len(cfg.Install.Agents))
	for id, agentState := range cfg.Install.Agents {
		records[id] = state.AgentRecord{
			Configured:       agentState.Configured,
			InstructionsPath: agentState.InstructionsPath,
			ConfigPath:       agentState.ConfigPath,
		}
	}
	phaseModels := cfg.PhaseModelsForState()

	return state.Update(func(st *state.State) {
		st.Persona = slug
		st.PersonaSource = state.PersonaSource(source)
		st.Skills = skillIDs
		st.Scope = state.Scope(scope)
		st.InstalledAgents = state.InstalledAgentsFrom(order, records)
		if len(st.InstalledAgents) > 0 {
			st.SelectionConfigured = true
		}
		st.PhaseModels = phaseModels
	})
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
		row.OpenCodeAssignment = config.OpenCodeModelAssignment(phaseModels.OpenCode[phase])
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
	if m.cfg != nil {
		if m.cfg.SDD.PhaseModels == nil {
			m.cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{}
		}
		if m.cfg.SDD.OpenCodePhaseModels == nil {
			m.cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{}
		}
		for _, row := range m.phaseModelRows {
			m.cfg.SDD.PhaseModels[row.Phase] = config.PhaseModelSelection{OpenCode: row.OpenCode, Claude: row.Claude}
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

func ensureOpenCodeLegacyOption(options []config.OpenCodeModelAssignment) []config.OpenCodeModelAssignment {
	if len(options) == 0 || options[0] == (config.OpenCodeModelAssignment{}) {
		return options
	}
	out := make([]config.OpenCodeModelAssignment, 0, len(options)+1)
	out = append(out, config.OpenCodeModelAssignment{})
	out = append(out, options...)
	return out
}

var openCodePhaseModelDiscoveryDiagnostics []string
var openCodePhaseModelProviderOptions []openCodeProviderOption

var discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
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

func openCodePhaseModelOptionsFromDiscovery(result opencode.DiscoveryResult) []config.OpenCodeModelAssignment {
	providers := append([]opencode.AvailableProvider(nil), result.Providers...)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Provider.ID < providers[j].Provider.ID
	})
	options := []config.OpenCodeModelAssignment{}
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
				options = append(options, config.OpenCodeModelAssignment{ProviderID: providerID, ModelID: modelID, Effort: effort})
			}
		}
	}
	return ensureOpenCodeLegacyOption(options)
}

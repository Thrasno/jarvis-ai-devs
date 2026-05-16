// Package tui provides the Bubbletea-based wizard for jarvis-cli first-run setup
// and auxiliary TUI components (timeline stub).
package tui

import (
	"embed"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
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

	PersonaFS  embed.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS

	Email       string
	Password    string
	APIToken    string
	activeField int

	Presets              []persona.Preset
	presetCur            int
	CustomYAML           string
	customEdit           bool
	customField          int
	customPresetName     string
	customDisplayName    string
	selectedPreset       *persona.ResolvedPreset
	previousPresetSlug   string
	previousPresetSource persona.PresetSource

	SkillList    []skills.Skill
	SkillPrompts []skillPrompt
	Selected     map[string]bool

	Agents        []agent.Agent
	agentProgress []string
	agentDone     bool
	reviewChoice  int

	phaseModelRows      []phaseModelRow
	phaseModelActiveRow int
	phaseModelActiveCol int
	phaseModelOpenCode  []string
	phaseModelClaude    []string

	cfg *config.AppConfig

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

// WizardConfig carries FSes needed to run the wizard, injected by main.
type WizardConfig struct {
	PersonaFS  embed.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS
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
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalOnly},
		noTUI:      noTUI,
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

	presets, err := persona.ListPresets(m.PersonaFS)
	if err == nil {
		m.Presets = append(m.Presets, presets...)
		m.Presets = append(m.Presets, persona.Preset{
			Name:        "custom",
			DisplayName: "Custom (crear nuevo)",
			Description: "Creá un preset propio con slug y display name, validado y persistido en ~/.jarvis/personas/<slug>.yaml.",
		})
		if m.cfg != nil {
			for i, p := range presets {
				if p.Name == m.cfg.PersonaPreset {
					m.presetCur = i
					break
				}
			}
		}
	}

	skillList, err := skills.ListSkills(m.SkillsFS)
	if err == nil {
		m.SkillList = skillList
		var existing []string
		if m.cfg != nil {
			existing = m.cfg.SelectedSkills
		}
		plan := buildSkillSelectionPlan(skillList, existing)
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
	m.previousPresetSlug = m.cfg.PersonaPreset
	switch m.cfg.PersonaPresetSource {
	case string(persona.PresetSourceUser):
		m.previousPresetSource = persona.PresetSourceUser
	default:
		m.previousPresetSource = persona.PresetSourceBuiltin
	}

	m.Agents = agent.Detect(wcfg.TemplateFS)
	m = initializePhaseModelEditor(m)

	return m
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
	case StepApply:
		return viewApply(m)
	case StepDone:
		return viewDone(m)
	}
	return ""
}

func initializePhaseModelEditor(m Model) Model {
	contract := sddruntime.DefaultContract()
	resolved := sddruntime.ResolvePhaseModels(m.cfg)
	m.phaseModelRows = make([]phaseModelRow, 0, len(contract.Phases))
	for _, phase := range contract.Phases {
		sel := resolved[phase]
		m.phaseModelRows = append(m.phaseModelRows, phaseModelRow{Phase: phase, OpenCode: sel.OpenCode, Claude: sel.Claude})
	}
	m.phaseModelOpenCode = append([]string(nil), contract.PlatformCatalogs[sddruntime.PlatformOpenCode]...)
	m.phaseModelClaude = append([]string(nil), contract.PlatformCatalogs[sddruntime.PlatformClaude]...)
	if m.phaseModelActiveCol == 0 {
		m.phaseModelActiveCol = 1
	}
	if m.cfg != nil {
		if m.cfg.SDD.PhaseModels == nil {
			m.cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{}
		}
		for _, row := range m.phaseModelRows {
			m.cfg.SDD.PhaseModels[row.Phase] = config.PhaseModelSelection{OpenCode: row.OpenCode, Claude: row.Claude}
		}
	}
	return m
}

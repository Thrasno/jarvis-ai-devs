// Package tui provides the Bubbletea-based wizard for jarvis-cli first-run setup
// and auxiliary TUI components (timeline stub).
package tui

import (
	"embed"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
)

// Step identifies the current wizard step.
type Step int

const (
	StepScope Step = iota
	StepHiveCloud
	StepPersona
	StepSkills
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

// Model is the root Bubbletea model for the jarvis-cli wizard.
type Model struct {
	Step  Step
	Mode  string
	Scope config.SetupScope

	PersonaFS  embed.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS

	Email    string
	Password string
	APIToken string
	activeField int

	Presets    []persona.Preset
	presetCur  int
	CustomYAML string
	customEdit bool

	SkillList    []skills.Skill
	SkillPrompts []skillPrompt
	Selected     map[string]bool

	Agents        []agent.Agent
	agentProgress []string
	agentDone     bool
	reviewChoice  int

	cfg *config.AppConfig

	Err  error
	Done bool

	width  int
	height int

	noTUI bool
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
		m.Presets = presets
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

	m.Agents = agent.Detect(wcfg.TemplateFS)

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
	switch m.Step {
	case StepScope:
		return viewScope(m)
	case StepHiveCloud:
		return viewHiveCloud(m)
	case StepPersona:
		return viewPersona(m)
	case StepSkills:
		return viewSkills(m)
	case StepReview:
		return viewReview(m)
	case StepApply:
		return viewApply(m)
	case StepDone:
		return viewDone(m)
	}
	return ""
}

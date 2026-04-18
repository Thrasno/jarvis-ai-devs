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
	// StepHiveLocal creates ~/.jarvis/memory.db (or probes for hive-daemon).
	StepHiveLocal Step = iota
	// StepHiveCloud authenticates with the Hive Cloud API (skippable).
	StepHiveCloud
	// StepPersona selects a persona preset.
	StepPersona
	// StepSkills selects optional skill files.
	StepSkills
	// StepAgentConfig configures all detected AI agents.
	StepAgentConfig
	// StepDone shows the completion summary.
	StepDone
)

// StepExtraSkills aliases StepSkills to keep explicit naming in tests/UX.
const StepExtraSkills = StepSkills

// Model is the root Bubbletea model for the jarvis-cli wizard.
// It holds all state across all 5 wizard steps.
type Model struct {
	// Current step in the wizard.
	Step Step
	// Mode describes setup intent: setup, reconfigure, recover.
	Mode string

	// Embedded FSes passed in from the root package (assets.PersonaFS, etc.).
	PersonaFS  embed.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS

	// HiveCloud inputs (Step 2).
	Email    string
	Password string
	APIToken string
	// activeField tracks which input is focused: 0=email, 1=password.
	activeField int

	// Persona (Step 3).
	Presets    []persona.Preset
	presetCur  int    // cursor index into Presets
	CustomYAML string // user-edited custom YAML (only when "custom" selected)
	customEdit bool   // true when in inline custom YAML edit mode

	// Skills (Step 4).
	SkillList    []skills.Skill
	SkillPrompts []skillPrompt
	Selected     map[string]bool

	// AgentConfig (Step 5).
	Agents        []agent.Agent
	agentProgress []string // status lines written during config
	agentDone     bool

	// Config accumulated through the wizard, saved at StepDone.
	cfg *config.AppConfig

	// Status / error display.
	Err  error
	Done bool

	// Terminal dimensions.
	width  int
	height int

	// noTUI is true when --no-tui was requested; wizard is bypassed externally.
	noTUI bool
}

// WizardConfig carries FSes needed to run the wizard, injected by main.
type WizardConfig struct {
	PersonaFS  embed.FS
	SkillsFS   embed.FS
	TemplateFS embed.FS
}

// NewModel creates a fresh wizard model at StepHiveLocal.
// noTUI is forwarded for informational purposes only (callers bypass the TUI when true).
func NewModel(wcfg WizardConfig, noTUI bool) Model {
	m := Model{
		Step:       StepHiveLocal,
		Mode:       string(config.ConfigStatusSetup),
		PersonaFS:  wcfg.PersonaFS,
		SkillsFS:   wcfg.SkillsFS,
		TemplateFS: wcfg.TemplateFS,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL},
		noTUI:      noTUI,
	}

	if loaded, err := config.Load(); err == nil {
		m.cfg = loaded
		m.Mode = string(loaded.ConfigStatus())
		if loaded.Cloud != nil {
			m.Email = loaded.Cloud.Email
		}
	}

	// Pre-load persona presets. Ignore errors — view will show them.
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

	// Pre-load skills, pre-select core skills.
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

	if m.cfg != nil {
		if m.cfg.SelectedSkills == nil {
			m.cfg.SelectedSkills = []string{}
		}
		if m.cfg.Install.Agents == nil {
			m.cfg.Install.Agents = map[string]config.AgentState{}
		}
	}

	if m.cfg == nil {
		m.cfg = &config.AppConfig{APIURL: config.DefaultAPIURL}
	}

	// Detect installed agents (inject TemplatesFS for WriteInstructions rendering).
	m.Agents = agent.Detect(wcfg.TemplateFS)

	return m
}

// Init returns the initial Bubbletea command (none needed for the wizard).
func (m Model) Init() tea.Cmd {
	return nil
}

// Update routes incoming messages to the per-step handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global quit shortcut.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m.updateStep(msg)

	case errMsg:
		m.Err = msg.err
		return m, nil

	default:
		// Delegate async messages (loginResultMsg, agentProgressMsg) to step handlers.
		updated, handled, cmd := handleStepMsg(m, msg)
		if handled {
			// After the first agentProgressMsg arrives, kick off the full sequence.
			if m.Step == StepAgentConfig && len(m.agentProgress) == 0 && len(updated.agentProgress) > 0 {
				return updated, tea.Batch(cmd, runAgentConfigSequence(updated))
			}
			return updated, cmd
		}
	}

	return m, nil
}

// errMsg carries an error from an async Cmd back into the model.
type errMsg struct{ err error }

// updateStep delegates key handling to the current step.
func (m Model) updateStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.Step {
	case StepHiveLocal:
		return updateHiveLocal(m, msg)
	case StepHiveCloud:
		return updateHiveCloud(m, msg)
	case StepPersona:
		return updatePersona(m, msg)
	case StepSkills:
		return updateSkills(m, msg)
	case StepAgentConfig:
		return updateAgentConfig(m, msg)
	case StepDone:
		return updateDone(m, msg)
	}
	return m, nil
}

// View renders the current step as a string.
func (m Model) View() string {
	switch m.Step {
	case StepHiveLocal:
		return viewHiveLocal(m)
	case StepHiveCloud:
		return viewHiveCloud(m)
	case StepPersona:
		return viewPersona(m)
	case StepSkills:
		return viewSkills(m)
	case StepAgentConfig:
		return viewAgentConfig(m)
	case StepDone:
		return viewDone(m)
	}
	return ""
}

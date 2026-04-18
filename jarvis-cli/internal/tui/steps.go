package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/apiclient"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// ──────────────────────────────────────────────────────────────────────────────
// Style helpers
// ──────────────────────────────────────────────────────────────────────────────

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Underline(true)
)

// stepHeader returns a formatted wizard header for the given step number.
func stepHeader(step, total int, title string) string {
	return titleStyle.Render(fmt.Sprintf("Jarvis-Dev Setup  [%d/%d]  %s", step, total, title)) + "\n\n"
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 1: HiveLocal
// ──────────────────────────────────────────────────────────────────────────────

func updateHiveLocal(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Create ~/.jarvis directory and touch the SQLite placeholder.
		home, err := os.UserHomeDir()
		if err != nil {
			m.Err = fmt.Errorf("cannot determine home dir: %w", err)
			return m, nil
		}
		jarvisDir := filepath.Join(home, ".jarvis")
		if err := os.MkdirAll(jarvisDir, 0755); err != nil {
			m.Err = fmt.Errorf("create ~/.jarvis: %w", err)
			return m, nil
		}
		// Touch memory.db — hive-daemon manages the actual SQLite schema.
		dbPath := filepath.Join(jarvisDir, "memory.db")
		if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
			f, createErr := os.Create(dbPath)
			if createErr != nil {
				m.Err = fmt.Errorf("create memory.db: %w", createErr)
				return m, nil
			}
			f.Close()
		}
		m.Err = nil
		m.Step = StepHiveCloud
	}
	return m, nil
}

func viewHiveLocal(m Model) string {
	var sb strings.Builder
	sb.WriteString(stepHeader(1, 6, "Local Memory Database"))
	sb.WriteString("This will create " + headerStyle.Render("~/.jarvis/memory.db") + " for local persistent memory.\n")
	sb.WriteString("The hive-daemon MCP server manages the SQLite schema.\n\n")
	if m.Err != nil {
		sb.WriteString(errorStyle.Render("Error: "+m.Err.Error()) + "\n\n")
	}
	sb.WriteString(dimStyle.Render("Press Enter to continue, Ctrl+C to exit"))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 2: HiveCloud
// ──────────────────────────────────────────────────────────────────────────────

func updateHiveCloud(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyShiftTab:
		// Toggle between email (0) and password (1) fields.
		if m.activeField == 0 {
			m.activeField = 1
		} else {
			m.activeField = 0
		}
	case tea.KeyEnter:
		if m.activeField == 0 {
			// Move focus to password field on Enter from email.
			m.activeField = 1
			return m, nil
		}
		// Enter on password field: attempt login.
		if m.Email == "" {
			// Skip cloud auth entirely.
			m.Step = StepPersona
			return m, nil
		}
		return m, loginCmd(m.cfg.APIURL, m.Email, m.Password)
	case tea.KeyRunes:
		if m.activeField == 0 {
			m.Email += string(msg.Runes)
		} else {
			m.Password += string(msg.Runes)
		}
	case tea.KeyBackspace:
		if m.activeField == 0 && len(m.Email) > 0 {
			m.Email = m.Email[:len(m.Email)-1]
		} else if m.activeField == 1 && len(m.Password) > 0 {
			m.Password = m.Password[:len(m.Password)-1]
		}
	case tea.KeyEsc:
		// Skip cloud auth step.
		m.Email = ""
		m.Password = ""
		m.Step = StepPersona
	}
	return m, nil
}

// loginResultMsg is returned by the login async command.
type loginResultMsg struct {
	token string
	email string
	err   error
}

// loginCmd performs an async Hive Cloud login.
func loginCmd(apiURL, email, password string) tea.Cmd {
	return func() tea.Msg {
		c := apiclient.New(apiURL)
		resp, err := c.Login(email, password)
		if err != nil {
			return loginResultMsg{err: err}
		}
		resolvedEmail := strings.TrimSpace(resp.User.Email)
		if resolvedEmail == "" {
			resolvedEmail = email
		}
		return loginResultMsg{token: resp.Token, email: resolvedEmail}
	}
}

func (m Model) handleLoginResult(msg loginResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.Err = msg.err
		return m, nil
	}
	m.APIToken = msg.token
	if m.cfg.Cloud == nil {
		m.cfg.Cloud = &config.CloudConfig{}
	}
	m.cfg.Cloud.Email = msg.email
	m.cfg.Cloud.SyncConfigured = true
	m.Email = msg.email
	m.cfg.Email = msg.email
	m.Err = nil
	m.Step = StepPersona
	return m, nil
}

// writeSyncJSON writes ~/.jarvis/sync.json with cloud credentials.
// Only api_url, email, and password are stored — token is intentionally
// excluded because hive-daemon's syncFileConfig uses DisallowUnknownFields()
// and manages the token internally after login.
func writeSyncJSON(apiURL, email, password string) error {
	return config.WriteSyncCredentials(apiURL, email, password)
}

// Override Update to also handle loginResultMsg (needs to be wired in root Update).
// We embed the handling here and call from model.go's Update.
func handleStepMsg(m Model, msg tea.Msg) (Model, bool, tea.Cmd) {
	if m.Step == StepHiveCloud {
		if lr, ok := msg.(loginResultMsg); ok {
			updated, cmd := m.handleLoginResult(lr)
			return updated.(Model), true, cmd
		}
	}
	if m.Step == StepAgentConfig {
		if pr, ok := msg.(agentProgressMsg); ok {
			m.agentProgress = append(m.agentProgress, pr.line)
			if pr.failed {
				m.Err = errors.New(pr.line)
			}
			if pr.done {
				m.agentDone = true
			}
			return m, true, nil
		}
	}
	return m, false, nil
}

func viewHiveCloud(m Model) string {
	var sb strings.Builder
	title := "Hive Cloud Authentication"
	if m.Mode == string(config.ConfigStatusReconfigure) {
		title = "Hive Cloud Authentication (Reconfigure)"
	}
	sb.WriteString(stepHeader(2, 6, title))
	sb.WriteString("Connect to Hive Cloud for team memory sync (press Esc to skip).\n\n")

	// Email field
	emailLabel := "Email:    "
	if m.activeField == 0 {
		emailLabel = selectedStyle.Render("> Email:  ")
	} else {
		emailLabel = dimStyle.Render("  Email:  ")
	}
	sb.WriteString(emailLabel + m.Email + "\n")

	// Password field (masked)
	passLabel := ""
	if m.activeField == 1 {
		passLabel = selectedStyle.Render("> Password:")
	} else {
		passLabel = dimStyle.Render("  Password:")
	}
	sb.WriteString(passLabel + " " + strings.Repeat("*", len(m.Password)) + "\n\n")

	if m.Err != nil {
		sb.WriteString(errorStyle.Render("Error: "+m.Err.Error()) + "\n\n")
	}
	sb.WriteString(dimStyle.Render("Tab: switch field  Enter: next/login  Esc: skip"))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 3: Persona
// ──────────────────────────────────────────────────────────────────────────────

func updatePersona(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.customEdit {
		return updatePersonaCustomEdit(m, msg)
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.presetCur > 0 {
			m.presetCur--
		}
	case tea.KeyDown:
		if m.presetCur < len(m.Presets)-1 {
			m.presetCur++
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if m.presetCur > 0 {
				m.presetCur--
			}
		case "j":
			if m.presetCur < len(m.Presets)-1 {
				m.presetCur++
			}
		}
	case tea.KeyEnter:
		if len(m.Presets) == 0 {
			m.Step = StepExtraSkills
			return m, nil
		}
		selected := m.Presets[m.presetCur]
		if selected.Name == "custom" {
			// Enter inline YAML edit mode.
			m.customEdit = true
			return m, nil
		}
		m.cfg.PersonaPreset = selected.Name
		m.cfg.Preset = selected.Name
		m.Step = StepExtraSkills
	}
	return m, nil
}

func updatePersonaCustomEdit(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlS:
		// Validate and confirm custom YAML.
		if err := persona.ValidateCustom([]byte(m.CustomYAML)); err != nil {
			m.Err = err
			return m, nil
		}
		m.cfg.PersonaPreset = "custom"
		m.cfg.Preset = "custom"
		m.customEdit = false
		m.Err = nil
		m.Step = StepExtraSkills
	case tea.KeyEsc:
		m.customEdit = false
		m.Err = nil
	case tea.KeyRunes:
		m.CustomYAML += string(msg.Runes)
	case tea.KeyBackspace:
		if len(m.CustomYAML) > 0 {
			m.CustomYAML = m.CustomYAML[:len(m.CustomYAML)-1]
		}
	case tea.KeyEnter:
		m.CustomYAML += "\n"
	}
	return m, nil
}

func viewPersona(m Model) string {
	var sb strings.Builder
	sb.WriteString(stepHeader(3, 6, "Select Persona Preset"))

	if m.customEdit {
		sb.WriteString(headerStyle.Render("Custom YAML Editor") + "\n")
		sb.WriteString(dimStyle.Render("Ctrl+S: confirm  Esc: cancel") + "\n\n")
		if m.Err != nil {
			sb.WriteString(errorStyle.Render("Validation error: "+m.Err.Error()) + "\n\n")
		}
		sb.WriteString(m.CustomYAML)
		sb.WriteString("_")
		return sb.String()
	}

	if len(m.Presets) == 0 {
		sb.WriteString(errorStyle.Render("No presets loaded. Press Enter to continue.") + "\n")
		return sb.String()
	}

	for i, p := range m.Presets {
		cursor := "  "
		name := p.DisplayName
		if name == "" {
			name = p.Name
		}
		desc := dimStyle.Render("  " + p.Description)
		if i == m.presetCur {
			cursor = selectedStyle.Render("> ")
			name = selectedStyle.Render(name)
		}
		sb.WriteString(cursor + name + "\n")
		sb.WriteString(desc + "\n")
	}

	sb.WriteString("\n" + dimStyle.Render("↑/↓ or j/k: navigate  Enter: select"))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 4: Skills
// ──────────────────────────────────────────────────────────────────────────────

func updateSkills(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Find the index of the currently highlighted skill.
	// We track cursor in the same field reusing presetCur for simplicity.
	cur := m.presetCur
	if cur >= len(m.SkillList) {
		cur = 0
	}

	switch msg.Type {
	case tea.KeyUp:
		if cur > 0 {
			m.presetCur = cur - 1
		}
	case tea.KeyDown:
		if cur < len(m.SkillList)-1 {
			m.presetCur = cur + 1
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if cur > 0 {
				m.presetCur = cur - 1
			}
		case "j":
			if cur < len(m.SkillList)-1 {
				m.presetCur = cur + 1
			}
		case " ":
			// Toggle selection (core skills cannot be toggled off).
			if cur < len(m.SkillList) {
				s := m.SkillList[cur]
				if !s.IsCore {
					m.Selected[s.ID] = !m.Selected[s.ID]
				}
			}
		}
	case tea.KeySpace:
		if cur < len(m.SkillList) {
			s := m.SkillList[cur]
			if !s.IsCore {
				m.Selected[s.ID] = !m.Selected[s.ID]
			}
		}
	case tea.KeyEnter:
		m.Step = StepAgentConfig
	}
	return m, nil
}

func viewSkills(m Model) string {
	var sb strings.Builder
	sb.WriteString(stepHeader(4, 6, "Select Extra Skills"))
	sb.WriteString(dimStyle.Render("Core skills (locked) are always installed. Select optional extras here.") + "\n\n")

	cur := m.presetCur
	for i, s := range m.SkillList {
		check := "[ ]"
		if m.Selected[s.ID] || s.IsCore {
			check = "[x]"
		}
		lock := ""
		if s.IsCore {
			lock = dimStyle.Render(" (core)")
		}

		line := fmt.Sprintf("%s %s%s — %s", check, s.Name, lock, s.Description)
		if i == cur {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n" + dimStyle.Render("↑/↓ or j/k: navigate  Space: toggle  Enter: confirm"))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 5: AgentConfig
// ──────────────────────────────────────────────────────────────────────────────

// agentProgressMsg reports a single status line from the agent config sequence.
type agentProgressMsg struct {
	line   string
	done   bool
	failed bool
}

func updateAgentConfig(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if len(m.agentProgress) == 0 || (m.agentDone && m.Err != nil) {
			// First Enter: start the config sequence.
			m.agentProgress = nil
			m.agentDone = false
			m.Err = nil
			return m, runAgentConfigCmd(m)
		}
		if m.agentDone && m.Err == nil {
			m.Step = StepDone
		}
	}
	return m, nil
}

// runAgentConfigCmd performs the full agent configuration sequence as a Cmd.
func runAgentConfigCmd(m Model) tea.Cmd {
	return func() tea.Msg {
		// We return a synthetic first progress message to start.
		return agentProgressMsg{line: "Starting agent configuration..."}
	}
}

// configureAgents performs the full agent setup and sends progress messages.
// This is called from the view/update flow after the first agentProgressMsg arrives.
func runAgentConfigSequence(m Model) tea.Cmd {
	return func() tea.Msg {
		home, _ := os.UserHomeDir()

		// Build the sub-FS rooted at embed/skills for InstallSkills.
		skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
		if err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Skills FS error: %v", err), done: true, failed: true}
		}

		// Build the list of selected skill IDs.
		selectedIDs := buildSelectedIDs(m)

		// Build SkillInfo list from registry for template rendering.
		skillInfos := buildSkillInfoList(m)

		// Build Layer1 + Layer2 content.
		layer1 := config.Layer1Content()
		var layer2 string
		if m.cfg.PersonaPreset != "" && m.cfg.PersonaPreset != "custom" {
			if preset, err := persona.LoadPreset(m.PersonaFS, m.cfg.PersonaPreset); err == nil {
				layer2 = persona.RenderLayer2(preset)
			}
		} else if m.CustomYAML != "" {
			layer2 = m.CustomYAML
		}

		// MCP entry for hive-daemon — point directly to the binary.
		// Credentials are read by hive-daemon from ~/.jarvis/sync.json (written above).
		entry := agent.MCPEntry{
			Name:       "hive",
			DaemonPath: agent.HiveDaemonBinaryPath(home),
		}

		// MCP entry for Context7 — auto-configured after Hive.
		context7Entry := agent.MCPEntry{Name: "context7"}

		// Configure each detected agent and collect structured outcomes.
		results := configureWizardAgents(m.Agents, entry, context7Entry, layer1, layer2, skillInfos, skillsSubFS, selectedIDs)
		var configuredAgents []string
		for _, res := range results {
			if res.Err != nil {
				return agentProgressMsg{line: fmt.Sprintf("[%s] Configuration FAILED: %v", res.AgentName, res.Err), done: true, failed: true}
			}
			configuredAgents = append(configuredAgents, res.AgentName)
		}

		// Stage sync.json before canonical config commit.
		if strings.TrimSpace(m.Email) != "" && strings.TrimSpace(m.Password) != "" {
			if err := writeSyncJSON(m.cfg.APIURL, m.Email, m.Password); err != nil {
				return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: write sync.json: %v", err), done: true, failed: true}
			}
			if m.cfg.Cloud == nil {
				m.cfg.Cloud = &config.CloudConfig{}
			}
			m.cfg.Cloud.Email = strings.TrimSpace(m.Email)
			m.cfg.Cloud.SyncConfigured = true
			m.cfg.Email = m.cfg.Cloud.Email
		}

		// Save canonical config as the final commit step.
		m.cfg.SchemaVersion = 2
		m.cfg.ConfiguredAgents = configuredAgents
		m.cfg.SelectedSkills = selectedIDs
		m.cfg.Install.Mode = string(config.ConfigStatusReconfigure)
		m.cfg.Install.Completed = true
		if m.cfg.Install.Agents == nil {
			m.cfg.Install.Agents = map[string]config.AgentState{}
		}
		for _, res := range results {
			m.cfg.Install.Agents[res.AgentName] = res.State
		}
		m.cfg.Version = "1.0.0"
		if err := config.Save(m.cfg); err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: save config: %v", err), done: true, failed: true}
		}

		summary := fmt.Sprintf("Configuration complete. Agents configured: %s", strings.Join(configuredAgents, ", "))
		if len(configuredAgents) == 0 {
			summary = "No agents detected. Install Claude Code or OpenCode and re-run jarvis."
		}
		return agentProgressMsg{line: summary, done: true}
	}
}

// buildSelectedIDs returns a slice of skill IDs for all selected and core skills.
// Used to pass to InstallSkills(skillsFS, selected).
func buildSelectedIDs(m Model) []string {
	var ids []string
	for _, s := range m.SkillList {
		if m.Selected[s.ID] || s.IsCore {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// buildSkillInfoList returns a slice of SkillInfo structs for template rendering.
// Only includes selected and core skills from the SkillList.
func buildSkillInfoList(m Model) []config.SkillInfo {
	var infos []config.SkillInfo
	for _, s := range m.SkillList {
		if m.Selected[s.ID] || s.IsCore {
			infos = append(infos, config.SkillInfo{
				Name:        s.Name,
				Description: s.Description,
				Trigger:     s.Trigger,
			})
		}
	}
	return infos
}

func viewAgentConfig(m Model) string {
	var sb strings.Builder
	sb.WriteString(stepHeader(5, 6, "Review & Apply Configuration"))

	if len(m.agentProgress) == 0 {
		agentNames := make([]string, 0, len(m.Agents))
		for _, a := range m.Agents {
			agentNames = append(agentNames, a.Name())
		}
		if len(agentNames) == 0 {
			sb.WriteString("No agents detected on this system.\n")
			sb.WriteString(dimStyle.Render("Install Claude Code or OpenCode, then re-run jarvis.") + "\n\n")
		} else {
			sb.WriteString("Detected agents: " + strings.Join(agentNames, ", ") + "\n\n")
		}
		sb.WriteString(dimStyle.Render("Press Enter to apply changes (agent files + config)."))
		return sb.String()
	}

	for _, line := range m.agentProgress {
		sb.WriteString(line + "\n")
	}

	if m.agentDone {
		if m.Err != nil {
			sb.WriteString("\n" + errorStyle.Render("Setup failed. Press Enter to retry."))
		} else {
			sb.WriteString("\n" + successStyle.Render("All done!") + "\n")
			sb.WriteString(dimStyle.Render("Press Enter to see the summary."))
		}
	}
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step Done
// ──────────────────────────────────────────────────────────────────────────────

func updateDone(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyRunes:
		if string(msg.Runes) == "q" || msg.Type == tea.KeyEnter {
			m.Done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func viewDone(m Model) string {
	var sb strings.Builder
	if m.Mode == string(config.ConfigStatusReconfigure) {
		sb.WriteString(titleStyle.Render("Jarvis-Dev Reconfiguration Complete!") + "\n\n")
	} else {
		sb.WriteString(titleStyle.Render("Jarvis-Dev Setup Complete!") + "\n\n")
	}
	sb.WriteString(successStyle.Render("Your AI coding environment is configured.") + "\n\n")
	sb.WriteString(headerStyle.Render("Next Steps:") + "\n")
	sb.WriteString("  1. Restart Claude Code or OpenCode to load the new MCP config.\n")
	sb.WriteString("  2. Use " + headerStyle.Render("'jarvis persona set <preset>'") + " to change persona.\n")
	sb.WriteString("  3. Use mem_sync in your agent only when you want a manual cloud sync.\n\n")
	sb.WriteString(dimStyle.Render("Press Enter or q to exit."))
	return sb.String()
}

// skillsSelectedList returns IDs of non-core selected skills.
func skillsSelectedList(m Model) []string {
	var result []string
	for id, on := range m.Selected {
		if on {
			result = append(result, id)
		}
	}
	return result
}

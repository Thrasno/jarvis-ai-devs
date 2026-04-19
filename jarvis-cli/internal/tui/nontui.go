package tui

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/apiclient"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
)

// RunNoTUI executes the full wizard using plain readline-style prompts.
// Used when --no-tui flag is set or when stdin is not a terminal.
func RunNoTUI(wcfg WizardConfig) error {
	return runNoTUI(wcfg, os.Stdin)
}

// runNoTUI is the testable implementation that accepts any io.Reader as input.
func runNoTUI(wcfg WizardConfig, input io.Reader) error {
	scanner := bufio.NewScanner(input)
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	mode := cfg.ConfigStatus()

	// ── Step 1: Scope ─────────────────────────────────────────────────────────
	fmt.Println("=== Jarvis-Dev Setup [1/6] Scope ===")
	fmt.Printf("Scope [local-only/local+cloud] (default: %s): ", cfg.Scope)
	scopeInput := strings.TrimSpace(readLine(scanner))
	scope := cfg.Scope
	if scope == "" {
		scope = config.ScopeLocalOnly
	}
	if scopeInput == string(config.ScopeLocalOnly) {
		scope = config.ScopeLocalOnly
	}
	if scopeInput == string(config.ScopeLocalCloud) {
		scope = config.ScopeLocalCloud
	}
	cfg.Scope = scope

	// ── Step 2: HiveCloud ─────────────────────────────────────────────────────
	header := "\n=== Jarvis-Dev Setup [2/6] Hive Cloud Authentication ==="
	if mode == config.ConfigStatusReconfigure {
		header = "\n=== Jarvis-Dev Reconfigure [2/6] Hive Cloud Authentication ==="
	}
	fmt.Println(header)
	currentEmail := ""
	if cfg.Cloud != nil {
		currentEmail = strings.TrimSpace(cfg.Cloud.Email)
	}
	email := ""
	var pendingPassword string

	if cfg.Scope == config.ScopeLocalCloud {
		if currentEmail == "" {
			fmt.Print("Email (press Enter to skip): ")
		} else {
			fmt.Printf("Email [%s] (Enter keeps current): ", currentEmail)
		}
		email = strings.TrimSpace(readLine(scanner))
		if email == "" {
			email = currentEmail
		}

		if email != "" {
			fmt.Print("Password (Enter keeps existing sync credentials): ")
			password := readLine(scanner)
			pendingPassword = password
			fmt.Printf("Authenticating as %s ...\n", email)
			c := apiclient.New(cfg.APIURL)
			resp, loginErr := c.Login(email, password)
			if loginErr != nil {
				fmt.Printf("Warning: authentication failed: %v\n", loginErr)
				fmt.Println("Skipping cloud auth. You can re-authenticate with 'jarvis login'.")
			} else {
				resolved := strings.TrimSpace(resp.User.Email)
				if resolved == "" {
					resolved = email
				}
				if cfg.Cloud == nil {
					cfg.Cloud = &config.CloudConfig{}
				}
				cfg.Cloud.Email = resolved
				cfg.Cloud.SyncConfigured = true
				cfg.Email = resolved
				fmt.Printf("Authenticated as %s.\n", resolved)
			}
		} else {
			fmt.Println("Skipping cloud auth.")
		}
	} else {
		fmt.Println("Scope local-only: cloud auth omitido.")
	}

	// ── Step 3: Persona ───────────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [3/6] Select Persona Preset ===")
	presets, err := persona.ListPresets(wcfg.PersonaFS)
	if err != nil {
		return fmt.Errorf("list presets: %w", err)
	}
	defaultPreset := cfg.PersonaPreset
	if defaultPreset == "" {
		defaultPreset = cfg.Preset
	}
	defaultIdx := 0
	for i, p := range presets {
		if p.Name == defaultPreset {
			defaultIdx = i
			break
		}
	}
	for i, p := range presets {
		name := p.DisplayName
		if name == "" {
			name = p.Name
		}
		fmt.Printf("  %d) %-20s — %s\n", i+1, name, p.Description)
	}
	fmt.Printf("Select preset number (default: %d): ", defaultIdx+1)
	choice := readLine(scanner)
	selectedPreset := defaultIdx
	if choice != "" {
		n := 0
		if _, scanErr := fmt.Sscanf(choice, "%d", &n); scanErr == nil && n >= 1 && n <= len(presets) {
			selectedPreset = n - 1
		}
	}
	cfg.PersonaPreset = presets[selectedPreset].Name
	cfg.Preset = cfg.PersonaPreset
	fmt.Printf("Selected: %s\n", cfg.PersonaPreset)

	// ── Step 4: Extra Skills ──────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [4/6] Select Extra Skills ===")
	skillList, err := skills.ListSkills(wcfg.SkillsFS)
	if err != nil {
		return fmt.Errorf("list skills: %w", err)
	}
	plan := buildSkillSelectionPlan(skillList, cfg.SelectedSkills)
	selected := plan.Selected
	for _, prompt := range plan.Prompts {
		defaultYes := false
		if len(prompt.SkillIDs) > 0 {
			defaultYes = selected[prompt.SkillIDs[0]]
		}
		if defaultYes {
			fmt.Printf("Install %s — %s? [Y/n]: ", prompt.Label, prompt.Description)
		} else {
			fmt.Printf("Install %s — %s? [y/N]: ", prompt.Label, prompt.Description)
		}
		ans := strings.ToLower(strings.TrimSpace(readLine(scanner)))
		next := defaultYes
		if ans == "y" || ans == "yes" {
			next = true
		}
		if ans == "n" || ans == "no" {
			next = false
		}
		for _, id := range prompt.SkillIDs {
			selected[id] = next
		}
	}

	// ── Step 5: Review/Apply ──────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [5/6] Review & Apply ===")
	fmt.Printf("Scope: %s\n", cfg.Scope)
	if cfg.Scope == config.ScopeLocalOnly {
		fmt.Println(localOnlyReviewWarning)
	}
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Persona: %s\n", cfg.PersonaPreset)
	fmt.Printf("Cloud: %s\n", strings.TrimSpace(cfg.Email))
	fmt.Print("Apply these changes now? [type 'yes' to continue]: ")
	applyAnswer := strings.ToLower(strings.TrimSpace(readLine(scanner)))
	if applyAnswer != "y" && applyAnswer != "yes" {
		fmt.Println("Aborted before apply. Existing config remains unchanged.")
		return nil
	}

	// ── Step 6: Apply ─────────────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [6/6] Configure AI Agents ===")
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	agents := agent.Detect(wcfg.TemplateFS)
	if len(agents) == 0 {
		fmt.Println("No agents detected. Install Claude Code or OpenCode and re-run jarvis.")
	}

	// Build the sub-FS rooted at embed/skills for InstallSkills.
	skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		return fmt.Errorf("skills sub-FS: %w", err)
	}

	// Build the list of selected skill IDs.
	var selectedIDs []string
	for _, s := range skillList {
		if selected[s.ID] || s.IsCore {
			selectedIDs = append(selectedIDs, s.ID)
		}
	}

	// Build SkillInfo list for template rendering.
	var skillInfos []config.SkillInfo
	for _, s := range skillList {
		if selected[s.ID] || s.IsCore {
			skillInfos = append(skillInfos, config.SkillInfo{
				Name:        s.Name,
				Description: s.Description,
				Trigger:     s.Trigger,
			})
		}
	}

	// Build Layer1 + Layer2 content.
	layer1 := config.Layer1Content()
	var layer2 string
	if cfg.PersonaPreset != "" {
		if preset, loadErr := persona.LoadPreset(wcfg.PersonaFS, cfg.PersonaPreset); loadErr == nil {
			layer2 = persona.RenderLayer2(preset)
		}
	}

	// Point MCP directly to the binary — credentials are read from ~/.jarvis/sync.json.
	entry := agent.MCPEntry{
		Name:       "hive",
		DaemonPath: agent.HiveDaemonBinaryPath(home),
	}
	context7Entry := agent.MCPEntry{Name: "context7"}

	results := configureWizardAgents(agents, entry, context7Entry, layer1, layer2, skillInfos, skillsSubFS, selectedIDs)
	var configuredAgents []string
	for _, res := range results {
		fmt.Printf("Configuring %s ...\n", res.AgentName)
		if res.Err != nil {
			return fmt.Errorf("configure %s: %w", res.AgentName, res.Err)
		}
		fmt.Printf("  %s configured.\n", res.AgentName)
		configuredAgents = append(configuredAgents, res.AgentName)
	}

	if cfg.Scope == config.ScopeLocalOnly {
		if err := config.DeleteSyncCredentials(); err != nil {
			return fmt.Errorf("cleanup cloud credentials: %w. Ver docs/setup-recovery.md", err)
		}
		cfg.Cloud = nil
		cfg.Email = ""
	} else if strings.TrimSpace(cfg.Email) != "" && strings.TrimSpace(pendingPassword) != "" {
		if err := writeSyncJSON(cfg.APIURL, cfg.Email, pendingPassword); err != nil {
			return fmt.Errorf("write sync.json: %w. Ver docs/setup-recovery.md", err)
		}
	}

	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		return fmt.Errorf("create ~/.jarvis: %w. Ver docs/setup-recovery.md", err)
	}
	dbPath := filepath.Join(jarvisDir, "memory.db")
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		f, createErr := os.Create(dbPath)
		if createErr != nil {
			return fmt.Errorf("create memory.db: %w. Ver docs/setup-recovery.md", createErr)
		}
		_ = f.Close()
	}

	cfg.ConfiguredAgents = configuredAgents
	cfg.SchemaVersion = 2
	cfg.Scope = scope
	cfg.Install.Mode = string(config.ConfigStatusReconfigure)
	cfg.Install.Completed = true
	if cfg.Install.Agents == nil {
		cfg.Install.Agents = map[string]config.AgentState{}
	}
	for _, res := range results {
		cfg.Install.Agents[res.AgentName] = res.State
	}

	selectedSet := make(map[string]bool)
	for _, id := range cfg.SelectedSkills {
		selectedSet[id] = true
	}
	for _, s := range skillList {
		if s.IsCore {
			selectedSet[s.ID] = true
			continue
		}
		if selected[s.ID] {
			selectedSet[s.ID] = true
		} else {
			delete(selectedSet, s.ID)
		}
	}
	var selectedIDsForConfig []string
	for id, on := range selectedSet {
		if on {
			selectedIDsForConfig = append(selectedIDsForConfig, id)
		}
	}
	cfg.SelectedSkills = selectedIDsForConfig
	cfg.Version = "1.0.0"
	if saveErr := config.Save(cfg); saveErr != nil {
		return fmt.Errorf("save config: %w", saveErr)
	}

	fmt.Println("\nConfiguration applied successfully!")
	fmt.Println("Existing choices were updated safely and persisted atomically.")
	fmt.Println("Next: restart Claude Code or OpenCode.")
	fmt.Println("Use mem_sync in your agent only when you want a manual cloud sync.")
	return nil
}

// readLine reads a single line from the scanner, trimming whitespace.
func readLine(scanner *bufio.Scanner) string {
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

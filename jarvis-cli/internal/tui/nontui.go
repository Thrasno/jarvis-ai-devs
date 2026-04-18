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

	// ── Step 1: HiveLocal ─────────────────────────────────────────────────────
	fmt.Println("=== Jarvis-Dev Setup [1/6] Local Memory Database ===")
	fmt.Println("Creating ~/.jarvis/memory.db ...")
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		return fmt.Errorf("create ~/.jarvis: %w", err)
	}
	dbPath := filepath.Join(jarvisDir, "memory.db")
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		f, createErr := os.Create(dbPath)
		if createErr != nil {
			return fmt.Errorf("create memory.db: %w", createErr)
		}
		f.Close()
	}
	fmt.Println("Done.")

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
	if currentEmail == "" {
		fmt.Print("Email (press Enter to skip): ")
	} else {
		fmt.Printf("Email [%s] (Enter keeps current): ", currentEmail)
	}
	email := strings.TrimSpace(readLine(scanner))
	if email == "" {
		email = currentEmail
	}
	var pendingPassword string

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
	selected := make(map[string]bool)
	for _, s := range skillList {
		if s.IsCore {
			selected[s.ID] = true
			fmt.Printf("  [core] %s — %s\n", s.Name, s.Description)
			continue
		}
		defaultYes := false
		for _, id := range cfg.SelectedSkills {
			if id == s.ID {
				defaultYes = true
				break
			}
		}
		if defaultYes {
			selected[s.ID] = true
			fmt.Printf("Install %s — %s? [Y/n]: ", s.Name, s.Description)
		} else {
			fmt.Printf("Install %s — %s? [y/N]: ", s.Name, s.Description)
		}
		ans := strings.ToLower(strings.TrimSpace(readLine(scanner)))
		if ans == "" && defaultYes {
			selected[s.ID] = true
		}
		if ans == "y" || ans == "yes" {
			selected[s.ID] = true
		}
		if ans == "n" || ans == "no" {
			selected[s.ID] = false
		}
	}

	// ── Step 5: Review/Apply ──────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [5/6] Review & Apply ===")
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Persona: %s\n", cfg.PersonaPreset)
	fmt.Printf("Cloud: %s\n", strings.TrimSpace(cfg.Email))
	fmt.Print("Apply these changes now? [Y/n]: ")
	applyAnswer := strings.ToLower(strings.TrimSpace(readLine(scanner)))
	if applyAnswer == "n" || applyAnswer == "no" {
		fmt.Println("Aborted before apply. Existing config remains unchanged.")
		return nil
	}

	// ── Step 6: AgentConfig ───────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [6/6] Configure AI Agents ===")
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

	// Stage sync.json first, then commit canonical config atomically.
	if strings.TrimSpace(cfg.Email) != "" && strings.TrimSpace(pendingPassword) != "" {
		if err := writeSyncJSON(cfg.APIURL, cfg.Email, pendingPassword); err != nil {
			return fmt.Errorf("write sync.json: %w", err)
		}
	}

	cfg.ConfiguredAgents = configuredAgents
	cfg.SchemaVersion = 2
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

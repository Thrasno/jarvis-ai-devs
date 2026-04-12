package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	cfg := &config.AppConfig{APIURL: config.DefaultAPIURL}

	// ── Step 1: HiveLocal ─────────────────────────────────────────────────────
	fmt.Println("=== Jarvis-Dev Setup [1/5] Local Memory Database ===")
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
	fmt.Println("\n=== Jarvis-Dev Setup [2/5] Hive Cloud Authentication ===")
	fmt.Print("Email (press Enter to skip): ")
	email := readLine(scanner)

	var apiToken string
	if email != "" {
		fmt.Print("Password: ")
		password := readLine(scanner)
		fmt.Printf("Authenticating as %s ...\n", email)
		c := apiclient.New(cfg.APIURL)
		resp, loginErr := c.Login(email, password)
		if loginErr != nil {
			fmt.Printf("Warning: authentication failed: %v\n", loginErr)
			fmt.Println("Skipping cloud auth. You can re-authenticate with 'jarvis login'.")
		} else {
			apiToken = resp.Token
			cfg.Email = resp.User.Email
			fmt.Printf("Authenticated as %s.\n", cfg.Email)
			_ = writeSyncJSON(cfg.APIURL, email, password, apiToken)
		}
	} else {
		fmt.Println("Skipping cloud auth.")
	}

	// ── Step 3: Persona ───────────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [3/5] Select Persona Preset ===")
	presets, err := persona.ListPresets(wcfg.PersonaFS)
	if err != nil {
		return fmt.Errorf("list presets: %w", err)
	}
	for i, p := range presets {
		name := p.DisplayName
		if name == "" {
			name = p.Name
		}
		fmt.Printf("  %d) %-20s — %s\n", i+1, name, p.Description)
	}
	fmt.Print("Select preset number (default: 1): ")
	choice := readLine(scanner)
	selectedPreset := 0
	if choice != "" {
		n := 0
		if _, scanErr := fmt.Sscanf(choice, "%d", &n); scanErr == nil && n >= 1 && n <= len(presets) {
			selectedPreset = n - 1
		}
	}
	cfg.Preset = presets[selectedPreset].Name
	fmt.Printf("Selected: %s\n", cfg.Preset)

	// ── Step 4: Skills ────────────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [4/5] Select Skills ===")
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
		fmt.Printf("Install %s — %s? [y/N]: ", s.Name, s.Description)
		ans := strings.ToLower(strings.TrimSpace(readLine(scanner)))
		if ans == "y" || ans == "yes" {
			selected[s.ID] = true
		}
	}

	// ── Step 5: AgentConfig ───────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [5/5] Configure AI Agents ===")
	agents := agent.Detect(wcfg.TemplateFS)
	if len(agents) == 0 {
		fmt.Println("No agents detected. Install Claude Code or OpenCode and re-run jarvis.")
	}

	// Build skill map.
	skillMap := make(map[string][]byte)
	for _, s := range skillList {
		if selected[s.ID] || s.IsCore {
			skillMap[s.ID] = s.Content
		}
	}

	// Build Layer1 + Layer2 content.
	layer1 := config.Layer1Content()
	var layer2 string
	if cfg.Preset != "" {
		if preset, loadErr := persona.LoadPreset(wcfg.PersonaFS, cfg.Preset); loadErr == nil {
			layer2 = persona.RenderLayer2(preset)
		}
	}

	daemonPath := filepath.Join(home, ".jarvis", "hive-daemon-start.sh")
	entry := agent.MCPEntry{
		Name:       "hive",
		APIURL:     cfg.APIURL,
		Email:      cfg.Email,
		DaemonPath: daemonPath,
	}

	var configuredAgents []string
	for _, a := range agents {
		agentName := a.Name()
		fmt.Printf("Configuring %s ...\n", agentName)

		if mergeErr := a.MergeConfig(entry); mergeErr != nil {
			fmt.Printf("  MCP config failed: %v\n", mergeErr)
			continue
		}
		if instrErr := a.WriteInstructions(layer1, layer2); instrErr != nil {
			fmt.Printf("  Instructions failed: %v\n", instrErr)
			continue
		}
		if skillErr := a.InstallSkills(skillMap); skillErr != nil {
			fmt.Printf("  Skills install failed: %v\n", skillErr)
			continue
		}
		fmt.Printf("  %s configured.\n", agentName)
		configuredAgents = append(configuredAgents, agentName)
	}

	// Generate start script if we have cloud creds.
	if cfg.Email != "" {
		scriptData := agent.StartScriptData{
			APIURL:     cfg.APIURL,
			Email:      cfg.Email,
			DaemonPath: daemonPath,
		}
		if scriptContent, scriptErr := agent.GenerateStartScript(scriptData); scriptErr == nil {
			_ = agent.WriteStartScript(daemonPath, scriptContent)
		}
	}

	// Save config.
	cfg.ConfiguredAgents = configuredAgents
	cfg.Version = "1.0.0"
	if saveErr := config.Save(cfg); saveErr != nil {
		return fmt.Errorf("save config: %w", saveErr)
	}

	fmt.Println("\nSetup complete!")
	fmt.Println("Next: restart Claude Code or OpenCode, then run 'jarvis sync'.")
	return nil
}

// readLine reads a single line from the scanner, trimming whitespace.
func readLine(scanner *bufio.Scanner) string {
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

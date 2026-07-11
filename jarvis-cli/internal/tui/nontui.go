package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/apiclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

var (
	loadAppConfig                   = config.Load
	listPersonaPresets              = persona.ListPresetsV2
	listAvailableSkills             = skills.ListSkills
	detectInstalledAgents           = agent.Detect
	noTUIStdout           io.Writer = os.Stdout
)

// RunNoTUI executes the full wizard using plain readline-style prompts.
// Used when --no-tui flag is set or when stdin is not a terminal.
func RunNoTUI(wcfg WizardConfig) error {
	return runNoTUI(wcfg, os.Stdin)
}

// runNoTUI is the testable implementation that accepts any io.Reader as input.
func runNoTUI(wcfg WizardConfig, input io.Reader) error {
	scanner := bufio.NewScanner(input)
	cfg, err := loadAppConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	previousPresetSlug := cfg.PersonaPreset
	previousPresetSource := persona.PresetSourceBuiltin
	if strings.TrimSpace(cfg.PersonaPresetSource) == string(persona.PresetSourceUser) {
		previousPresetSource = persona.PresetSourceUser
	}
	mode := cfg.ConfigStatus()

	// ── Step 1: Scope ─────────────────────────────────────────────────────────
	fmt.Println("=== Jarvis-Dev Setup [1/7] Scope ===")
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
	header := "\n=== Jarvis-Dev Setup [2/7] Hive Cloud Authentication ==="
	if mode == config.ConfigStatusReconfigure {
		header = "\n=== Jarvis-Dev Reconfigure [2/7] Hive Cloud Authentication ==="
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
	fmt.Println("\n=== Jarvis-Dev Setup [3/7] Select Persona Preset ===")
	presets, err := listPersonaPresets(wcfg.PersonaFS)
	if err != nil {
		return fmt.Errorf("list presets: %w", err)
	}
	configExists, err := hasPersistedConfig()
	if err != nil {
		return err
	}
	if configExists {
		if err := validateConfiguredPersonaPresetForV2Selection(wcfg.PersonaFS, cfg.PersonaPreset); err != nil {
			return err
		}
	}
	presets = append(presets, persona.PresetV2{
		Name:        "custom",
		DisplayName: "Custom (crear nuevo)",
	})
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
		fmt.Printf("  %d) %-20s — %s\n", i+1, name, schemaV2PresetDescription(p.Name))
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
	selectedPersona := presets[selectedPreset]
	var customDraft *customPresetDraft
	if selectedPersona.Name == "custom" {
		fmt.Print("Custom preset name (slug base): ")
		name := readLine(scanner)
		fmt.Print("Custom display name: ")
		displayName := readLine(scanner)
		fmt.Print("Custom YAML override (optional, single line; Enter keeps generated base): ")
		yamlOverride := readLine(scanner)
		customDraft = &customPresetDraft{Name: name, DisplayName: displayName, YAML: yamlOverride}
	}

	selection, resolvedPreset, err := resolveNoTUIPresetSelection(wcfg.PersonaFS, selectedPersona.Name, customDraft)
	if err != nil {
		return fmt.Errorf("resolve selected preset: %w", err)
	}

	cfg.PersonaPreset = resolvedPreset.Slug
	cfg.Preset = resolvedPreset.Slug
	cfg.PersonaPresetSource = string(resolvedPreset.Source)
	fmt.Printf("Selected: %s (%s)\n", resolvedPreset.Slug, resolvedPreset.Source)

	// ── Step 4: Extra Skills ──────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [4/7] Select Extra Skills ===")
	skillList, err := listAvailableSkills(wcfg.SkillsFS)
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

	// ── Step 5: SDD Phase Models ──────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [5/7] SDD Phase Models ===")
	resolvedPhaseModels := sddruntime.ResolvePhaseModels(cfg)
	openCodePhaseModelDiscoveryDiagnostics = nil
	opencodeAssignments := discoverOpenCodePhaseModelOptions()
	for _, diagnostic := range openCodePhaseModelDiscoveryDiagnostics {
		fmt.Fprintln(noTUIStdout, diagnostic)
	}
	applyPhaseModelEdits := func() {
		contract := sddruntime.DefaultContract()
		printOpenCodeAssignmentOptions(opencodeAssignments)
		if cfg.SDD.OpenCodePhaseModels == nil {
			cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{}
		}
		if cfg.SDD.ClaudePhaseModels == nil {
			cfg.SDD.ClaudePhaseModels = map[string]config.ClaudeModelAssignment{}
		}
		for _, phase := range contract.Phases {
			current := resolvedPhaseModels[phase]
			claudeAssignment := cfg.SDD.ClaudePhaseModels[phase]
			currentClaude := current.Claude
			if strings.TrimSpace(claudeAssignment.Model) != "" {
				currentClaude = strings.TrimSpace(claudeAssignment.Model)
			}
			fmt.Printf("%s [opencode=%s claude=%s]\n", phase, current.OpenCode, currentClaude)
			if len(opencodeAssignments) > 0 {
				currentAssignment := cfg.SDD.OpenCodePhaseModels[phase]
				fmt.Printf("  OpenCode provider/model [%s] (Enter keeps, number selects): ", openCodeAssignmentPromptValue(currentAssignment, current.OpenCode))
				opencodeInput := strings.TrimSpace(readLine(scanner))
				if assignment, ok := selectOpenCodeAssignmentForPrompt(opencodeInput, opencodeAssignments); ok {
					if assignment.ProviderID == "" || assignment.ModelID == "" {
						delete(cfg.SDD.OpenCodePhaseModels, phase)
					} else {
						cfg.SDD.OpenCodePhaseModels[phase] = assignment
					}
				}
			} else {
				fmt.Printf("  OpenCode model [%s] (Enter keeps): ", current.OpenCode)
				opencodeInput := strings.ToLower(strings.TrimSpace(readLine(scanner)))
				if opencodeInput != "" {
					current.OpenCode = normalizePlatformValueForPrompt(opencodeInput, current.OpenCode, contract.PlatformCatalogs[sddruntime.PlatformOpenCode])
					delete(cfg.SDD.OpenCodePhaseModels, phase)
				}
			}
			fmt.Printf("  Claude model [%s] (Enter keeps): ", currentClaude)
			claudeInput := strings.ToLower(strings.TrimSpace(readLine(scanner)))
			row := current
			row.Claude = normalizePlatformValueForPrompt(claudeInput, currentClaude, contract.PlatformCatalogs[sddruntime.PlatformClaude])
			resolvedPhaseModels[phase] = row
			fmt.Printf("  Claude effort [%s] (Enter keeps): ", displayPromptDefault(claudeAssignment.Effort))
			effortInput := strings.ToLower(strings.TrimSpace(readLine(scanner)))
			claudeAssignment.Model = row.Claude
			claudeAssignment.Effort = normalizeClaudeEffortForPrompt(effortInput, claudeAssignment.Effort)
			cfg.SDD.ClaudePhaseModels[phase] = claudeAssignment
		}
	}
	if cfg.SDD.PhaseModels == nil {
		cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{}
	}
	if cfg.SDD.OpenCodePhaseModels == nil {
		cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{}
	}
	if cfg.SDD.ClaudePhaseModels == nil {
		cfg.SDD.ClaudePhaseModels = map[string]config.ClaudeModelAssignment{}
	}
	for phase, row := range resolvedPhaseModels {
		cfg.SDD.PhaseModels[phase] = row
		if assignment, ok := cfg.SDD.ClaudePhaseModels[phase]; ok && (strings.TrimSpace(assignment.Model) != "" || strings.TrimSpace(assignment.Effort) != "") {
			if strings.TrimSpace(assignment.Model) == "" {
				assignment.Model = row.Claude
			}
			cfg.SDD.ClaudePhaseModels[phase] = assignment
		}
	}

	// ── Step 6: Review/Apply ──────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [6/7] Review & Apply ===")
	fmt.Printf("Scope: %s\n", cfg.Scope)
	if cfg.Scope == config.ScopeLocalOnly {
		fmt.Println(localOnlyReviewWarning)
	}
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Persona: %s\n", cfg.PersonaPreset)
	fmt.Printf("Cloud: %s\n", strings.TrimSpace(cfg.Email))
	printNoTUIPhaseModelReview(resolvedPhaseModels, cfg.SDD.OpenCodePhaseModels, cfg.SDD.ClaudePhaseModels)
	fmt.Print("Apply these changes now? [type 'yes' to continue, 'edit' to edit phase models]: ")
	applyAnswer := strings.ToLower(strings.TrimSpace(readLine(scanner)))
	if applyAnswer == "edit" {
		applyPhaseModelEdits()
		for phase, row := range resolvedPhaseModels {
			cfg.SDD.PhaseModels[phase] = row
		}
		printNoTUIPhaseModelReview(resolvedPhaseModels, cfg.SDD.OpenCodePhaseModels, cfg.SDD.ClaudePhaseModels)
		fmt.Print("Apply these changes now? [type 'yes' to continue]: ")
		applyAnswer = strings.ToLower(strings.TrimSpace(readLine(scanner)))
	}
	if applyAnswer != "y" && applyAnswer != "yes" {
		fmt.Println("Aborted before apply. Existing config remains unchanged.")
		return nil
	}

	// ── Step 7: Apply ─────────────────────────────────────────────────────────
	fmt.Println("\n=== Jarvis-Dev Setup [7/7] Configure AI Agents ===")
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	agents := detectInstalledAgents(wcfg.TemplateFS)
	if len(agents) == 0 {
		fmt.Println("No agents detected. Install Claude Code or OpenCode and re-run jarvis.")
	}

	// Build the sub-FS rooted at embed/skills for InstallSkills.
	skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		return fmt.Errorf("skills sub-FS: %w", err)
	}

	// Build the sub-FS rooted at embed/agents/claude for ClaudeAgent.InstallAgents.
	agentsSubFS, err := fs.Sub(jarvis.AgentsFS, "embed/agents/claude")
	if err != nil {
		return fmt.Errorf("agents sub-FS: %w", err)
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

	// Point MCP directly to the binary — credentials are read from ~/.jarvis/sync.json.
	entry := agent.MCPEntry{
		Name:       "hive",
		DaemonPath: agent.HiveDaemonBinaryPath(home),
	}
	context7Entry := agent.MCPEntry{Name: "context7"}

	// Determine statusline overwrite policy before the pipeline goroutine.
	// If the script already exists, prompt the user once; otherwise use a no-op
	// closure (confirm is never called on fresh install).
	statuslineConfirm, err := buildNoTUIStatuslineConfirm(home, scanner)
	if err != nil {
		return fmt.Errorf("check statusline script: %w", err)
	}

	results := configureWizardAgents(agents, cfg, entry, context7Entry, selection, wizardPresetApplyContext{
		Layer1:               config.Layer1Content(),
		Skills:               skillInfos,
		PreviousPresetSlug:   previousPresetSlug,
		PreviousPresetSource: previousPresetSource,
	}, skillsSubFS, selectedIDs, agentsSubFS, statuslineConfirm)
	var configuredAgents []string
	for _, res := range results {
		fmt.Printf("Configuring %s ...\n", res.AgentName)
		if res.Err != nil {
			return fmt.Errorf("configure %s: %w", res.AgentName, res.Err)
		}
		for _, warning := range res.Warnings {
			fmt.Fprintln(noTUIStdout, warning)
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
		enable := true
		if err := writeSyncJSON(cfg.APIURL, cfg.Email, pendingPassword, &enable); err != nil {
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
	registryWarnings, registryErr := refreshProjectRegistryForApply(context.Background(), wcfg.ProjectCWD)
	if registryErr != nil {
		return fmt.Errorf("project skill registry refresh failed: %w", registryErr)
	}
	for _, warning := range registryWarnings {
		fmt.Fprintln(noTUIStdout, warning)
	}

	fmt.Println("\nConfiguration applied successfully!")
	fmt.Println("Existing choices were updated safely and persisted atomically.")
	fmt.Println("Next: restart Claude Code or OpenCode.")
	fmt.Println("Use mem_sync in your agent only when you want a manual cloud sync.")
	return nil
}

// resolveNoTUIPresetSelection resolves a validated schema-v2 presentation
// profile before forwarding it through the versioned apply seam.
func resolveNoTUIPresetSelection(personaFS fs.FS, requestedSlug string, custom *customPresetDraft) (persona.PresetSelection, *persona.ResolvedPresetV2, error) {
	resolved, err := resolveWizardPresetSelection(personaFS, requestedSlug, custom)
	if err != nil {
		return persona.PresetSelection{}, nil, err
	}
	return persona.PresetSelection{V2: resolved}, resolved, nil
}

func schemaV2PresetDescription(name string) string {
	if name == "custom" {
		return "Create a schema-v2 presentation profile with a name and display label."
	}
	return "Validated schema-v2 presentation profile."
}

// readLine reads a single line from the scanner, trimming whitespace.
func readLine(scanner *bufio.Scanner) string {
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func printNoTUIPhaseModelReview(resolved map[string]config.PhaseModelSelection, assignments map[string]config.OpenCodeModelAssignment, claudeAssignments map[string]config.ClaudeModelAssignment) {
	fmt.Fprintln(noTUIStdout, "SDD phase models:")
	for _, phase := range sddruntime.DefaultContract().Phases {
		sel := resolved[phase]
		opencodeDisplay := sel.OpenCode
		effortDisplay := ""
		if assignment := assignments[phase]; assignment.ProviderID != "" && assignment.ModelID != "" {
			opencodeDisplay = assignment.ProviderID + "/" + assignment.ModelID
			if strings.TrimSpace(assignment.Effort) != "" {
				effortDisplay = ", effort=" + strings.TrimSpace(assignment.Effort)
			}
		}
		claudeDisplay := sel.Claude
		claudeEffortDisplay := ""
		if assignment, ok := claudeAssignments[phase]; ok {
			if model := strings.TrimSpace(assignment.Model); model != "" {
				claudeDisplay = model
			}
			if effort := strings.TrimSpace(assignment.Effort); effort != "" {
				claudeEffortDisplay = ", effort=" + effort
			}
		}
		fmt.Fprintf(noTUIStdout, "- %s: opencode=%s%s, claude=%s%s\n", phase, opencodeDisplay, effortDisplay, claudeDisplay, claudeEffortDisplay)
	}
}

func displayPromptDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return strings.TrimSpace(value)
}

func normalizeClaudeEffortForPrompt(input, fallback string) string {
	if input == "" {
		return strings.TrimSpace(fallback)
	}
	for _, allowed := range claudeEffortOptions() {
		if input == allowed {
			return input
		}
	}
	return strings.TrimSpace(fallback)
}

func printOpenCodeAssignmentOptions(options []config.OpenCodeModelAssignment) {
	options = providerOnlyOpenCodeAssignments(options)
	if len(options) == 0 {
		return
	}
	fmt.Fprintln(noTUIStdout, "Available OpenCode provider/model options:")
	fmt.Fprintln(noTUIStdout, "  0) legacy")
	for i, option := range options {
		fmt.Fprintf(noTUIStdout, "  %d) %s\n", i+1, openCodeAssignmentPromptValue(option, ""))
	}
}

func providerOnlyOpenCodeAssignments(options []config.OpenCodeModelAssignment) []config.OpenCodeModelAssignment {
	out := make([]config.OpenCodeModelAssignment, 0, len(options))
	for _, option := range options {
		if option.ProviderID == "" || option.ModelID == "" {
			continue
		}
		out = append(out, option)
	}
	return out
}

func openCodeAssignmentPromptValue(assignment config.OpenCodeModelAssignment, legacyAlias string) string {
	if assignment.ProviderID != "" && assignment.ModelID != "" {
		display := assignment.ProviderID + "/" + assignment.ModelID
		if strings.TrimSpace(assignment.Effort) != "" {
			display += " (effort=" + strings.TrimSpace(assignment.Effort) + ")"
		}
		return display
	}
	if strings.TrimSpace(legacyAlias) != "" {
		return "legacy=" + strings.TrimSpace(legacyAlias)
	}
	return "none"
}

func selectOpenCodeAssignmentForPrompt(input string, options []config.OpenCodeModelAssignment) (config.OpenCodeModelAssignment, bool) {
	if input == "" {
		return config.OpenCodeModelAssignment{}, false
	}
	if strings.EqualFold(strings.TrimSpace(input), "legacy") {
		return config.OpenCodeModelAssignment{}, true
	}
	options = providerOnlyOpenCodeAssignments(options)
	selected, err := strconv.Atoi(input)
	if err != nil || selected < 0 || selected > len(options) {
		return config.OpenCodeModelAssignment{}, false
	}
	if selected == 0 {
		return config.OpenCodeModelAssignment{}, true
	}
	return options[selected-1], true
}

func normalizePlatformValueForPrompt(input, fallback string, catalog []string) string {
	if input == "" {
		return fallback
	}
	for _, item := range catalog {
		if input == item {
			return input
		}
	}
	return fallback
}

// buildNoTUIStatuslineConfirm checks whether ~/.claude/statusline-command.sh
// already exists. If absent, it returns a closure that always returns true
// (confirm is never called on a fresh install). If present, it prompts the user
// once via the provided scanner and returns a constant-returning closure based
// on their answer. A non-ENOENT stat error (e.g. permission denied) is returned
// as an error so callers can handle it explicitly.
func buildNoTUIStatuslineConfirm(home string, scanner *bufio.Scanner) (func() bool, error) {
	scriptPath := filepath.Join(home, ".claude", "statusline-command.sh")
	_, err := os.Stat(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File absent — confirm will never be called; return true as a no-op.
			return func() bool { return true }, nil
		}
		return nil, err
	}
	fmt.Fprint(noTUIStdout, "~/.claude/statusline-command.sh already exists. Overwrite? [y/N]: ")
	ans := strings.ToLower(strings.TrimSpace(readLine(scanner)))
	overwrite := ans == "y" || ans == "yes"
	return func() bool { return overwrite }, nil
}

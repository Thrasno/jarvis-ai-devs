package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// Ensure OpenCodeAgent implements Agent at compile time.
var _ Agent = (*OpenCodeAgent)(nil)

// OpenCodeAgent implements Agent for the OpenCode AI coding assistant.
// Config dir: ~/.config/opencode/
// Settings file: ~/.config/opencode/opencode.json
// Instructions file: ~/.config/opencode/AGENTS.md
// Skills dir: ~/.config/opencode/skills/
type OpenCodeAgent struct {
	home        string
	templatesFS fs.FS
}

func newOpenCodeAgent(fsys fs.FS) *OpenCodeAgent {
	home, _ := os.UserHomeDir()
	return &OpenCodeAgent{home: home, templatesFS: fsys}
}

func (a *OpenCodeAgent) Name() string { return "opencode" }

func (a *OpenCodeAgent) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return runtimePlanFor(a.Name())
}

func (a *OpenCodeAgent) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return a.ObserveRuntimeWithConfig(nil)
}

func (a *OpenCodeAgent) ObserveRuntimeWithConfig(cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
	plan, err := a.RuntimePlan()
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	return observeRuntimeWithConfig(a.ConfigDir(), plan, cfg)
}

func (a *OpenCodeAgent) IsInstalled() bool {
	_, err := os.Stat(a.ConfigDir())
	return err == nil
}

func (a *OpenCodeAgent) ConfigDir() string {
	return filepath.Join(a.home, ".config", "opencode")
}

func (a *OpenCodeAgent) settingsPath() string {
	return filepath.Join(a.ConfigDir(), "opencode.json")
}

func (a *OpenCodeAgent) instructionsPath() string {
	return filepath.Join(a.ConfigDir(), "AGENTS.md")
}

func (a *OpenCodeAgent) skillsDir() string {
	return filepath.Join(a.ConfigDir(), "skills")
}

type opencodeConfigTemplateData struct {
	IncludeSchema       bool
	SchemaURL           string
	OrchestratorModel   string
	OrchestratorVariant string
	Agents              []opencodeGeneratedAgent
	TaskAllows          []string
}

type opencodeGeneratedAgent struct {
	Name        string
	Description string
	Mode        string
	Hidden      bool
	Model       string
	Variant     string
	Prompt      string
	Permission  string
	Last        bool
}

// MergeGeneratedConfig installs Jarvis-owned OpenCode defaults that are not MCP
// server entries: share-disabled behavior, global safety permissions, and the
// SDD/Judgment Day agent topology. The patch is rendered from the embedded
// template and deep-merged so user-owned keys and existing MCP servers remain.
func (a *OpenCodeAgent) MergeGeneratedConfig(cfg *config.AppConfig) error {
	existingBytes, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read opencode.json: %w", err)
	}
	includeSchema, err := shouldIncludeJSONSchema(existingBytes)
	if err != nil {
		return fmt.Errorf("inspect opencode.json schema: %w", err)
	}

	patchBytes, err := a.renderGeneratedConfigPatch(cfg, includeSchema)
	if err != nil {
		return err
	}
	merged, err := MergeJSON(existingBytes, patchBytes)
	if err != nil {
		return fmt.Errorf("merge opencode.json generated config: %w", err)
	}
	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

func (a *OpenCodeAgent) renderGeneratedConfigPatch(cfg *config.AppConfig, includeSchema bool) ([]byte, error) {
	assignments, err := sddruntime.ResolveOpenCodeProviderQualifiedAssignments(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenCode model assignments: %w", err)
	}
	variants, err := sddruntime.ResolveVariantsForPlatform(sddruntime.PlatformOpenCode, cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenCode model variants: %w", err)
	}

	templateBytes, err := fs.ReadFile(a.templatesFS, "embed/templates/opencode.json.tmpl")
	if err != nil {
		templateBytes, err = fs.ReadFile(jarvis.TemplatesFS, "embed/templates/opencode.json.tmpl")
		if err != nil {
			return nil, fmt.Errorf("read opencode generated config template: %w", err)
		}
	}

	agents := buildOpenCodeGeneratedAgents(assignments, variants)
	for i := range agents {
		agents[i].Last = i == len(agents)-1
	}
	data := opencodeConfigTemplateData{
		IncludeSchema:       includeSchema,
		SchemaURL:           "https://opencode.ai/config.json",
		OrchestratorModel:   modelForGeneratedAgent(assignments, "orchestrator"),
		OrchestratorVariant: variants["orchestrator"],
		Agents:              agents,
		TaskAllows:          append(openCodeSDDSubagents(), openCodeJudgmentDaySubagents()...),
	}

	tmpl, err := template.New("opencode.json.tmpl").Funcs(template.FuncMap{"json": jsonTemplateValue}).Parse(string(templateBytes))
	if err != nil {
		return nil, fmt.Errorf("parse opencode generated config template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("execute opencode generated config template: %w", err)
	}
	return out.Bytes(), nil
}

func jsonTemplateValue(value any) string {
	out, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(out)
}

func shouldIncludeJSONSchema(existing []byte) (bool, error) {
	if len(strings.TrimSpace(string(existing))) == 0 {
		return true, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(existing, &decoded); err != nil {
		return false, err
	}
	_, exists := decoded["$schema"]
	return !exists, nil
}

func buildOpenCodeGeneratedAgents(assignments, variants map[string]string) []opencodeGeneratedAgent {
	agents := []opencodeGeneratedAgent{
		{
			Name:        "sdd-explore",
			Description: "Explore SDD ideas and codebase context before committing to a change.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-explore"),
			Variant:     variants["sdd-explore"],
			Prompt:      jarvisSkillPrompt("sdd-explore"),
			Permission:  `{"task":"deny","edit":"deny","bash":"ask"}`,
		},
		{
			Name:        "sdd-propose",
			Description: "Create SDD change proposals with intent, scope, and approach.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-propose"),
			Variant:     variants["sdd-propose"],
			Prompt:      jarvisSkillPrompt("sdd-propose"),
			Permission:  `{"task":"deny","edit":"allow","bash":"ask"}`,
		},
		{
			Name:        "sdd-spec",
			Description: "Write SDD delta specifications with requirements and scenarios.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-spec"),
			Variant:     variants["sdd-spec"],
			Prompt:      jarvisSkillPrompt("sdd-spec"),
			Permission:  `{"task":"deny","edit":"allow","bash":"ask"}`,
		},
		{
			Name:        "sdd-design",
			Description: "Create technical designs and architecture approaches for SDD changes.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-design"),
			Variant:     variants["sdd-design"],
			Prompt:      jarvisSkillPrompt("sdd-design"),
			Permission:  `{"task":"deny","edit":"allow","bash":"ask"}`,
		},
		{
			Name:        "sdd-tasks",
			Description: "Break SDD changes into implementation tasks and reviewable PR slices.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-tasks"),
			Variant:     variants["sdd-tasks"],
			Prompt:      jarvisSkillPrompt("sdd-tasks"),
			Permission:  `{"task":"deny","edit":"allow","bash":"ask"}`,
		},
		{
			Name:        "sdd-apply",
			Description: "Implement assigned SDD tasks using the active testing contract.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-apply"),
			Variant:     variants["sdd-apply"],
			Prompt:      jarvisSkillPrompt("sdd-apply"),
			Permission:  `{"task":"deny","edit":"allow","bash":{"*":"ask","go test *":"allow"}}`,
		},
		{
			Name:        "sdd-verify",
			Description: "Verify implementation against SDD specs, design, tasks, and tests.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-verify"),
			Variant:     variants["sdd-verify"],
			Prompt:      jarvisSkillPrompt("sdd-verify"),
			Permission:  `{"task":"deny","edit":"deny","bash":{"*":"ask","go test *":"allow","go vet *":"allow"}}`,
		},
		{
			Name:        "sdd-archive",
			Description: "Archive completed SDD changes by syncing delta specs and closing artifacts.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-archive"),
			Variant:     variants["sdd-archive"],
			Prompt:      jarvisSkillPrompt("sdd-archive"),
			Permission:  `{"task":"deny","edit":"allow","bash":"ask"}`,
		},
		{
			Name:        "sdd-init",
			Description: "Initialize SDD context, testing capabilities, registry, and persistence.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "default"),
			Variant:     variants["default"],
			Prompt:      jarvisSkillPrompt("sdd-init"),
			Permission:  `{"task":"deny","edit":"allow","bash":{"*":"ask","go test *":"allow"}}`,
		},
		{
			Name:        "sdd-onboard",
			Description: "Walk users through the SDD workflow on the real codebase.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "default"),
			Variant:     variants["default"],
			Prompt:      jarvisSkillPrompt("sdd-onboard"),
			Permission:  `{"task":"deny","edit":"deny","bash":"ask"}`,
		},
		{
			Name:        "jd-judge-a",
			Description: "Run the first blind Judgment Day review pass.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "default"),
			Variant:     variants["default"],
			Prompt:      judgmentDayPrompt("jd-judge-a"),
			Permission:  `{"task":"deny","edit":"deny","bash":"ask"}`,
		},
		{
			Name:        "jd-judge-b",
			Description: "Run the second blind Judgment Day review pass.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "default"),
			Variant:     variants["default"],
			Prompt:      judgmentDayPrompt("jd-judge-b"),
			Permission:  `{"task":"deny","edit":"deny","bash":"ask"}`,
		},
		{
			Name:        "jd-fix-agent",
			Description: "Fix confirmed Judgment Day issues without expanding scope.",
			Mode:        "subagent",
			Hidden:      true,
			Model:       modelForGeneratedAgent(assignments, "sdd-apply"),
			Variant:     variants["sdd-apply"],
			Prompt:      judgmentDayPrompt("jd-fix-agent"),
			Permission:  `{"task":"deny","edit":"allow","bash":{"*":"ask","go test *":"allow"}}`,
		},
	}
	return agents
}

func openCodeSDDSubagents() []string {
	return []string{"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-init", "sdd-onboard"}
}

func openCodeJudgmentDaySubagents() []string {
	return []string{"jd-judge-a", "jd-judge-b", "jd-fix-agent"}
}

func modelForGeneratedAgent(assignments map[string]string, phase string) string {
	if model := strings.TrimSpace(assignments[phase]); model != "" {
		return model
	}
	return strings.TrimSpace(assignments["default"])
}

func jarvisSkillPrompt(skill string) string {
	return "Read and follow the Jarvis skill at `.jarvis/skills/" + skill + "/SKILL.md` before doing this role's work. Generated technical artifacts must be English and preserve Jarvis/Hive naming."
}

func judgmentDayPrompt(agent string) string {
	return "Read and follow the Jarvis Judgment Day workflow before acting as `" + agent + "`. Keep reviews blind where required, preserve scope, and write generated technical artifacts in English."
}

// MergeConfig adds MCP entries to ~/.config/opencode/opencode.json based on entry.Name.
// Supported entries: "hive" (local daemon), "context7" (remote URL).
// OpenCode format: command is an array of strings (local mode) or type+url (remote mode).
// Uses deep merge to preserve all existing config keys (agents, permissions, etc).
func (a *OpenCodeAgent) MergeConfig(entry MCPEntry) error {
	var patch map[string]any

	switch entry.Name {
	case "hive":
		// Build the hive MCP patch for OpenCode format
		// command is an array, env vars carry credentials
		hiveCfg := map[string]any{
			"command": []string{entry.DaemonPath},
			"type":    "local",
		}

		// Only add environment when credentials are complete.
		// Partial environment blocks can override ~/.jarvis/sync.json and break daemon precedence.
		if hasCompleteHiveCloudCreds(entry) {
			hiveCfg["environment"] = map[string]string{
				"HIVE_API_URL":      strings.TrimSpace(entry.APIURL),
				"HIVE_API_EMAIL":    strings.TrimSpace(entry.Email),
				"HIVE_API_PASSWORD": strings.TrimSpace(entry.Password),
			}
		}

		patch = map[string]any{
			"mcp": map[string]any{
				"hive": hiveCfg,
			},
		}
	case "context7":
		// Build the Context7 MCP patch for OpenCode format (remote mode)
		patch = map[string]any{
			"mcp": map[string]any{
				"context7": map[string]any{
					"type":    "remote",
					"url":     "https://mcp.context7.com/mcp",
					"enabled": true,
				},
			},
		}
	default:
		return fmt.Errorf("unknown MCP entry name: %s", entry.Name)
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal hive MCP patch: %w", err)
	}

	existingBytes, err := readFileOrEmpty(a.settingsPath())
	if err != nil {
		return fmt.Errorf("read opencode.json: %w", err)
	}

	merged, err := MergeJSON(existingBytes, patchBytes)
	if err != nil {
		return fmt.Errorf("merge opencode.json: %w", err)
	}

	return writeFileAtomic(a.settingsPath(), merged, 0644)
}

func hasCompleteHiveCloudCreds(entry MCPEntry) bool {
	return strings.TrimSpace(entry.APIURL) != "" &&
		strings.TrimSpace(entry.Email) != "" &&
		strings.TrimSpace(entry.Password) != ""
}

// WriteInstructions writes ~/.config/opencode/AGENTS.md with Layer1+Layer2 sentinel blocks.
//
// Decision logic:
//   - File absent or empty → render fresh via RenderAGENTSMd ("created")
//   - File exists with Jarvis sentinels → patch in-place via PatchFile ("updated")
//   - File exists without sentinels → render fresh via RenderAGENTSMd, replacing foreign content ("replaced")
//
// After determining the final content, the Hive protocol is injected via InjectProtocol.
// Any legacy gentle-ai protocol blocks are cleaned up first via CleanupOldProtocol.
func (a *OpenCodeAgent) WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error {
	path := a.instructionsPath()

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read AGENTS.md: %w", err)
	}

	var content string
	if os.IsNotExist(err) || len(existing) == 0 {
		// Create new file from scratch using the canonical template renderer.
		content, err = config.RenderAGENTSMd(a.templatesFS, layer1, layer2, "", skills)
		if err != nil {
			return fmt.Errorf("render AGENTS.md: %w", err)
		}
	} else {
		existingStr := string(existing)
		if err := ValidateSentinels(existingStr); err == nil {
			// Sentinels present — patch in-place (preserves user content outside blocks).
			content, err = PatchFile(existingStr, layer1, layer2)
			if err != nil {
				return fmt.Errorf("patch AGENTS.md sentinels: %w", err)
			}
		} else {
			// Sentinels missing — discard foreign content and render a clean Jarvis file.
			content, err = config.RenderAGENTSMd(a.templatesFS, layer1, layer2, "", skills)
			if err != nil {
				return fmt.Errorf("render AGENTS.md (replace): %w", err)
			}
		}
	}

	// Clean up legacy gentle-ai protocol blocks and inject Hive protocol
	content = CleanupOldProtocol(content)
	content = InjectProtocol(content, getHiveProtocol())

	return writeFileAtomic(path, []byte(content), 0644)
}

// InstallSkills installs selected skills from skillsFS to ~/.config/opencode/skills/.
// skillsFS must be a sub-FS rooted at the embed/skills directory.
// The _shared/ directory is always installed regardless of the selected list.
// Idempotent: existing files are overwritten silently.
func (a *OpenCodeAgent) InstallSkills(skillsFS fs.FS, selected []string) error {
	dir := a.skillsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	return installSkillsFromFS(dir, skillsFS, selected)
}

// InstallOrchestrator installs rendered sdd-orchestrator.md to ~/.config/opencode/.
// Idempotent: existing file is overwritten silently.
func (a *OpenCodeAgent) InstallOrchestrator(orchestratorContent []byte) error {
	destPath := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	return installOrchestrator(destPath, orchestratorContent)
}

// InstallPromptHook writes the Hive OpenCode TypeScript plugin to
// ~/.config/opencode/plugins/hive.ts. OpenCode auto-loads plugins from
// this directory — no opencode.json registration needed.
func (a *OpenCodeAgent) InstallPromptHook(hooksFS fs.FS) error {
	pluginDir := filepath.Join(a.ConfigDir(), "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}

	content, err := fs.ReadFile(hooksFS, "embed/hooks/opencode/hive.ts")
	if err != nil {
		return fmt.Errorf("read opencode plugin: %w", err)
	}

	dest := filepath.Join(pluginDir, "hive.ts")
	return writeFileAtomic(dest, content, 0644)
}

// InstallSessionHooks is a no-op for OpenCode — session memory is handled by the Hive TypeScript plugin.
func (a *OpenCodeAgent) InstallSessionHooks(_ fs.FS) error { return nil }

// SupportsOutputStyles returns false for OpenCodeAgent since OpenCode
// does not have native output-style support.
func (a *OpenCodeAgent) SupportsOutputStyles() bool {
	return false
}

// WriteOutputStyle is a no-op for OpenCodeAgent since OpenCode doesn't support
// output-styles. Returns nil to allow graceful handling in mixed agent environments.
func (a *OpenCodeAgent) WriteOutputStyle(preset *persona.Preset) error {
	return nil
}

// ClearOutputStyle is a no-op for OpenCodeAgent since OpenCode has no
// output-style artifact contract to clean.
func (a *OpenCodeAgent) ClearOutputStyle(name string) error {
	return nil
}

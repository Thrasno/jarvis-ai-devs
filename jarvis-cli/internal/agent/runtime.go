package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// loadDesiredPhaseModels reads the per-phase model assignments from
// ~/.jarvis/state.yaml, which owns them.
//
// This is the observation path: `jarvis doctor`, `jarvis verify` and a dry-run
// reconcile all reach it, and none of them may change the machine they are
// looking at. It therefore does not migrate. It cannot simply read the manifest
// either, because a machine upgrading into this version still has these
// assignments in config.yaml and no manifest at all, and an empty manifest would
// verify the runtime against the contract defaults instead of the models the
// user chose. state.LoadWithoutMigrating answers that question without writing:
// it reads the manifest when there is one and derives the same values from
// config.yaml in memory when there is not.
var loadDesiredPhaseModels = func() (state.PhaseModels, error) {
	manifest, err := state.LoadWithoutMigrating()
	if errors.Is(err, state.ErrNotFound) {
		return state.New().NormalizedPhaseModels(), nil
	}
	if err != nil {
		return state.PhaseModels{}, err
	}
	return manifest.NormalizedPhaseModels(), nil
}

func runtimePlanFor(name string) (sddruntime.RuntimePlan, error) {
	return sddruntime.Build(name)
}

type runtimeObserverWithConfig interface {
	ObserveRuntimeWithConfig(*state.PhaseModels) (sddruntime.ObservedRuntime, error)
}

// ObserveRuntimeWithConfig collects adapter-normalized runtime state using
// models as the pending expected model-assignment source when the adapter
// supports it. A nil models means "no pending assignments to apply", exactly as
// a nil config did.
func ObserveRuntimeWithConfig(a Agent, models *state.PhaseModels) (sddruntime.ObservedRuntime, error) {
	if observer, ok := a.(runtimeObserverWithConfig); ok {
		return observer.ObserveRuntimeWithConfig(models)
	}

	observed, err := a.ObserveRuntime()
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	if models == nil {
		return observed, nil
	}

	resolvedAssignments, err := resolvedAssignmentsForAgentWithConfig(a.Name(), models)
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	observed.ResolvedModelAssignments = resolvedAssignments
	if len(observed.ModelAssignments) == 0 {
		observed.ModelAssignments = resolvedAssignments
	}
	return observed, nil
}

func observeRuntime(configDir string, plan sddruntime.RuntimePlan) (sddruntime.ObservedRuntime, error) {
	return observeRuntimeWithConfig(configDir, plan, nil)
}

func observeRuntimeWithConfig(configDir string, plan sddruntime.RuntimePlan, models *state.PhaseModels) (sddruntime.ObservedRuntime, error) {
	artifacts := map[string]sddruntime.ObservedArtifact{}
	presentIDs := make([]string, 0, len(plan.Contract.ManagedArtifacts))

	for _, managed := range plan.Contract.ManagedArtifacts {
		artifact, err := observeArtifactForAgent(configDir, plan.Paths, managed, plan.Agent)
		if err != nil {
			return sddruntime.ObservedRuntime{}, err
		}
		artifacts[managed.ID] = artifact
		if artifact.Exists {
			presentIDs = append(presentIDs, managed.ID)
		}
	}

	requiredCount := 0
	for _, managed := range plan.Contract.ManagedArtifacts {
		if managed.Required {
			requiredCount++
		}
	}
	requiredPresent := 0
	for _, managed := range plan.Contract.ManagedArtifacts {
		if !managed.Required {
			continue
		}
		if artifacts[managed.ID].Exists {
			requiredPresent++
		}
	}
	manifestPresent := requiredPresent == requiredCount
	manifestVersion := ""
	if manifestPresent {
		manifestVersion = plan.Contract.Version
	}

	resolvedAssignments, err := resolvedAssignmentsForAgentWithConfig(plan.Agent, models)
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	modelAssignments := resolvedAssignments
	observedAssignments, err := observeOrchestratorModelAssignments(configDir, plan.Paths)
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	if len(observedAssignments) > 0 {
		modelAssignments = observedAssignments
	}

	promptSources, err := sddruntime.DefaultPromptContract(plan.Agent, "orchestrator").OrderedRequiredSources()
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	promptSourceIDs := make([]string, 0, len(promptSources))
	for _, source := range promptSources {
		promptSourceIDs = append(promptSourceIDs, source.ID)
	}

	storeContract, err := sddruntime.ResolveRuntimeStoreContract(sddruntime.StoreModeHive)
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}

	// Populate OpenCode-specific observed config for the opencode agent.
	// Claude leaves this at zero value (ParseSucceeded==false), which is safe.
	var openCodeCfg sddruntime.ObservedOpenCodeConfig
	var claudeSDDHiveTools map[string][]string
	if plan.Agent == "opencode" {
		settingsPath := filepath.Join(configDir, filepath.Base(plan.Paths.Settings))
		openCodeCfg = parseOpenCodeConfig(settingsPath)
		// PluginHiveExists reflects whether plugins/hive.ts was observed present.
		openCodeCfg.PluginHiveExists = artifacts["prompt_hook"].Exists
	} else if plan.Agent == "claude" {
		claudeSDDHiveTools = observeClaudeSDDSubagentHiveTools(filepath.Join(configDir, "agents"))
	}

	return sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            manifestPresent,
			Corrupted:          false,
			ContractVersion:    manifestVersion,
			ManagedArtifactIDs: presentIDs,
		},
		RegistryPath:               plan.Contract.RegistryPath,
		PromptSourceIDs:            promptSourceIDs,
		StoreMode:                  string(storeContract.Mode),
		StoreReadFrom:              storeContract.ReadFrom,
		StoreWriteTo:               storeContract.WriteTo,
		ArtifactTopics:             []string{"sdd/runtime/verify"},
		GeneralMemoryTopics:        []string{"runtime/notes"},
		ModelAssignments:           modelAssignments,
		ResolvedModelAssignments:   resolvedAssignments,
		Artifacts:                  artifacts,
		OpenCode:                   openCodeCfg,
		ClaudeSDDSubagentHiveTools: claudeSDDHiveTools,
	}, nil
}

func observeClaudeSDDSubagentHiveTools(agentsDir string) map[string][]string {
	observed := make(map[string][]string)
	for _, def := range SDDPhaseAgentDefinitions() {
		content, err := os.ReadFile(filepath.Join(agentsDir, def.Name+".md"))
		if err != nil {
			continue
		}
		observed[def.Name] = parseClaudeAgentToolsFrontmatter(string(content))
	}
	return observed
}

func parseClaudeAgentToolsFrontmatter(content string) []string {
	inFrontmatter := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			return nil
		}
		if !inFrontmatter || !strings.HasPrefix(line, "tools:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "tools:"))
		if value == "" {
			return nil
		}
		parts := strings.Split(value, ",")
		tools := make([]string, 0, len(parts))
		for _, part := range parts {
			tool := strings.TrimSpace(part)
			if tool != "" {
				tools = append(tools, tool)
			}
		}
		return tools
	}
	return nil
}

func observeOrchestratorModelAssignments(configDir string, paths sddruntime.RuntimePaths) (map[string]string, error) {
	orchestratorPath := filepath.Join(configDir, filepath.Base(paths.Orchestrator))
	content, err := os.ReadFile(orchestratorPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read orchestrator artifact: %w", err)
	}
	return parseOrchestratorAssignments(string(content)), nil
}

func parseOrchestratorAssignments(content string) map[string]string {
	assignments := map[string]string{}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "---") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		phase := strings.ToLower(strings.TrimSpace(parts[1]))
		model := strings.TrimSpace(parts[2])
		if phase == "" || model == "" || phase == "phase" || model == "default model" {
			continue
		}
		assignments[phase] = model
	}
	return assignments
}

func resolvedAssignmentsForAgent(agent string) (map[string]string, error) {
	return resolvedAssignmentsForAgentWithConfig(agent, nil)
}

// resolvedAssignmentsForAgentWithConfig resolves an agent's phase assignments.
// A nil models keeps the historical fallback of loading the persisted desired
// state, so callers with no pending assignments still verify against what is on
// disk.
func resolvedAssignmentsForAgentWithConfig(agent string, models *state.PhaseModels) (map[string]string, error) {
	resolved := state.PhaseModels{}
	if models == nil {
		loaded, err := loadDesiredPhaseModels()
		if err != nil {
			return nil, fmt.Errorf("load the desired-state manifest for runtime verification: %w", err)
		}
		resolved = loaded
	} else {
		resolved = *models
	}

	platform, err := platformForAgent(agent)
	if err != nil {
		return nil, err
	}

	return sddruntime.ResolveAssignmentsForPlatform(platform, resolved)
}

func platformForAgent(agent string) (sddruntime.Platform, error) {
	switch agent {
	case "opencode":
		return sddruntime.PlatformOpenCode, nil
	case "claude":
		return sddruntime.PlatformClaude, nil
	default:
		return "", fmt.Errorf("unsupported agent %q", agent)
	}
}

func observeArtifact(configDir string, paths sddruntime.RuntimePaths, artifact sddruntime.ManagedArtifact) (sddruntime.ObservedArtifact, error) {
	return observeArtifactForAgent(configDir, paths, artifact, "")
}

// observeArtifactForAgent is the agent-aware variant of observeArtifact.
// The agent parameter controls path resolution for artifacts whose filesystem
// location differs per agent (e.g. prompt_hook: opencode uses plugins/hive.ts,
// claude uses hive-hooks/).
func observeArtifactForAgent(configDir string, paths sddruntime.RuntimePaths, artifact sddruntime.ManagedArtifact, agent string) (sddruntime.ObservedArtifact, error) {
	switch artifact.ID {
	case "instructions":
		instructionsPath := filepath.Join(configDir, filepath.Base(paths.Instructions))
		content, err := os.ReadFile(instructionsPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("read instructions artifact: %w", err)
		}
		return sddruntime.ObservedArtifact{Exists: true, MarkersValid: ValidateSentinels(string(content)) == nil}, nil
	case "orchestrator":
		orchestratorPath := filepath.Join(configDir, filepath.Base(paths.Orchestrator))
		_, err := os.Stat(orchestratorPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("stat orchestrator artifact: %w", err)
		}
		return sddruntime.ObservedArtifact{Exists: true}, nil
	case "skills":
		skillsPath := filepath.Join(configDir, filepath.Base(filepath.Clean(artifact.RelativePath)))
		stat, err := os.Stat(skillsPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("stat skills artifact: %w", err)
		}
		return sddruntime.ObservedArtifact{Exists: stat.IsDir()}, nil
	case "settings", "output_style_settings":
		settingsPath := filepath.Join(configDir, filepath.Base(paths.Settings))
		_, err := os.Stat(settingsPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("stat settings artifact: %w", err)
		}
		return sddruntime.ObservedArtifact{Exists: true}, nil
	case "output_style":
		stylesPath := filepath.Join(configDir, filepath.Base(filepath.Clean(artifact.RelativePath)))
		stat, err := os.Stat(stylesPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("stat output_style artifact: %w", err)
		}
		return sddruntime.ObservedArtifact{Exists: stat.IsDir()}, nil
	case "prompt_hook":
		return observePromptHookArtifact(configDir, artifact, agent)
	default:
		return sddruntime.ObservedArtifact{}, fmt.Errorf("unsupported managed artifact id %q", artifact.ID)
	}
}

// observePromptHookArtifact resolves the prompt hook path agent-aware:
//   - opencode: checks plugins/hive.ts as a regular non-empty file
//   - claude (and default): checks hive-hooks/ as a directory
func observePromptHookArtifact(configDir string, artifact sddruntime.ManagedArtifact, agent string) (sddruntime.ObservedArtifact, error) {
	if agent == "opencode" {
		// OpenCode installs the hook as plugins/hive.ts (a TypeScript file).
		pluginPath := filepath.Join(configDir, "plugins", "hive.ts")
		stat, err := os.Stat(pluginPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("stat prompt_hook artifact: %w", err)
		}
		exists := !stat.IsDir() && stat.Size() > 0
		return sddruntime.ObservedArtifact{Exists: exists}, nil
	}

	// Claude (and unknown agents): hive-hooks/ directory.
	hooksPath := filepath.Join(configDir, filepath.Base(filepath.Clean(artifact.RelativePath)))
	stat, err := os.Stat(hooksPath)
	if os.IsNotExist(err) {
		return sddruntime.ObservedArtifact{Exists: false}, nil
	}
	if err != nil {
		return sddruntime.ObservedArtifact{}, fmt.Errorf("stat prompt_hook artifact: %w", err)
	}
	return sddruntime.ObservedArtifact{Exists: stat.IsDir()}, nil
}

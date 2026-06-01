package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

var loadAppConfig = config.Load

func runtimePlanFor(name string) (sddruntime.RuntimePlan, error) {
	return sddruntime.Build(name)
}

type runtimeObserverWithConfig interface {
	ObserveRuntimeWithConfig(*config.AppConfig) (sddruntime.ObservedRuntime, error)
}

// ObserveRuntimeWithConfig collects adapter-normalized runtime state using cfg
// as the pending expected model-assignment source when the adapter supports it.
func ObserveRuntimeWithConfig(a Agent, cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
	if observer, ok := a.(runtimeObserverWithConfig); ok {
		return observer.ObserveRuntimeWithConfig(cfg)
	}

	observed, err := a.ObserveRuntime()
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	if cfg == nil {
		return observed, nil
	}

	resolvedAssignments, err := resolvedAssignmentsForAgentWithConfig(a.Name(), cfg)
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

func observeRuntimeWithConfig(configDir string, plan sddruntime.RuntimePlan, cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
	artifacts := map[string]sddruntime.ObservedArtifact{}
	presentIDs := make([]string, 0, len(plan.Contract.ManagedArtifacts))

	for _, managed := range plan.Contract.ManagedArtifacts {
		artifact, err := observeArtifact(configDir, plan.Paths, managed)
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

	resolvedAssignments, err := resolvedAssignmentsForAgentWithConfig(plan.Agent, cfg)
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

	return sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            manifestPresent,
			Corrupted:          false,
			ContractVersion:    manifestVersion,
			ManagedArtifactIDs: presentIDs,
		},
		RegistryPath:             plan.Contract.RegistryPath,
		PromptSourceIDs:          promptSourceIDs,
		StoreMode:                string(storeContract.Mode),
		StoreReadFrom:            storeContract.ReadFrom,
		StoreWriteTo:             storeContract.WriteTo,
		ArtifactTopics:           []string{"sdd/runtime/verify"},
		GeneralMemoryTopics:      []string{"runtime/notes"},
		ModelAssignments:         modelAssignments,
		ResolvedModelAssignments: resolvedAssignments,
		Artifacts:                artifacts,
	}, nil
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

func resolvedAssignmentsForAgentWithConfig(agent string, cfg *config.AppConfig) (map[string]string, error) {
	var err error
	if cfg == nil {
		cfg, err = loadAppConfig()
		if err != nil {
			return nil, fmt.Errorf("load config for runtime verification: %w", err)
		}
	}

	platform, err := platformForAgent(agent)
	if err != nil {
		return nil, err
	}

	return sddruntime.ResolveAssignmentsForPlatform(platform, cfg)
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
		hooksPath := filepath.Join(configDir, filepath.Base(filepath.Clean(artifact.RelativePath)))
		stat, err := os.Stat(hooksPath)
		if os.IsNotExist(err) {
			return sddruntime.ObservedArtifact{Exists: false}, nil
		}
		if err != nil {
			return sddruntime.ObservedArtifact{}, fmt.Errorf("stat prompt_hook artifact: %w", err)
		}
		return sddruntime.ObservedArtifact{Exists: stat.IsDir()}, nil
	default:
		return sddruntime.ObservedArtifact{}, fmt.Errorf("unsupported managed artifact id %q", artifact.ID)
	}
}

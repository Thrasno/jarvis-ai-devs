package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

func runtimePlanFor(name string) (sddruntime.RuntimePlan, error) {
	return sddruntime.Build(name)
}

func observeRuntime(configDir string, plan sddruntime.RuntimePlan) (sddruntime.ObservedRuntime, error) {
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

	manifestPresent := len(presentIDs) == len(plan.Contract.ManagedArtifacts)
	manifestVersion := ""
	if manifestPresent {
		manifestVersion = plan.Contract.Version
	}

	return sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            manifestPresent,
			Corrupted:          false,
			ContractVersion:    manifestVersion,
			ManagedArtifactIDs: presentIDs,
		},
		RegistryPath:     plan.Contract.RegistryPath,
		ModelAssignments: cloneModelAssignments(plan.Contract.ModelAssignments),
		Artifacts:        artifacts,
	}, nil
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
	default:
		return sddruntime.ObservedArtifact{}, fmt.Errorf("unsupported managed artifact id %q", artifact.ID)
	}
}

func cloneModelAssignments(src map[string]string) map[string]string {
	cloned := make(map[string]string, len(src))
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}

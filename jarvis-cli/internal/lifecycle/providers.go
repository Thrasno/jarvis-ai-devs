package lifecycle

import (
	"os"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type ProviderAdapter interface {
	Name() string
	Observe() (ObservedProviderState, error)
	Apply([]DoctorStep) error
	BackupTargets([]DoctorStep) ([]BackupTarget, error)
	Restore(BackupManifest) (int, error)
}

type fakeProviderAdapter struct {
	name               string
	observed           ObservedProviderState
	observeErr         error
	applyErr           error
	restoreErr         error
	applyCalls         int
	applyStages        []string
	verifyCalls        int
	applyNonOwnedCount int
	restoreWrites      int
	appliedAssets      []string
	backupTargetPaths  []string
}

func (f *fakeProviderAdapter) Name() string {
	return f.name
}

func (f *fakeProviderAdapter) Observe() (ObservedProviderState, error) {
	f.verifyCalls++
	if f.observeErr != nil {
		return ObservedProviderState{}, f.observeErr
	}
	return f.observed, nil
}

func (f *fakeProviderAdapter) Apply(steps []DoctorStep) error {
	f.applyCalls++
	if f.applyErr != nil {
		return f.applyErr
	}
	for _, step := range steps {
		f.appliedAssets = append(f.appliedAssets, step.AssetID)
		if step.SafetyClass == "non-owned" {
			f.applyNonOwnedCount++
		}
	}
	if f.observed.Artifacts == nil {
		f.observed.Artifacts = map[string]sddruntime.ObservedArtifact{}
	}
	f.observed.Artifacts["instructions"] = sddruntime.ObservedArtifact{Exists: true, MarkersValid: true}
	f.observed.Artifacts["orchestrator"] = sddruntime.ObservedArtifact{Exists: true}
	f.observed.Artifacts["skills"] = sddruntime.ObservedArtifact{Exists: true}
	f.observed.NonOwnedChanges = nil
	f.observed.UnknownChanges = nil
	// For the opencode fake adapter, populate OpenCode so that opencode-specific
	// verifier checks do not fire spurious failures in lifecycle engine tests.
	if f.name == "opencode" && !f.observed.OpenCode.ParseSucceeded {
		f.observed.OpenCode = fakeCompliantOpenCodeConfig()
	}
	return nil
}

// fakeCompliantOpenCodeConfig returns a fully passing ObservedOpenCodeConfig
// for use in fake lifecycle adapter tests. It mirrors what a properly
// installed opencode config would produce.
func fakeCompliantOpenCodeConfig() sddruntime.ObservedOpenCodeConfig {
	subagents := []string{
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
		"sdd-init", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent",
		"review-risk", "review-readability", "review-reliability", "review-resilience",
	}
	return sddruntime.ObservedOpenCodeConfig{
		ParseSucceeded:     true,
		ShareMode:          "disabled",
		DefaultAgent:       "sdd-orchestrator",
		OrchestratorMode:   "primary",
		OrchestratorModel:  "legacy=opus",
		OrchestratorPrompt: "{file:./sdd-orchestrator.md}",
		AgentNames:         append([]string{"sdd-orchestrator"}, subagents...),
		HiddenSubagents:    subagents,
		TaskAllows:         subagents,
		TaskWildcardDeny:   true,
		BashWildcardAllow:  true,
		ReadSecretDenies:   true,
		MCPHivePresent:     true,
		MCPContext7Present: true,
		PluginHiveExists:   true,
	}
}

func (f *fakeProviderAdapter) BackupTargets(steps []DoctorStep) ([]BackupTarget, error) {
	targets := make([]BackupTarget, 0, len(steps))
	for _, step := range steps {
		tmp, err := os.CreateTemp("", "jarvis-lifecycle-backup-*")
		if err != nil {
			return nil, err
		}
		if err := tmp.Close(); err != nil {
			return nil, err
		}
		targets = append(targets, BackupTarget{Path: tmp.Name()})
		f.backupTargetPaths = append(f.backupTargetPaths, step.AssetID)
	}
	return targets, nil
}

func (f *fakeProviderAdapter) Restore(manifest BackupManifest) (int, error) {
	if f.restoreErr != nil {
		return 0, f.restoreErr
	}
	f.restoreWrites += len(manifest.Entries)
	return len(manifest.Entries), nil
}

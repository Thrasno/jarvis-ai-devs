package sddruntime

import (
	"strings"
	"testing"
)

func TestVerify_PassReportForCompliantRuntime(t *testing.T) {
	plan, err := Build("claude")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	observed := ObservedRuntime{
		Manifest: RuntimeManifestState{
			Present:            true,
			ContractVersion:    plan.Contract.Version,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath:        plan.Contract.RegistryPath,
		PromptSourceIDs:     []string{"layer1.behavior", "layer2.persona", "skill.sdd-orchestrator", "registry.skill-index", "protocol.hive"},
		StoreMode:           "hybrid",
		StoreReadFrom:       []string{"hive", "openspec"},
		StoreWriteTo:        []string{"hive", "openspec"},
		ArtifactTopics:      []string{"sdd/jarvis-agent-parity-vs-gentle/spec"},
		GeneralMemoryTopics: []string{"runtime/notes"},
		ModelAssignments:    cloneModelAssignments(plan.Contract.ModelAssignments),
		Artifacts: map[string]ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}

	report := Verify("claude", observed)
	if report.Status != StatusPass {
		t.Fatalf("expected pass status, got %q", report.Status)
	}
	if report.ContractVersion != plan.Contract.Version {
		t.Fatalf("contract version mismatch in report: got %q want %q", report.ContractVersion, plan.Contract.Version)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected checks in report")
	}
}

func TestVerify_FailsWhenManagedArtifactMissing(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.Artifacts["orchestrator"] = ObservedArtifact{Exists: false}

	report := Verify("opencode", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected fail status, got %q", report.Status)
	}
	check := findCheckByKey(report.Checks, "artifact.orchestrator.present")
	if check == nil {
		t.Fatal("expected missing orchestrator check")
	}
	if check.DriftClass != DriftOwned {
		t.Fatalf("expected owned drift for missing managed artifact, got %q", check.DriftClass)
	}
}

func TestVerify_FailsOnContradictoryInvariantMismatch(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.RegistryPath = ".jarvis/other-registry.md"

	report := Verify("claude", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected fail status, got %q", report.Status)
	}
	check := findCheckByKey(report.Checks, "invariant.registry_path")
	if check == nil {
		t.Fatal("expected invariant.registry_path check")
	}
	if check.Expected != ".jarvis/skill-registry.md" {
		t.Fatalf("unexpected expected value: %q", check.Expected)
	}
	if check.Observed != ".jarvis/other-registry.md" {
		t.Fatalf("unexpected observed value: %q", check.Observed)
	}
}

func TestVerify_ParityInvariants_PromptStoreRegistryAndMemoryBoundaries(t *testing.T) {
	tests := []struct {
		name            string
		mutate          func(*ObservedRuntime)
		wantStatus      IntegrityStatus
		wantFailedCheck string
	}{
		{
			name:       "passes when parity invariants are canonical",
			mutate:     func(*ObservedRuntime) {},
			wantStatus: StatusPass,
		},
		{
			name: "fails when prompt source order drifts",
			mutate: func(observed *ObservedRuntime) {
				observed.PromptSourceIDs = []string{"layer2.persona", "layer1.behavior", "skill.sdd-orchestrator", "registry.skill-index", "protocol.hive"}
			},
			wantStatus:      StatusFail,
			wantFailedCheck: "invariant.prompt.required_sources_order",
		},
		{
			name: "fails when store mode is unsupported",
			mutate: func(observed *ObservedRuntime) {
				observed.StoreMode = "sqlite"
			},
			wantStatus:      StatusFail,
			wantFailedCheck: "invariant.store.mode",
		},
		{
			name: "fails when registry path uses legacy alias",
			mutate: func(observed *ObservedRuntime) {
				observed.RegistryPath = ".atl/skill-registry.md"
			},
			wantStatus:      StatusFail,
			wantFailedCheck: "invariant.registry_path",
		},
		{
			name: "fails when general-memory topics leak into reserved sdd namespace",
			mutate: func(observed *ObservedRuntime) {
				observed.GeneralMemoryTopics = append(observed.GeneralMemoryTopics, "sdd/jarvis-agent-parity-vs-gentle/spec")
			},
			wantStatus:      StatusFail,
			wantFailedCheck: "invariant.memory.general_topics_boundary",
		},
		{
			name: "fails when artifact topic escapes metadata boundary contract",
			mutate: func(observed *ObservedRuntime) {
				observed.ArtifactTopics = []string{"sdd/jarvis-agent-parity-vs-gentle/spec/v2"}
			},
			wantStatus:      StatusFail,
			wantFailedCheck: "invariant.memory.artifact_topics_boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantObservedRuntime(t)
			tt.mutate(&observed)

			report := Verify("opencode", observed)
			if report.Status != tt.wantStatus {
				t.Fatalf("expected status %q, got %q", tt.wantStatus, report.Status)
			}
			if tt.wantFailedCheck == "" {
				return
			}

			check := findCheckByKey(report.Checks, tt.wantFailedCheck)
			if check == nil {
				t.Fatalf("expected check %q", tt.wantFailedCheck)
			}
			if check.Status != StatusFail {
				t.Fatalf("expected check %q status fail, got %q", tt.wantFailedCheck, check.Status)
			}
		})
	}
}

func TestVerify_UsesResolvedConfiguredAssignmentsWhenPresent(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.ResolvedModelAssignments = map[string]string{
		"sdd-apply": "openai/gpt-5.1-codex-max",
	}
	observed.ModelAssignments["sdd-apply"] = "openai/gpt-5.1-codex-max"

	report := Verify("opencode", observed)
	if report.Status != StatusPass {
		t.Fatalf("expected status pass, got %q", report.Status)
	}

	check := findCheckByKey(report.Checks, "invariant.model.sdd-apply")
	if check == nil {
		t.Fatal("expected invariant.model.sdd-apply check")
	}
	if check.Expected != "openai/gpt-5.1-codex-max" || check.Observed != "openai/gpt-5.1-codex-max" {
		t.Fatalf("unexpected check expected/observed: %+v", *check)
	}
}

func TestVerify_FailsWhenObservedDoesNotMatchResolvedConfiguredAssignments(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.ResolvedModelAssignments = map[string]string{
		"sdd-apply": "openai/gpt-5.1-codex-max",
	}
	observed.ModelAssignments["sdd-apply"] = "sonnet"

	report := Verify("opencode", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected status fail, got %q", report.Status)
	}

	check := findCheckByKey(report.Checks, "invariant.model.sdd-apply")
	if check == nil {
		t.Fatal("expected invariant.model.sdd-apply check")
	}
	if check.Expected != "openai/gpt-5.1-codex-max" || check.Observed != "sonnet" {
		t.Fatalf("unexpected check expected/observed: %+v", *check)
	}
}

func TestVerify_UsesPlatformDefaultsWhenResolvedAssignmentsAreAbsent(t *testing.T) {
	tests := []struct {
		name           string
		observedApply  string
		expectedStatus IntegrityStatus
	}{
		{name: "passes when observed matches platform defaults with no resolved phase", observedApply: "sonnet", expectedStatus: StatusPass},
		{name: "fails on observed drift with no resolved phase", observedApply: "haiku", expectedStatus: StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantObservedRuntime(t)
			observed.ResolvedModelAssignments = map[string]string{
				"default":      "sonnet",
				"orchestrator": "opus",
			}
			observed.ModelAssignments["sdd-apply"] = tt.observedApply

			report := Verify("claude", observed)
			if report.Status != tt.expectedStatus {
				t.Fatalf("expected status %q, got %q", tt.expectedStatus, report.Status)
			}

			check := findCheckByKey(report.Checks, "invariant.model.sdd-apply")
			if check == nil {
				t.Fatal("expected invariant.model.sdd-apply check")
			}
			if check.Expected != "sonnet" {
				t.Fatalf("expected value = %q, want %q", check.Expected, "sonnet")
			}
			if check.Observed != tt.observedApply {
				t.Fatalf("observed value = %q, want %q", check.Observed, tt.observedApply)
			}
		})
	}
}

func TestVerify_FailsDeterministicallyForUnsupportedAgent(t *testing.T) {
	observed := compliantObservedRuntime(t)

	report := Verify("gemini", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected fail status, got %q", report.Status)
	}
	check := findCheckByKey(report.Checks, "invariant.model.platform")
	if check == nil {
		t.Fatal("expected invariant.model.platform check")
	}
	if check.DriftClass != DriftOwned {
		t.Fatalf("expected owned drift class, got %q", check.DriftClass)
	}
}

func TestVerify_NonOwnedDriftDoesNotFailVerification(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.NonOwnedChanges = []string{"user customization outside managed block"}

	report := Verify("claude", observed)
	if report.Status == StatusFail {
		t.Fatalf("non-owned drift must not fail verification, got %q", report.Status)
	}
	check := findCheckByKey(report.Checks, "drift.non_owned")
	if check == nil {
		t.Fatal("expected non-owned drift check")
	}
	if check.DriftClass != DriftNonOwned {
		t.Fatalf("expected non-owned drift class, got %q", check.DriftClass)
	}
}

func TestVerify_FailsForMissingManifestWithRemediation(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.Manifest = RuntimeManifestState{}

	report := Verify("opencode", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected fail status, got %q", report.Status)
	}
	check := findCheckByKey(report.Checks, "manifest.present")
	if check == nil {
		t.Fatal("expected manifest.present check")
	}
	if !strings.Contains(check.Message, "rerun setup/repair") {
		t.Fatalf("expected remediation guidance in message, got %q", check.Message)
	}
}

func TestVerify_FailsForCorruptedManifestWithRemediation(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.Manifest.Corrupted = true

	report := Verify("opencode", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected fail status, got %q", report.Status)
	}
	check := findCheckByKey(report.Checks, "manifest.integrity")
	if check == nil {
		t.Fatal("expected manifest.integrity check")
	}
	if !strings.Contains(check.Message, "rerun setup/repair") {
		t.Fatalf("expected remediation guidance in message, got %q", check.Message)
	}
}

func TestVerify_RegistryQualityProblemsWarnWithoutFailingRuntime(t *testing.T) {
	tests := []struct {
		name    string
		quality ObservedRegistryQuality
		wantKey string
		wantMsg string
	}{
		{
			name:    "missing canonical registry",
			quality: ObservedRegistryQuality{Checked: true, Path: ".jarvis/skill-registry.md", Exists: false},
			wantKey: "registry.quality.missing",
			wantMsg: "run jarvis skill-registry refresh",
		},
		{
			name:    "stale canonical registry",
			quality: ObservedRegistryQuality{Checked: true, Path: ".jarvis/skill-registry.md", Exists: true, Stale: true},
			wantKey: "registry.quality.stale",
			wantMsg: "refresh the project skill registry",
		},
		{
			name:    "registry warning section",
			quality: ObservedRegistryQuality{Checked: true, Path: ".jarvis/skill-registry.md", Exists: true, HasWarnings: true},
			wantKey: "registry.quality.warnings",
			wantMsg: "inspect registry warnings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantObservedRuntime(t)
			observed.RegistryQuality = tt.quality

			report := Verify("claude", observed)
			if report.Status != StatusWarn {
				t.Fatalf("registry quality problem should warn, got status %q", report.Status)
			}
			check := findCheckByKey(report.Checks, tt.wantKey)
			if check == nil {
				t.Fatalf("expected %s check", tt.wantKey)
			}
			if check.Status != StatusWarn || check.DriftClass != DriftUnknown {
				t.Fatalf("unexpected check severity/class: %+v", *check)
			}
			if !strings.Contains(check.Message, tt.wantMsg) {
				t.Fatalf("message %q does not contain %q", check.Message, tt.wantMsg)
			}
		})
	}
}

func TestVerify_LegacyLayoutOutsideOwnershipContractFailsFast(t *testing.T) {
	tests := []struct {
		name     string
		observed ObservedRuntime
	}{
		{
			name: "unmanaged external state without trusted manifest is unsupported",
			observed: func() ObservedRuntime {
				observed := compliantObservedRuntime(t)
				observed.Manifest = RuntimeManifestState{}
				observed.NonOwnedChanges = []string{"legacy instructions at unmanaged path", "external state outside jarvis ownership"}
				return observed
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Verify("opencode", tt.observed)
			if report.Status != StatusFail {
				t.Fatalf("expected fail-fast status for unsupported legacy layout, got %q", report.Status)
			}

			manifestCheck := findCheckByKey(report.Checks, "manifest.present")
			if manifestCheck == nil {
				t.Fatal("expected manifest.present check for unsupported legacy layout")
			}
			if manifestCheck.DriftClass != DriftOwned {
				t.Fatalf("expected owned drift classification, got %q", manifestCheck.DriftClass)
			}
			if !strings.Contains(manifestCheck.Message, "rerun setup/repair") {
				t.Fatalf("expected remediation guidance, got %q", manifestCheck.Message)
			}
		})
	}
}

func compliantObservedRuntime(t *testing.T) ObservedRuntime {
	t.Helper()
	plan, err := Build("opencode")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	return ObservedRuntime{
		Manifest: RuntimeManifestState{
			Present:            true,
			ContractVersion:    plan.Contract.Version,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath:        plan.Contract.RegistryPath,
		PromptSourceIDs:     []string{"layer1.behavior", "layer2.persona", "skill.sdd-orchestrator", "registry.skill-index", "protocol.hive"},
		StoreMode:           "hybrid",
		StoreReadFrom:       []string{"hive", "openspec"},
		StoreWriteTo:        []string{"hive", "openspec"},
		ArtifactTopics:      []string{"sdd/jarvis-agent-parity-vs-gentle/spec", "sdd/jarvis-agent-parity-vs-gentle/design"},
		GeneralMemoryTopics: []string{"architecture/runtime-parity", "discovery/agent-tests"},
		ModelAssignments:    cloneModelAssignments(plan.Contract.ModelAssignments),
		Artifacts: map[string]ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
		// OpenCode is populated so that opencode-agent verifier checks do not
		// fire spurious failures in tests that do not exercise those checks.
		OpenCode: compliantOpenCodeObserved(),
	}
}

// compliantOpenCodeObserved returns an ObservedOpenCodeConfig with all
// invariant fields set to their canonical passing values. It is used by
// test helpers that build a fully-compliant ObservedRuntime for the opencode
// agent so that pre-existing tests remain unaffected by the new check layer.
func compliantOpenCodeObserved() ObservedOpenCodeConfig {
	subagents := []string{
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
		"sdd-init", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent",
		"review-risk", "review-readability", "review-reliability", "review-resilience",
	}
	return ObservedOpenCodeConfig{
		ParseSucceeded:     true,
		ShareMode:          "disabled",
		DefaultAgent:       "sdd-orchestrator",
		OrchestratorMode:   "primary",
		OrchestratorModel:  "claude-opus-4-5",
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

func cloneModelAssignments(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func findCheckByKey(checks []CheckResult, key string) *CheckResult {
	for i := range checks {
		if checks[i].Key == key {
			return &checks[i]
		}
	}
	return nil
}

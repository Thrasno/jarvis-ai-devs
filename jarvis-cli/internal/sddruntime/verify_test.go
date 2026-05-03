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
		RegistryPath: plan.Contract.RegistryPath,
		ModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
			"default":      "sonnet",
		},
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
		RegistryPath: plan.Contract.RegistryPath,
		ModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
			"default":      "sonnet",
		},
		Artifacts: map[string]ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}
}

func findCheckByKey(checks []CheckResult, key string) *CheckResult {
	for i := range checks {
		if checks[i].Key == key {
			return &checks[i]
		}
	}
	return nil
}

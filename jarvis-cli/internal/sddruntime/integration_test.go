package sddruntime

import (
	"testing"
)

func TestRuntimeIntegrity_ManagedArtifactCompletenessAndContractVersion(t *testing.T) {
	tests := []struct {
		name              string
		mutate            func(*ObservedRuntime)
		expectedStatus    IntegrityStatus
		expectedCheckKey  string
		expectedDrift     DriftClass
		expectedObserved  string
	}{
		{
			name:           "passes when manifest and managed artifacts are complete",
			mutate:         func(*ObservedRuntime) {},
			expectedStatus: StatusPass,
		},
		{
			name: "fails when manifest managed artifact catalog is incomplete",
			mutate: func(observed *ObservedRuntime) {
				observed.Manifest.ManagedArtifactIDs = []string{"instructions", "orchestrator"}
			},
			expectedStatus:   StatusFail,
			expectedCheckKey: "manifest.managed_artifacts",
			expectedDrift:    DriftOwned,
			expectedObserved: "2/3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := compliantObservedRuntime(t)
			tt.mutate(&observed)

			report := Verify("opencode", observed)
			if report.Status != tt.expectedStatus {
				t.Fatalf("expected status %q, got %q", tt.expectedStatus, report.Status)
			}
			if report.ContractVersion != DefaultContractVersion {
				t.Fatalf("expected contract version %q, got %q", DefaultContractVersion, report.ContractVersion)
			}

			if tt.expectedCheckKey == "" {
				return
			}

			check := findCheckByKey(report.Checks, tt.expectedCheckKey)
			if check == nil {
				t.Fatalf("expected check %q", tt.expectedCheckKey)
			}
			if check.DriftClass != tt.expectedDrift {
				t.Fatalf("expected drift class %q, got %q", tt.expectedDrift, check.DriftClass)
			}
			if check.Observed != tt.expectedObserved {
				t.Fatalf("expected observed %q, got %q", tt.expectedObserved, check.Observed)
			}
		})
	}
}

func TestRuntimeIntegrity_OwnedFieldTamperDetected(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.ModelAssignments["orchestrator"] = "haiku"

	report := Verify("claude", observed)
	if report.Status != StatusFail {
		t.Fatalf("expected fail status for owned tamper, got %q", report.Status)
	}

	check := findCheckByKey(report.Checks, "invariant.model.orchestrator")
	if check == nil {
		t.Fatal("expected owned invariant model check")
	}
	if check.DriftClass != DriftOwned {
		t.Fatalf("expected owned drift class, got %q", check.DriftClass)
	}
	if check.Expected != "opus" {
		t.Fatalf("expected value mismatch: got %q want %q", check.Expected, "opus")
	}
	if check.Observed != "haiku" {
		t.Fatalf("observed value mismatch: got %q want %q", check.Observed, "haiku")
	}
}

func TestRuntimeIntegrity_NonOwnedCustomizationDoesNotFail(t *testing.T) {
	observed := compliantObservedRuntime(t)
	observed.NonOwnedChanges = []string{"custom user note outside managed block"}

	report := Verify("claude", observed)
	if report.Status == StatusFail {
		t.Fatalf("expected non-owned customization to avoid fail, got %q", report.Status)
	}

	check := findCheckByKey(report.Checks, "drift.non_owned")
	if check == nil {
		t.Fatal("expected non-owned drift check")
	}
	if check.Status != StatusWarn {
		t.Fatalf("expected warn status for non-owned drift, got %q", check.Status)
	}
	if check.DriftClass != DriftNonOwned {
		t.Fatalf("expected non-owned drift class, got %q", check.DriftClass)
	}
}

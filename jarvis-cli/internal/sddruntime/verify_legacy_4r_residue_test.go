package sddruntime

import "testing"

// TestVerify_Claude_LegacyResidue_Table covers the informational
// invariant.claude.legacy_4r_residue check: no residue, full residue (all
// four retired files), and partial residue (a subset only). It must never
// fail (StatusFail) and must never be reported as owned drift.
func TestVerify_Claude_LegacyResidue_Table(t *testing.T) {
	cases := []struct {
		name           string
		residue        []string
		wantStatus     IntegrityStatus
		wantDrift      DriftClass
		wantObserved   string
		wantReportWarn bool
	}{
		{
			name:         "no residue",
			residue:      nil,
			wantStatus:   StatusPass,
			wantDrift:    DriftNone,
			wantObserved: "none",
		},
		{
			name:           "full residue",
			residue:        []string{"review-risk.md", "review-readability.md", "review-reliability.md", "review-resilience.md"},
			wantStatus:     StatusWarn,
			wantDrift:      DriftNonOwned,
			wantObserved:   "review-risk.md,review-readability.md,review-reliability.md,review-resilience.md",
			wantReportWarn: true,
		},
		{
			name:           "partial residue",
			residue:        []string{"review-risk.md"},
			wantStatus:     StatusWarn,
			wantDrift:      DriftNonOwned,
			wantObserved:   "review-risk.md",
			wantReportWarn: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observed := compliantObservedRuntime(t)
			observed.ClaudeLegacy4RResidue = tc.residue

			report := Verify("claude", observed)

			check := findCheckByKey(report.Checks, "invariant.claude.legacy_4r_residue")
			if check == nil {
				t.Fatal("expected invariant.claude.legacy_4r_residue check to be present")
			}
			if check.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", check.Status, tc.wantStatus)
			}
			if check.DriftClass != tc.wantDrift {
				t.Fatalf("drift class = %q, want %q", check.DriftClass, tc.wantDrift)
			}
			if check.Observed != tc.wantObserved {
				t.Fatalf("observed = %q, want %q", check.Observed, tc.wantObserved)
			}
			if check.Status == StatusFail {
				t.Fatal("legacy 4R residue must never be reported as StatusFail")
			}
			if tc.wantReportWarn && report.Status != StatusWarn {
				t.Fatalf("report status = %q, want %q", report.Status, StatusWarn)
			}
			if !tc.wantReportWarn && report.Status != StatusPass {
				t.Fatalf("report status = %q, want %q", report.Status, StatusPass)
			}
		})
	}
}

// TestVerify_OpenCode_LegacyResidue_Table covers the informational
// invariant.opencode.legacy_4r_residue check across agent-entry-only,
// task-allow-only, full, and no-residue scenarios.
func TestVerify_OpenCode_LegacyResidue_Table(t *testing.T) {
	base := compliantOpenCodeObserved()

	cases := []struct {
		name         string
		mutate       func(ObservedOpenCodeConfig) ObservedOpenCodeConfig
		wantStatus   IntegrityStatus
		wantDrift    DriftClass
		wantObserved string
	}{
		{
			name:         "no residue",
			mutate:       func(oc ObservedOpenCodeConfig) ObservedOpenCodeConfig { return oc },
			wantStatus:   StatusPass,
			wantDrift:    DriftNone,
			wantObserved: "none",
		},
		{
			name: "agent entries only",
			mutate: func(oc ObservedOpenCodeConfig) ObservedOpenCodeConfig {
				oc.AgentNames = append(oc.AgentNames, "review-risk", "review-readability")
				oc.HiddenSubagents = append(oc.HiddenSubagents, "review-risk", "review-readability")
				return oc
			},
			wantStatus:   StatusWarn,
			wantDrift:    DriftNonOwned,
			wantObserved: "agent:review-risk,agent:review-readability",
		},
		{
			name: "task allows only",
			mutate: func(oc ObservedOpenCodeConfig) ObservedOpenCodeConfig {
				oc.TaskAllows = append(oc.TaskAllows, "review-reliability")
				return oc
			},
			wantStatus:   StatusWarn,
			wantDrift:    DriftNonOwned,
			wantObserved: "task_allow:review-reliability",
		},
		{
			name: "full residue",
			mutate: func(oc ObservedOpenCodeConfig) ObservedOpenCodeConfig {
				retired := []string{"review-risk", "review-readability", "review-reliability", "review-resilience"}
				oc.AgentNames = append(oc.AgentNames, retired...)
				oc.HiddenSubagents = append(oc.HiddenSubagents, retired...)
				oc.TaskAllows = append(oc.TaskAllows, retired...)
				return oc
			},
			wantStatus: StatusWarn,
			wantDrift:  DriftNonOwned,
			wantObserved: "agent:review-risk,agent:review-readability,agent:review-reliability,agent:review-resilience," +
				"task_allow:review-risk,task_allow:review-readability,task_allow:review-reliability,task_allow:review-resilience",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oc := tc.mutate(base)
			checks := verifyOpenCodeConfigInvariants(oc, "hybrid")

			check := findCheckByKey(checks, "invariant.opencode.legacy_4r_residue")
			if check == nil {
				t.Fatal("expected invariant.opencode.legacy_4r_residue check to be present")
			}
			if check.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", check.Status, tc.wantStatus)
			}
			if check.DriftClass != tc.wantDrift {
				t.Fatalf("drift class = %q, want %q", check.DriftClass, tc.wantDrift)
			}
			if check.Observed != tc.wantObserved {
				t.Fatalf("observed = %q, want %q", check.Observed, tc.wantObserved)
			}
			if check.Status == StatusFail {
				t.Fatal("legacy 4R residue must never be reported as StatusFail")
			}
		})
	}
}

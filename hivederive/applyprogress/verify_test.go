package applyprogress

import "testing"

func TestVerifyReportArchiveReadinessRequiresCanonicalActiveReport(t *testing.T) {
	canonical := func(verdict, blockers string, criticals int) string {
		return "## Verdict\n\n**" + verdict + "**\n\n## Critical Findings\n\n" + string(rune('0'+criticals)) + "\n\n## Blockers\n\n" + blockers + "\n"
	}

	tests := []struct {
		name     string
		content  string
		ready    bool
		wantCode bool
	}{
		{name: "exact producer output", content: canonical("PASS — archive ready.", "None", 0), ready: true},
		{name: "pass with warnings ready", content: canonical("PASS WITH WARNINGS — archive ready.", "None", 0), ready: true},
		{name: "emphasized none", content: canonical("PASS — archive ready.", "**None**", 0), ready: true},
		{name: "italic none", content: canonical("PASS — archive ready.", "_None_", 0), ready: true},
		{name: "negated prose outside active sections", content: canonical("PASS — archive ready.", "None", 0) + "\n## History\n\nNo failures remain; no work is pending.\n", ready: true},
		{name: "fail", content: canonical("FAIL", "None", 0), ready: false},
		{name: "critical finding", content: canonical("PASS — archive ready.", "None", 1), ready: false},
		{name: "blocker", content: canonical("PASS — archive ready.", "Windows smoke test failed.", 0), ready: false},
		{name: "duplicate required heading", content: canonical("PASS — archive ready.", "None", 0) + "\n## Verdict\n\n**PASS — archive ready.**\n", ready: false, wantCode: true},
		{name: "missing archive ready marker", content: canonical("PASS", "None", 0), ready: false, wantCode: true},
		{name: "archived style report", content: "## Verification Report\n\nStatus: PASS\n\nAll historical checks passed.\n", ready: false, wantCode: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readiness := InspectVerifyReport(tt.content)
			if readiness.Ready != tt.ready {
				t.Fatalf("ready = %t, want %t; report:\n%s", readiness.Ready, tt.ready, tt.content)
			}
			if tt.wantCode && readiness.Code != VerifyReportReasonRegenerateWithSDDVerify {
				t.Fatalf("code = %q, want %q", readiness.Code, VerifyReportReasonRegenerateWithSDDVerify)
			}
			if !tt.wantCode && readiness.Code != "" {
				t.Fatalf("code = %q, want empty for a canonical non-ready report", readiness.Code)
			}
			if got := VerifyReportArchiveReady(tt.content); got != tt.ready {
				t.Fatalf("VerifyReportArchiveReady() = %t, want %t", got, tt.ready)
			}
		})
	}
}

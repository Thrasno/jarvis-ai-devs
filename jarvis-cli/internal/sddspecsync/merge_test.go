package sddspecsync

import (
	"errors"
	"strings"
	"testing"
)

const (
	mainHead = "# Auth Specification\n\n## Purpose\n\nAuthenticate users.\n\n## Requirements\n\n"
	reqLogin = "### Requirement: Login\n\nThe system MUST log users in.\n\n" +
		"#### Scenario: Valid credentials\n\n- GIVEN a user\n- WHEN they log in\n- THEN a session starts\n"
	reqExpiry = "### Requirement: Session Expiration\n\nSessions MUST expire after 30 minutes.\n\n" +
		"#### Scenario: Idle\n\n- GIVEN an idle session\n- WHEN 30 minutes pass\n- THEN it expires\n"
	reqLegacy = "### Requirement: Legacy Token\n\nThe system MUST accept legacy tokens.\n"
	mainTail  = "\n## Non-Goals\n\n- SSO.\n"

	reqLogout = "### Requirement: Logout\n\nThe system MUST end sessions on logout.\n\n" +
		"#### Scenario: Explicit logout\n\n- GIVEN a session\n- WHEN the user logs out\n- THEN the session ends\n"
	reqExpiryV2 = "### Requirement: Session Expiration\n\nSessions MUST expire after 15 minutes.\n(Previously: 30 minutes)\n\n" +
		"#### Scenario: Idle\n\n- GIVEN an idle session\n- WHEN 15 minutes pass\n- THEN it expires\n"
)

func authMain() string {
	return mainHead + reqLogin + "\n" + reqExpiry + "\n" + reqLegacy + mainTail
}

func delta(sections ...string) string {
	return "# Delta for Auth\n\n" + strings.Join(sections, "\n")
}

func TestMergeSpecAppliesDeltaOperations(t *testing.T) {
	tests := []struct {
		name  string
		main  string
		delta string
		want  string
	}{
		{
			name:  "added appends to the end of the requirements section",
			main:  authMain(),
			delta: delta("## ADDED Requirements\n\n" + reqLogout),
			want:  mainHead + reqLogin + "\n" + reqExpiry + "\n" + reqLegacy + "\n" + reqLogout + mainTail,
		},
		{
			name:  "modified replaces the whole matching block in place",
			main:  authMain(),
			delta: delta("## MODIFIED Requirements\n\n" + reqExpiryV2),
			want:  mainHead + reqLogin + "\n" + reqExpiryV2 + "\n" + reqLegacy + mainTail,
		},
		{
			name: "removed deletes the matching block",
			main: authMain(),
			delta: delta("## REMOVED Requirements\n\n### Requirement: Legacy Token\n\n" +
				"(Reason: Legacy tokens were retired in v2.)\n(Migration: Clients use OAuth tokens instead.)\n"),
			want: mainHead + reqLogin + "\n" + reqExpiry + mainTail,
		},
		{
			name: "removed accepts justified Migration None",
			main: authMain(),
			delta: delta("## REMOVED Requirements\n\n### Requirement: Legacy Token\n\n" +
				"(Reason: Nobody issues legacy tokens.)\n(Migration: None — no client has used them since 2024.)\n"),
			want: mainHead + reqLogin + "\n" + reqExpiry + mainTail,
		},
		{
			name: "renamed changes only the heading and preserves the body",
			main: authMain(),
			delta: delta("## RENAMED Requirements\n\n### Requirement: Sign In\n\n" +
				"(Old name: Login)\n(New name: Sign In)\n(Reason: Product wording.)\n"),
			want: mainHead + strings.Replace(reqLogin, "Requirement: Login", "Requirement: Sign In", 1) +
				"\n" + reqExpiry + "\n" + reqLegacy + mainTail,
		},
		{
			name: "renamed and modified under the new name compose",
			main: authMain(),
			delta: delta(
				"## MODIFIED Requirements\n\n### Requirement: Sign In\n\nThe system MUST sign users in.\n",
				"## RENAMED Requirements\n\n### Requirement: Sign In\n\n(Old name: Login)\n(New name: Sign In)\n",
			),
			want: mainHead + "### Requirement: Sign In\n\nThe system MUST sign users in.\n" +
				"\n" + reqExpiry + "\n" + reqLegacy + mainTail,
		},
		{
			name:  "CRLF input is normalized to LF output",
			main:  strings.ReplaceAll(authMain(), "\n", "\r\n"),
			delta: strings.ReplaceAll(delta("## ADDED Requirements\n\n"+reqLogout), "\n", "\r\n"),
			want:  mainHead + reqLogin + "\n" + reqExpiry + "\n" + reqLegacy + "\n" + reqLogout + mainTail,
		},
		{
			name:  "output ends with exactly one newline",
			main:  strings.TrimSuffix(authMain(), "\n"),
			delta: delta("## MODIFIED Requirements\n\n" + reqExpiryV2 + "\n\n"),
			want:  mainHead + reqLogin + "\n" + reqExpiryV2 + "\n" + reqLegacy + mainTail,
		},
		{
			name:  "legacy main with an ADDED Requirements section accepts additions",
			main:  "# Quarantine Center\n\n## ADDED Requirements\n\n### Requirement: A\n\nText A.\n",
			delta: delta("## ADDED Requirements\n\n### Requirement: B\n\nText B.\n"),
			want:  "# Quarantine Center\n\n## ADDED Requirements\n\n### Requirement: A\n\nText A.\n\n### Requirement: B\n\nText B.\n",
		},
		{
			name:  "headings inside fenced code are not requirements",
			main:  mainHead + "### Requirement: Fenced\n\n```md\n### Requirement: Login\n## Non-Goals\n```\n" + mainTail,
			delta: delta("## ADDED Requirements\n\n" + reqLogin),
			want: mainHead + "### Requirement: Fenced\n\n```md\n### Requirement: Login\n## Non-Goals\n```\n\n" +
				reqLogin + mainTail,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeSpec([]byte(tt.main), []byte(tt.delta))
			if err != nil {
				t.Fatalf("MergeSpec() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("MergeSpec() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tt.want)
			}
		})
	}
}

func TestMergeSpecNewCapabilityCopiesFullSpec(t *testing.T) {
	full := "# Billing Specification\n\n## Purpose\n\nCharge users.\n\n## Requirements\n\n" +
		"### Requirement: Invoice\n\nThe system MUST invoice.\n\n#### Scenario: Monthly\n\n- GIVEN a plan\n- WHEN a month ends\n- THEN an invoice is issued\n"
	tests := []struct {
		name string
		in   string
	}{
		{name: "verbatim", in: full},
		{name: "CRLF", in: strings.ReplaceAll(full, "\n", "\r\n")},
		{name: "missing final newline", in: strings.TrimSuffix(full, "\n")},
		{name: "extra trailing blank lines", in: full + "\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeSpec(nil, []byte(tt.in))
			if err != nil {
				t.Fatalf("MergeSpec() error = %v", err)
			}
			if string(got) != full {
				t.Fatalf("MergeSpec() = %q, want %q", got, full)
			}
		})
	}

	legacy := "# Quarantine Center\n\n## ADDED Requirements\n\n### Requirement: A\n\nText A.\n"
	got, err := MergeSpec(nil, []byte(legacy))
	if err != nil || string(got) != legacy {
		t.Fatalf("MergeSpec(ADDED-only new capability) = %q, %v; want verbatim copy", got, err)
	}
}

func TestMergeSpecFailsClosed(t *testing.T) {
	removed := func(body string) string {
		return delta("## REMOVED Requirements\n\n### Requirement: Legacy Token\n\n" + body)
	}
	renamed := func(heading, body string) string {
		return delta("## RENAMED Requirements\n\n### Requirement: " + heading + "\n\n" + body)
	}
	tests := []struct {
		name    string
		main    *string
		delta   string
		wantErr error
		wantReq string
	}{
		{name: "added duplicate", delta: delta("## ADDED Requirements\n\n" + reqLogin), wantErr: ErrDuplicateRequirement, wantReq: "Login"},
		{name: "modified missing", delta: delta("## MODIFIED Requirements\n\n### Requirement: Ghost\n\nText.\n"), wantErr: ErrRequirementNotFound, wantReq: "Ghost"},
		{name: "removed missing", delta: delta("## REMOVED Requirements\n\n### Requirement: Ghost\n\n(Reason: Gone.)\n(Migration: Use X.)\n"), wantErr: ErrRequirementNotFound, wantReq: "Ghost"},
		{name: "removed without migration", delta: removed("(Reason: Retired.)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "removed without reason", delta: removed("(Migration: Use OAuth.)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "removed placeholder reason", delta: removed("(Reason: {why this requirement is being removed})\n(Migration: Use OAuth.)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "removed TBD migration", delta: removed("(Reason: Retired.)\n(Migration: TBD)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "removed unjustified None", delta: removed("(Reason: Retired.)\n(Migration: None)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "removed unjustified None with period", delta: removed("(Reason: Retired.)\n(Migration: `None`.)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "removed duplicated evidence", delta: removed("(Reason: Retired.)\n(Reason: Again.)\n(Migration: Use OAuth.)\n"), wantErr: ErrRemovalEvidence, wantReq: "Legacy Token"},
		{name: "renamed without old name", delta: renamed("Sign In", "(New name: Sign In)\n"), wantErr: ErrRenameEvidence, wantReq: "Sign In"},
		{name: "renamed without new name", delta: renamed("Sign In", "(Old name: Login)\n"), wantErr: ErrRenameEvidence, wantReq: "Sign In"},
		{name: "renamed new name differs from heading", delta: renamed("Sign In", "(Old name: Login)\n(New name: Log In)\n"), wantErr: ErrRenameEvidence, wantReq: "Sign In"},
		{name: "renamed to the same name", delta: renamed("Login", "(Old name: Login)\n(New name: Login)\n"), wantErr: ErrRenameEvidence, wantReq: "Login"},
		{name: "renamed from missing", delta: renamed("Sign In", "(Old name: Ghost)\n(New name: Sign In)\n"), wantErr: ErrRequirementNotFound, wantReq: "Ghost"},
		{name: "renamed to existing", delta: renamed("Session Expiration", "(Old name: Login)\n(New name: Session Expiration)\n"), wantErr: ErrDuplicateRequirement, wantReq: "Session Expiration"},
		{name: "renamed with body text", delta: renamed("Sign In", "(Old name: Login)\n(New name: Sign In)\n\nThe system MUST sign users in.\n"), wantErr: ErrInvalidDelta, wantReq: "Sign In"},
		{name: "modified uses the old name of a rename", delta: delta(
			"## RENAMED Requirements\n\n### Requirement: Sign In\n\n(Old name: Login)\n(New name: Sign In)\n",
			"## MODIFIED Requirements\n\n### Requirement: Login\n\nText.\n"), wantErr: ErrInvalidDelta, wantReq: "Login"},
		{name: "same name modified and removed", delta: delta(
			"## MODIFIED Requirements\n\n### Requirement: Login\n\nText.\n",
			"## REMOVED Requirements\n\n### Requirement: Login\n\n(Reason: Retired.)\n(Migration: Use OAuth.)\n"), wantErr: ErrInvalidDelta, wantReq: "Login"},
		{name: "duplicate name within a section", delta: delta("## ADDED Requirements\n\n" + reqLogout + "\n" + reqLogout), wantErr: ErrInvalidDelta, wantReq: "Logout"},
		{name: "duplicate section", delta: delta("## ADDED Requirements\n\n"+reqLogout, "## ADDED Requirements\n\n### Requirement: Other\n\nText.\n"), wantErr: ErrInvalidDelta},
		{name: "requirement inside an ignored section", delta: delta("## Notes\n\n"+reqLogout, "## ADDED Requirements\n\n### Requirement: Other\n\nText.\n"), wantErr: ErrInvalidDelta, wantReq: "Logout"},
		{name: "misspelled operation section", delta: delta("## Added Requirements\n\nText.\n", "## ADDED Requirements\n\n"+reqLogout), wantErr: ErrInvalidDelta},
		{name: "lowercase operation section", delta: delta("## MODIFIED requirements\n\nText.\n", "## ADDED Requirements\n\n"+reqLogout), wantErr: ErrInvalidDelta},
		{name: "only ignored sections", delta: delta("## Notes\n\nNothing to merge.\n"), wantErr: ErrInvalidDelta},
		{name: "level-1 heading inside an ignored section", delta: delta("## ADDED Requirements\n\n"+reqLogout, "## Notes\n\n# Title\n"), wantErr: ErrInvalidDelta},
		{name: "full spec against existing main", delta: "# Auth Specification\n\n## Requirements\n\n" + reqLogout, wantErr: ErrInvalidDelta},
		{name: "prose outside a requirement block", delta: delta("## ADDED Requirements\n\nIntro prose.\n\n" + reqLogout), wantErr: ErrInvalidDelta},
		{name: "non-requirement level-3 heading", delta: delta("## ADDED Requirements\n\n### Notes\n\nText.\n"), wantErr: ErrInvalidDelta},
		{name: "requirement before any section", delta: "# Delta for Auth\n\n" + reqLogout, wantErr: ErrInvalidDelta},
		{name: "empty section", delta: delta("## ADDED Requirements\n\n", "## MODIFIED Requirements\n\n"+reqExpiryV2), wantErr: ErrInvalidDelta},
		{name: "no operations", delta: "# Delta for Auth\n\nNothing here.\n", wantErr: ErrInvalidDelta},
		{name: "empty requirement name", delta: delta("## ADDED Requirements\n\n### Requirement:   \n\nText.\n"), wantErr: ErrInvalidDelta},
		{name: "added without requirements section", main: ptr("# Auth\n\n## Purpose\n\nText.\n\n### Requirement: Login\n\nText.\n"), delta: delta("## ADDED Requirements\n\n" + reqLogout), wantErr: ErrNoRequirementsSection},
		{name: "ambiguous requirements section", main: ptr("# Auth\n\n## Requirements\n\n## Requirements\n"), delta: delta("## ADDED Requirements\n\n" + reqLogout), wantErr: ErrNoRequirementsSection},
		{name: "main with duplicate requirement", main: ptr(mainHead + reqLogin + "\n" + reqLogin), delta: delta("## MODIFIED Requirements\n\n" + reqLogin), wantErr: ErrInvalidMainSpec, wantReq: "Login"},
		{name: "fenced heading is not a match target", main: ptr(mainHead + "### Requirement: Fenced\n\n```\n### Requirement: Hidden\n```\n"), delta: delta("## MODIFIED Requirements\n\n### Requirement: Hidden\n\nText.\n"), wantErr: ErrRequirementNotFound, wantReq: "Hidden"},
		{name: "new capability with modified section", main: nil, delta: delta("## MODIFIED Requirements\n\n" + reqExpiryV2), wantErr: ErrInvalidDelta},
		{name: "new capability without requirements", main: nil, delta: "# Billing\n\n## Purpose\n\nText.\n", wantErr: ErrInvalidDelta},
		{name: "new capability with duplicate requirement", main: nil, delta: "# Billing\n\n## Requirements\n\n" + reqLogin + "\n" + reqLogin, wantErr: ErrDuplicateRequirement, wantReq: "Login"},
		{name: "blank delta", delta: " \n\n", wantErr: ErrInvalidDelta},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var main []byte
			switch {
			case tt.main != nil:
				main = []byte(*tt.main)
			case !strings.HasPrefix(tt.name, "new capability"):
				main = []byte(authMain())
			}
			got, err := MergeSpec(main, []byte(tt.delta))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MergeSpec() = %q, %v; want %v", got, err, tt.wantErr)
			}
			if got != nil {
				t.Fatalf("MergeSpec() returned bytes on error: %q", got)
			}
			var mergeErr *MergeError
			if !errors.As(err, &mergeErr) {
				t.Fatalf("MergeSpec() error %T is not *MergeError", err)
			}
			if tt.wantReq != "" && mergeErr.Requirement != tt.wantReq {
				t.Fatalf("MergeError.Requirement = %q, want %q", mergeErr.Requirement, tt.wantReq)
			}
		})
	}
}

func TestMergeSpecReportsChanges(t *testing.T) {
	d := delta(
		"## ADDED Requirements\n\n"+reqLogout,
		"## MODIFIED Requirements\n\n"+reqExpiryV2,
		"## REMOVED Requirements\n\n### Requirement: Legacy Token\n\n(Reason: Retired.)\n(Migration: Use OAuth.)\n",
		"## RENAMED Requirements\n\n### Requirement: Sign In\n\n(Old name: Login)\n(New name: Sign In)\n",
	)
	_, changes, err := merge([]byte(authMain()), true, []byte(d))
	if err != nil {
		t.Fatalf("merge() error = %v", err)
	}
	if strings.Join(changes.Added, ",") != "Logout" ||
		strings.Join(changes.Modified, ",") != "Session Expiration" ||
		strings.Join(changes.Removed, ",") != "Legacy Token" ||
		len(changes.Renamed) != 1 || changes.Renamed[0] != (Rename{From: "Login", To: "Sign In"}) ||
		changes.NewCapability {
		t.Fatalf("changes = %+v", changes)
	}
	if !changes.Destructive() {
		t.Fatal("Destructive() = false with a REMOVED requirement")
	}
}

func TestMergeSpecIgnoresNonDeltaSections(t *testing.T) {
	notes := "## Notes\n\nWhy this change exists.\n\n### Open question\n\n```md\n### Requirement: Fenced\n```\n"
	want := mainHead + reqLogin + "\n" + reqExpiryV2 + "\n" + reqLegacy + "\n" + reqLogout + mainTail
	tests := []struct {
		name        string
		delta       string
		wantIgnored []string
	}{
		{
			name:        "before the operation sections",
			delta:       delta(notes, "## ADDED Requirements\n\n"+reqLogout, "## MODIFIED Requirements\n\n"+reqExpiryV2),
			wantIgnored: []string{"Notes"},
		},
		{
			name:        "between and after the operation sections",
			delta:       delta("## ADDED Requirements\n\n"+reqLogout, notes, "## MODIFIED Requirements\n\n"+reqExpiryV2, "## Purpose\n\nText.\n"),
			wantIgnored: []string{"Notes", "Purpose"},
		},
		{
			name:        "CRLF input",
			delta:       strings.ReplaceAll(delta(notes, "## ADDED Requirements\n\n"+reqLogout, "## MODIFIED Requirements\n\n"+reqExpiryV2), "\n", "\r\n"),
			wantIgnored: []string{"Notes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changes, err := merge([]byte(authMain()), true, []byte(tt.delta))
			if err != nil {
				t.Fatalf("merge() error = %v", err)
			}
			if string(got) != want {
				t.Fatalf("merge() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
			if strings.Join(changes.IgnoredSections, "|") != strings.Join(tt.wantIgnored, "|") {
				t.Fatalf("IgnoredSections = %q, want %q", changes.IgnoredSections, tt.wantIgnored)
			}
			if strings.Join(changes.Added, ",") != "Logout" || strings.Join(changes.Modified, ",") != "Session Expiration" {
				t.Fatalf("changes = %+v", changes)
			}
		})
	}
}

func ptr(s string) *string { return &s }

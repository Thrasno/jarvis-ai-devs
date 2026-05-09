package repository

import "testing"

// CRIT-4 — isSentinelID's legacy-pre-lifecycle-* branch was dead code: the
// length guard checked `len(id) > 24 && id[:24] == "legacy-pre-lifecycle-"`
// but the prefix is 21 characters, so the slice/comparison was inconsistent
// and the branch was effectively unreachable. The migration creates legacy
// sentinels and they MUST resolve as sentinels for UpsertSession to keep
// FR-S-4 / SC-16 working when a daemon re-pushes one.
func TestIsSentinelID_LegacyPrefixDetected(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"legacy-pre-lifecycle-foo", true},
		{"legacy-pre-lifecycle-jarvis-dev", true},
		{"manual-save-foo", true},
		{"manual-save-jarvis-dev", true},
		{"some-uuid-1234", false},
		{"", false},
		{"legacy-pre-lifecycle-", true}, // exact prefix only — accepted as sentinel
		{"manual-save-", true},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := isSentinelID(c.id); got != c.want {
				t.Fatalf("isSentinelID(%q) = %v, want %v", c.id, got, c.want)
			}
		})
	}
}

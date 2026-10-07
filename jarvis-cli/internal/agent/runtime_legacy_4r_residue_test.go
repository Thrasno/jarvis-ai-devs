package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClaudeAgent_ObserveRuntime_ReportsLegacy4RResidue covers no residue,
// full residue (all four retired files), and partial residue under
// ~/.claude/agents/, verifying observeClaudeLegacy4RResidue never mutates
// anything and reports exactly the files it found.
func TestClaudeAgent_ObserveRuntime_ReportsLegacy4RResidue(t *testing.T) {
	cases := []struct {
		name   string
		files  []string
		expect []string
	}{
		{name: "no residue", files: nil, expect: nil},
		{
			name:   "full residue",
			files:  []string{"review-risk.md", "review-readability.md", "review-reliability.md", "review-resilience.md"},
			expect: []string{"review-risk.md", "review-readability.md", "review-reliability.md", "review-resilience.md"},
		},
		{
			name:   "partial residue",
			files:  []string{"review-risk.md"},
			expect: []string{"review-risk.md"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}
			agentsDir := filepath.Join(a.ConfigDir(), "agents")
			if err := os.MkdirAll(agentsDir, 0755); err != nil {
				t.Fatalf("mkdir agents dir: %v", err)
			}
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(agentsDir, name), []byte("# retired"), 0644); err != nil {
					t.Fatalf("write residue file %s: %v", name, err)
				}
			}
			// A non-retired user agent file must never be reported as residue.
			if err := os.WriteFile(filepath.Join(agentsDir, "my-custom-agent.md"), []byte("# custom"), 0644); err != nil {
				t.Fatalf("write custom agent file: %v", err)
			}

			observed, err := a.ObserveRuntime()
			if err != nil {
				t.Fatalf("ObserveRuntime: %v", err)
			}

			if len(observed.ClaudeLegacy4RResidue) != len(tc.expect) {
				t.Fatalf("ClaudeLegacy4RResidue = %v, want %v", observed.ClaudeLegacy4RResidue, tc.expect)
			}
			gotSet := make(map[string]struct{}, len(observed.ClaudeLegacy4RResidue))
			for _, name := range observed.ClaudeLegacy4RResidue {
				gotSet[name] = struct{}{}
			}
			for _, name := range tc.expect {
				if _, ok := gotSet[name]; !ok {
					t.Fatalf("expected %q in ClaudeLegacy4RResidue, got %v", name, observed.ClaudeLegacy4RResidue)
				}
			}

			// The user's custom agent file must still exist untouched: this
			// observer must never delete or mutate anything.
			if _, err := os.Stat(filepath.Join(agentsDir, "my-custom-agent.md")); err != nil {
				t.Fatalf("expected custom agent file to remain untouched: %v", err)
			}
			for _, name := range tc.files {
				if _, err := os.Stat(filepath.Join(agentsDir, name)); err != nil {
					t.Fatalf("expected residue file %q to remain untouched by observation: %v", name, err)
				}
			}
		})
	}
}

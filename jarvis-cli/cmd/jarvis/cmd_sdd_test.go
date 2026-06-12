package main

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

// TestPrintStatusHuman_BlockedWithNoNextRecommended guards against the regression
// where printStatusHuman showed "all phases complete ✓" while also showing blocked
// reasons — contradictory output that occurred when NextRecommended was "" and
// BlockedReasons was non-empty (e.g., archive soft-blocked by empty verify content).
func TestPrintStatusHuman_BlockedWithNoNextRecommended(t *testing.T) {
	s := &sddstatus.ChangeStatus{
		Schema:        sddstatus.StatusSchema,
		ChangeName:    "my-feature",
		ArtifactStore: "hive",
		ArtifactPaths: map[string]string{},
		Artifacts:     map[string]sddstatus.ArtifactState{},
		Dependencies:  map[string]sddstatus.DependencyState{},
		// NextRecommended empty + BlockedReasons non-empty: the blocked-but-no-next state.
		NextRecommended: "",
		BlockedReasons:  []string{"phase sdd-archive blocked — verify report is empty (re-run sdd-verify to generate content)"},
	}

	for _, phase := range sddstatus.PhaseOrder {
		s.Dependencies[phase] = sddstatus.DepAllDone
		s.Artifacts[sddstatus.PhaseOutput[phase]] = sddstatus.ArtifactDone
	}
	s.Dependencies[sddstatus.PhaseArchive] = sddstatus.DepBlocked
	s.Artifacts[sddstatus.ArtifactArchiveReport] = sddstatus.ArtifactMissing

	var buf strings.Builder
	printStatusHuman(&buf, s, false)
	out := buf.String()

	if strings.Contains(out, "all phases complete") {
		t.Errorf("printStatusHuman must NOT say 'all phases complete' when BlockedReasons is non-empty; got:\n%s", out)
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("printStatusHuman must say 'blocked' in the next-recommended line; got:\n%s", out)
	}
}

// TestPrintStatusHuman_AllDone_ShowsComplete verifies the "all phases complete ✓"
// message is still shown correctly when there are no blocked reasons.
func TestPrintStatusHuman_AllDone_ShowsComplete(t *testing.T) {
	s := &sddstatus.ChangeStatus{
		Schema:          sddstatus.StatusSchema,
		ChangeName:      "my-feature",
		ArtifactStore:   "hive",
		ArtifactPaths:   map[string]string{},
		Artifacts:       map[string]sddstatus.ArtifactState{},
		Dependencies:    map[string]sddstatus.DependencyState{},
		NextRecommended: "none",
		BlockedReasons:  nil,
	}

	for _, phase := range sddstatus.PhaseOrder {
		s.Dependencies[phase] = sddstatus.DepAllDone
		s.Artifacts[sddstatus.PhaseOutput[phase]] = sddstatus.ArtifactDone
	}

	var buf strings.Builder
	printStatusHuman(&buf, s, false)
	out := buf.String()

	if !strings.Contains(out, "all phases complete") {
		t.Errorf("printStatusHuman must say 'all phases complete' when all done and no blocked reasons; got:\n%s", out)
	}
}

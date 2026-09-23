package applyprogress

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildSuccessorGenesisPreservesHistoricalBytes(t *testing.T) {
	old, _, err := SealSnapshot(Snapshot{Schema: SupersessionSnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 2, TaskManifestSHA256: strings.Repeat("a", 64), Status: StatusSuperseded, Coverage: []Coverage{}, Batches: []BatchRef{}, SealIntent: &SealIntent{SuccessorProject: "project", SuccessorChange: "new", SuccessorManifestSHA256: strings.Repeat("b", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "request-1"}})
	if err != nil {
		t.Fatal(err)
	}
	pointer := &SupersedesPointer{Project: old.Project, Change: old.Change, SealDigest: old.Digest, OriginalManifestSHA256: old.TaskManifestSHA256, Actor: old.SealIntent.Actor, Reason: old.SealIntent.Reason, Timestamp: old.SealIntent.Timestamp, OperationID: old.SealIntent.OperationID}
	expected, expectedBytes, err := SealSnapshot(Snapshot{Schema: SupersessionSnapshotSchema, Project: "project", Change: "new", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("b", 64), Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}, Supersedes: pointer})
	if err != nil {
		t.Fatal(err)
	}
	got, data, err := BuildSuccessorGenesis(old)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, expectedBytes) || got.Digest != expected.Digest {
		t.Fatalf("historical genesis bytes/digest changed: %s vs %s", data, expectedBytes)
	}
	if err := ValidateSuccessorGenesisPair(old, got); err != nil {
		t.Fatal(err)
	}
	if len(got.Coverage) != 0 || len(got.Batches) != 0 || got.PreviousDigest != "" {
		t.Fatal("inherited progress")
	}
	for name, mutate := range map[string]func(*Snapshot){
		"foreign digest": func(s *Snapshot) { s.Digest = strings.Repeat("f", 64) },
		"not sealed":     func(s *Snapshot) { s.Status = StatusPartial },
		"missing intent": func(s *Snapshot) { s.SealIntent = nil },
		"invalid target": func(s *Snapshot) { s.SealIntent.SuccessorChange = "../foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := old
			intent := *old.SealIntent
			bad.SealIntent = &intent
			mutate(&bad)
			if _, _, err := BuildSuccessorGenesis(bad); err == nil {
				t.Fatal("accepted invalid predecessor")
			}
		})
	}
}

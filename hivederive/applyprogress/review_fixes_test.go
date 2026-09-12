package applyprogress

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPlanCheckpointWarningCarriesCapacityProjection(t *testing.T) {
	entry := planEntry("warning", "1", "evidence")
	stream, err := StreamSHA256([]EvidenceEntry{entry})
	if err != nil {
		t.Fatal(err)
	}
	base := planNearCapacityBase(t)
	for len(base.Batches) > 0 {
		base.Batches = base.Batches[:len(base.Batches)-1]
		base.StreamSHA256, base.NextEntryIndex, base.NextEntryID = stream, 0, entry.EntryID
		base, _, err = SealSnapshot(base)
		if err != nil {
			t.Fatal(err)
		}
		result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{entry}, EntryID: entry.EntryID, BatchID: planBatchID(9500), StreamSHA256: stream})
		if err != nil || result.Warning == nil {
			continue
		}
		if result.Capacity == nil {
			t.Fatalf("warning result capacity = nil; want canonical current and projected capacity")
		}
		baseData, sealErr := snapshotCanonicalJSON(base)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if result.Capacity.CurrentRunes != utf8.RuneCount(baseData) || result.Capacity.ProjectedRunes != utf8.RuneCount(result.SnapshotJSON) || result.Capacity.CeilingRunes != MaxDocumentRunes {
			t.Fatalf("warning capacity = %#v; want exact base/current, successor/projected, and ceiling runes", result.Capacity)
		}
		return
	}
	t.Fatal("did not construct a successful warning checkpoint")
}

func TestFlatContinuationBindsCursorAndTerminalStream(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []EvidenceEntry{
		planEntry("e1", "1", strings.Repeat("x", 21000)),
		planEntry("e2", "2", strings.Repeat("x", 21000)),
		planEntry("e3", "3", strings.Repeat("x", 21000)),
	}
	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(799)})
	if err != nil || first.Outcome != PlanContinuationRequired {
		t.Fatalf("PlanCheckpoint() = %#v, %v; want continuation", first, err)
	}
	if !ValidDigest(first.Snapshot.StreamSHA256) || first.Snapshot.NextEntryIndex != first.EndIndex || first.Snapshot.NextEntryID != "e2" {
		t.Fatalf("continuation = %#v, want stream digest and cursor", first.Snapshot)
	}

	tampered := first.Snapshot
	tampered.NextEntryID = "e3"
	tampered, _, err = SealSnapshot(tampered)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &tampered, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: planBatchID(800)})
	if !errors.Is(err, ErrInvalidValue) || second.Outcome != "" {
		t.Fatalf("PlanCheckpoint() = %#v, %v; want stale cursor rejection", second, err)
	}
}

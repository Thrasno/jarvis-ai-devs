package applyprogress

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPlanCheckpointPrefix(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []EvidenceEntry{planEntry("e1", "1", strings.Repeat("x", 21000)), planEntry("e2", "2", strings.Repeat("x", 21000)), planEntry("e3", "3", "last")}
	result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(1)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PlanContinuationRequired || result.EndIndex != 1 || result.NextEntryIndex != 1 || result.NextEntryID != "e2" {
		t.Fatalf("continuation = %#v, want maximal first-entry prefix and e2 cursor", result)
	}
	assertPlanBounded(t, result)

	all, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks[:1], Entries: entries[:1], EntryID: "e1", BatchID: planBatchID(2)})
	if err != nil || all.Outcome != PlanCommitted || all.NextEntryID != "" || all.EndIndex != 1 {
		t.Fatalf("committed = %#v, %v; want final committed prefix", all, err)
	}
}

func TestPlanCheckpointBindsOrderedStreamDigest(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []EvidenceEntry{
		planEntry("e1", "1", strings.Repeat("é", 21000)),
		planEntry("e2", "2", strings.Repeat("é", 21000)),
		planEntry("e3", "3", "last"),
	}
	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(30)})
	if err != nil || first.Outcome != PlanContinuationRequired {
		t.Fatalf("initial checkpoint = %#v, %v", first, err)
	}
	const wantDigest = "238ac30b188ce33b3a933a007c5bc0c797c28adb2c584ac50191bcc270babed4"
	if first.StreamSHA256 != wantDigest {
		t.Fatalf("stream digest = %q, want deterministic %q", first.StreamSHA256, wantDigest)
	}

	resume := PlanInput{Project: "jarvis", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, BatchID: planBatchID(31), StreamSHA256: first.StreamSHA256}
	if _, err := PlanCheckpoint(resume); err != nil {
		t.Fatalf("unchanged continuation = %v", err)
	}
	missingDigest := resume
	missingDigest.StreamSHA256 = ""
	if _, err := PlanCheckpoint(missingDigest); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("missing continuation digest error = %v, want invalid value", err)
	}
	for _, name := range []string{"reordered", "modified", "truncated", "extended"} {
		t.Run(name, func(t *testing.T) {
			changed := append([]EvidenceEntry(nil), entries...)
			switch name {
			case "reordered":
				changed[0], changed[2] = changed[2], changed[0]
			case "modified":
				changed[0].Summary = "changed"
			case "truncated":
				changed = changed[:2]
			case "extended":
				changed = append(changed, EvidenceEntry{EntryID: "e4", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "extra", Command: "go test", Outcome: OutcomePass, Files: []string{}})
			}
			resume.Entries = changed
			if _, err := PlanCheckpoint(resume); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("changed continuation error = %v, want invalid value", err)
			}
		})
	}
}

func TestPlanCheckpointExactCapacityAndTerminalOutcomes(t *testing.T) {
	for _, target := range []int{MaxDocumentRunes, MaxDocumentRunes + 1} {
		t.Run("batch rune boundary", func(t *testing.T) {
			entry := planEntry("e", "1", "")
			batch, raw, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(3), Entries: []EvidenceEntry{entry}})
			if err != nil {
				t.Fatal(err)
			}
			entry.Summary = strings.Repeat("é", target-utf8.RuneCount(raw))
			result, err := PlanCheckpoint(PlanInput{Project: batch.Project, Change: batch.Change, Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{entry}, EntryID: "e", BatchID: batch.BatchID})
			if err != nil {
				t.Fatal(err)
			}
			want := PlanCommitted
			if target > MaxDocumentRunes {
				want = PlanEvidenceItemTooLarge
			}
			if result.Outcome != want {
				t.Fatalf("outcome = %q, want %q", result.Outcome, want)
			}
			if want == PlanCommitted {
				assertPlanBounded(t, result)
			}
		})
	}

	overflow, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{planEntry("e", "1", strings.Repeat("x", MaxDocumentRunes))}, EntryID: "e", BatchID: planBatchID(4)})
	if err != nil || overflow.Outcome != PlanEvidenceItemTooLarge || overflow.Batch.Entries != nil {
		t.Fatalf("oversized entry = %#v, %v; want no plan", overflow, err)
	}

	base := planNearCapacityBase(t)
	exhausted, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{planEntry("e", "1", "fits")}, EntryID: "e", BatchID: planBatchID(5)})
	if err != nil || exhausted.Outcome != PlanSnapshotCapacityExhausted || exhausted.Batch.Entries != nil {
		t.Fatalf("snapshot exhaustion = %#v, %v; want no plan", exhausted, err)
	}
}

func TestPlanCheckpointCursorAndDeterminism(t *testing.T) {
	input := PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{planEntry("e", "1", "evidence")}, EntryID: "e", BatchID: planBatchID(6)}
	first, err := PlanCheckpoint(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanCheckpoint(input)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("pure repeated plan = %#v, %v; want identical %#v", second, err, first)
	}
	input.EntryIndex, input.EntryID = 1, "e"
	if _, err := PlanCheckpoint(input); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("index mismatch error = %v, want invalid value", err)
	}
	input.EntryIndex, input.EntryID = 0, "wrong"
	if _, err := PlanCheckpoint(input); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("ID mismatch error = %v, want invalid value", err)
	}
	base := first.Snapshot
	base.TaskManifestSHA256 = strings.Repeat("a", 64)
	base, _, err = SealSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	input.EntryIndex, input.EntryID, input.Base = 0, "e", &base
	if _, err := PlanCheckpoint(input); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("manifest mismatch error = %v, want invalid value", err)
	}
}

func TestPlanCheckpointRetains162725RunesInOrder(t *testing.T) {
	tasks, entries := make([]Task, 5), make([]EvidenceEntry, 5)
	for i := range entries {
		id := string(rune('1' + i))
		tasks[i], entries[i] = Task{ID: id, Text: id}, planEntry("e"+id, id, strings.Repeat("é", 32545))
	}
	var base *Snapshot
	streamSHA256 := ""
	cursor, batches := 0, []Batch{}
	for cursor < len(entries) {
		result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: base, Tasks: tasks, Entries: entries, EntryIndex: cursor, EntryID: entries[cursor].EntryID, BatchID: planBatchID(cursor + 10), StreamSHA256: streamSHA256})
		if err != nil {
			t.Fatal(err)
		}
		if result.Outcome != PlanContinuationRequired && result.Outcome != PlanCommitted || result.EndIndex <= cursor {
			t.Fatalf("result = %#v, want forward committed prefix", result)
		}
		assertPlanBounded(t, result)
		if result.Snapshot.Generation != uint64(len(batches)+1) || result.Snapshot.Revision != uint64(len(batches)+1) {
			t.Fatalf("successor coordinates = %d/%d", result.Snapshot.Generation, result.Snapshot.Revision)
		}
		batches, base, cursor, streamSHA256 = append(batches, result.Batch), &result.Snapshot, result.EndIndex, result.StreamSHA256
		if cursor < len(entries) && (result.NextEntryIndex != cursor || result.NextEntryID != entries[cursor].EntryID || len(streamSHA256) != 64) {
			t.Fatalf("continuation cursor = %d/%q/%q, want %d/%q and stream digest", result.NextEntryIndex, result.NextEntryID, streamSHA256, cursor, entries[cursor].EntryID)
		}
	}
	got := make([]EvidenceEntry, 0, len(entries))
	for _, batch := range batches {
		got = append(got, batch.Entries...)
	}
	if !reflect.DeepEqual(got, entries) || utf8.RuneCountInString(strings.Join(entrySummaries(got), "")) != 162725 {
		t.Fatalf("recovered entries lost order/content or rune total: %#v", got)
	}
}

func planEntry(id, task, summary string) EvidenceEntry {
	return EvidenceEntry{EntryID: id, TaskIDs: []string{task}, CompletesTaskIDs: []string{task}, Kind: EvidenceGreen, Summary: summary, Command: "go test", Outcome: OutcomePass, Files: []string{}}
}
func planBatchID(n int) string { return fmt.Sprintf("apb-%032x", n) }
func entrySummaries(entries []EvidenceEntry) []string {
	out := make([]string, len(entries))
	for i := range entries {
		out[i] = entries[i].Summary
	}
	return out
}
func assertPlanBounded(t *testing.T, result PlanResult) {
	t.Helper()
	if utf8.RuneCount(result.BatchJSON) > MaxDocumentRunes || utf8.RuneCount(result.SnapshotJSON) > MaxDocumentRunes {
		t.Fatalf("plan exceeds capacity: %d/%d", utf8.RuneCount(result.BatchJSON), utf8.RuneCount(result.SnapshotJSON))
	}
}
func planNearCapacityBase(t *testing.T) Snapshot {
	t.Helper()
	_, manifest, err := TaskManifest([]Task{{ID: "1", Text: "one"}})
	if err != nil {
		t.Fatal(err)
	}
	base := Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}}
	for i := 0; ; i++ {
		base.Batches = append(base.Batches, BatchRef{BatchID: planBatchID(i + 20), SHA256: strings.Repeat("a", 64)})
		sealed, _, err := SealSnapshot(base)
		if err != nil {
			base.Batches = base.Batches[:len(base.Batches)-1]
			sealed, _, err = SealSnapshot(base)
			if err != nil {
				t.Fatal(err)
			}
			return sealed
		}
	}
}

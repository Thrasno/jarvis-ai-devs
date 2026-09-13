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

func TestPlanCheckpointPreflightsTaskCompletePrefixWithTrailingOversizedEvidence(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}}
	entries := []EvidenceEntry{
		planEntry("complete-task", "1", "task complete"),
		{EntryID: "trailing", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceVerification, Summary: strings.Repeat("x", MaxDocumentRunes), Command: "go test", Outcome: OutcomePass, Files: []string{}},
	}

	result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "complete-task", BatchID: planBatchID(8)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PlanStreamPreflightRequired || result.Capacity == nil || result.Capacity.Document != "batch" {
		t.Fatalf("task-complete prefix = %#v, want no-write preflight at trailing oversized evidence", result)
	}
}

func TestPlanCheckpointSelectsMaximalPrefixAcrossManySmallEntries(t *testing.T) {
	const entryCount = 400
	tasks := []Task{{ID: "1", Text: "one"}}
	entries := make([]EvidenceEntry, entryCount)
	for i := range entries {
		entries[i] = EvidenceEntry{
			EntryID:          fmt.Sprintf("e%d", i),
			TaskIDs:          []string{},
			CompletesTaskIDs: []string{},
			Kind:             EvidenceGreen,
			Summary:          "small",
			Command:          "go test",
			Outcome:          OutcomePass,
			Files:            []string{},
		}
	}

	result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: entries[0].EntryID, BatchID: planBatchID(7)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PlanContinuationRequired || result.EndIndex <= 0 || result.EndIndex >= len(entries) {
		t.Fatalf("result = %#v, want a non-empty maximal continuation prefix", result)
	}
	if !reflect.DeepEqual(result.Batch.Entries, entries[:result.EndIndex]) {
		t.Fatalf("batch entries = %#v, want exact input prefix %#v", result.Batch.Entries, entries[:result.EndIndex])
	}
	_, _, err = SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(7), Entries: entries[:result.EndIndex+1]})
	if !isCapacity(err) {
		t.Fatalf("next whole-entry prefix must exceed batch capacity, error = %v", err)
	}
}

func TestPlanCheckpointPreflightsNewStreamThatCannotReachItsTerminalSnapshot(t *testing.T) {
	entries := make([]EvidenceEntry, 400)
	for i := range entries {
		entries[i] = EvidenceEntry{
			EntryID:          fmt.Sprintf("e%d", i),
			TaskIDs:          []string{},
			CompletesTaskIDs: []string{},
			Kind:             EvidenceVerification,
			Summary:          strings.Repeat("x", 20000),
			Command:          "go test",
			Outcome:          OutcomePass,
			Files:            []string{},
		}
	}

	result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}}, Entries: entries, EntryID: entries[0].EntryID, BatchID: planBatchID(9)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "stream_preflight_required" || result.Batch.Entries != nil || result.Snapshot.Schema != "" || result.Capacity == nil || result.Capacity.Document != "snapshot" {
		t.Fatalf("result = %#v, want no-write stream preflight for future snapshot capacity", result)
	}
}

func TestPlanCheckpointPreflightRejectsFutureOversizedEntry(t *testing.T) {
	entries := []EvidenceEntry{
		planEntry("first", "1", "fits"),
		planEntry("second", "2", strings.Repeat("x", MaxDocumentRunes)),
	}
	result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}}, Entries: entries, EntryID: "first", BatchID: planBatchID(10)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != PlanStreamPreflightRequired || result.Capacity == nil || result.Capacity.Document != "batch" {
		t.Fatalf("result = %#v, want no-write preflight for a future oversized entry", result)
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
	wrongCursor := first.Snapshot
	wrongCursor.NextEntryID = "e1"
	wrongCursor, _, err = SealSnapshot(wrongCursor)
	if err != nil {
		t.Fatal(err)
	}
	wrongResume := resume
	wrongResume.Base, wrongResume.EntryID = &wrongCursor, wrongCursor.NextEntryID
	if _, err := PlanCheckpoint(wrongResume); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("wrong next-entry resume error = %v, want invalid value", err)
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
				changed[1].Summary = "changed"
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

func TestPlanCheckpointRejectsStaleSnapshotCursorAndClonesEntries(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}}
	entries := []EvidenceEntry{planEntry("e1", "1", strings.Repeat("x", 21000)), planEntry("e2", "2", strings.Repeat("x", 21000))}
	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(40)})
	if err != nil || first.Outcome != PlanContinuationRequired {
		t.Fatalf("first checkpoint = %#v, %v", first, err)
	}
	if first.Snapshot.StreamSHA256 != first.StreamSHA256 || first.Snapshot.NextEntryIndex != first.NextEntryIndex || first.Snapshot.NextEntryID != first.NextEntryID {
		t.Fatalf("snapshot continuation identity = %#v, want result identity", first.Snapshot)
	}

	stale := PlanInput{Project: "jarvis", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: 0, EntryID: "e1", BatchID: planBatchID(41), StreamSHA256: first.StreamSHA256}
	if _, err := PlanCheckpoint(stale); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("stale cursor error = %v, want invalid value", err)
	}

	entries[0].Summary = "mutated after planning"
	if first.Batch.Entries[0].Summary == entries[0].Summary {
		t.Fatal("planned batch aliases the caller entry")
	}
}

func TestPlanCheckpointExactCapacityAndTerminalOutcomes(t *testing.T) {
	for _, target := range []int{MaxDocumentRunes, MaxDocumentRunes + 1} {
		t.Run(planBoundaryName(target), func(t *testing.T) {
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
				return
			}
			if result.Capacity == nil || result.Capacity.Document != "batch" || result.Capacity.Runes != target {
				t.Fatalf("batch capacity = %#v, want exact %d-rune batch", result.Capacity, target)
			}
		})
	}

	overflow, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{planEntry("e", "1", strings.Repeat("x", MaxDocumentRunes))}, EntryID: "e", BatchID: planBatchID(4)})
	if err != nil || overflow.Outcome != PlanEvidenceItemTooLarge || overflow.Batch.Entries != nil {
		t.Fatalf("oversized entry = %#v, %v; want no plan", overflow, err)
	}

	base := planNearCapacityBase(t)
	entries := []EvidenceEntry{planEntry("e", "1", "fits")}
	streamSHA256, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	base.StreamSHA256, base.NextEntryIndex, base.NextEntryID = streamSHA256, 0, "e"
	base, _, err = SealSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: []Task{{ID: "1", Text: "one"}}, Entries: entries, EntryID: "e", BatchID: planBatchID(5), StreamSHA256: streamSHA256})
	if err != nil || exhausted.Outcome != PlanSnapshotCapacityExhausted || exhausted.Batch.Entries != nil {
		t.Fatalf("snapshot exhaustion = %#v, %v; want no plan", exhausted, err)
	}
	if exhausted.Capacity == nil || exhausted.Capacity.Document != "snapshot" || exhausted.Capacity.Runes <= MaxDocumentRunes {
		t.Fatalf("snapshot capacity = %#v, want exact overflowing snapshot bounds", exhausted.Capacity)
	}
}

func TestPlanCheckpointGuardsTinyNonterminalBatchesOnlyBeforeStreamBinding(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	entries := []EvidenceEntry{
		planEntry("first", "1", "tiny"),
		planEntry("second", "2", strings.Repeat("x", MaxDocumentRunes)),
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	base := Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: make([]BatchRef, SnapshotCheckpointReferenceGuard), StreamSHA256: stream, NextEntryID: "first"}
	for i := range base.Batches {
		base.Batches[i] = BatchRef{BatchID: planBatchID(10000 + i), SHA256: strings.Repeat("a", 64)}
	}
	base, _, err = SealSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	active, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: tasks, Entries: entries, EntryID: "first", BatchID: planBatchID(20000), StreamSHA256: stream})
	if err != nil || active.Outcome != PlanContinuationRequired || active.Frequency != nil {
		t.Fatalf("active continuation = %#v, %v; want a normal continuation without consolidation", active, err)
	}

	unbound := base
	unbound.StreamSHA256, unbound.NextEntryIndex, unbound.NextEntryID = "", 0, ""
	unbound, _, err = SealSnapshot(unbound)
	if err != nil {
		t.Fatal(err)
	}
	guarded, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &unbound, Tasks: tasks, Entries: entries, EntryID: "first", BatchID: planBatchID(20001)})
	if err != nil || guarded.Outcome != PlanCheckpointConsolidationRequired || guarded.Batch.Entries != nil || guarded.Frequency == nil {
		t.Fatalf("unbound checkpoint = %#v, %v; want no-write consolidation guard", guarded, err)
	}
}

func TestPlanCheckpointSelectsFittingTerminalAfterOverflowingPartialCandidate(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "already complete"}}
	entries := []EvidenceEntry{
		planEntry("e1", "1", "first"),
		planEntry("e2", "2", "second"),
		{EntryID: "e3", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "trailing", Command: "go test", Outcome: OutcomePass, Files: []string{}},
	}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}

	for refs := 250; refs <= 310; refs++ {
		for padding := 1; padding <= 120; padding++ {
			base := Snapshot{
				Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1,
				TaskManifestSHA256: manifest, Status: StatusPartial,
				Coverage: []Coverage{{TaskID: "3", BatchID: planBatchID(1000), EntryID: strings.Repeat("p", padding)}},
				Batches:  make([]BatchRef, refs), StreamSHA256: stream, NextEntryIndex: 0, NextEntryID: "e1",
			}
			for index := range base.Batches {
				base.Batches[index] = BatchRef{BatchID: planBatchID(index + 1000), SHA256: strings.Repeat("a", 64)}
			}
			base, _, err = SealSnapshot(base)
			if err != nil {
				continue
			}
			input := PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(900), StreamSHA256: stream}
			completed := map[string]Coverage{"3": base.Coverage[0]}
			first, firstErr := planCheckpointCandidate(input, base, completed, tasks, manifest, stream, 1, 2, 1)
			_, partialErr := planCheckpointCandidate(input, base, completed, tasks, manifest, stream, 1, 2, 2)
			terminal, terminalErr := planCheckpointCandidate(input, base, completed, tasks, manifest, stream, 1, 2, len(entries))
			if firstErr != nil || !isCapacity(partialErr) || terminalErr != nil {
				continue
			}

			result, err := PlanCheckpoint(input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != PlanCommitted || result.EndIndex != len(entries) || result.Snapshot.Status != StatusComplete {
				t.Fatalf("plan = %#v, want fitting full terminal result after partial overflow", result)
			}
			if first.EndIndex != 1 || terminal.EndIndex != len(entries) {
				t.Fatalf("boundary candidates = %#v / %#v", first, terminal)
			}
			assertPlanBounded(t, result)
			return
		}
	}
	t.Fatal("could not construct a partial-overflow/fitting-terminal boundary")
}

func TestPlanCheckpointCommitsMaximalPrefixAtCanonicalSnapshotBoundary(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}, {ID: "4", Text: "four"}, {ID: "5", Text: "five"}}
	entries := []EvidenceEntry{planEntry("e1", "1", "first"), planEntry("e2", "2", "second"), planEntry("e3", "3", "third")}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	for refs := 304; refs <= 304; refs++ {
		for firstPadding := 47; firstPadding <= 47; firstPadding++ {
			for secondPadding := 64; secondPadding <= 64; secondPadding++ {
				base := Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{{TaskID: "4", BatchID: planBatchID(100), EntryID: strings.Repeat("a", firstPadding)}, {TaskID: "5", BatchID: planBatchID(100), EntryID: strings.Repeat("b", secondPadding)}}, Batches: make([]BatchRef, refs), StreamSHA256: stream, NextEntryID: "e1"}
				for index := range base.Batches {
					base.Batches[index] = BatchRef{BatchID: planBatchID(index + 100), SHA256: strings.Repeat("a", 64)}
				}
				base, _, err = SealSnapshot(base)
				if isCapacity(err) {
					t.Fatal("could not construct a canonical boundary base")
				}
				if err != nil {
					t.Fatal(err)
				}
				firstBatch, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(900), Entries: entries[:1]})
				if err != nil {
					t.Fatal(err)
				}
				firstCoverage := []Coverage{{TaskID: "1", BatchID: firstBatch.BatchID, EntryID: "e1"}, base.Coverage[0], base.Coverage[1]}
				_, _, firstErr := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 2, PreviousDigest: base.Digest, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: firstCoverage, Batches: append(append([]BatchRef{}, base.Batches...), BatchRef{BatchID: firstBatch.BatchID, SHA256: firstBatch.SHA256}), StreamSHA256: stream, NextEntryIndex: 1, NextEntryID: "e2"})
				secondBatch, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(900), Entries: entries[:2]})
				if err != nil {
					t.Fatal(err)
				}
				secondCoverage := []Coverage{{TaskID: "1", BatchID: secondBatch.BatchID, EntryID: "e1"}, {TaskID: "2", BatchID: secondBatch.BatchID, EntryID: "e2"}, base.Coverage[0], base.Coverage[1]}
				_, _, secondErr := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 2, PreviousDigest: base.Digest, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: secondCoverage, Batches: append(append([]BatchRef{}, base.Batches...), BatchRef{BatchID: secondBatch.BatchID, SHA256: secondBatch.SHA256}), StreamSHA256: stream, NextEntryIndex: 2, NextEntryID: "e3"})
				if firstErr != nil || !isCapacity(secondErr) {
					continue
				}
				result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(900), StreamSHA256: stream})
				if err != nil {
					t.Fatal(err)
				}
				if result.Outcome != PlanCommitted || result.EndIndex != len(entries) || result.Snapshot.Status != StatusComplete {
					t.Fatalf("plan = %#v, want the fitting full terminal result", result)
				}
				assertPlanBounded(t, result)
				return
			}
		}
	}
	t.Fatal("could not construct a canonical boundary base")
}

func TestCheckpointSizePlannerMatchesCanonicalDocuments(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}}
	entries := []EvidenceEntry{
		{EntryID: "first", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "<é>", Command: "go test", Outcome: OutcomePass, Files: []string{}},
		planEntry("second", "1", "finishes"),
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	input := PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "first", BatchID: planBatchID(700)}
	base := Snapshot{Batches: []BatchRef{}, Coverage: []Coverage{}}
	planner, err := newCheckpointSizePlanner(input, base, map[string]Coverage{}, tasks, manifest, stream, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	for end := 1; end <= len(entries); end++ {
		t.Run(fmt.Sprintf("prefix-%d", end), func(t *testing.T) {
			candidate, err := planCheckpointCandidate(input, base, map[string]Coverage{}, tasks, manifest, stream, 1, 1, end)
			if err != nil {
				t.Fatal(err)
			}
			batchRunes, snapshotRunes, err := planner.runes(end)
			if err != nil {
				t.Fatal(err)
			}
			if batchRunes != utf8.RuneCount(candidate.BatchJSON) || snapshotRunes != utf8.RuneCount(candidate.SnapshotJSON) {
				t.Fatalf("estimated runes = %d/%d, actual = %d/%d", batchRunes, snapshotRunes, utf8.RuneCount(candidate.BatchJSON), utf8.RuneCount(candidate.SnapshotJSON))
			}
		})
	}
}

func TestCheckpointSizePlannerMatchesIncompleteTerminalSnapshot(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	entries := []EvidenceEntry{planEntry("first", "1", "first task only")}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	input := PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "first", BatchID: planBatchID(701)}
	base := Snapshot{Batches: []BatchRef{}, Coverage: []Coverage{}}
	planner, err := newCheckpointSizePlanner(input, base, map[string]Coverage{}, tasks, manifest, stream, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := planCheckpointCandidate(input, base, map[string]Coverage{}, tasks, manifest, stream, 1, 1, len(entries))
	if err != nil {
		t.Fatal(err)
	}
	_, snapshotRunes, err := planner.runes(len(entries))
	if err != nil {
		t.Fatal(err)
	}
	if snapshotRunes != utf8.RuneCount(candidate.SnapshotJSON) {
		t.Fatalf("estimated incomplete terminal snapshot runes = %d, actual = %d", snapshotRunes, utf8.RuneCount(candidate.SnapshotJSON))
	}
}

func TestPlanCheckpointScansPastAnOverflowingLongContinuationCursor(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}}
	entries := []EvidenceEntry{
		{EntryID: "e1", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "first", Command: "go test", Outcome: OutcomePass, Files: []string{}},
		{EntryID: strings.Repeat("l", 64), TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "second", Command: "go test", Outcome: OutcomePass, Files: []string{}},
		planEntry("e3", "1", strings.Repeat("x", MaxDocumentRunes)),
	}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	for refs := 250; refs < 400; refs++ {
		base := Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: make([]BatchRef, refs), StreamSHA256: stream, NextEntryID: "e1"}
		for i := range base.Batches {
			base.Batches[i] = BatchRef{BatchID: planBatchID(800 + i), SHA256: strings.Repeat("a", 64)}
		}
		base, _, err = SealSnapshot(base)
		if err != nil {
			continue
		}
		input := PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(799), StreamSHA256: stream}
		completed := map[string]Coverage{}
		_, firstErr := planCheckpointCandidate(input, base, completed, tasks, manifest, stream, 1, 2, 1)
		second, secondErr := planCheckpointCandidate(input, base, completed, tasks, manifest, stream, 1, 2, 2)
		if !isCapacity(firstErr) || secondErr != nil {
			continue
		}
		result, err := PlanCheckpoint(input)
		if err != nil {
			t.Fatal(err)
		}
		if result.Outcome != PlanContinuationRequired || result.EndIndex != 2 || result.NextEntryID != "e3" {
			t.Fatalf("plan = %#v, want maximal fitting prefix ending at 2", result)
		}
		if second.EndIndex != 2 {
			t.Fatalf("second candidate = %#v, want prefix 2", second)
		}
		return
	}
	t.Fatal("could not construct long-cursor continuation boundary")
}

func TestUpgradeLegacyContinuationPreservesPreContinuationPrefix(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	prior, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(61), Entries: []EvidenceEntry{planEntry("prior", "1", "prior evidence")}})
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 7, Revision: 9, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{{TaskID: "1", BatchID: prior.BatchID, EntryID: "prior"}}, Batches: []BatchRef{{BatchID: prior.BatchID, SHA256: prior.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the literal pre-continuation v2 shape that serialized its zero
	// continuation group into the digest, then decode it as an upgrade authority.
	base.historicalZeroContinuation = true
	payload, err := canonicalJSON(snapshotPayloadForDigest(base))
	if err != nil {
		t.Fatal(err)
	}
	base.Digest = digest(payload)
	historical, err := snapshotCanonicalJSON(base)
	if err != nil {
		t.Fatal(err)
	}
	base, err = DecodeCanonicalSnapshot(historical)
	if err != nil {
		t.Fatal(err)
	}
	entries := []EvidenceEntry{planEntry("e", "2", "evidence")}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := UpgradeLegacyContinuation(base, tasks, entries, stream)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.Generation != base.Generation || upgraded.Revision != base.Revision+1 || upgraded.PreviousDigest != base.Digest || upgraded.StreamSHA256 != stream || upgraded.NextEntryIndex != 0 || upgraded.NextEntryID != "e" {
		t.Fatalf("upgraded = %#v", upgraded)
	}
	if upgraded.Project != base.Project || upgraded.Change != base.Change || upgraded.Status != base.Status || upgraded.TaskManifestSHA256 != base.TaskManifestSHA256 || !reflect.DeepEqual(upgraded.Coverage, base.Coverage) || !reflect.DeepEqual(upgraded.Batches, base.Batches) {
		t.Fatalf("upgrade changed immutable snapshot content: %#v", upgraded)
	}
}

func TestUpgradeLegacyContinuationRejectsNearCapacityBindingWithoutCursor(t *testing.T) {
	base := planNearCapacityBase(t)
	base.StreamSHA256, base.NextEntryIndex, base.NextEntryID = "", 0, ""
	var err error
	base, _, err = SealSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	base = historicalSnapshotForTest(t, base)
	entries := []EvidenceEntry{planEntry("e", "1", "fits")}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}

	upgraded, err := UpgradeLegacyContinuation(base, []Task{{ID: "1", Text: "one"}}, entries, stream)
	if upgraded.Digest != "" {
		t.Fatalf("near-capacity upgrade returned a publishable cursor: %#v", upgraded)
	}
	var capacity *CapacityError
	if !errors.As(err, &capacity) || capacity.Document != "snapshot" || capacity.Runes <= MaxDocumentRunes {
		t.Fatalf("near-capacity upgrade error = %v, want no-write snapshot capacity rejection", err)
	}
}

func TestUpgradeLegacyContinuationPreflightsFutureOversizedEntry(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	prior, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(63), Entries: []EvidenceEntry{planEntry("prior", "1", "prior evidence")}})
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{{TaskID: "1", BatchID: prior.BatchID, EntryID: "prior"}}, Batches: []BatchRef{{BatchID: prior.BatchID, SHA256: prior.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	base = historicalSnapshotForTest(t, base)
	entries := []EvidenceEntry{planEntry("second", "2", "fits"), planEntry("third", "3", strings.Repeat("x", MaxDocumentRunes))}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UpgradeLegacyContinuation(base, tasks, entries, stream)
	var capacity *CapacityError
	if !errors.As(err, &capacity) || capacity.Document != "batch" {
		t.Fatalf("upgrade error = %v, want a future oversized batch preflight", err)
	}
}

func TestUpgradeLegacyContinuationRejectsStreamThatCannotFinishRemainingTasks(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	prior, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(62), Entries: []EvidenceEntry{planEntry("prior", "1", "prior evidence")}})
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{{TaskID: "1", BatchID: prior.BatchID, EntryID: "prior"}}, Batches: []BatchRef{{BatchID: prior.BatchID, SHA256: prior.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	base = historicalSnapshotForTest(t, base)
	entries := []EvidenceEntry{planEntry("e", "2", "only one remaining task")}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UpgradeLegacyContinuation(base, tasks, entries, stream)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeInvalidPlan {
		t.Fatalf("error = %#v, want invalid plan for uncompletable stream", err)
	}
}

func TestValidateFutureStreamAcceptsCompleteRemainingTasksInEvidenceOrder(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "imported"}, {ID: "2", Text: "second"}, {ID: "3", Text: "third"}}
	existing := []Coverage{{TaskID: "1", BatchID: planBatchID(64), EntryID: "legacy"}}
	entries := []EvidenceEntry{
		planEntry("third", "3", "completed before second"),
		planEntry("second", "2", "completed after third"),
	}

	if err := ValidateFutureStream(tasks, entries, existing); err != nil {
		t.Fatalf("ValidateFutureStream() = %v, want complete future coverage to be accepted regardless of evidence order", err)
	}
}

func TestPlanCheckpointRejectsImportedEvidenceAndWarnsBeforeSnapshotExhaustion(t *testing.T) {
	imported := planEntry("legacy", "1", "legacy")
	imported.Kind = EvidenceImported
	if _, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{imported}, EntryID: "legacy", BatchID: planBatchID(63)}); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("ordinary imported checkpoint error = %v, want invalid value", err)
	}

	entry := planEntry("near-capacity", "1", "evidence")
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
		result, planErr := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{entry}, EntryID: entry.EntryID, BatchID: planBatchID(9000), StreamSHA256: stream})
		if planErr == nil && result.Warning != nil {
			if result.Warning.Document != "snapshot" || result.Warning.Runes <= 0 || result.Warning.Remaining <= 0 || result.Warning.Threshold != MaxDocumentRunes*SnapshotCapacityWarningPercent/100 {
				t.Fatalf("pre-exhaustion warning = %#v, want structured snapshot warning with threshold", result.Warning)
			}
			return
		}
	}
	t.Fatal("could not construct a warning-sized successor snapshot")
}

func TestPlanCheckpointReportsTypedPlannerValidation(t *testing.T) {
	input := PlanInput{Project: "jarvis", Change: "change", Tasks: []Task{{ID: "1", Text: "one"}}, Entries: []EvidenceEntry{planEntry("e", "1", "evidence")}, EntryID: "wrong", BatchID: planBatchID(60)}
	_, err := PlanCheckpoint(input)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeInvalidCursor || validation.Detail == "" {
		t.Fatalf("error = %#v, want typed cursor validation", err)
	}
	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("error = %v, want historical invalid value sentinel", err)
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
		if result.Snapshot.Generation != 1 || result.Snapshot.Revision != uint64(len(batches)+1) {
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
	rawBatches := make(map[string][]byte, len(batches))
	for _, batch := range batches {
		_, data, err := SealBatch(batch)
		if err != nil {
			t.Fatal(err)
		}
		rawBatches[batch.BatchID] = data
	}
	_, snapshotData, err := SealSnapshot(*base)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProgress(snapshotData, tasks, rawBatches); err != nil {
		t.Fatalf("ValidateProgress() = %v, want committed continuation lineage", err)
	}
}

func TestPlanCheckpointCommitsExhaustedIncompleteStreamForAFutureStream(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	firstStream := []EvidenceEntry{planEntry("first", "1", "first task only")}

	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: firstStream, EntryID: "first", BatchID: planBatchID(910)})
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != PlanCommitted || first.Snapshot.Status != StatusPartial || first.Snapshot.StreamSHA256 != "" || first.Snapshot.NextEntryIndex != 0 || first.Snapshot.NextEntryID != "" {
		t.Fatalf("exhausted incomplete stream = %#v, want committed unbound partial snapshot", first)
	}
	assertPlanBounded(t, first)

	secondStream := []EvidenceEntry{planEntry("second", "2", "remaining task")}
	second, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: secondStream, EntryID: "second", BatchID: planBatchID(911)})
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != PlanCommitted || second.Snapshot.Status != StatusComplete {
		t.Fatalf("future stream = %#v, want completed successor", second)
	}
}

func TestPlanCheckpointProtectsIncompleteUnboundTerminalAtInjectableSafeThreshold(t *testing.T) {
	previous := checkpointConsolidationSafeRunes
	checkpointConsolidationSafeRunes = 1
	t.Cleanup(func() { checkpointConsolidationSafeRunes = previous })

	tasks := []Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	incomplete := []EvidenceEntry{planEntry("first", "1", "only first task")}
	guarded, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: incomplete, EntryID: "first", BatchID: planBatchID(912)})
	if err != nil {
		t.Fatal(err)
	}
	if guarded.Outcome != PlanCheckpointConsolidationRequired || guarded.Frequency == nil {
		t.Fatalf("exhausted incomplete stream = %#v, want executable consolidation guard", guarded)
	}

	complete := []EvidenceEntry{planEntry("first", "1", "first task"), planEntry("second", "2", "second task")}
	terminal, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: complete, EntryID: "first", BatchID: planBatchID(913)})
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Outcome != PlanCommitted || terminal.Snapshot.Status != StatusComplete {
		t.Fatalf("complete terminal = %#v, want full bounded terminal commit", terminal)
	}

	checkpointConsolidationSafeRunes = MaxDocumentRunes
	progressed, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: incomplete, EntryID: "first", BatchID: planBatchID(914)})
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Outcome != PlanCommitted || progressed.Snapshot.Status != StatusPartial {
		t.Fatalf("after consolidation threshold = %#v, want same stream to progress", progressed)
	}
}

func TestPlanCheckpointResumesExistingLegacyContinuationWithLegacyPlanner(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []EvidenceEntry{
		planEntry("e1", "1", "historical first entry"),
		planEntry("e2", "2", strings.Repeat("x", 21000)),
		planEntry("e3", "3", strings.Repeat("x", 21000)),
	}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	prior, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: planBatchID(918), Entries: []EvidenceEntry{entries[0]}})
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{{TaskID: "1", BatchID: prior.BatchID, EntryID: "e1"}}, Batches: []BatchRef{{BatchID: prior.BatchID, SHA256: prior.SHA256}}, StreamSHA256: stream, NextEntryIndex: 1, NextEntryID: "e2"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &base, Tasks: tasks, Entries: entries, EntryIndex: 1, EntryID: "e2", StreamSHA256: stream, BatchID: planBatchID(919)})
	if err != nil || result.Outcome != PlanContinuationRequired {
		t.Fatalf("legacy continuation = %#v, %v; want a compatible continuation", result, err)
	}
}

func planBoundaryName(target int) string {
	if target == MaxDocumentRunes {
		return "accepts batch at exact rune ceiling"
	}
	return "rejects batch above rune ceiling"
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
	base := Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}, StreamSHA256: strings.Repeat("a", 64), NextEntryID: "e"}
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

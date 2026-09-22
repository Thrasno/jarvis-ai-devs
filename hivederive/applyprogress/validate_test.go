package applyprogress

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSupersessionSealTransition(t *testing.T) {
	previous := Snapshot{Schema: SnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}, StreamSHA256: strings.Repeat("b", 64), NextEntryID: "next"}
	previous, _, err := SealSnapshot(previous)
	if err != nil {
		t.Fatal(err)
	}
	intent := &SealIntent{SuccessorProject: "project", SuccessorChange: "new", SuccessorManifestSHA256: strings.Repeat("c", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "request-1"}
	candidate := previous
	candidate.Schema = SupersessionSnapshotSchema
	candidate.Revision++
	candidate.PreviousDigest = previous.Digest
	candidate.Status = StatusSuperseded
	candidate.StreamSHA256, candidate.NextEntryIndex, candidate.NextEntryID = "", 0, ""
	candidate.SealIntent = intent
	candidate, raw, err := SealSnapshot(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !IsSupersessionSeal(previous, candidate) {
		t.Fatal("expected exact seal shape")
	}
	if err := ValidateSuccessor(previous, candidate, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "next_entry_") || strings.Contains(string(raw), "stream_sha256") {
		t.Fatalf("terminal seal retained continuation: %s", raw)
	}
	if _, err := DecodeCanonicalSnapshot(raw); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Snapshot){
		"new evidence": func(s *Snapshot) {
			s.Batches = append(s.Batches, BatchRef{BatchID: "apb-0123456789abcdef0123456789abcdef", SHA256: strings.Repeat("d", 64)})
		},
		"changed coverage": func(s *Snapshot) {
			s.Coverage = append(s.Coverage, Coverage{TaskID: "task", BatchID: "apb-0123456789abcdef0123456789abcdef", EntryID: "entry"})
		},
		"changed manifest":      func(s *Snapshot) { s.TaskManifestSHA256 = strings.Repeat("d", 64) },
		"retained continuation": func(s *Snapshot) { s.StreamSHA256, s.NextEntryID = previous.StreamSHA256, previous.NextEntryID },
		"changed epoch":         func(s *Snapshot) { s.Generation++ },
	} {
		t.Run(name, func(t *testing.T) {
			altered := candidate
			mutate(&altered)
			altered, _, err := SealSnapshot(altered)
			if err == nil && (IsSupersessionSeal(previous, altered) || ValidateSuccessor(previous, altered, nil) == nil) {
				t.Fatal("accepted altered seal")
			}
		})
	}
	badIntent := candidate
	badIntent.SealIntent = &SealIntent{SuccessorProject: "project", SuccessorChange: "new", SuccessorManifestSHA256: strings.Repeat("c", 64), Actor: "agent", Reason: "replanned", Timestamp: "not-a-timestamp", OperationID: "request-1"}
	if _, _, err := SealSnapshot(badIntent); err == nil {
		t.Fatal("accepted malformed intent timestamp")
	}
	if err := ValidateSuccessor(candidate, candidate, nil); err == nil {
		t.Fatal("extended superseded head")
	}
	ordinary := candidate
	ordinary.Schema = SnapshotSchema
	ordinary.Status = StatusPartial
	ordinary.SealIntent = nil
	ordinary, _, err = SealSnapshot(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(previous, ordinary, nil); err == nil {
		t.Fatal("ordinary advance discarded continuation")
	}
}

// Pair validation uses supplied snapshots; these tests do not establish store authority.
func TestValidateSuccessorGenesisPairShape(t *testing.T) {
	predecessor := Snapshot{Schema: SupersessionSnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 2, TaskManifestSHA256: strings.Repeat("a", 64), Status: StatusSuperseded, Coverage: []Coverage{}, Batches: []BatchRef{}, SealIntent: &SealIntent{SuccessorProject: "project", SuccessorChange: "new", SuccessorManifestSHA256: strings.Repeat("c", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "request-1"}}
	predecessor, _, err := SealSnapshot(predecessor)
	if err != nil {
		t.Fatal(err)
	}
	pointer := &SupersedesPointer{Project: predecessor.Project, Change: predecessor.Change, SealDigest: predecessor.Digest, OriginalManifestSHA256: predecessor.TaskManifestSHA256, Actor: predecessor.SealIntent.Actor, Reason: predecessor.SealIntent.Reason, Timestamp: predecessor.SealIntent.Timestamp, OperationID: predecessor.SealIntent.OperationID}
	root := Snapshot{Schema: SupersessionSnapshotSchema, Project: "project", Change: "new", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("c", 64), Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}, Supersedes: pointer}
	check := func(name string, before, after Snapshot, valid bool) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if got := ValidateSuccessorGenesisPair(before, after); (got == nil) != valid {
				t.Fatalf("validation = %v, want valid %v", got, valid)
			}
		})
	}
	seal := func(s Snapshot) Snapshot {
		t.Helper()
		sealed, _, err := SealSnapshot(s)
		if err != nil {
			t.Fatalf("seal snapshot %+v: %v", s, err)
		}
		return sealed
	}
	check("matching pair shape", predecessor, seal(root), true)
	for _, tt := range []struct {
		name string
		edit func(*Snapshot)
	}{
		{"pointer digest", func(s *Snapshot) { p := *s.Supersedes; p.SealDigest = strings.Repeat("d", 64); s.Supersedes = &p }},
		{"pointer identity", func(s *Snapshot) { p := *s.Supersedes; p.Change = "other"; s.Supersedes = &p }},
		{"pointer intent", func(s *Snapshot) { p := *s.Supersedes; p.Reason = "other"; s.Supersedes = &p }},
		{"pointer manifest", func(s *Snapshot) {
			p := *s.Supersedes
			p.OriginalManifestSHA256 = strings.Repeat("d", 64)
			s.Supersedes = &p
		}},
		{"project", func(s *Snapshot) { s.Project = "other" }},
		{"change", func(s *Snapshot) { s.Change = "other" }},
		{"manifest", func(s *Snapshot) { s.TaskManifestSHA256 = strings.Repeat("d", 64) }},
		{"generation", func(s *Snapshot) { s.Generation = 2 }},
		{"revision", func(s *Snapshot) { s.Revision = 2 }},
		{"previous digest", func(s *Snapshot) { s.PreviousDigest = predecessor.Digest }},
		{"coverage", func(s *Snapshot) { s.Coverage = []Coverage{{TaskID: "task", BatchID: "batch", EntryID: "entry"}} }},
		{"evidence", func(s *Snapshot) {
			s.Batches = []BatchRef{{BatchID: "apb-0123456789abcdef0123456789abcdef", SHA256: strings.Repeat("d", 64)}}
		}},
		{"continuation", func(s *Snapshot) { s.StreamSHA256 = strings.Repeat("d", 64); s.NextEntryID = "next" }},
	} {
		s := root
		tt.edit(&s)
		sealed, _, err := SealSnapshot(s)
		if err != nil {
			// Some cross-change mismatches also violate local snapshot shape.
			check(tt.name, predecessor, s, false)
		} else {
			check(tt.name, predecessor, sealed, false)
		}
	}
	broken := predecessor
	broken.Digest = strings.Repeat("d", 64)
	check("invalid predecessor digest", broken, seal(root), false)
	changed := predecessor
	intent := *changed.SealIntent
	intent.Reason = "changed"
	changed.SealIntent = &intent
	changed = seal(changed)
	check("changed signed intent", changed, seal(root), false)
	check("same change remains strict", predecessor, predecessor, false)
}

func TestSuccessorPointerLocalShape(t *testing.T) {
	root := Snapshot{Schema: SupersessionSnapshotSchema, Project: "project", Change: "new", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("c", 64), Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}, Supersedes: &SupersedesPointer{Project: "project", Change: "old", SealDigest: strings.Repeat("d", 64), OriginalManifestSHA256: strings.Repeat("a", 64), Actor: "agent", Reason: "replanned", OperationID: "request-1", Timestamp: "2026-01-01T00:00:00Z"}}
	sealed, raw, err := SealSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeCanonicalSnapshot(raw); err != nil || decoded.Supersedes == nil || *decoded.Supersedes != *sealed.Supersedes {
		t.Fatalf("pointer roundtrip: %v", err)
	}
	for name, mutate := range map[string]func(*SupersedesPointer){
		"missing original manifest":   func(p *SupersedesPointer) { p.OriginalManifestSHA256 = "" },
		"malformed original manifest": func(p *SupersedesPointer) { p.OriginalManifestSHA256 = "invalid" },
		"missing actor":               func(p *SupersedesPointer) { p.Actor = "" },
		"missing reason":              func(p *SupersedesPointer) { p.Reason = "" },
		"missing request identity":    func(p *SupersedesPointer) { p.OperationID = "" },
		"invalid request identity":    func(p *SupersedesPointer) { p.OperationID = "not valid" },
		"missing timestamp":           func(p *SupersedesPointer) { p.Timestamp = "" },
		"malformed timestamp":         func(p *SupersedesPointer) { p.Timestamp = "tomorrow" },
	} {
		t.Run(name, func(t *testing.T) {
			broken := root
			pointer := *root.Supersedes
			mutate(&pointer)
			broken.Supersedes = &pointer
			if _, _, err := SealSnapshot(broken); err == nil {
				t.Fatal("accepted malformed successor pointer")
			}
		})
	}
	root.Supersedes.SealDigest = "invalid"
	if _, _, err := SealSnapshot(root); err == nil {
		t.Fatal("accepted invalid signed pointer")
	}
	root.Supersedes.SealDigest = strings.Repeat("d", 64)
	root.Schema = SnapshotSchema
	if _, _, err := SealSnapshot(root); err == nil {
		t.Fatal("accepted pointer on v2")
	}
	root.Schema = SupersessionSnapshotSchema
	root, _, err = SealSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	advance := root
	advance.Revision++
	advance.PreviousDigest = root.Digest
	advance.Supersedes = &SupersedesPointer{Project: "project", Change: "other", SealDigest: strings.Repeat("d", 64), OriginalManifestSHA256: strings.Repeat("a", 64), Actor: "agent", Reason: "replanned", OperationID: "request-1", Timestamp: "2026-01-01T00:00:00Z"}
	advance, _, err = SealSnapshot(advance)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(root, advance, nil); err == nil {
		t.Fatal("accepted pointer rewrite")
	}
	advance.Supersedes = root.Supersedes
	advance, _, err = SealSnapshot(advance)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(root, advance, nil); err == nil {
		t.Fatal("accepted empty ordinary advance")
	}
}

func TestTaskManifestNormalizesAndDerivesLegacyIDs(t *testing.T) {
	input := []Task{{ID: "1.1", Text: "  RED\r\n test\tcase  "}, {Path: "1.2", Text: "cafe\u0301"}}
	tasks, digest, err := TaskManifest(input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tasks[0].Text, "RED test case"; got != want {
		t.Fatalf("normalized text = %q, want %q", got, want)
	}
	if got, want := tasks[1].ID, "legacy-c50b87d6a5edbaa53b6b55518b50e83e"; got != want {
		t.Fatalf("legacy ID = %q, want %q", got, want)
	}
	if digest != "20dc785e8ebcc7d2789fbe1db616df1fbdc2496bb58edf563ecb66e910bc9d4a" {
		t.Fatalf("manifest digest = %q, want exact canonical SHA-256", digest)
	}
	reorderedTasks, reordered, err := TaskManifest([]Task{input[1], input[0]})
	if err != nil || reordered == digest || reorderedTasks[0].ID != tasks[1].ID {
		t.Fatalf("reordered manifest digest/error = %q/%v, want changed/nil", reordered, err)
	}
	_, changed, err := TaskManifest([]Task{{ID: "1.1", Text: "RED test case"}, {Path: "1.2", Text: "café!"}})
	if err != nil || changed == digest {
		t.Fatalf("edited manifest digest/error = %q/%v, want changed/nil", changed, err)
	}
}

func TestValidateProgress(t *testing.T) {
	type testCase struct {
		name  string
		edit  func(*Snapshot, map[string][]byte)
		batch func(*Batch)
		want  ValidationCode
	}
	for _, tt := range []testCase{
		{"accepts ordered full coverage", nil, nil, ""},
		{"rejects reordered coverage", func(s *Snapshot, _ map[string][]byte) { s.Coverage[0], s.Coverage[1] = s.Coverage[1], s.Coverage[0] }, nil, CodeInvalidCoverage},
		{"accepts manifest coverage after reversed evidence", nil, func(b *Batch) { b.Entries[0], b.Entries[1] = b.Entries[1], b.Entries[0] }, ""},
		{"rejects evidence-ordered coverage after reversed evidence", func(s *Snapshot, _ map[string][]byte) { s.Coverage[0], s.Coverage[1] = s.Coverage[1], s.Coverage[0] }, func(b *Batch) { b.Entries[0], b.Entries[1] = b.Entries[1], b.Entries[0] }, CodeInvalidCoverage},
		{"rejects missing referenced batch", func(s *Snapshot, batches map[string][]byte) { delete(batches, s.Batches[0].BatchID) }, nil, CodeMissingBatch},
		{"rejects corrupt referenced batch", func(s *Snapshot, batches map[string][]byte) {
			batches[s.Batches[0].BatchID] = append(batches[s.Batches[0].BatchID], ' ')
		}, nil, CodeCorruptBatch},
		{"rejects duplicate batch reference", func(s *Snapshot, _ map[string][]byte) { s.Batches = append(s.Batches, s.Batches[0]) }, nil, CodeDuplicateEvidence},
		{"rejects referenced hash mismatch", func(s *Snapshot, _ map[string][]byte) { s.Batches[0].SHA256 = strings.Repeat("d", 64) }, nil, CodeCorruptBatch},
		{"rejects duplicate completed task", nil, func(b *Batch) { b.Entries[1].TaskIDs, b.Entries[1].CompletesTaskIDs = []string{"1.1"}, []string{"1.1"} }, CodeDuplicateEvidence},
		{"rejects duplicate entry ID", nil, func(b *Batch) { b.Entries[1].EntryID = b.Entries[0].EntryID }, CodeDuplicateEvidence},
		{"rejects duplicate entry ID across batches", func(s *Snapshot, batches map[string][]byte) {
			batch := mustBatch(t, batches)
			batch.BatchID = "apb-ffffffffffffffffffffffffffffffff"
			batch.Entries = batch.Entries[:1]
			sealed, raw, err := SealBatch(batch)
			if err != nil {
				t.Fatal(err)
			}
			s.Batches = append(s.Batches, BatchRef{BatchID: sealed.BatchID, SHA256: sealed.SHA256})
			batches[sealed.BatchID] = raw
		}, nil, CodeDuplicateEvidence},
		{"rejects unknown attribution", nil, func(b *Batch) { b.Entries[1].TaskIDs = []string{"unknown"} }, CodeInvalidCoverage},
		{"rejects repeated attribution", nil, func(b *Batch) { b.Entries[1].TaskIDs = []string{"1.2", "1.2"} }, CodeInvalidCoverage},
		{"rejects completion without attribution", nil, func(b *Batch) { b.Entries[1].CompletesTaskIDs = []string{"1.1"} }, CodeInvalidCoverage},
		{"rejects repeated completion", nil, func(b *Batch) { b.Entries[1].CompletesTaskIDs = []string{"1.2", "1.2"} }, CodeDuplicateEvidence},
		{"rejects batch project mismatch", nil, func(b *Batch) { b.Project = "other" }, CodeCorruptBatch},
		{"rejects batch change mismatch", nil, func(b *Batch) { b.Change = "other" }, CodeCorruptBatch},
		{"rejects batch identity mismatch", nil, func(b *Batch) { b.BatchID = "apb-ffffffffffffffffffffffffffffffff" }, CodeCorruptBatch},
		{"rejects changed task manifest", func(s *Snapshot, _ map[string][]byte) { s.TaskManifestSHA256 = strings.Repeat("c", 64) }, nil, CodeTaskManifestMismatch},
		{"rejects partial claiming complete coverage without continuation", func(s *Snapshot, _ map[string][]byte) { s.Status = StatusPartial }, nil, CodeInvalidCoverage},
		{"accepts partial full coverage with continuation", func(s *Snapshot, _ map[string][]byte) {
			s.Status = StatusPartial
			s.StreamSHA256, s.NextEntryIndex, s.NextEntryID = strings.Repeat("a", 64), 1, "trailing"
		}, nil, ""},
		{"rejects complete with incomplete coverage", func(s *Snapshot, _ map[string][]byte) { s.Coverage = s.Coverage[:1] }, func(b *Batch) { b.Entries[1].CompletesTaskIDs = []string{} }, CodeInvalidCoverage},
		{"accepts valid partial coverage", func(s *Snapshot, _ map[string][]byte) { s.Coverage = s.Coverage[:1]; s.Status = StatusPartial }, func(b *Batch) { b.Entries[1].CompletesTaskIDs = []string{} }, ""},
		{"ignores corrupt orphan batch", func(_ *Snapshot, batches map[string][]byte) {
			batches["apb-ffffffffffffffffffffffffffffffff"] = []byte("corrupt")
		}, nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, tasks, batches := validatedFixture(t)
			if tt.edit != nil {
				tt.edit(&snapshot, batches)
			}
			if tt.batch != nil {
				batch := mustBatch(t, batches)
				tt.batch(&batch)
				replaceReferencedBatch(t, &snapshot, batches, batch)
			}
			sealed, raw, err := SealSnapshot(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if err := VerifySnapshot(sealed); err != nil {
				t.Fatal(err)
			}
			err = ValidateProgress(raw, tasks, batches)
			if tt.want == "" && err == nil {
				return
			}
			var outcome *ValidationError
			if !errors.As(err, &outcome) || outcome.Code != tt.want {
				t.Fatalf("ValidateProgress() error = %#v, want code %q", err, tt.want)
			}
		})
	}
}

func TestValidationErrorsIdentifyInvalidTaskAndReferencedBatch(t *testing.T) {
	_, _, err := TaskManifest([]Task{{ID: "not a valid ID", Text: "task"}})
	var invalidTask *ValidationError
	if !errors.As(err, &invalidTask) || invalidTask.Code != CodeInvalidTask || invalidTask.Detail != "task 0" {
		t.Fatalf("TaskManifest() error = %#v, want invalid task 0", err)
	}

	snapshot, tasks, batches := validatedFixture(t)
	delete(batches, snapshot.Batches[0].BatchID)
	err = ValidateProgress(mustSnapshot(t, snapshot), tasks, batches)
	var missing *ValidationError
	if !errors.As(err, &missing) || missing.Code != CodeMissingBatch || missing.Detail != snapshot.Batches[0].BatchID {
		t.Fatalf("ValidateProgress() error = %#v, want missing batch detail", err)
	}
}

func TestValidateEvidenceCoverage(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*Snapshot, map[string]Batch)
		want ValidationCode
	}{
		{"accepts matching completed evidence", nil, ""},
		{"accepts partial matching completed evidence", func(snapshot *Snapshot, batches map[string]Batch) {
			snapshot.Status = StatusPartial
			snapshot.Coverage = snapshot.Coverage[:1]
			for id, batch := range batches {
				batch.Entries[1].CompletesTaskIDs = []string{}
				batches[id] = batch
			}
		}, ""},
		{"rejects completed evidence omitted from partial coverage", func(snapshot *Snapshot, _ map[string]Batch) {
			snapshot.Status = StatusPartial
			snapshot.Coverage = []Coverage{}
		}, CodeInvalidCoverage},
		{"rejects coverage without matching completion", func(snapshot *Snapshot, _ map[string]Batch) {
			snapshot.Coverage[0].EntryID = "green"
		}, CodeInvalidCoverage},
		{"rejects complete snapshot without coverage or batches", func(snapshot *Snapshot, batches map[string]Batch) {
			snapshot.Coverage = []Coverage{}
			snapshot.Batches = []BatchRef{}
			clear(batches)
		}, CodeInvalidCoverage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, _, rawBatches := validatedFixture(t)
			batches := map[string]Batch{}
			for id, raw := range rawBatches {
				batches[id] = mustBatch(t, map[string][]byte{id: raw})
			}
			if tt.edit != nil {
				tt.edit(&snapshot, batches)
			}
			err := ValidateEvidenceCoverage(snapshot, batches)
			if tt.want == "" && err == nil {
				return
			}
			var outcome *ValidationError
			if !errors.As(err, &outcome) || outcome.Code != tt.want {
				t.Fatalf("ValidateEvidenceCoverage() error = %#v, want code %q", err, tt.want)
			}
		})
	}
}

func validatedFixture(t *testing.T) (Snapshot, []Task, map[string][]byte) {
	t.Helper()
	tasks, manifest, err := TaskManifest([]Task{{ID: "1.1", Text: "RED"}, {ID: "1.2", Text: "GREEN"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, raw, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis-dev", Change: "issue-653", BatchID: "apb-0123456789abcdef0123456789abcdef", Entries: []EvidenceEntry{
		{EntryID: "red", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: EvidenceRed, Summary: "red", Command: "go test", Outcome: OutcomeFail, Files: []string{}},
		{EntryID: "green", TaskIDs: []string{"1.2"}, CompletesTaskIDs: []string{"1.2"}, Kind: EvidenceGreen, Summary: "green", Command: "go test", Outcome: OutcomePass, Files: []string{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: batch.Project, Change: batch.Change, TaskManifestSHA256: manifest, Status: StatusComplete, Coverage: []Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "red"}, {TaskID: "1.2", BatchID: batch.BatchID, EntryID: "green"}}, Batches: []BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, tasks, map[string][]byte{batch.BatchID: raw}
}

func mustSnapshot(t *testing.T, snapshot Snapshot) []byte {
	t.Helper()
	_, raw, err := SealSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustBatch(t *testing.T, batches map[string][]byte) Batch {
	t.Helper()
	for _, raw := range batches {
		b, err := DecodeCanonicalBatch(raw)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	t.Fatal("missing fixture batch")
	return Batch{}
}
func replaceReferencedBatch(t *testing.T, snapshot *Snapshot, batches map[string][]byte, batch Batch) {
	t.Helper()
	sealed, raw, err := SealBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Batches[0].SHA256 = sealed.SHA256
	batches[snapshot.Batches[0].BatchID] = raw
}

func TestValidateSuccessorAcceptsOnlyLegacyEpochTransitionsBeforeContinuation(t *testing.T) {
	_, manifest, err := TaskManifest([]Task{{ID: "1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	predecessor, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	batch, _, err := SealBatch(Batch{Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: "apb-00000000000000000000000000000001", Entries: []EvidenceEntry{{EntryID: "legacy-1", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "legacy", Command: "import", Outcome: OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	legacySuccessor := cloneSnapshot(predecessor)
	legacySuccessor.Generation, legacySuccessor.Revision, legacySuccessor.PreviousDigest = 2, 2, predecessor.Digest
	legacySuccessor.Batches = []BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}
	legacySuccessor, _, err = SealSnapshot(legacySuccessor)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(predecessor, legacySuccessor, map[string]Batch{batch.BatchID: batch}); err != nil {
		t.Fatalf("legacy-v2 epoch transition = %v", err)
	}
	imported := batch
	imported.Entries[0].Kind = EvidenceImported
	imported, _, err = SealBatch(imported)
	if err != nil {
		t.Fatal(err)
	}
	importedSuccessor := cloneSnapshot(legacySuccessor)
	importedSuccessor.Batches[0].SHA256 = imported.SHA256
	importedSuccessor, _, err = SealSnapshot(importedSuccessor)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(predecessor, importedSuccessor, map[string]Batch{imported.BatchID: imported}); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("imported v2 successor error = %v, want invalid successor", err)
	} else {
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != CodeLegacyMigration {
			t.Fatalf("imported v2 successor validation = %#v, want legacy migration", err)
		}
	}

	upgraded := cloneSnapshot(legacySuccessor)
	upgraded.Revision, upgraded.PreviousDigest = 3, legacySuccessor.Digest
	upgraded.StreamSHA256, upgraded.NextEntryIndex, upgraded.NextEntryID = strings.Repeat("a", 64), 0, "entry-1"
	upgraded, _, err = SealSnapshot(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(legacySuccessor, upgraded, map[string]Batch{}); err != nil {
		t.Fatalf("continuation upgrade after legacy epoch = %v", err)
	}

	changedEpoch := cloneSnapshot(upgraded)
	changedEpoch.Generation, changedEpoch.Revision, changedEpoch.PreviousDigest = 3, 4, upgraded.Digest
	changedEpoch, _, err = SealSnapshot(changedEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(upgraded, changedEpoch, map[string]Batch{}); err == nil {
		t.Fatal("accepted epoch change after continuation identity exists")
	}
}

func TestValidateSuccessorPreservesPriorCoverageAcrossManifestOrderedInsertion(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}, {ID: "3", Text: "third"}}
	entries := []EvidenceEntry{
		planEntry("third", "3", strings.Repeat("x", 21000)),
		planEntry("first", "1", strings.Repeat("x", 21000)),
		planEntry("second", "2", strings.Repeat("x", 21000)),
	}
	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "third", BatchID: planBatchID(91)})
	if err != nil || first.Outcome != PlanContinuationRequired {
		t.Fatalf("first checkpoint = %#v, %v", first, err)
	}
	second, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: planBatchID(92)})
	if err != nil || second.Outcome != PlanContinuationRequired {
		t.Fatalf("second checkpoint = %#v, %v", second, err)
	}
	if got, want := second.Snapshot.Coverage, []Coverage{{TaskID: "1", BatchID: second.Batch.BatchID, EntryID: "first"}, {TaskID: "3", BatchID: first.Batch.BatchID, EntryID: "third"}}; !slices.Equal(got, want) {
		t.Fatalf("manifest-ordered coverage = %#v, want %#v", got, want)
	}
	if err := ValidateSuccessor(first.Snapshot, second.Snapshot, map[string]Batch{first.Batch.BatchID: first.Batch, second.Batch.BatchID: second.Batch}); err != nil {
		t.Fatalf("manifest-ordered insertion successor = %v", err)
	}

	for _, name := range []string{"dropped", "changed", "duplicated"} {
		t.Run(name, func(t *testing.T) {
			invalidSuccessor := cloneSnapshot(second.Snapshot)
			switch name {
			case "dropped":
				invalidSuccessor.Coverage = invalidSuccessor.Coverage[:1]
			case "changed":
				invalidSuccessor.Coverage[1].EntryID = "rewritten"
			case "duplicated":
				invalidSuccessor.Coverage = append(invalidSuccessor.Coverage, first.Snapshot.Coverage[0])
			}
			invalidSuccessor, _, err = SealSnapshot(invalidSuccessor)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateSuccessor(first.Snapshot, invalidSuccessor, map[string]Batch{first.Batch.BatchID: first.Batch, second.Batch.BatchID: second.Batch}); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("%s successor error = %v, want invalid successor", name, err)
			}
		})
	}
}

func TestValidateSuccessorBindsCursorToAppendedEvidence(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []EvidenceEntry{
		planEntry("e1", "1", strings.Repeat("x", 21000)),
		planEntry("e2", "2", strings.Repeat("x", 21000)),
		planEntry("e3", "3", strings.Repeat("x", 21000)),
	}
	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(72)})
	if err != nil || first.Outcome != PlanContinuationRequired {
		t.Fatalf("first checkpoint = %#v, %v", first, err)
	}
	second, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: planBatchID(73)})
	if err != nil || second.Outcome != PlanContinuationRequired {
		t.Fatalf("second checkpoint = %#v, %v", second, err)
	}
	batches := map[string]Batch{second.Batch.BatchID: second.Batch}
	if err := ValidateSuccessor(first.Snapshot, second.Snapshot, batches); err != nil {
		t.Fatalf("ValidateSuccessor() valid continuation error = %v", err)
	}

	for _, tt := range []struct {
		name string
		edit func(*Snapshot)
	}{
		{"cursor only", func(s *Snapshot) { s.Batches = slices.Clone(first.Snapshot.Batches) }},
		{"over advance", func(s *Snapshot) { s.NextEntryIndex, s.NextEntryID = 3, "e3" }},
		{"under advance", func(s *Snapshot) { s.NextEntryIndex, s.NextEntryID = 1, "e2" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			successor := cloneSnapshot(second.Snapshot)
			tt.edit(&successor)
			sealed, _, err := SealSnapshot(successor)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateSuccessor(first.Snapshot, sealed, batches); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("ValidateSuccessor() error = %v, want invalid successor", err)
			}
		})
	}

	prior := cloneSnapshot(first.Snapshot)
	prior.NextEntryID = "e1"
	prior, _, err = SealSnapshot(prior)
	if err != nil {
		t.Fatal(err)
	}
	mismatched := cloneSnapshot(second.Snapshot)
	mismatched.PreviousDigest = prior.Digest
	mismatched, _, err = SealSnapshot(mismatched)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(prior, mismatched, batches); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("ValidateSuccessor() prior next ID error = %v, want invalid successor", err)
	}

	terminal, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &second.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: second.NextEntryIndex, EntryID: second.NextEntryID, StreamSHA256: second.StreamSHA256, BatchID: planBatchID(74)})
	if err != nil || terminal.Outcome != PlanCommitted {
		t.Fatalf("terminal checkpoint = %#v, %v", terminal, err)
	}
	if err := ValidateSuccessor(second.Snapshot, terminal.Snapshot, map[string]Batch{first.Batch.BatchID: first.Batch, second.Batch.BatchID: second.Batch, terminal.Batch.BatchID: terminal.Batch}); err != nil {
		t.Fatalf("ValidateSuccessor() terminal successor error = %v", err)
	}
}

func TestValidateSuccessorRejectsImmutableLifecycleMutations(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []EvidenceEntry{
		planEntry("e1", "1", strings.Repeat("x", 21000)),
		planEntry("e2", "2", strings.Repeat("x", 21000)),
		planEntry("e3", "3", strings.Repeat("x", 21000)),
	}
	first, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: planBatchID(70)})
	if err != nil || first.Outcome != PlanContinuationRequired {
		t.Fatalf("first checkpoint = %#v, %v", first, err)
	}
	second, err := PlanCheckpoint(PlanInput{Project: "jarvis", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: planBatchID(71)})
	if err != nil || second.Outcome != PlanContinuationRequired {
		t.Fatalf("second checkpoint = %#v, %v", second, err)
	}

	for _, tt := range []struct {
		name string
		edit func(*Snapshot)
	}{
		{"task manifest", func(s *Snapshot) { s.TaskManifestSHA256 = strings.Repeat("a", 64) }},
		{"stream hash", func(s *Snapshot) { s.StreamSHA256 = strings.Repeat("b", 64) }},
		{"prior continuation cursor", func(s *Snapshot) {
			s.NextEntryIndex, s.NextEntryID = first.Snapshot.NextEntryIndex, first.Snapshot.NextEntryID
		}},
		{"batch prefix", func(s *Snapshot) { s.Batches[0].SHA256 = strings.Repeat("c", 64) }},
		{"coverage prefix", func(s *Snapshot) { s.Coverage[0].EntryID = "other" }},
		{"terminal partial status", func(s *Snapshot) {
			s.Status, s.StreamSHA256, s.NextEntryIndex, s.NextEntryID = StatusComplete, "", 0, ""
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			successor := cloneSnapshot(second.Snapshot)
			tt.edit(&successor)
			sealed, _, err := SealSnapshot(successor)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateSuccessor(first.Snapshot, sealed, map[string]Batch{second.Batch.BatchID: second.Batch}); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("ValidateSuccessor() error = %v, want invalid successor", err)
			}
		})
	}
}

func TestValidateSuccessorAllowsOnlyPreContinuationIdentityUpgrade(t *testing.T) {
	tasks := []Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "pending"}}
	_, manifest, err := TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := SealSnapshot(Snapshot{
		Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 4, Revision: 8,
		TaskManifestSHA256: manifest, Status: StatusPartial,
		Coverage: []Coverage{{TaskID: "1.1", BatchID: "apb-0123456789abcdef0123456789abcdef", EntryID: "imported-1"}},
		Batches:  []BatchRef{{BatchID: "apb-0123456789abcdef0123456789abcdef", SHA256: strings.Repeat("a", 64)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	base = historicalSnapshotForTest(t, base)
	entries := []EvidenceEntry{planEntry("entry-1", "1.2", "evidence")}
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	successor, err := UpgradeLegacyContinuation(base, tasks, entries, stream)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(base, successor, nil); err != nil {
		t.Fatalf("ValidateSuccessor() valid continuation identity upgrade: %v", err)
	}
	if successor.Generation != base.Generation || successor.Revision != base.Revision+1 || successor.PreviousDigest != base.Digest {
		t.Fatalf("successor CAS identity = %#v, base = %#v", successor, base)
	}

	for _, tt := range []struct {
		name string
		edit func(*Snapshot)
	}{
		{name: "appended evidence", edit: func(s *Snapshot) {
			s.Batches = append(s.Batches, BatchRef{BatchID: "apb-0123456789abcdef0123456789abcdef", SHA256: strings.Repeat("a", 64)})
		}},
		{name: "coverage mutation", edit: func(s *Snapshot) {
			s.Coverage = append(s.Coverage, Coverage{TaskID: "1.2", BatchID: "apb-0123456789abcdef0123456789abcdef", EntryID: "entry-1"})
		}},
		{name: "status mutation", edit: func(s *Snapshot) {
			s.Status = StatusComplete
			s.StreamSHA256, s.NextEntryIndex, s.NextEntryID = "", 0, ""
		}},
		{name: "manifest mutation", edit: func(s *Snapshot) { s.TaskManifestSHA256 = strings.Repeat("b", 64) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneSnapshot(successor)
			tt.edit(&candidate)
			candidate, _, err = SealSnapshot(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateSuccessor(base, candidate, nil); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("ValidateSuccessor() error = %v, want rejected mutation", err)
			}
		})
	}
}

func TestValidateSuccessorAcceptsHistoricalMultiRevisionLegacyEpochLineage(t *testing.T) {
	_, manifest, err := TaskManifest([]Task{{ID: "1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	batch := func(id, entry string) Batch {
		t.Helper()
		sealed, _, sealErr := SealBatch(Batch{
			Schema: EvidenceSchema, Project: "jarvis", Change: "change", BatchID: id,
			Entries: []EvidenceEntry{{EntryID: entry, TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: EvidenceGreen, Summary: "evidence", Command: "go test", Outcome: OutcomePass, Files: []string{}}},
		})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return sealed
	}
	first, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	secondBatch := batch("apb-11111111111111111111111111111111", "legacy-2")
	second, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 2, Revision: 2, PreviousDigest: first.Digest, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{{BatchID: secondBatch.BatchID, SHA256: secondBatch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	thirdBatch := batch("apb-22222222222222222222222222222222", "legacy-3")
	third, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 3, Revision: 3, PreviousDigest: second.Digest, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: append(slices.Clone(second.Batches), BatchRef{BatchID: thirdBatch.BatchID, SHA256: thirdBatch.SHA256})})
	if err != nil {
		t.Fatal(err)
	}
	batches := map[string]Batch{secondBatch.BatchID: secondBatch, thirdBatch.BatchID: thirdBatch}
	if err := ValidateSuccessor(first, second, batches); err != nil {
		t.Fatalf("first historical legacy successor = %v", err)
	}
	if err := ValidateSuccessor(second, third, batches); err != nil {
		t.Fatalf("second historical legacy successor = %v", err)
	}

	badHash := cloneSnapshot(third)
	badHash.Batches[len(badHash.Batches)-1].SHA256 = strings.Repeat("f", 64)
	badHash, _, err = SealSnapshot(badHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(second, badHash, batches); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("historical successor with rewritten batch hash error = %v, want invalid transition", err)
	}

	mixed := cloneSnapshot(third)
	mixed.Generation, mixed.Revision, mixed.PreviousDigest = third.Generation+1, third.Revision+1, third.Digest
	mixed.StreamSHA256, mixed.NextEntryIndex, mixed.NextEntryID = strings.Repeat("a", 64), 0, "entry-1"
	mixed, _, err = SealSnapshot(mixed)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(third, mixed, batches); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("mixed legacy/continuation successor error = %v, want invalid transition", err)
	}
}

func TestValidateSuccessorRejectsHistoricalLegacyEpochNoOp(t *testing.T) {
	_, manifest, err := TaskManifest([]Task{{ID: "1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	previous, _, err := SealSnapshot(Snapshot{Schema: SnapshotSchema, Project: "jarvis", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	successor := cloneSnapshot(previous)
	successor.Generation, successor.Revision, successor.PreviousDigest = 2, 2, previous.Digest
	successor, _, err = SealSnapshot(successor)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSuccessor(previous, successor, map[string]Batch{}); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("historical legacy no-op successor error = %v, want invalid transition", err)
	}
}

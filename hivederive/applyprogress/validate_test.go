package applyprogress

import (
	"errors"
	"strings"
	"testing"
)

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
		{"rejects unknown attribution", nil, func(b *Batch) { b.Entries[1].TaskIDs = []string{"unknown"} }, CodeInvalidCoverage},
		{"rejects repeated attribution", nil, func(b *Batch) { b.Entries[1].TaskIDs = []string{"1.2", "1.2"} }, CodeInvalidCoverage},
		{"rejects completion without attribution", nil, func(b *Batch) { b.Entries[1].CompletesTaskIDs = []string{"1.1"} }, CodeInvalidCoverage},
		{"rejects repeated completion", nil, func(b *Batch) { b.Entries[1].CompletesTaskIDs = []string{"1.2", "1.2"} }, CodeDuplicateEvidence},
		{"rejects batch project mismatch", nil, func(b *Batch) { b.Project = "other" }, CodeCorruptBatch},
		{"rejects batch change mismatch", nil, func(b *Batch) { b.Change = "other" }, CodeCorruptBatch},
		{"rejects batch identity mismatch", nil, func(b *Batch) { b.BatchID = "apb-ffffffffffffffffffffffffffffffff" }, CodeCorruptBatch},
		{"rejects changed task manifest", func(s *Snapshot, _ map[string][]byte) { s.TaskManifestSHA256 = strings.Repeat("c", 64) }, nil, CodeTaskManifestMismatch},
		{"rejects partial claiming complete coverage", func(s *Snapshot, _ map[string][]byte) { s.Status = StatusPartial }, nil, CodeInvalidCoverage},
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

func TestValidateEvidenceCoverage(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*Snapshot, map[string]Batch)
		want ValidationCode
	}{
		{"accepts matching completed evidence", nil, ""},
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

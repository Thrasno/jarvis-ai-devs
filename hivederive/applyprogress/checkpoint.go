package applyprogress

import (
	"errors"
	"maps"
	"slices"
)

type PlanOutcome string

const (
	PlanCommitted                 PlanOutcome = "committed"
	PlanContinuationRequired      PlanOutcome = "continuation_required"
	PlanEvidenceItemTooLarge      PlanOutcome = "evidence_item_too_large"
	PlanSnapshotCapacityExhausted PlanOutcome = "snapshot_capacity_exhausted"
)

// PlanInput contains the stable stream and the snapshot from which one batch may advance.
type PlanInput struct {
	Project, Change  string
	Base             *Snapshot
	Tasks            []Task
	Entries          []EvidenceEntry
	EntryIndex       int
	EntryID, BatchID string
	StreamSHA256     string
}

type PlanResult struct {
	Outcome                  PlanOutcome
	Batch                    Batch
	BatchJSON                []byte
	Snapshot                 Snapshot
	SnapshotJSON             []byte
	EndIndex, NextEntryIndex int
	NextEntryID              string
	StreamSHA256             string
}

// PlanCheckpoint returns the maximal whole-entry prefix that fits one batch and successor snapshot.
func PlanCheckpoint(input PlanInput) (PlanResult, error) {
	streamSHA256, err := StreamSHA256(input.Entries)
	if err != nil || (input.EntryIndex > 0 && input.StreamSHA256 == "") || (input.StreamSHA256 != "" && input.StreamSHA256 != streamSHA256) {
		return PlanResult{}, ErrInvalidValue
	}
	tasks, manifest, err := TaskManifest(input.Tasks)
	if err != nil || !validID(input.Project) || !validID(input.Change) || !batchIDPattern.MatchString(input.BatchID) || input.EntryIndex < 0 || input.EntryIndex >= len(input.Entries) || input.EntryID != input.Entries[input.EntryIndex].EntryID {
		return PlanResult{}, ErrInvalidValue
	}
	known := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		known[task.ID] = true
	}
	if err := validateStream(input.Entries, known); err != nil {
		return PlanResult{}, err
	}

	base, completed, err := planBase(input, manifest, known)
	if err != nil {
		return PlanResult{}, err
	}
	last := PlanResult{StreamSHA256: streamSHA256}
	for end := input.EntryIndex + 1; end <= len(input.Entries); end++ {
		batch, batchJSON, err := SealBatch(Batch{Schema: EvidenceSchema, Project: input.Project, Change: input.Change, BatchID: input.BatchID, Entries: input.Entries[input.EntryIndex:end]})
		if err != nil {
			if isCapacity(err) && end == input.EntryIndex+1 {
				return PlanResult{Outcome: PlanEvidenceItemTooLarge, StreamSHA256: streamSHA256}, nil
			}
			if isCapacity(err) {
				break
			}
			return PlanResult{}, err
		}
		coverage, err := mergeCoverage(completed, batch, tasks)
		if err != nil {
			return PlanResult{}, err
		}
		snapshot := Snapshot{Schema: SnapshotSchema, Project: input.Project, Change: input.Change, Generation: base.Generation + 1, Revision: base.Revision + 1, PreviousDigest: base.Digest, TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: coverage, Batches: append(slices.Clone(base.Batches), BatchRef{BatchID: batch.BatchID, SHA256: batch.SHA256})}
		if len(coverage) == len(tasks) {
			snapshot.Status = StatusComplete
		}
		snapshot, snapshotJSON, err := SealSnapshot(snapshot)
		if err != nil {
			if isCapacity(err) && end == input.EntryIndex+1 {
				return PlanResult{Outcome: PlanSnapshotCapacityExhausted, StreamSHA256: streamSHA256}, nil
			}
			if isCapacity(err) {
				break
			}
			return PlanResult{}, err
		}
		last = PlanResult{Batch: batch, BatchJSON: batchJSON, Snapshot: snapshot, SnapshotJSON: snapshotJSON, EndIndex: end, StreamSHA256: streamSHA256}
		if end == len(input.Entries) {
			last.Outcome = PlanCommitted
			return last, nil
		}
	}
	last.Outcome, last.NextEntryIndex = PlanContinuationRequired, last.EndIndex
	last.NextEntryID = input.Entries[last.EndIndex].EntryID
	return last, nil
}

// StreamSHA256 returns the canonical SHA-256 binding for the full ordered entry stream.
func StreamSHA256(entries []EvidenceEntry) (string, error) {
	data, err := canonicalJSON(entries)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func planBase(input PlanInput, manifest string, known map[string]bool) (Snapshot, map[string]Coverage, error) {
	base := Snapshot{Batches: []BatchRef{}}
	if input.Base == nil {
		if input.EntryIndex != 0 {
			return Snapshot{}, nil, ErrInvalidValue
		}
	} else {
		base = *input.Base
		if VerifySnapshot(base) != nil || base.Project != input.Project || base.Change != input.Change || base.TaskManifestSHA256 != manifest {
			return Snapshot{}, nil, ErrInvalidValue
		}
	}
	seenBatches, completed := map[string]bool{}, map[string]Coverage{}
	for _, ref := range base.Batches {
		if seenBatches[ref.BatchID] {
			return Snapshot{}, nil, ErrInvalidValue
		}
		seenBatches[ref.BatchID] = true
	}
	for _, item := range base.Coverage {
		if _, exists := completed[item.TaskID]; !known[item.TaskID] || exists {
			return Snapshot{}, nil, ErrInvalidValue
		}
		completed[item.TaskID] = item
	}
	if (base.Status == StatusComplete) != (len(completed) == len(known)) {
		return Snapshot{}, nil, ErrInvalidValue
	}
	return base, completed, nil
}

func validateStream(entries []EvidenceEntry, known map[string]bool) error {
	seenEntries, completed := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		if seenEntries[entry.EntryID] {
			return ErrInvalidValue
		}
		seenEntries[entry.EntryID] = true
		attributed := map[string]bool{}
		for _, id := range entry.TaskIDs {
			if !known[id] || attributed[id] {
				return ErrInvalidValue
			}
			attributed[id] = true
		}
		for _, id := range entry.CompletesTaskIDs {
			if !attributed[id] || completed[id] {
				return ErrInvalidValue
			}
			completed[id] = true
		}
	}
	return nil
}

func mergeCoverage(completed map[string]Coverage, batch Batch, tasks []Task) ([]Coverage, error) {
	items := maps.Clone(completed)
	for _, entry := range batch.Entries {
		for _, id := range entry.CompletesTaskIDs {
			if _, exists := items[id]; exists {
				return nil, ErrInvalidValue
			}
			items[id] = Coverage{TaskID: id, BatchID: batch.BatchID, EntryID: entry.EntryID}
		}
	}
	coverage := make([]Coverage, 0, len(items))
	for _, task := range tasks {
		if item, ok := items[task.ID]; ok {
			coverage = append(coverage, item)
		}
	}
	return coverage, nil
}

func isCapacity(err error) bool { var capacity *CapacityError; return errors.As(err, &capacity) }

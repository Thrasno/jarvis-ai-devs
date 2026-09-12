package applyprogress

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

type PlanOutcome string

const (
	PlanCommitted                 PlanOutcome = "committed"
	PlanContinuationRequired      PlanOutcome = "continuation_required"
	PlanEvidenceItemTooLarge      PlanOutcome = "evidence_item_too_large"
	PlanSnapshotCapacityExhausted PlanOutcome = "snapshot_capacity_exhausted"
	// PlanStreamPreflightRequired rejects a new multi-checkpoint stream before
	// its first continuation binding when its simulated terminal topology cannot
	// fit the immutable snapshot capacity.
	PlanStreamPreflightRequired         PlanOutcome = "stream_preflight_required"
	PlanCheckpointConsolidationRequired PlanOutcome = "checkpoint_consolidation_required"
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
	Capacity                 *CapacityError
	Frequency                *CheckpointFrequencyError
	Warning                  *CapacityWarning
}

// PlanCheckpoint returns the maximal whole-entry prefix that fits one batch and successor snapshot.
func PlanCheckpoint(input PlanInput) (PlanResult, error) {
	return planCheckpoint(input, true)
}

func planCheckpoint(input PlanInput, preflightNewStream bool) (PlanResult, error) {
	streamSHA256, err := StreamSHA256(input.Entries)
	if err != nil || invalidStreamBinding(input, streamSHA256) {
		return PlanResult{}, invalid(CodeInvalidPlan, "stream_sha256")
	}
	tasks, manifest, err := TaskManifest(input.Tasks)
	if err != nil {
		return PlanResult{}, err
	}
	if invalidPlanIdentity(input) {
		return PlanResult{}, invalid(CodeInvalidPlan, "identity")
	}
	if invalidEntryCursor(input) {
		return PlanResult{}, invalid(CodeInvalidCursor, "entry_index or entry_id")
	}
	known := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		known[task.ID] = true
	}
	if err := validateStream(input.Entries, known, false); err != nil {
		return PlanResult{}, err
	}

	base, completed, err := planBase(input, manifest, known, streamSHA256)
	if err != nil {
		return PlanResult{}, err
	}
	generation, revision, err := successorCoordinates(input.Base, base)
	if err != nil {
		return PlanResult{}, err
	}
	if hasBatchID(base.Batches, input.BatchID) {
		return PlanResult{}, invalid(CodeInvalidPlan, "batch_id")
	}

	planner, err := newCheckpointSizePlanner(input, base, completed, tasks, manifest, streamSHA256, generation, revision)
	if err != nil {
		return PlanResult{}, err
	}
	bestEnd := -1
	firstBatchOverflows := false
	var capacity *CapacityError
	for end := input.EntryIndex + 1; end <= len(input.Entries); end++ {
		batchRunes, snapshotRunes, err := planner.runes(end)
		if err != nil {
			return PlanResult{}, err
		}
		// Entries only add canonical JSON runes, so no later batch can fit after
		// this point. Snapshot sizes are intentionally scanned exactly: a later,
		// shorter continuation cursor can fit after an earlier long cursor did not.
		if batchRunes > MaxDocumentRunes {
			firstBatchOverflows = end == input.EntryIndex+1
			if firstBatchOverflows {
				capacity = &CapacityError{Document: "batch", Runes: batchRunes, Limit: MaxDocumentRunes}
			}
			break
		}
		if snapshotRunes <= MaxDocumentRunes {
			bestEnd = end
		} else if capacity == nil || snapshotRunes < capacity.Runes {
			// When no candidate snapshot fits, report the smallest exact candidate
			// instead of a derived estimate or an arbitrary later cursor.
			capacity = checkpointCapacity(base, input.Base != nil, snapshotRunes)
		}
	}
	if bestEnd < 0 {
		if firstBatchOverflows {
			return PlanResult{Outcome: PlanEvidenceItemTooLarge, StreamSHA256: streamSHA256, Capacity: capacity}, nil
		}
		return PlanResult{Outcome: PlanSnapshotCapacityExhausted, StreamSHA256: streamSHA256, Capacity: capacity}, nil
	}
	best, err := planCheckpointCandidate(input, base, completed, tasks, manifest, streamSHA256, generation, revision, bestEnd)
	if err != nil {
		return PlanResult{}, err
	}
	// An exhausted but incomplete unbound stream must leave enough canonical
	// snapshot room for the next independent stream. Guard the projected successor
	// bytes before publishing it: counting references alone cannot see large
	// coverage or identifier encodings. A complete terminal snapshot is final, so
	// it may use the full protocol capacity.
	projectedSnapshotRunes := utf8.RuneCount(best.SnapshotJSON)
	if requiresProjectedCheckpointConsolidation(base, best.Snapshot, projectedSnapshotRunes) {
		return checkpointConsolidationResult(base, best, streamSHA256, projectedSnapshotRunes), nil
	}
	// Retain the reference-count backstop for a tiny nonterminal checkpoint. It
	// protects already-large unbound prefixes whose projected successor remains
	// below the safe byte threshold only because the current cursor is short.
	if requiresTinyCheckpointConsolidation(base, best, bestEnd, input.EntryIndex, len(input.Entries)) {
		return checkpointConsolidationResult(base, best, streamSHA256, projectedSnapshotRunes), nil
	}
	best.Warning = SnapshotCapacityWarning(best.SnapshotJSON)
	if best.Warning != nil {
		best.Capacity = checkpointCapacity(base, input.Base != nil, utf8.RuneCount(best.SnapshotJSON))
	}
	if bestEnd == len(input.Entries) {
		best.Outcome = PlanCommitted
		return best, nil
	}
	best.Outcome = PlanContinuationRequired
	best.NextEntryIndex = best.EndIndex
	best.NextEntryID = input.Entries[best.EndIndex].EntryID
	if preflightNewStream && !base.hasContinuation() {
		capacity, err := preflightNewStreamCapacity(input, best)
		if err != nil {
			return PlanResult{}, err
		}
		if capacity != nil {
			// The simulated continuation is never durable. Report its projection
			// against the persisted/base head rather than treating an intermediate
			// simulated successor as current status.
			if capacity.Document == "snapshot" {
				capacity = checkpointCapacity(base, input.Base != nil, capacity.Runes)
			}
			return PlanResult{Outcome: PlanStreamPreflightRequired, StreamSHA256: streamSHA256, Capacity: capacity}, nil
		}
	}
	return best, nil
}

// preflightNewStreamCapacity proves that a new continuation binding can reach
// its terminal snapshot before the first batch becomes durable. Every simulated
// batch ID has the same protocol shape as a caller-provided ID, so snapshot size
// accounting remains exact while no caller authority or storage is touched.
func preflightNewStreamCapacity(input PlanInput, first PlanResult) (*CapacityError, error) {
	current := first
	for attempt := uint64(0); current.Outcome == PlanContinuationRequired; attempt++ {
		successorInput := input
		successorInput.Base = &current.Snapshot
		successorInput.EntryIndex = current.NextEntryIndex
		successorInput.EntryID = current.NextEntryID
		successorInput.StreamSHA256 = current.StreamSHA256
		successorInput.BatchID = simulatedBatchID(current.Snapshot, attempt)
		next, err := planCheckpoint(successorInput, false)
		if err != nil {
			return nil, fmt.Errorf("stream preflight at entry %d: %w", successorInput.EntryIndex, err)
		}
		switch next.Outcome {
		case PlanEvidenceItemTooLarge, PlanSnapshotCapacityExhausted:
			return next.Capacity, nil
		case PlanCommitted:
			return nil, nil
		case PlanContinuationRequired:
			current = next
		default:
			return nil, nil
		}
	}
	return nil, nil
}

// PreflightCompleteStream proves that a prospective continuation binding can
// consume its complete ordered stream before that binding becomes durable.
// Legacy migrations and historical continuation upgrades use it because they
// bind a cursor without first publishing an ordinary checkpoint batch.
func PreflightCompleteStream(base Snapshot, tasks []Task, entries []EvidenceEntry, streamSHA256 string) error {
	input := PlanInput{
		Project: base.Project, Change: base.Change, Base: &base, Tasks: tasks,
		Entries: entries, EntryIndex: base.NextEntryIndex, EntryID: base.NextEntryID,
		StreamSHA256: streamSHA256, BatchID: simulatedBatchID(base, 0),
	}
	first, err := PlanCheckpoint(input)
	if err != nil {
		return err
	}
	switch first.Outcome {
	case PlanCommitted:
		return nil
	case PlanEvidenceItemTooLarge, PlanSnapshotCapacityExhausted:
		return first.Capacity
	case PlanContinuationRequired:
		capacity, err := preflightNewStreamCapacity(input, first)
		if err != nil {
			return err
		}
		return capacity
	default:
		return invalid(CodeInvalidPlan, "complete stream preflight")
	}
}

func simulatedBatchID(snapshot Snapshot, attempt uint64) string {
	for candidate := attempt; ; candidate++ {
		batchID := fmt.Sprintf("apb-%032x", candidate)
		if !hasBatchID(snapshot.Batches, batchID) {
			return batchID
		}
	}
}

// checkpointSizePlanner computes exact canonical rune counts for every candidate
// in one pass. It never serializes a growing prefix: entries, coverage records,
// and base references are each encoded once, while each endpoint adds only its
// precomputed contributions and the endpoint-specific continuation overhead.
type checkpointSizePlanner struct {
	input                      PlanInput
	base                       Snapshot
	streamSHA256               string
	generation                 uint64
	revision                   uint64
	batchBaseRunes             int
	batchPrefixRunes           []int
	coveragePrefixRunes        []int
	coverageCounts             []int
	baseBatchRunes             int
	baseBatchCount             int
	partialFixedRunes          []int
	partialTerminalFixedRunes  int
	completeTerminalFixedRunes int
	taskCount                  int
}

func newCheckpointSizePlanner(input PlanInput, base Snapshot, completed map[string]Coverage, tasks []Task, manifest, streamSHA256 string, generation, revision uint64) (*checkpointSizePlanner, error) {
	count := len(input.Entries) - input.EntryIndex
	planner := &checkpointSizePlanner{
		input: input, base: base, streamSHA256: streamSHA256, generation: generation, revision: revision,
		batchPrefixRunes: make([]int, count+1), coveragePrefixRunes: make([]int, count+1), coverageCounts: make([]int, count+1), partialFixedRunes: make([]int, count+1), taskCount: len(tasks),
	}
	batchBase, err := canonicalJSON(Batch{Schema: EvidenceSchema, Project: input.Project, Change: input.Change, BatchID: input.BatchID, Entries: []EvidenceEntry{}, SHA256: strings.Repeat("0", 64)})
	if err != nil {
		return nil, err
	}
	planner.batchBaseRunes = utf8.RuneCount(batchBase)
	for _, ref := range base.Batches {
		encoded, err := canonicalJSON(ref)
		if err != nil {
			return nil, err
		}
		planner.baseBatchRunes += utf8.RuneCount(encoded)
		if planner.baseBatchCount > 0 {
			planner.baseBatchRunes++
		}
		planner.baseBatchCount++
	}
	newRef, err := canonicalJSON(BatchRef{BatchID: input.BatchID, SHA256: strings.Repeat("0", 64)})
	if err != nil {
		return nil, err
	}
	newRefRunes := utf8.RuneCount(newRef)

	known := make(map[string]bool, len(tasks))
	coverage := maps.Clone(completed)
	baseCoverageRunes, baseCoverageCount := 0, 0
	for _, item := range base.Coverage {
		encoded, err := canonicalJSON(item)
		if err != nil {
			return nil, err
		}
		baseCoverageRunes += utf8.RuneCount(encoded)
		if baseCoverageCount > 0 {
			baseCoverageRunes++
		}
		baseCoverageCount++
	}
	for _, task := range tasks {
		known[task.ID] = true
	}
	planner.coveragePrefixRunes[0], planner.coverageCounts[0] = baseCoverageRunes, baseCoverageCount
	for offset := 1; offset <= count; offset++ {
		entry := input.Entries[input.EntryIndex+offset-1]
		encoded, err := canonicalJSON(entry)
		if err != nil {
			return nil, err
		}
		planner.batchPrefixRunes[offset] = planner.batchPrefixRunes[offset-1] + utf8.RuneCount(encoded)
		if offset > 1 {
			planner.batchPrefixRunes[offset]++
		}
		planner.coveragePrefixRunes[offset] = planner.coveragePrefixRunes[offset-1]
		planner.coverageCounts[offset] = planner.coverageCounts[offset-1]
		for _, taskID := range entry.CompletesTaskIDs {
			if !known[taskID] || coverage[taskID].TaskID != "" {
				return nil, invalid(CodeInvalidEvidence, taskID)
			}
			item := Coverage{TaskID: taskID, BatchID: input.BatchID, EntryID: entry.EntryID}
			coverage[taskID] = item
			encoded, err := canonicalJSON(item)
			if err != nil {
				return nil, err
			}
			if planner.coverageCounts[offset] > 0 {
				planner.coveragePrefixRunes[offset]++
			}
			planner.coveragePrefixRunes[offset] += utf8.RuneCount(encoded)
			planner.coverageCounts[offset]++
		}
		end := input.EntryIndex + offset
		if end < len(input.Entries) {
			fixed, err := checkpointFixedSnapshotRunes(input, base, manifest, streamSHA256, generation, revision, StatusPartial, end, input.Entries[end].EntryID)
			if err != nil {
				return nil, err
			}
			planner.partialFixedRunes[offset] = fixed
		}
	}
	partialTerminal, err := checkpointFixedSnapshotRunes(input, base, manifest, streamSHA256, generation, revision, StatusPartial, 0, "")
	if err != nil {
		return nil, err
	}
	planner.partialTerminalFixedRunes = partialTerminal
	completeTerminal, err := checkpointFixedSnapshotRunes(input, base, manifest, streamSHA256, generation, revision, StatusComplete, 0, "")
	if err != nil {
		return nil, err
	}
	planner.completeTerminalFixedRunes = completeTerminal
	// The appended reference is part of every candidate. Its digest is unknown
	// until sealing but a SHA-256 hex digest is always exactly 64 ASCII runes.
	planner.baseBatchRunes += newRefRunes
	if planner.baseBatchCount > 0 {
		planner.baseBatchRunes++
	}
	return planner, nil
}

func (p *checkpointSizePlanner) runes(end int) (int, int, error) {
	offset := end - p.input.EntryIndex
	if offset <= 0 || offset >= len(p.batchPrefixRunes) {
		return 0, 0, invalid(CodeInvalidCursor, "candidate end")
	}
	batchRunes := p.batchBaseRunes + p.batchPrefixRunes[offset]
	terminal := end == len(p.input.Entries)
	fixed := p.partialFixedRunes[offset]
	if terminal {
		fixed = p.partialTerminalFixedRunes
		if p.coverageCounts[offset] == p.taskCount {
			fixed = p.completeTerminalFixedRunes
		}
	}
	return batchRunes, fixed + p.coveragePrefixRunes[offset] + p.baseBatchRunes, nil
}

func checkpointFixedSnapshotRunes(input PlanInput, base Snapshot, manifest, streamSHA256 string, generation, revision uint64, status Status, nextIndex int, nextID string) (int, error) {
	snapshot := Snapshot{
		Schema: SnapshotSchema, Project: input.Project, Change: input.Change, Generation: generation, Revision: revision,
		PreviousDigest: base.Digest, TaskManifestSHA256: manifest, Status: status, Coverage: []Coverage{}, Batches: []BatchRef{}, Digest: strings.Repeat("0", 64),
	}
	if status == StatusPartial && nextID != "" {
		snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = streamSHA256, nextIndex, nextID
	}
	data, err := canonicalJSON(snapshot)
	if err != nil {
		return 0, err
	}
	return utf8.RuneCount(data), nil
}

func planCheckpointCandidate(input PlanInput, base Snapshot, completed map[string]Coverage, tasks []Task, manifest, streamSHA256 string, generation, revision uint64, end int) (PlanResult, error) {
	batch, batchJSON, err := SealBatch(Batch{
		Schema: EvidenceSchema, Project: input.Project, Change: input.Change, BatchID: input.BatchID,
		Entries: cloneEvidenceEntries(input.Entries[input.EntryIndex:end]),
	})
	if err != nil {
		return PlanResult{}, err
	}
	coverage, err := mergeCoverage(completed, batch, tasks)
	if err != nil {
		return PlanResult{}, err
	}
	nextEntryID := ""
	if end < len(input.Entries) {
		nextEntryID = input.Entries[end].EntryID
	}
	snapshot := Snapshot{
		Schema: SnapshotSchema, Project: input.Project, Change: input.Change,
		Generation: generation, Revision: revision, PreviousDigest: base.Digest,
		TaskManifestSHA256: manifest, Status: StatusPartial, Coverage: coverage,
		Batches: append(slices.Clone(base.Batches), BatchRef{BatchID: batch.BatchID, SHA256: batch.SHA256}),
	}
	if end < len(input.Entries) {
		snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = streamSHA256, end, nextEntryID
	} else if len(coverage) == len(tasks) {
		snapshot.Status = StatusComplete
	}
	snapshot, snapshotJSON, err := SealSnapshot(snapshot)
	if err != nil {
		return PlanResult{}, err
	}
	return PlanResult{Batch: batch, BatchJSON: batchJSON, Snapshot: snapshot, SnapshotJSON: snapshotJSON, EndIndex: end, StreamSHA256: streamSHA256}, nil
}

func requiresProjectedCheckpointConsolidation(base Snapshot, candidate Snapshot, projectedRunes int) bool {
	return !base.hasContinuation() && !candidate.hasContinuation() && candidate.Status == StatusPartial && projectedRunes >= checkpointConsolidationSafeRunes
}

func requiresTinyCheckpointConsolidation(base Snapshot, candidate PlanResult, end, entryIndex, entryCount int) bool {
	return !base.hasContinuation() && end == entryIndex+1 && end < entryCount && len(base.Batches) >= SnapshotCheckpointReferenceGuard && utf8.RuneCount(candidate.BatchJSON) < MinNonterminalCheckpointRunes
}

// checkpointCapacity reports exact canonical rune counts. The current count is
// always derived from the persisted/base snapshot supplied to this plan; a
// rejected successor never becomes status state.
func checkpointCapacity(base Snapshot, hasBase bool, projectedRunes int) *CapacityError {
	currentRunes := 0
	if hasBase {
		data, err := snapshotCanonicalJSON(base)
		if err == nil {
			currentRunes = utf8.RuneCount(data)
		}
	}
	return &CapacityError{Document: "snapshot", Runes: projectedRunes, Limit: MaxDocumentRunes, CurrentRunes: currentRunes, ProjectedRunes: projectedRunes, CeilingRunes: MaxDocumentRunes}
}

func checkpointConsolidationResult(base Snapshot, candidate PlanResult, stream string, snapshotRunes int) PlanResult {
	return PlanResult{Outcome: PlanCheckpointConsolidationRequired, StreamSHA256: stream, Frequency: &CheckpointFrequencyError{References: len(base.Batches), Threshold: SnapshotCheckpointReferenceGuard, BatchRunes: utf8.RuneCount(candidate.BatchJSON), MinimumRunes: MinNonterminalCheckpointRunes, SnapshotRunes: snapshotRunes, SafeRunes: checkpointConsolidationSafeRunes}}
}

func capacityDocument(err error) string {
	var capacity *CapacityError
	if errors.As(err, &capacity) {
		return capacity.Document
	}
	return ""
}

func invalidStreamBinding(input PlanInput, streamSHA256 string) bool {
	if input.Base != nil && input.StreamSHA256 == "" && (input.Base.Status == StatusComplete || input.Base.hasContinuation()) {
		return true
	}
	return input.StreamSHA256 != "" && input.StreamSHA256 != streamSHA256
}

func invalidPlanIdentity(input PlanInput) bool {
	return !validID(input.Project) || !validID(input.Change) || !batchIDPattern.MatchString(input.BatchID)
}

func invalidEntryCursor(input PlanInput) bool {
	return input.EntryIndex < 0 || input.EntryIndex == math.MaxInt || input.EntryIndex >= len(input.Entries) || input.EntryID != input.Entries[input.EntryIndex].EntryID
}

// StreamSHA256 returns the canonical SHA-256 binding for the full ordered entry stream.
func StreamSHA256(entries []EvidenceEntry) (string, error) {
	data, err := canonicalJSON(entries)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func planBase(input PlanInput, manifest string, known map[string]bool, streamSHA256 string) (Snapshot, map[string]Coverage, error) {
	base := Snapshot{Batches: []BatchRef{}, Coverage: []Coverage{}}
	if input.Base == nil {
		if input.EntryIndex != 0 {
			return Snapshot{}, nil, invalid(CodeInvalidBase, "initial entry_index")
		}
	} else {
		base = cloneSnapshot(*input.Base)
		if invalidBaseSnapshot(base, input, manifest, streamSHA256) {
			return Snapshot{}, nil, invalid(CodeInvalidBase, "snapshot or continuation")
		}
	}
	seenBatches, completed := map[string]bool{}, map[string]Coverage{}
	for _, ref := range base.Batches {
		if seenBatches[ref.BatchID] {
			return Snapshot{}, nil, invalid(CodeInvalidBase, "duplicate batch "+ref.BatchID)
		}
		seenBatches[ref.BatchID] = true
	}
	for _, item := range base.Coverage {
		if _, exists := completed[item.TaskID]; !known[item.TaskID] || exists {
			return Snapshot{}, nil, invalid(CodeInvalidBase, "coverage "+item.TaskID)
		}
		completed[item.TaskID] = item
	}
	if invalidBaseCoverage(base, completed, known) {
		return Snapshot{}, nil, invalid(CodeInvalidBase, "coverage status")
	}
	return base, completed, nil
}

func invalidBaseSnapshot(base Snapshot, input PlanInput, manifest, streamSHA256 string) bool {
	if VerifySnapshot(base) != nil || base.Project != input.Project || base.Change != input.Change || base.TaskManifestSHA256 != manifest || base.Status != StatusPartial {
		return true
	}
	if base.RequiresContinuationUpgrade() {
		return true
	}
	if !base.hasContinuation() {
		return input.EntryIndex != 0
	}
	return base.StreamSHA256 != streamSHA256 || base.NextEntryIndex != input.EntryIndex || base.NextEntryID != input.EntryID
}

func invalidBaseCoverage(base Snapshot, completed map[string]Coverage, known map[string]bool) bool {
	if base.Status == "" {
		return false
	}
	full := len(completed) == len(known)
	if !full {
		return base.Status != StatusPartial
	}
	return base.Status != StatusComplete && !(base.Status == StatusPartial && base.hasContinuation())
}

func successorCoordinates(inputBase *Snapshot, base Snapshot) (uint64, uint64, error) {
	if base.Revision == math.MaxUint64 {
		return 0, 0, invalid(CodeInvalidBase, "revision overflow")
	}
	if inputBase == nil {
		return 1, 1, nil
	}
	return base.Generation, base.Revision + 1, nil
}

func hasBatchID(batches []BatchRef, batchID string) bool {
	for _, batch := range batches {
		if batch.BatchID == batchID {
			return true
		}
	}
	return false
}

func validateStream(entries []EvidenceEntry, known map[string]bool, allowImported bool) error {
	seenEntries, completed := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		if validateEvidenceEntry(entry) != nil || (!allowImported && entry.Kind == EvidenceImported) || seenEntries[entry.EntryID] {
			return invalid(CodeInvalidEvidence, entry.EntryID)
		}
		seenEntries[entry.EntryID] = true
		attributed := map[string]bool{}
		for _, id := range entry.TaskIDs {
			if !known[id] || attributed[id] {
				return invalid(CodeInvalidEvidence, entry.EntryID)
			}
			attributed[id] = true
		}
		for _, id := range entry.CompletesTaskIDs {
			if !attributed[id] || completed[id] {
				return invalid(CodeInvalidEvidence, entry.EntryID)
			}
			completed[id] = true
		}
	}
	return nil
}

// ValidateFutureStream verifies that ordinary future evidence is structurally
// valid and completes every task not covered by an immutable prefix. Legacy
// import uses this before it persists a stream hash or cursor, and explicit
// continuation upgrade uses the same validator.
func ValidateFutureStream(tasks []Task, entries []EvidenceEntry, existing []Coverage) error {
	normalized, _, err := TaskManifest(tasks)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(normalized))
	for _, task := range normalized {
		known[task.ID] = true
	}
	if err := validateStream(entries, known, false); err != nil {
		return err
	}
	return validateFutureStream(entries, normalized, existing)
}

func validateFutureStream(entries []EvidenceEntry, tasks []Task, existing []Coverage) error {
	remaining := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		remaining[task.ID] = true
	}
	for _, coverage := range existing {
		if !remaining[coverage.TaskID] {
			return invalid(CodeInvalidPlan, "existing coverage")
		}
		delete(remaining, coverage.TaskID)
	}
	for _, entry := range entries {
		for _, id := range entry.CompletesTaskIDs {
			if !remaining[id] {
				return invalid(CodeInvalidPlan, "future stream completion")
			}
			delete(remaining, id)
		}
	}
	if len(remaining) != 0 {
		return invalid(CodeInvalidPlan, "future stream cannot complete remaining tasks")
	}
	return nil
}

func mergeCoverage(completed map[string]Coverage, batch Batch, tasks []Task) ([]Coverage, error) {
	items := maps.Clone(completed)
	for _, entry := range batch.Entries {
		for _, id := range entry.CompletesTaskIDs {
			if _, exists := items[id]; exists {
				return nil, invalid(CodeInvalidEvidence, id)
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

func cloneEvidenceEntries(entries []EvidenceEntry) []EvidenceEntry {
	out := make([]EvidenceEntry, len(entries))
	for i, entry := range entries {
		out[i] = entry
		out[i].TaskIDs = slices.Clone(entry.TaskIDs)
		out[i].CompletesTaskIDs = slices.Clone(entry.CompletesTaskIDs)
		out[i].Files = slices.Clone(entry.Files)
	}
	return out
}

// UpgradeLegacyContinuation creates a continuation-bound successor for a
// pre-continuation partial v2 snapshot. Existing evidence remains immutable: this
// transition changes only the CAS coordinates, digest, and atomic continuation
// group. ValidateSuccessor enforces that callers did not append or rewrite it.
func UpgradeLegacyContinuation(base Snapshot, tasks []Task, entries []EvidenceEntry, streamSHA256 string) (Snapshot, error) {
	if VerifySnapshot(base) != nil || !base.RequiresContinuationUpgrade() || base.hasContinuation() || base.Status != StatusPartial {
		return Snapshot{}, invalid(CodeInvalidBase, "legacy snapshot is not a historical pre-continuation partial prefix")
	}
	if len(entries) == 0 {
		return Snapshot{}, invalid(CodeInvalidPlan, "entries")
	}
	normalized, manifest, err := TaskManifest(tasks)
	if err != nil {
		return Snapshot{}, err
	}
	if base.TaskManifestSHA256 != manifest {
		return Snapshot{}, invalid(CodeInvalidBase, "task_manifest_sha256")
	}
	actualStream, err := StreamSHA256(entries)
	if err != nil || streamSHA256 == "" || streamSHA256 != actualStream {
		return Snapshot{}, invalid(CodeInvalidPlan, "stream_sha256")
	}
	// Binding a continuation upgrades a previously unbound partial snapshot. It
	// is the only point at which the future stream becomes durable, so reject a
	// stream that cannot finish the authoritative remaining tasks before callers
	// write the successor and make recovery depend on it.
	if err := ValidateFutureStream(normalized, entries, base.Coverage); err != nil {
		return Snapshot{}, err
	}
	generation, revision, err := successorCoordinates(&base, base)
	if err != nil {
		return Snapshot{}, err
	}
	upgraded := cloneSnapshot(base)
	upgraded.Generation, upgraded.Revision, upgraded.PreviousDigest = generation, revision, base.Digest
	upgraded.StreamSHA256, upgraded.NextEntryIndex, upgraded.NextEntryID = streamSHA256, 0, entries[0].EntryID
	sealed, _, err := SealSnapshot(upgraded)
	if err != nil {
		return Snapshot{}, err
	}
	if err := PreflightCompleteStream(sealed, normalized, entries, streamSHA256); err != nil {
		return Snapshot{}, err
	}
	return sealed, nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Coverage = slices.Clone(snapshot.Coverage)
	snapshot.Batches = slices.Clone(snapshot.Batches)
	return snapshot
}

// SnapshotCapacityWarning reports a conservative preflight warning for a
// canonical successor snapshot. Callers should avoid starting tiny new streams
// and consolidate evidence before the terminal capacity ceiling.
func SnapshotCapacityWarning(snapshotJSON []byte) *CapacityWarning {
	runes := utf8.RuneCount(snapshotJSON)
	remaining := MaxDocumentRunes - runes
	if runes*100 < MaxDocumentRunes*SnapshotCapacityWarningPercent && remaining > SnapshotCapacityWarningRemainingRunes {
		return nil
	}
	return &CapacityWarning{Document: "snapshot", Runes: runes, Limit: MaxDocumentRunes, Remaining: remaining, Threshold: MaxDocumentRunes * SnapshotCapacityWarningPercent / 100}
}

func isCapacity(err error) bool {
	var capacity *CapacityError
	return errors.As(err, &capacity)
}

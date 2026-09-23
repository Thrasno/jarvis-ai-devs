package applyprogress

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type ValidationCode string

const (
	CodeCorruptSnapshot      ValidationCode = "corrupt_snapshot"
	CodeCorruptBatch         ValidationCode = "corrupt_batch"
	CodeInvalidTask          ValidationCode = "invalid_task"
	CodeInvalidTaskManifest  ValidationCode = "invalid_task_manifest"
	CodeTaskManifestMismatch ValidationCode = "task_manifest_mismatch"
	CodeDuplicateEvidence    ValidationCode = "duplicate_evidence"
	CodeMissingBatch         ValidationCode = "missing_batch"
	CodeInvalidCoverage      ValidationCode = "invalid_coverage"
	CodeLegacyAmbiguous      ValidationCode = "legacy_ambiguous"
	CodeInvalidPlan          ValidationCode = "invalid_plan"
	CodeInvalidCursor        ValidationCode = "invalid_cursor"
	CodeInvalidBase          ValidationCode = "invalid_base"
	CodeInvalidEvidence      ValidationCode = "invalid_evidence"
	// CodeLegacyMigration rejects imported evidence that is not bound to the
	// exact authoritative legacy apply-progress source.
	CodeLegacyMigration ValidationCode = "legacy_migration"
)

// ValidationError provides a stable machine-readable reason and the offending task, batch, or entry when available.
type ValidationError struct {
	Code   ValidationCode
	Detail string
}

func (e *ValidationError) Error() string {
	if e.Detail == "" {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Detail
}

// Unwrap preserves the historical sentinel contract while callers migrate to
// machine-readable validation codes.
func (e *ValidationError) Unwrap() error { return ErrInvalidValue }
func invalid(code ValidationCode, detail ...string) error {
	if len(detail) == 0 {
		return &ValidationError{Code: code}
	}
	return &ValidationError{Code: code, Detail: detail[0]}
}

// Task uses an explicit stable ID when available; Path exists only to derive a compatible legacy ID.
type Task struct {
	ID   string
	Path string
	Text string
}

// TaskManifest normalizes task text and returns its deterministic ordered digest.
func TaskManifest(tasks []Task) ([]Task, string, error) {
	if len(tasks) == 0 {
		return nil, "", invalid(CodeInvalidTaskManifest, "tasks")
	}
	out := make([]Task, len(tasks))
	seen := map[string]bool{}
	for i, task := range tasks {
		text, err := NormalizeTaskText(task.Text)
		if err != nil {
			return nil, "", invalid(CodeInvalidTask, taskDetail(i))
		}
		id, err := taskIdentity(task, text)
		if err != nil {
			return nil, "", invalid(CodeInvalidTask, taskDetail(i))
		}
		if seen[id] {
			return nil, "", invalid(CodeInvalidTaskManifest, id)
		}
		seen[id] = true
		out[i] = Task{ID: id, Path: task.Path, Text: text}
	}
	manifest := make([]struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}, len(out))
	for i, task := range out {
		manifest[i] = struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}{task.ID, task.Text}
	}
	data, err := canonicalJSON(manifest)
	if err != nil {
		return nil, "", err
	}
	return out, digest(data), nil
}

func taskDetail(index int) string { return "task " + strconv.Itoa(index) }

// taskIdentity prefers an explicit stable task ID; Path-derived IDs remain only for legacy compatibility.
// The manifest freezes the normalized text paired with that identity for a future shared tasks.md parser.
func taskIdentity(task Task, text string) (string, error) {
	if task.ID != "" {
		if !validID(task.ID) {
			return "", ErrInvalidID
		}
		return task.ID, nil
	}
	return LegacyTaskID(task.Path, text)
}

func NormalizeTaskText(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", invalid(CodeLegacyAmbiguous, "task text is not valid UTF-8")
	}
	text = strings.Join(strings.Fields(norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))), " ")
	if text == "" {
		return "", invalid(CodeLegacyAmbiguous, "task text is empty")
	}
	return text, nil
}

func LegacyTaskID(path, text string) (string, error) {
	if !validID(path) {
		return "", invalid(CodeLegacyAmbiguous, "legacy task path")
	}
	text, err := NormalizeTaskText(text)
	if err != nil {
		return "", err
	}
	return "legacy-" + digest([]byte(path + "\n" + text))[:32], nil
}

// ValidateEvidenceCoverage verifies only snapshot-to-batch coverage against immutable
// evidence. It intentionally does not inspect the task manifest or task ordering;
// callers that validate a full lifecycle state must use ValidateProgress as well.
func ValidateEvidenceCoverage(snapshot Snapshot, batches map[string]Batch) error {
	if snapshot.Status == StatusComplete && (len(snapshot.Coverage) == 0 || len(snapshot.Batches) == 0) {
		return invalid(CodeInvalidCoverage, "complete snapshot requires coverage and batches")
	}
	completed := make(map[string]Coverage)
	seenEntries := make(map[string]bool)
	for _, ref := range snapshot.Batches {
		batch, ok := batches[ref.BatchID]
		if !ok {
			return invalid(CodeMissingBatch, ref.BatchID)
		}
		for _, entry := range batch.Entries {
			if seenEntries[entry.EntryID] {
				return invalid(CodeDuplicateEvidence, entry.EntryID)
			}
			seenEntries[entry.EntryID] = true
			for _, id := range entry.CompletesTaskIDs {
				if _, exists := completed[id]; exists {
					return invalid(CodeDuplicateEvidence, entry.EntryID)
				}
				completed[id] = Coverage{TaskID: id, BatchID: batch.BatchID, EntryID: entry.EntryID}
			}
		}
	}
	covered := map[string]bool{}
	for _, coverage := range snapshot.Coverage {
		if covered[coverage.TaskID] || completed[coverage.TaskID] != coverage {
			return invalid(CodeInvalidCoverage, coverage.TaskID)
		}
		covered[coverage.TaskID] = true
	}
	if len(covered) != len(completed) {
		return invalid(CodeInvalidCoverage, "coverage count")
	}
	return nil
}

// ValidateSuccessor verifies the immutable chain transition between two snapshots.
// For stream-bound partial snapshots, the cursor advances by exactly the number of
// entries appended through successor batch refs. The next entry itself is not yet
// durable, so PlanCheckpoint validates NextEntryID against the complete stream on
// the next call. Pre-continuation snapshots retain legacy prefix-only compatibility.
func ValidateSuccessor(previous, successor Snapshot, batches map[string]Batch) error {
	if VerifySnapshot(previous) != nil || VerifySnapshot(successor) != nil {
		return invalid(CodeInvalidBase, "snapshot")
	}
	legacyEpochTransition := !previous.hasContinuation() && !successor.hasContinuation() && successor.Generation == previous.Generation+1
	sameEpochTransition := successor.Generation == previous.Generation
	if previous.Project != successor.Project || previous.Change != successor.Change || previous.Revision == ^uint64(0) || successor.Revision != previous.Revision+1 || successor.PreviousDigest != previous.Digest || (!sameEpochTransition && !legacyEpochTransition) {
		return invalid(CodeInvalidBase, "coordinates")
	}
	if previous.Status == StatusComplete || previous.Status == StatusSuperseded || previous.TaskManifestSHA256 != successor.TaskManifestSHA256 {
		return invalid(CodeInvalidBase, "immutable identity")
	}
	if successor.Status == StatusSuperseded {
		if !IsSupersessionSeal(previous, successor) {
			return invalid(CodeInvalidBase, "supersession seal")
		}
		return nil
	}
	if previous.Schema != successor.Schema || previous.SealIntent != nil || successor.SealIntent != nil || !equalSupersedes(previous.Supersedes, successor.Supersedes) {
		return invalid(CodeInvalidBase, "snapshot version or link")
	}
	if len(successor.Batches) < len(previous.Batches) || !slices.Equal(successor.Batches[:len(previous.Batches)], previous.Batches) {
		return invalid(CodeInvalidBase, "batches")
	}
	// Coverage is canonical task-manifest order, not evidence append order. A newly
	// completed earlier task may therefore be inserted before an existing coverage
	// record. Preserve every prior task-to-evidence binding exactly once instead of
	// treating prior coverage as a positional prefix.
	if !preservesCoverage(previous.Coverage, successor.Coverage) {
		return invalid(CodeInvalidBase, "coverage")
	}
	if successor.Status == StatusComplete && successor.hasContinuation() {
		return invalid(CodeInvalidBase, "terminal continuation")
	}
	if previous.StreamSHA256 == "" {
		entries, err := appendedEvidence(previous, successor, batches)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			if containsImportedEntries(entries) {
				return invalid(CodeLegacyMigration, "imported evidence may only initialize a stream")
			}
			// Historical v2 writers advanced both generation and revision before
			// continuation identity existed. Preserve that immutable shape.
			if legacyEpochTransition && !successor.hasContinuation() {
				return nil
			}
			// An exhausted ordinary stream leaves a partial snapshot without a
			// continuation binding. A later ordinary stream may append from that
			// immutable prefix without proving every remaining task up front; that
			// full completion proof belongs only to legacy continuation migration.
			if !sameEpochTransition {
				return invalid(CodeInvalidBase, "future stream epoch")
			}
			if successor.Status == StatusPartial && successor.hasContinuation() && successor.NextEntryIndex != len(entries) {
				return invalid(CodeInvalidBase, "future stream cursor")
			}
			return nil
		}
		// Historical v2 writers advanced both generation and revision before the
		// continuation identity existed. Their snapshots remain readable only when
		// both endpoints are pre-continuation and the immutable evidence prefix was
		// extended without rewrites. New writers do not use this shape: their
		// continuation-aware successors retain the established generation.
		if !successor.hasContinuation() {
			if !legacyEpochTransition {
				return invalid(CodeInvalidBase, "legacy epoch")
			}
			return invalid(CodeInvalidBase, "legacy evidence")
		}
		// A pre-continuation snapshot may then acquire only its continuation identity.
		// This continuation-aware successor stays in the established generation.
		if !sameEpochTransition || successor.NextEntryIndex != 0 || successor.NextEntryID == "" || successor.Status != previous.Status || !slices.Equal(successor.Batches, previous.Batches) || !slices.Equal(successor.Coverage, previous.Coverage) {
			return invalid(CodeInvalidBase, "legacy continuation")
		}
		return nil
	}
	if successor.Status == StatusPartial && (successor.StreamSHA256 != previous.StreamSHA256 || successor.NextEntryID == "") || previous.NextEntryID == "" {
		return invalid(CodeInvalidBase, "continuation")
	}
	entries, err := appendedEvidence(previous, successor, batches)
	if err != nil {
		return err
	}
	if len(entries) == 0 || entries[0].EntryID != previous.NextEntryID {
		return invalid(CodeInvalidBase, "continuation evidence")
	}
	if containsImportedEntries(entries) {
		return invalid(CodeLegacyMigration, "imported evidence may only initialize a stream")
	}
	if successor.Status == StatusPartial {
		if successor.NextEntryIndex != previous.NextEntryIndex+len(entries) {
			return invalid(CodeInvalidBase, "continuation")
		}
		return nil
	}
	stream, err := successorStream(previous, successor, batches)
	if err != nil || stream != previous.StreamSHA256 {
		return invalid(CodeInvalidBase, "terminal stream exhaustion")
	}
	return nil
}

// ValidateSuccessorGenesisPair checks self-digests and pair shape for a
// cross-change successor genesis against a superseded predecessor seal.
// The caller must supply the canonical authoritative stored sealed predecessor
// head, never one obtained from the pointer or an arbitrary supplied snapshot.
// The caller must separately validate store publication, receipt, and CAS;
// this pure check does not establish authority or inspect task contents.
// Subsequent same-change transitions use ValidateSuccessor.
func ValidateSuccessorGenesisPair(predecessor, successor Snapshot) error {
	if err := ValidateSuccessorHeadPair(predecessor, successor); err != nil {
		return err
	}
	if successor.Status != StatusPartial || successor.Generation != 1 || successor.Revision != 1 || successor.PreviousDigest != "" || len(successor.Batches) != 0 || len(successor.Coverage) != 0 || successor.hasContinuation() {
		return invalid(CodeInvalidBase, "successor genesis")
	}
	return nil
}

// ValidateSuccessorHeadPair checks the signed cross-change link at the current
// head. The caller must first authenticate both heads and their store lineages;
// this pure predicate does not prove a genesis or a publication by itself.
func ValidateSuccessorHeadPair(predecessor, successor Snapshot) error {
	if VerifySnapshot(predecessor) != nil || VerifySnapshot(successor) != nil {
		return invalid(CodeInvalidBase, "snapshot")
	}
	intent, pointer := predecessor.SealIntent, successor.Supersedes
	if predecessor.Schema != SupersessionSnapshotSchema || predecessor.Status != StatusSuperseded || intent == nil ||
		successor.Schema != SupersessionSnapshotSchema || (successor.Status != StatusPartial && successor.Status != StatusComplete && successor.Status != StatusSuperseded) || pointer == nil ||
		(successor.Status == StatusSuperseded) != (successor.SealIntent != nil) ||
		predecessor.Project != pointer.Project || predecessor.Change != pointer.Change || predecessor.Digest != pointer.SealDigest || predecessor.TaskManifestSHA256 != pointer.OriginalManifestSHA256 ||
		intent.SuccessorProject != successor.Project || intent.SuccessorChange != successor.Change || intent.SuccessorManifestSHA256 != successor.TaskManifestSHA256 ||
		successor.Project == predecessor.Project && successor.Change == predecessor.Change ||
		intent.Actor != pointer.Actor || intent.Reason != pointer.Reason || intent.Timestamp != pointer.Timestamp || intent.OperationID != pointer.OperationID {
		return invalid(CodeInvalidBase, "successor head")
	}
	return nil
}

// IsSupersessionSeal reports the sole allowed terminal transition without task lookup.
// It compares signed heads and requires identical immutable evidence and coverage.
func IsSupersessionSeal(previous, candidate Snapshot) bool {
	return VerifySnapshot(previous) == nil && VerifySnapshot(candidate) == nil &&
		previous.Status == StatusPartial && candidate.Status == StatusSuperseded &&
		(previous.Schema == SnapshotSchema || previous.Schema == SupersessionSnapshotSchema) &&
		candidate.Schema == SupersessionSnapshotSchema && candidate.SealIntent != nil &&
		previous.Project == candidate.Project && previous.Change == candidate.Change &&
		previous.Generation != 0 && previous.Generation == candidate.Generation && previous.Revision != ^uint64(0) &&
		candidate.Revision == previous.Revision+1 && candidate.PreviousDigest == previous.Digest &&
		candidate.TaskManifestSHA256 == previous.TaskManifestSHA256 &&
		slices.Equal(previous.Batches, candidate.Batches) && slices.Equal(previous.Coverage, candidate.Coverage) &&
		!candidate.hasContinuation() && equalSupersedes(previous.Supersedes, candidate.Supersedes)
}

func equalSupersedes(a, b *SupersedesPointer) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ValidateHistoricalZeroRootUpgrade validates the one migration transition from a
// decoder-confirmed explicit-zero v2 root. It preserves the authenticated root
// bytes and permits only the continuation binding validated by ValidateSuccessor.
func ValidateHistoricalZeroRootUpgrade(previous, successor Snapshot, batches map[string]Batch) error {
	if !previous.RequiresContinuationUpgrade() || previous.Generation != 0 || previous.Revision != 0 || previous.PreviousDigest != "" || successor.Status == StatusSuperseded {
		return invalid(CodeInvalidBase, "historical explicit-zero root")
	}
	return ValidateSuccessor(previous, successor, batches)
}

func preservesCoverage(previous, successor []Coverage) bool {
	if len(successor) < len(previous) {
		return false
	}
	remaining := make(map[Coverage]int, len(previous))
	for _, item := range previous {
		remaining[item]++
	}
	for _, item := range successor {
		if count := remaining[item]; count > 0 {
			remaining[item] = count - 1
		}
	}
	for _, count := range remaining {
		if count != 0 {
			return false
		}
	}
	// An exact duplicate of a prior record would otherwise be accepted after the
	// original decrements its counter. It is never a valid successor mutation.
	seen := make(map[Coverage]bool, len(successor))
	for _, item := range successor {
		if seen[item] {
			return false
		}
		seen[item] = true
	}
	return true
}

func appendedEvidence(previous, successor Snapshot, batches map[string]Batch) ([]EvidenceEntry, error) {
	return evidenceForRefs(successor, successor.Batches[len(previous.Batches):], batches)
}

func containsImportedEntries(entries []EvidenceEntry) bool {
	for _, entry := range entries {
		if entry.Kind == EvidenceImported {
			return true
		}
	}
	return false
}

func successorStream(previous, successor Snapshot, batches map[string]Batch) (string, error) {
	priorEntries, err := evidenceForRefs(previous, previous.Batches, batches)
	if err != nil || previous.NextEntryIndex > len(priorEntries) {
		return "", invalid(CodeInvalidBase, "stream prefix")
	}
	appended, err := appendedEvidence(previous, successor, batches)
	if err != nil {
		return "", err
	}
	// Imported or legacy evidence may precede a newly bound stream. The cursor
	// counts only that stream, so select precisely its durable prefix before
	// appending the terminal batch and bind its exact exhaustion hash.
	entries := append(slices.Clone(priorEntries[len(priorEntries)-previous.NextEntryIndex:]), appended...)
	return StreamSHA256(entries)
}

func evidenceForRefs(snapshot Snapshot, refs []BatchRef, batches map[string]Batch) ([]EvidenceEntry, error) {
	entries := []EvidenceEntry{}
	for _, ref := range refs {
		batch, ok := batches[ref.BatchID]
		if !ok || VerifyBatch(batch) != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return nil, invalid(CodeInvalidBase, "appended batch")
		}
		entries = append(entries, batch.Entries...)
	}
	return entries, nil
}

// ValidateProgress accepts only one canonical snapshot and its referenced batches.
// Batches not referenced by the snapshot are deliberately non-authoritative.
func ValidateProgress(snapshotData []byte, tasks []Task, batches map[string][]byte) error {
	snapshot, err := DecodeCanonicalSnapshot(snapshotData)
	if err != nil {
		return invalid(CodeCorruptSnapshot, "snapshot")
	}
	normalized, manifest, err := TaskManifest(tasks)
	if err != nil {
		return err
	}
	if snapshot.TaskManifestSHA256 != manifest {
		return invalid(CodeTaskManifestMismatch, "task_manifest_sha256")
	}
	known := make(map[string]bool, len(normalized))
	for _, task := range normalized {
		known[task.ID] = true
	}
	seenBatches, seenEntries, completed := map[string]bool{}, map[string]bool{}, map[string]Coverage{}
	for _, ref := range snapshot.Batches {
		if seenBatches[ref.BatchID] {
			return invalid(CodeDuplicateEvidence, ref.BatchID)
		}
		seenBatches[ref.BatchID] = true
		raw, ok := batches[ref.BatchID]
		if !ok {
			return invalid(CodeMissingBatch, ref.BatchID)
		}
		batch, err := DecodeCanonicalBatch(raw)
		if err != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return invalid(CodeCorruptBatch, ref.BatchID)
		}
		for _, entry := range batch.Entries {
			if seenEntries[entry.EntryID] {
				return invalid(CodeDuplicateEvidence, entry.EntryID)
			}
			seenEntries[entry.EntryID] = true
			attributed := map[string]bool{}
			for _, id := range entry.TaskIDs {
				if !known[id] || attributed[id] {
					return invalid(CodeInvalidCoverage, entry.EntryID)
				}
				attributed[id] = true
			}
			for _, id := range entry.CompletesTaskIDs {
				if !known[id] || !attributed[id] {
					return invalid(CodeInvalidCoverage, entry.EntryID)
				}
				if _, exists := completed[id]; exists {
					return invalid(CodeDuplicateEvidence, entry.EntryID)
				}
				completed[id] = Coverage{TaskID: id, BatchID: batch.BatchID, EntryID: entry.EntryID}
			}
		}
	}
	coverage := make([]Coverage, 0, len(completed))
	for _, task := range normalized {
		if item, ok := completed[task.ID]; ok {
			coverage = append(coverage, item)
		}
	}
	if !slices.Equal(snapshot.Coverage, coverage) {
		return invalid(CodeInvalidCoverage, "coverage ordering")
	}
	full := len(completed) == len(normalized)
	if !full && snapshot.Status != StatusPartial {
		return invalid(CodeInvalidCoverage, "status coverage")
	}
	if full && snapshot.Status != StatusComplete && !(snapshot.Status == StatusPartial && snapshot.hasContinuation()) {
		return invalid(CodeInvalidCoverage, "status coverage")
	}
	return nil
}

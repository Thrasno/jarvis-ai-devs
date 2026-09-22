package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/google/uuid"
)

var (
	ErrApplyProgressNotFound         = errors.New("apply progress not found")
	ErrApplyProgressReceiptNotFound  = errors.New("apply progress receipt not found")
	ErrApplyProgressInvalid          = errors.New("invalid apply progress request")
	ErrApplyProgressRequestConflict  = errors.New("apply progress request ID conflicts with a prior payload")
	ErrApplyProgressBatchCollision   = errors.New("apply progress batch ID has different immutable bytes")
	ErrApplyProgressEvidenceNotFound = errors.New("apply progress evidence not found")
	ErrApplyProgressTopicReserved    = errors.New("apply progress topic is reserved for guarded writes")
)

// ApplyProgressState is the current guarded snapshot and its CAS coordinates.
type ApplyProgressState struct {
	Generation uint64                 `json:"generation"`
	Revision   uint64                 `json:"revision"`
	Digest     string                 `json:"digest"`
	Snapshot   applyprogress.Snapshot `json:"snapshot"`
	// Batches is internal resolution data used by guarded writes and replay. It is
	// never serialized: read clients retrieve one referenced immutable document at
	// a time through GetApplyProgressEvidence.
	Batches []applyprogress.Batch `json:"-"`
}

type ApplyProgressReceipt struct {
	RequestID     string `json:"request_id"`
	PayloadSHA256 string `json:"payload_sha256"`
}

// ApplyProgressAdvance is intentionally distinct from models.Memory and SaveMemory.
type ApplyProgressAdvance struct {
	Project            string `json:"project"`
	Change             string `json:"change"`
	RequestID          string `json:"request_id"`
	ExpectedGeneration uint64 `json:"expected_generation"`
	ExpectedRevision   uint64 `json:"expected_revision"`
	ExpectedDigest     string `json:"expected_digest"`
	// LegacySourceSHA256 binds imported evidence to the exact authoritative
	// legacy apply-progress artifact. It exists only for low-level recovery.
	LegacySourceSHA256 string                 `json:"legacy_source_sha256,omitempty"`
	Snapshot           applyprogress.Snapshot `json:"snapshot"`
	Batches            []applyprogress.Batch  `json:"batches"`
}

type ApplyProgressAdvanceResult struct {
	Outcome string               `json:"outcome"`
	State   ApplyProgressState   `json:"state"`
	Receipt ApplyProgressReceipt `json:"receipt"`
}

// applyProgressReceiptResult intentionally stores replay identity only. Keeping
// a resolved state in every receipt copied the cumulative evidence batch list
// into each retry record and made receipt storage grow quadratically.
type applyProgressReceiptResult struct {
	Outcome    string `json:"outcome"`
	Generation uint64 `json:"generation"`
	Revision   uint64 `json:"revision"`
	Digest     string `json:"digest"`
}

// GetApplyProgress reads only the guarded immutable snapshot/head metadata.
// It derives a peer-created successor without changing the guarded local head and
// never exposes aggregate historical batch bodies.
func (d *DB) GetApplyProgress(project, change string) (ApplyProgressState, error) {
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return ApplyProgressState{}, fmt.Errorf("begin apply progress read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := d.deriveApplyProgressState(tx, canonicalProjectKey(project), change)
	if err != nil {
		return ApplyProgressState{}, err
	}
	state.Batches = nil
	return state, nil
}

// GetApplyProgressEvidence returns one canonical immutable batch only when the
// batch is referenced by the current fully resolved guarded snapshot lineage.
func (d *DB) GetApplyProgressEvidence(project, change, batchID, expectedHeadDigest string) (applyprogress.Batch, error) {
	project = canonicalProjectKey(project)
	change = strings.TrimSpace(change)
	batchID = strings.TrimSpace(batchID)
	expectedHeadDigest = strings.TrimSpace(expectedHeadDigest)
	if project == "" || change == "" || !applyprogress.ValidID(batchID) || !applyprogress.ValidDigest(expectedHeadDigest) {
		return applyprogress.Batch{}, ErrApplyProgressInvalid
	}
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return applyprogress.Batch{}, fmt.Errorf("begin apply evidence read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := d.deriveApplyProgressState(tx, project, change)
	if err != nil {
		return applyprogress.Batch{}, err
	}
	if state.Digest != expectedHeadDigest {
		return applyprogress.Batch{}, ErrApplyProgressEvidenceNotFound
	}
	var reference applyprogress.BatchRef
	found := false
	for _, ref := range state.Snapshot.Batches {
		if ref.BatchID == batchID {
			reference, found = ref, true
			break
		}
	}
	if !found {
		return applyprogress.Batch{}, ErrApplyProgressEvidenceNotFound
	}
	topic := "sdd/" + change + "/apply-evidence/" + batchID
	rows, err := tx.Query(`SELECT content FROM memories WHERE project = ? AND topic_key = ? AND deleted_at IS NULL`, project, topic)
	if err != nil {
		return applyprogress.Batch{}, fmt.Errorf("read guarded apply evidence: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var canonical []byte
	var batch applyprogress.Batch
	for rows.Next() {
		var content []byte
		if err := rows.Scan(&content); err != nil {
			return applyprogress.Batch{}, ErrApplyProgressInvalid
		}
		decoded, err := applyprogress.DecodeCanonicalBatch(content)
		if err != nil || decoded.Project != project || decoded.Change != change || decoded.BatchID != batchID || decoded.SHA256 != reference.SHA256 {
			return applyprogress.Batch{}, ErrApplyProgressEvidenceNotFound
		}
		if canonical != nil && !bytes.Equal(canonical, content) {
			return applyprogress.Batch{}, ErrApplyProgressEvidenceNotFound
		}
		canonical, batch = bytes.Clone(content), decoded
	}
	if rows.Err() != nil || canonical == nil {
		return applyprogress.Batch{}, ErrApplyProgressEvidenceNotFound
	}
	return batch, nil
}

// deriveApplyProgressState accepts exactly one complete validated immutable
// lineage. A local head may be an ancestor of its terminal snapshot, but forks,
// gaps, rewrites, and malformed remote documents fail closed as typed invalid
// requests. It never persists its derived result.
func (d *DB) deriveApplyProgressState(query applyProgressQuery, project, change string) (ApplyProgressState, error) {
	lineage, err := newApplyProgressLineageResolver(d, query).resolve(project, change)
	if err != nil {
		return ApplyProgressState{}, err
	}
	return applyProgressStateFromSnapshot(lineage.terminal.snapshot, lineage.terminal.batches), nil
}

// reconcileApplyProgressHeadTx persists a derived immutable successor only while
// an authorized AdvanceApplyProgress transaction is already in progress.
func (d *DB) reconcileApplyProgressHeadTx(tx *sql.Tx, project, change string) (ApplyProgressState, error) {
	lineage, err := newApplyProgressLineageResolver(d, tx).resolve(project, change)
	if err != nil {
		return ApplyProgressState{}, err
	}
	if !lineage.currentPresent {
		if _, err := tx.Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES (?, ?, ?, ?, ?, ?)`, project, change, lineage.terminal.id, lineage.terminal.snapshot.Generation, lineage.terminal.snapshot.Revision, lineage.terminal.snapshot.Digest); err != nil {
			return ApplyProgressState{}, fmt.Errorf("restore apply progress head: %w", err)
		}
	} else if lineage.current.Digest != lineage.terminal.snapshot.Digest {
		result, err := tx.Exec(`UPDATE sdd_apply_heads SET snapshot_memory_id = ?, generation = ?, revision = ?, digest = ? WHERE project = ? AND change_name = ? AND generation = ? AND revision = ? AND digest = ?`, lineage.terminal.id, lineage.terminal.snapshot.Generation, lineage.terminal.snapshot.Revision, lineage.terminal.snapshot.Digest, project, change, lineage.current.Generation, lineage.current.Revision, lineage.current.Digest)
		if err != nil {
			return ApplyProgressState{}, fmt.Errorf("advance reconciled apply progress head: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return ApplyProgressState{}, invalidApplyProgressTopology()
		}
	}
	return applyProgressStateFromSnapshot(lineage.terminal.snapshot, lineage.terminal.batches), nil
}

func invalidApplyProgressTopology() error {
	return fmt.Errorf("%w: immutable snapshot topology", ErrApplyProgressInvalid)
}

type applyProgressCandidate struct {
	id       int64
	content  []byte
	snapshot applyprogress.Snapshot
	batches  map[string]applyprogress.Batch
}

type applyProgressBatchCacheEntry struct {
	batch applyprogress.Batch
	err   error
}

// applyProgressLineageResolver resolves one immutable snapshot lineage. Its batch
// cache is deliberately scoped to one read or write transaction: snapshots are
// cumulative, so a batch may be referenced by every successor in the lineage.
type applyProgressLineageResolver struct {
	db      *DB
	query   applyProgressQuery
	batches map[string]applyProgressBatchCacheEntry
}

type applyProgressLineage struct {
	current        ApplyProgressState
	currentPresent bool
	terminal       applyProgressCandidate
}

func newApplyProgressLineageResolver(d *DB, query applyProgressQuery) *applyProgressLineageResolver {
	return &applyProgressLineageResolver{db: d, query: query, batches: make(map[string]applyProgressBatchCacheEntry)}
}

// resolve is the single live-lineage authority for both read derivation and
// write reconciliation. It intentionally preserves the former fail-closed
// topology behavior for missing heads, forks, gaps, and corrupt documents.
func (r *applyProgressLineageResolver) resolve(project, change string) (applyProgressLineage, error) {
	current, currentErr := r.db.applyProgressHeadMetadata(r.query, project, change)
	currentPresent := currentErr == nil
	if currentErr != nil && !errors.Is(currentErr, ErrApplyProgressNotFound) {
		return applyProgressLineage{}, currentErr
	}
	if currentPresent {
		batches, err := r.resolveBatches(current.Snapshot)
		if err != nil {
			return applyProgressLineage{}, err
		}
		current = applyProgressStateFromSnapshot(current.Snapshot, batches)
	}
	candidates, err := r.loadCandidates(project, change)
	if err != nil {
		return applyProgressLineage{}, invalidApplyProgressTopology()
	}
	if len(candidates) == 0 {
		if !currentPresent {
			return applyProgressLineage{}, ErrApplyProgressNotFound
		}
		return applyProgressLineage{}, invalidApplyProgressTopology()
	}
	terminal, ok := uniqueApplyProgressTerminal(candidates)
	if !ok {
		return applyProgressLineage{}, invalidApplyProgressTopology()
	}
	if currentPresent {
		if _, found := candidates[current.Digest]; !found {
			return applyProgressLineage{}, invalidApplyProgressTopology()
		}
	}
	return applyProgressLineage{current: current, currentPresent: currentPresent, terminal: terminal}, nil
}

func (r *applyProgressLineageResolver) loadCandidates(project, change string) (map[string]applyProgressCandidate, error) {
	topic := "sdd/" + change + "/apply-progress/v2"
	rows, err := r.query.Query(`SELECT id, content FROM memories WHERE project = ? AND topic_key = ? AND deleted_at IS NULL`, project, topic)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byDigest := map[string]applyProgressCandidate{}
	for rows.Next() {
		var item applyProgressCandidate
		var content []byte
		if err := rows.Scan(&item.id, &content); err != nil {
			return nil, err
		}
		item.content = bytes.Clone(content)
		item.snapshot, err = applyprogress.DecodeCanonicalSnapshot(content)
		if err != nil || item.snapshot.Project != project || item.snapshot.Change != change {
			return nil, errors.New("invalid immutable snapshot")
		}
		item.batches, err = r.resolveBatches(item.snapshot)
		if err != nil {
			return nil, err
		}
		if duplicate, found := byDigest[item.snapshot.Digest]; found {
			if !bytes.Equal(duplicate.content, item.content) {
				return nil, errors.New("rewritten immutable snapshot")
			}
			continue
		}
		byDigest[item.snapshot.Digest] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return byDigest, nil
}

func uniqueApplyProgressTerminal(snapshots map[string]applyProgressCandidate) (applyProgressCandidate, bool) {
	predecessors := make(map[string]bool, len(snapshots))
	for _, item := range snapshots {
		if item.snapshot.PreviousDigest != "" {
			predecessors[item.snapshot.PreviousDigest] = true
		}
	}
	var terminal applyProgressCandidate
	count := 0
	for digest, item := range snapshots {
		if !predecessors[digest] {
			terminal, count = item, count+1
		}
	}
	return terminal, count == 1 && validApplyProgressLineage(terminal, snapshots)
}

func validApplyProgressLineage(head applyProgressCandidate, snapshots map[string]applyProgressCandidate) bool {
	current := head
	visited := make(map[string]bool, len(snapshots))
	for {
		if visited[current.snapshot.Digest] {
			return false
		}
		visited[current.snapshot.Digest] = true
		if current.snapshot.PreviousDigest == "" {
			initial := current.snapshot.Generation == 1 && current.snapshot.Revision == 1
			historicalZeroRoot := current.snapshot.Generation == 0 && current.snapshot.Revision == 0 && current.snapshot.RequiresContinuationUpgrade()
			return (initial || historicalZeroRoot) && len(visited) == len(snapshots)
		}
		previous, found := snapshots[current.snapshot.PreviousDigest]
		if !found {
			return false
		}
		if previous.snapshot.RequiresContinuationUpgrade() {
			if applyprogress.ValidateHistoricalZeroRootUpgrade(previous.snapshot, current.snapshot, current.batches) != nil {
				return false
			}
		} else if current.snapshot.Generation == 0 || current.snapshot.Revision == 0 || applyprogress.ValidateSuccessor(previous.snapshot, current.snapshot, current.batches) != nil {
			return false
		}
		current = previous
	}
}

// GetApplyProgressReceipt returns the bounded identity of a committed request.
// It deliberately excludes the stored response payload and requires the request's
// project and change binding, so an ID cannot be probed across apply-progress streams.
func (d *DB) GetApplyProgressReceipt(project, change, requestID string) (ApplyProgressReceipt, error) {
	project = canonicalProjectKey(project)
	change = strings.TrimSpace(change)
	requestID = strings.TrimSpace(requestID)
	if project == "" || change == "" || !applyprogress.ValidID(requestID) {
		return ApplyProgressReceipt{}, ErrApplyProgressInvalid
	}

	var receipt ApplyProgressReceipt
	err := d.sqlDB.QueryRow(`SELECT request_id, payload_sha256 FROM sdd_apply_receipts WHERE request_id = ? AND project = ? AND change_name = ?`, requestID, project, change).Scan(&receipt.RequestID, &receipt.PayloadSHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return ApplyProgressReceipt{}, ErrApplyProgressReceiptNotFound
	}
	if err != nil {
		return ApplyProgressReceipt{}, fmt.Errorf("read apply progress receipt identity: %w", err)
	}
	return receipt, nil
}

// AdvanceApplyProgress atomically enforces batch immutability, snapshot CAS, and
// request-id idempotency. It never calls SaveMemory or changes general memory writes.
func (d *DB) AdvanceApplyProgress(request ApplyProgressAdvance) (ApplyProgressAdvanceResult, error) {
	request.Project = canonicalProjectKey(request.Project)
	request.Change = strings.TrimSpace(request.Change)
	if request.Project == "" || request.Change == "" || !applyprogress.ValidID(request.RequestID) || request.Snapshot.Project != request.Project || request.Snapshot.Change != request.Change || request.Batches == nil {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	if request.ExpectedRevision == ^uint64(0) {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	// A fresh stream starts at generation one. The only valid generation-zero
	// successor is the decoder-confirmed historical explicit-zero root upgrade;
	// it is checked against the resolved head inside this transaction below.
	validRequestedGeneration := request.Snapshot.Generation == request.ExpectedGeneration
	if request.ExpectedGeneration == 0 {
		validRequestedGeneration = request.Snapshot.Generation == 0 || request.Snapshot.Generation == 1
	}
	if !validRequestedGeneration || request.Snapshot.Revision != request.ExpectedRevision+1 || request.Snapshot.PreviousDigest != request.ExpectedDigest {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	snapshot, snapshotBytes, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		var capacity *applyprogress.CapacityError
		if errors.As(err, &capacity) && capacity.Document == "snapshot" {
			capacityDetails, capacityErr := d.applyProgressSnapshotCapacity(request.Project, request.Change, capacity.Runes)
			if capacityErr != nil {
				return ApplyProgressAdvanceResult{}, capacityErr
			}
			return ApplyProgressAdvanceResult{}, fmt.Errorf("%w: %w", ErrApplyProgressInvalid, &applyprogress.SnapshotCapacityExhaustedError{Capacity: capacityDetails})
		}
		return ApplyProgressAdvanceResult{}, fmt.Errorf("%w: snapshot: %w", ErrApplyProgressInvalid, err)
	}
	batches, batchBytes, err := sealApplyProgressBatches(request.Batches, request.Project, request.Change)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	payloadDigest := applyProgressPayloadDigest(request, snapshotBytes, batchBytes)

	tx, err := d.sqlDB.Begin()
	if err != nil {
		return ApplyProgressAdvanceResult{}, fmt.Errorf("begin apply progress advance: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var storedDigest, storedResponse string
	err = tx.QueryRow(`SELECT payload_sha256, response_json FROM sdd_apply_receipts WHERE request_id = ?`, request.RequestID).Scan(&storedDigest, &storedResponse)
	switch {
	case err == nil:
		if storedDigest != payloadDigest {
			return ApplyProgressAdvanceResult{}, ErrApplyProgressRequestConflict
		}
		stored, err := decodeApplyProgressReceiptResult([]byte(storedResponse))
		if err != nil {
			return ApplyProgressAdvanceResult{}, err
		}
		if stored.Outcome != "committed" || stored.Generation != snapshot.Generation || stored.Revision != snapshot.Revision || stored.Digest != snapshot.Digest {
			return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
		}
		// Re-resolve immutable evidence from its canonical store on every replay;
		// receipt metadata is not an authority for a historical batch copy.
		if _, err := validateApplyProgressBatchRefs(tx, snapshot, batches, batchBytes, request.Project, request.Change); err != nil {
			return ApplyProgressAdvanceResult{}, err
		}
		canonical, err := resolveApplyProgressBatches(tx, snapshot)
		if err != nil {
			return ApplyProgressAdvanceResult{}, err
		}
		return ApplyProgressAdvanceResult{Outcome: "committed", State: applyProgressStateFromSnapshot(snapshot, canonical), Receipt: ApplyProgressReceipt{RequestID: request.RequestID, PayloadSHA256: payloadDigest}}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return ApplyProgressAdvanceResult{}, fmt.Errorf("read apply progress receipt: %w", err)
	}
	// A committed request must remain replayable even if project governance later
	// blocks writes. Registration and writable-state checks apply only to new work.
	if _, err := registerProjectIdentity(context.Background(), tx, request.Project); err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	if err := ensureProjectWritableInTx(tx, request.Project); err != nil {
		return ApplyProgressAdvanceResult{}, err
	}

	current, err := d.reconcileApplyProgressHeadTx(tx, request.Project, request.Change)
	headPresent := err == nil
	if errors.Is(err, ErrApplyProgressNotFound) {
		current = ApplyProgressState{}
	} else if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	if current.Generation != request.ExpectedGeneration || current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return ApplyProgressAdvanceResult{Outcome: "conflict", State: current}, nil
	}
	if !headPresent && request.Snapshot.Generation != 1 {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	if headPresent && request.Snapshot.Generation == 0 && !current.Snapshot.RequiresContinuationUpgrade() {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	referenced, err := validateApplyProgressBatchRefs(tx, snapshot, batches, batchBytes, request.Project, request.Change)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	if !headPresent && snapshot.StreamSHA256 != "" {
		initialEntries := 0
		for _, ref := range snapshot.Batches {
			initialEntries += len(referenced[ref.BatchID].Entries)
		}
		if snapshot.NextEntryIndex != initialEntries {
			return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
		}
	}
	if err := validateImportedApplyProgressEvidence(tx, request, current, snapshotBytes, referenced); err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	// A seal preserves the signed predecessor's immutable evidence under its old
	// manifest. Only that exact shared transition may skip changed Hive tasks.
	seal := headPresent && applyprogress.IsSupersessionSeal(current.Snapshot, snapshot)
	if !seal {
		if err := validateApplyProgressAgainstAuthoritativeTasks(tx, request, snapshotBytes, referenced); err != nil {
			return ApplyProgressAdvanceResult{}, err
		}
	}
	if headPresent {
		var validationErr error
		if current.Snapshot.RequiresContinuationUpgrade() {
			validationErr = applyprogress.ValidateHistoricalZeroRootUpgrade(current.Snapshot, snapshot, referenced)
		} else {
			validationErr = applyprogress.ValidateSuccessor(current.Snapshot, snapshot, referenced)
		}
		if validationErr != nil {
			return ApplyProgressAdvanceResult{}, fmt.Errorf("%w: %w", ErrApplyProgressInvalid, validationErr)
		}
	}
	for i, batch := range batches {
		if err := d.storeApplyProgressBatch(tx, request.Project, request.Change, batch, batchBytes[i]); err != nil {
			return ApplyProgressAdvanceResult{}, err
		}
	}
	snapshotID, err := d.insertApplyProgressMemory(tx, request.Project, request.Change, "apply-progress/v2", "Apply progress snapshot", snapshotBytes)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	if !headPresent {
		var insert sql.Result
		insert, err = tx.Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES (?, ?, ?, ?, ?, ?)`, request.Project, request.Change, snapshotID, snapshot.Generation, snapshot.Revision, snapshot.Digest)
		if err == nil {
			var changed int64
			changed, err = insert.RowsAffected()
			if err == nil && changed != 1 {
				observed, fallbackErr := d.applyProgressState(tx, request.Project, request.Change)
				if fallbackErr != nil {
					return ApplyProgressAdvanceResult{}, fmt.Errorf("%w: validate insert-race conflict state: %v", ErrApplyProgressInvalid, fallbackErr)
				}
				return ApplyProgressAdvanceResult{Outcome: "conflict", State: observed}, nil
			}
		}
	} else {
		var update sql.Result
		update, err = tx.Exec(`UPDATE sdd_apply_heads SET snapshot_memory_id = ?, generation = ?, revision = ?, digest = ? WHERE project = ? AND change_name = ? AND generation = ? AND revision = ? AND digest = ?`, snapshotID, snapshot.Generation, snapshot.Revision, snapshot.Digest, request.Project, request.Change, current.Generation, current.Revision, current.Digest)
		if err == nil {
			var changed int64
			changed, err = update.RowsAffected()
			if err == nil && changed != 1 {
				return ApplyProgressAdvanceResult{Outcome: "conflict", State: current}, nil
			}
		}
	}
	if err != nil {
		return ApplyProgressAdvanceResult{}, fmt.Errorf("advance apply progress head: %w", err)
	}
	result := ApplyProgressAdvanceResult{
		Outcome: "committed",
		State:   applyProgressStateFromSnapshot(snapshot, referenced),
		Receipt: ApplyProgressReceipt{RequestID: request.RequestID, PayloadSHA256: payloadDigest},
	}
	response, err := json.Marshal(applyProgressReceiptResult{Outcome: result.Outcome, Generation: result.State.Generation, Revision: result.State.Revision, Digest: result.State.Digest})
	if err != nil {
		return ApplyProgressAdvanceResult{}, fmt.Errorf("encode apply progress receipt: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES (?, ?, ?, ?, ?)`, request.RequestID, request.Project, request.Change, payloadDigest, response); err != nil {
		return ApplyProgressAdvanceResult{}, fmt.Errorf("store apply progress receipt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ApplyProgressAdvanceResult{}, fmt.Errorf("commit apply progress advance: %w", err)
	}
	return result, nil
}

// decodeApplyProgressReceiptResult accepts the legacy response_json shape while
// all new writes use the bounded metadata form above.
func decodeApplyProgressReceiptResult(data []byte) (applyProgressReceiptResult, error) {
	var stored applyProgressReceiptResult
	if err := json.Unmarshal(data, &stored); err != nil {
		return applyProgressReceiptResult{}, fmt.Errorf("decode apply progress receipt: %w", err)
	}
	if stored.Generation != 0 || stored.Revision != 0 || stored.Digest != "" {
		return stored, nil
	}
	var legacy ApplyProgressAdvanceResult
	if err := json.Unmarshal(data, &legacy); err != nil {
		return applyProgressReceiptResult{}, fmt.Errorf("decode legacy apply progress receipt: %w", err)
	}
	return applyProgressReceiptResult{Outcome: legacy.Outcome, Generation: legacy.State.Generation, Revision: legacy.State.Revision, Digest: legacy.State.Digest}, nil
}

func sealApplyProgressBatches(input []applyprogress.Batch, project, change string) ([]applyprogress.Batch, [][]byte, error) {
	out := make([]applyprogress.Batch, len(input))
	data := make([][]byte, len(input))
	seen := map[string]bool{}
	for i, batch := range input {
		if batch.Project != project || batch.Change != change || seen[batch.BatchID] {
			return nil, nil, ErrApplyProgressInvalid
		}
		seen[batch.BatchID] = true
		sealed, bytes, err := applyprogress.SealBatch(batch)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: batch: %w", ErrApplyProgressInvalid, err)
		}
		out[i], data[i] = sealed, bytes
	}
	return out, data, nil
}

func validateApplyProgressBatchRefs(tx *sql.Tx, snapshot applyprogress.Snapshot, batches []applyprogress.Batch, batchBytes [][]byte, project, change string) (map[string]applyprogress.Batch, error) {
	provided := make(map[string]int, len(batches))
	for i, batch := range batches {
		provided[batch.BatchID] = i
	}
	referenced := make(map[string]applyprogress.Batch, len(snapshot.Batches))
	seen := make(map[string]bool, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		if seen[ref.BatchID] {
			return nil, ErrApplyProgressInvalid
		}
		seen[ref.BatchID] = true
		if i, ok := provided[ref.BatchID]; ok {
			if batches[i].SHA256 != ref.SHA256 {
				return nil, ErrApplyProgressInvalid
			}
			referenced[ref.BatchID] = batches[i]
			continue
		}
		var content []byte
		topic := "sdd/" + change + "/apply-evidence/" + ref.BatchID
		if err := tx.QueryRow(`SELECT content FROM memories WHERE project = ? AND topic_key = ? AND deleted_at IS NULL`, project, topic).Scan(&content); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrApplyProgressInvalid
			}
			return nil, fmt.Errorf("read referenced apply progress batch: %w", err)
		}
		batch, err := applyprogress.DecodeCanonicalBatch(content)
		if err != nil || batch.Project != project || batch.Change != change || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 {
			return nil, ErrApplyProgressInvalid
		}
		referenced[ref.BatchID] = batch
	}
	for id := range provided {
		if !seen[id] {
			return nil, ErrApplyProgressInvalid
		}
	}
	// Imported evidence has a stricter authority path below: it must first bind
	// the canonical legacy tasks/progress conversion through ValidateProgress.
	// Do not let the taskless coverage projection hide that typed migration error.
	if !applyprogress.ContainsImportedEvidence(batches) {
		if err := applyprogress.ValidateEvidenceCoverage(snapshot, referenced); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrApplyProgressInvalid, err)
		}
	}
	return referenced, nil
}

type applyProgressQuery interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

// applyProgressSnapshotCapacity derives diagnostic counts exclusively from the
// persisted guarded head. A rejected projection is never persisted or reused as
// status state.
func (d *DB) applyProgressSnapshotCapacity(project, change string, projectedRunes int) (*applyprogress.CapacityError, error) {
	currentRunes := 0
	state, err := d.applyProgressHeadMetadata(d.sqlDB, project, change)
	if err != nil && !errors.Is(err, ErrApplyProgressNotFound) {
		return nil, err
	}
	if err == nil {
		data, marshalErr := json.Marshal(state.Snapshot)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode current apply progress snapshot: %w", marshalErr)
		}
		currentRunes = utf8.RuneCount(data)
	}
	return &applyprogress.CapacityError{Document: "snapshot", Runes: projectedRunes, Limit: applyprogress.MaxDocumentRunes, CurrentRunes: currentRunes, ProjectedRunes: projectedRunes, CeilingRunes: applyprogress.MaxDocumentRunes}, nil
}

func (d *DB) applyProgressHeadMetadata(query applyProgressQuery, project, change string) (ApplyProgressState, error) {
	var state ApplyProgressState
	var snapshotBytes []byte
	err := query.QueryRow(`SELECT h.generation, h.revision, h.digest, m.content FROM sdd_apply_heads h JOIN memories m ON m.id = h.snapshot_memory_id AND m.deleted_at IS NULL WHERE h.project = ? AND h.change_name = ?`, project, change).Scan(&state.Generation, &state.Revision, &state.Digest, &snapshotBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return ApplyProgressState{}, ErrApplyProgressNotFound
	}
	if err != nil {
		return ApplyProgressState{}, fmt.Errorf("read apply progress head: %w", err)
	}
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(snapshotBytes)
	if err != nil || snapshot.Generation != state.Generation || snapshot.Revision != state.Revision || snapshot.Digest != state.Digest {
		return ApplyProgressState{}, fmt.Errorf("%w: corrupt head", ErrApplyProgressInvalid)
	}
	state.Snapshot = snapshot
	return state, nil
}

func (d *DB) applyProgressState(query applyProgressQuery, project, change string) (ApplyProgressState, error) {
	state, err := d.applyProgressHeadMetadata(query, project, change)
	if err != nil {
		return ApplyProgressState{}, err
	}
	batches, err := resolveApplyProgressBatches(query, state.Snapshot)
	if err != nil {
		return ApplyProgressState{}, err
	}
	return applyProgressStateFromSnapshot(state.Snapshot, batches), nil
}

func applyProgressStateFromSnapshot(snapshot applyprogress.Snapshot, batches map[string]applyprogress.Batch) ApplyProgressState {
	ordered := make([]applyprogress.Batch, len(snapshot.Batches))
	for i, ref := range snapshot.Batches {
		ordered[i] = batches[ref.BatchID]
	}
	return ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot, Batches: ordered}
}

func resolveApplyProgressBatches(query applyProgressQuery, snapshot applyprogress.Snapshot) (map[string]applyprogress.Batch, error) {
	return newApplyProgressLineageResolver(nil, query).resolveBatches(snapshot)
}

func (r *applyProgressLineageResolver) resolveBatches(snapshot applyprogress.Snapshot) (map[string]applyprogress.Batch, error) {
	batches := make(map[string]applyprogress.Batch, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		if _, exists := batches[ref.BatchID]; exists {
			return nil, fmt.Errorf("%w: duplicate referenced batch", ErrApplyProgressInvalid)
		}
		batch, err := r.loadBatch(snapshot, ref.BatchID)
		if err != nil {
			return nil, err
		}
		if batch.SHA256 != ref.SHA256 {
			return nil, fmt.Errorf("%w: corrupt referenced batch", ErrApplyProgressInvalid)
		}
		batches[ref.BatchID] = batch
	}
	if err := applyprogress.ValidateEvidenceCoverage(snapshot, batches); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrApplyProgressInvalid, err)
	}
	return batches, nil
}

func (r *applyProgressLineageResolver) loadBatch(snapshot applyprogress.Snapshot, batchID string) (applyprogress.Batch, error) {
	if cached, found := r.batches[batchID]; found {
		return cached.batch, cached.err
	}

	entry := applyProgressBatchCacheEntry{}
	topic := "sdd/" + snapshot.Change + "/apply-evidence/" + batchID
	rows, err := r.query.Query(`SELECT content FROM memories WHERE project = ? AND topic_key = ? AND deleted_at IS NULL`, snapshot.Project, topic)
	if err != nil {
		entry.err = fmt.Errorf("%w: read referenced batch", ErrApplyProgressInvalid)
		r.batches[batchID] = entry
		return entry.batch, entry.err
	}
	count := 0
	var canonical []byte
	for rows.Next() {
		count++
		var content []byte
		if err := rows.Scan(&content); err != nil {
			entry.err = fmt.Errorf("%w: read referenced batch", ErrApplyProgressInvalid)
			break
		}
		batch, err := applyprogress.DecodeCanonicalBatch(content)
		if err != nil || batch.Project != snapshot.Project || batch.Change != snapshot.Change || batch.BatchID != batchID {
			entry.err = fmt.Errorf("%w: corrupt referenced batch", ErrApplyProgressInvalid)
			break
		}
		if canonical == nil {
			canonical = bytes.Clone(content)
			entry.batch = batch
		} else if !bytes.Equal(canonical, content) {
			entry.err = fmt.Errorf("%w: forked referenced batch", ErrApplyProgressInvalid)
			break
		}
	}
	if closeErr := rows.Close(); entry.err == nil && (closeErr != nil || count == 0) {
		entry.err = fmt.Errorf("%w: missing referenced batch", ErrApplyProgressInvalid)
	}
	r.batches[batchID] = entry
	return entry.batch, entry.err
}

func (d *DB) storeApplyProgressBatch(tx *sql.Tx, project, change string, batch applyprogress.Batch, bytes []byte) error {
	topic := "sdd/" + change + "/apply-evidence/" + batch.BatchID
	rows, err := tx.Query(`SELECT content FROM memories WHERE project = ? AND topic_key = ? AND deleted_at IS NULL`, project, topic)
	if err != nil {
		return fmt.Errorf("read apply progress batch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := false
	for rows.Next() {
		var existing []byte
		if err := rows.Scan(&existing); err != nil {
			return err
		}
		if string(existing) != string(bytes) {
			return ErrApplyProgressBatchCollision
		}
		found = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = d.insertApplyProgressMemory(tx, project, change, "apply-evidence/"+batch.BatchID, "Apply progress evidence "+batch.BatchID, bytes)
	return err
}

func (d *DB) insertApplyProgressMemory(tx *sql.Tx, project, change, suffix, title string, content []byte) (int64, error) {
	sessionID := "sdd-apply-" + project
	if _, err := tx.Exec(`INSERT OR IGNORE INTO sessions (id, sync_id, project, directory, dev_id, client) VALUES (?, ?, ?, '', 'system', 'sdd-apply')`, sessionID, uuid.NewString(), project); err != nil {
		return 0, fmt.Errorf("create apply progress session: %w", err)
	}
	now := time.Now().UTC()
	syncID := uuid.NewString()
	topic := "sdd/" + change + "/" + suffix
	var id int64
	if err := tx.QueryRow(`INSERT INTO memories (sync_id, project, topic_key, category, title, content, tags, files_affected, created_by, created_at, updated_at, session_id) VALUES (?, ?, ?, 'architecture', ?, ?, '[]', '[]', 'system', ?, ?, ?) RETURNING id`, syncID, project, topic, title, string(content), now, now, sessionID).Scan(&id); err != nil {
		return 0, fmt.Errorf("store apply progress document: %w", err)
	}
	memory := &models.Memory{Project: project, TopicKey: &topic, Category: "architecture", Title: title, Content: string(content), SessionID: sessionID}
	if err := insertMemoryMutation(tx, memoryMutationRecord{EventID: uuid.NewString(), EntitySyncID: syncID, Project: project, Op: MutationOpCreate, OccurredAt: now.Format("2006-01-02 15:04:05"), ActorID: "system", Payload: mutationPayload{Memory: memoryPayloadFromModel(memory, syncID, "system", now)}}); err != nil {
		return 0, fmt.Errorf("journal apply progress document: %w", err)
	}
	return id, nil
}

// validateImportedApplyProgressEvidence permits imported evidence only for the
// first write of a stream and only when its compatibility provenance binds the
// exact active Hive legacy artifact. Receipt replay returns before this check:
// the stored request payload already proves that original authorized mutation.
func validateImportedApplyProgressEvidence(tx *sql.Tx, request ApplyProgressAdvance, current ApplyProgressState, snapshotData []byte, batches map[string]applyprogress.Batch) error {
	imported := applyprogress.ContainsImportedEvidence(request.Batches)
	if !imported && request.LegacySourceSHA256 == "" {
		return nil
	}
	if !imported || current.Digest != "" || !applyprogress.ValidDigest(request.LegacySourceSHA256) {
		return legacyMigrationValidation("imported evidence requires an initial authoritative legacy source")
	}
	progress, err := authoritativeLegacyApplyProgressSource(tx, request.Project, request.Change)
	if err != nil {
		return err
	}
	if applyprogress.LegacySourceSHA256(progress) != request.LegacySourceSHA256 {
		return legacyMigrationValidation("legacy_source_sha256")
	}
	tasks, err := authoritativeLegacyTasks(tx, request.Project, request.Change)
	if err != nil {
		return err
	}
	converted, err := applyprogress.ConvertLegacy(applyprogress.LegacyProgress{Tasks: tasks.Tasks, Completed: tasks.CompletedIDs})
	if err != nil {
		return legacyMigrationValidation("authoritative legacy tasks")
	}
	if err := validateDeterministicLegacyImport(request, progress, converted, snapshotData, batches); err != nil {
		return err
	}
	return nil
}

func authoritativeLegacyTasks(tx *sql.Tx, project, change string) (applyprogress.ParsedTasks, error) {
	content, found, err := authoritativeApplyProgressTasksContent(tx, project, change)
	if err != nil {
		return applyprogress.ParsedTasks{}, err
	}
	if !found {
		return applyprogress.ParsedTasks{}, legacyMigrationValidation("authoritative tasks are missing")
	}
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(content)
	if err != nil || len(parsed.Tasks) == 0 {
		return applyprogress.ParsedTasks{}, legacyMigrationValidation("authoritative legacy tasks")
	}
	return parsed, nil
}

// validateApplyProgressAgainstAuthoritativeTasks upgrades older taskless streams
// conservatively, but whenever Hive has a canonical tasks artifact it validates the
// complete snapshot/evidence protocol before inserting either immutable document.
func validateApplyProgressAgainstAuthoritativeTasks(tx *sql.Tx, request ApplyProgressAdvance, snapshotData []byte, batches map[string]applyprogress.Batch) error {
	if applyprogress.ContainsImportedEvidence(request.Batches) {
		return nil // validateImportedApplyProgressEvidence already performed the stricter conversion proof.
	}
	content, found, err := authoritativeApplyProgressTasksContent(tx, request.Project, request.Change)
	if err != nil || !found {
		return err
	}
	parsed, strictErr := applyprogress.ParseTasksMarkdown(content)
	var tasks []applyprogress.Task
	if strictErr == nil && len(parsed.Tasks) > 0 {
		tasks = parsed.Tasks
	} else {
		legacy, legacyErr := applyprogress.ParseLegacyTasksMarkdown(content)
		if legacyErr != nil || len(legacy.Tasks) == 0 {
			return fmt.Errorf("%w: authoritative tasks", ErrApplyProgressInvalid)
		}
		converted, convertErr := applyprogress.ConvertLegacy(applyprogress.LegacyProgress{Tasks: legacy.Tasks, Completed: legacy.CompletedIDs})
		if convertErr != nil {
			return fmt.Errorf("%w: authoritative legacy tasks", ErrApplyProgressInvalid)
		}
		tasks = converted.Tasks
	}
	if err := applyprogress.ValidateProgress(snapshotData, tasks, canonicalBatchBytes(batches)); err != nil {
		return fmt.Errorf("%w: %w", ErrApplyProgressInvalid, err)
	}
	return nil
}

func authoritativeApplyProgressTasksContent(tx *sql.Tx, project, change string) (string, bool, error) {
	prefix := "sdd/" + change
	candidates, args, err := sddCandidateQuery(project, prefix, []string{"tasks"})
	if err != nil {
		return "", false, fmt.Errorf("read authoritative tasks: %w", err)
	}
	query := `WITH candidates AS (` + candidates + `),
		eligible AS (SELECT * FROM candidates WHERE logical_topic = ?),
		ranked AS (SELECT *, ROW_NUMBER() OVER (ORDER BY created_at DESC, id DESC) AS revision_rank FROM eligible)
		SELECT content FROM ranked WHERE revision_rank = 1`
	args = append(args, prefix+"/tasks")
	var content string
	if err := tx.QueryRow(query, args...).Scan(&content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read authoritative tasks: %w", err)
	}
	return content, true, nil
}

func validateDeterministicLegacyImport(request ApplyProgressAdvance, progress []byte, converted applyprogress.LegacyConversion, snapshotData []byte, batches map[string]applyprogress.Batch) error {
	complete, partial := false, false
	for _, line := range strings.Split(string(progress), "\n") {
		switch strings.TrimSpace(line) {
		case "status: complete":
			complete = true
		case "status: partial":
			partial = true
		default:
			if strings.HasPrefix(strings.TrimSpace(line), "status:") {
				return legacyMigrationValidation("legacy progress status")
			}
		}
	}
	if complete && partial || complete && len(converted.Completed) != len(converted.Tasks) {
		return legacyMigrationValidation("legacy progress completion")
	}
	if err := applyprogress.ValidateProgress(snapshotData, converted.Tasks, canonicalBatchBytes(batches)); err != nil {
		return legacyMigrationValidation("legacy progress validation")
	}
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(snapshotData)
	if err != nil || snapshot.TaskManifestSHA256 != converted.TaskManifestSHA256 {
		return legacyMigrationValidation("legacy task manifest")
	}
	sum := sha256.Sum256(progress)
	batchID := "apb-" + hex.EncodeToString(sum[:16])
	entryID := "imported-" + hex.EncodeToString(sum[:8])
	if len(converted.Completed) == 0 {
		if len(snapshot.Batches) != 0 || len(snapshot.Coverage) != 0 || snapshot.Status != applyprogress.StatusPartial {
			return legacyMigrationValidation("legacy empty import")
		}
		return nil
	}
	if len(snapshot.Batches) != 1 || len(snapshot.Coverage) != len(converted.Completed) || snapshot.Status != legacyImportStatus(complete, len(converted.Completed), len(converted.Tasks)) {
		return legacyMigrationValidation("legacy import topology")
	}
	batch, found := batches[batchID]
	if !found || len(batch.Entries) != 1 {
		return legacyMigrationValidation("legacy import batch")
	}
	entry := batch.Entries[0]
	if entry.EntryID != entryID || entry.Kind != applyprogress.EvidenceImported || !slices.Equal(entry.TaskIDs, converted.Completed) || !slices.Equal(entry.CompletesTaskIDs, converted.Completed) || entry.Summary != "imported legacy progress sha256="+hex.EncodeToString(sum[:]) || entry.Command != "legacy import" || entry.Outcome != applyprogress.OutcomePass || !slices.Equal(entry.Files, []string{"apply-progress.md"}) {
		return legacyMigrationValidation("legacy import evidence")
	}
	return nil
}

func legacyImportStatus(complete bool, completed, total int) applyprogress.Status {
	if complete || completed == total {
		return applyprogress.StatusComplete
	}
	return applyprogress.StatusPartial
}

func canonicalBatchBytes(batches map[string]applyprogress.Batch) map[string][]byte {
	out := make(map[string][]byte, len(batches))
	for id, batch := range batches {
		_, data, err := applyprogress.SealBatch(batch)
		if err == nil {
			out[id] = data
		}
	}
	return out
}

func authoritativeLegacyApplyProgressSource(tx *sql.Tx, project, change string) ([]byte, error) {
	prefix := "sdd/" + change
	candidates, args, err := sddCandidateQuery(project, prefix, []string{"apply-progress"})
	if err != nil {
		return nil, legacyMigrationValidation("authoritative legacy apply-progress")
	}
	topic := prefix + "/apply-progress"
	query := `WITH candidates AS (` + candidates + `),
		eligible AS (SELECT * FROM candidates WHERE logical_topic = ?),
		ranked AS (
			SELECT *, ROW_NUMBER() OVER (ORDER BY created_at DESC, id DESC) AS revision_rank
			FROM eligible
		)
		SELECT content FROM ranked WHERE revision_rank = 1`
	args = append(args, topic)
	var source string
	if err := tx.QueryRow(query, args...).Scan(&source); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, legacyMigrationValidation("authoritative legacy apply-progress is missing")
		}
		return nil, fmt.Errorf("read authoritative legacy apply-progress: %w", err)
	}
	data := []byte(source)
	if bytes.HasPrefix(data, []byte(`{"schema":"jarvis.sdd-apply-progress/v2"`)) {
		return nil, legacyMigrationValidation("authoritative apply-progress is already v2")
	}
	return data, nil
}

func legacyMigrationValidation(detail string) error {
	return fmt.Errorf("%w: %w", ErrApplyProgressInvalid, &applyprogress.ValidationError{Code: applyprogress.CodeLegacyMigration, Detail: detail})
}

func applyProgressPayloadDigest(request ApplyProgressAdvance, snapshot []byte, batches [][]byte) string {
	payload, _ := json.Marshal(struct {
		Project            string   `json:"project"`
		Change             string   `json:"change"`
		RequestID          string   `json:"request_id"`
		Generation         uint64   `json:"expected_generation"`
		Revision           uint64   `json:"expected_revision"`
		Digest             string   `json:"expected_digest"`
		LegacySourceSHA256 string   `json:"legacy_source_sha256,omitempty"`
		Snapshot           []byte   `json:"snapshot"`
		Batches            [][]byte `json:"batches"`
	}{request.Project, request.Change, request.RequestID, request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest, request.LegacySourceSHA256, snapshot, batches})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

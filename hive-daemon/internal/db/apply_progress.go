package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/google/uuid"
)

var (
	ErrApplyProgressNotFound        = errors.New("apply progress not found")
	ErrApplyProgressInvalid         = errors.New("invalid apply progress request")
	ErrApplyProgressRequestConflict = errors.New("apply progress request ID conflicts with a prior payload")
	ErrApplyProgressBatchCollision  = errors.New("apply progress batch ID has different immutable bytes")
)

// ApplyProgressState is the current guarded snapshot and its CAS coordinates.
type ApplyProgressState struct {
	Generation uint64                 `json:"generation"`
	Revision   uint64                 `json:"revision"`
	Digest     string                 `json:"digest"`
	Snapshot   applyprogress.Snapshot `json:"snapshot"`
}

type ApplyProgressReceipt struct {
	RequestID     string `json:"request_id"`
	PayloadSHA256 string `json:"payload_sha256"`
}

// ApplyProgressAdvance is intentionally distinct from models.Memory and SaveMemory.
type ApplyProgressAdvance struct {
	Project            string                 `json:"project"`
	Change             string                 `json:"change"`
	RequestID          string                 `json:"request_id"`
	ExpectedGeneration uint64                 `json:"expected_generation"`
	ExpectedRevision   uint64                 `json:"expected_revision"`
	ExpectedDigest     string                 `json:"expected_digest"`
	Snapshot           applyprogress.Snapshot `json:"snapshot"`
	Batches            []applyprogress.Batch  `json:"batches"`
}

type ApplyProgressAdvanceResult struct {
	Outcome string               `json:"outcome"`
	State   ApplyProgressState   `json:"state"`
	Receipt ApplyProgressReceipt `json:"receipt"`
}

// GetApplyProgress reads the snapshot selected by the durable head; it never uses
// the general SDD "latest memory" projection.
func (d *DB) GetApplyProgress(project, change string) (ApplyProgressState, error) {
	project = canonicalProjectKey(project)
	return d.applyProgressState(d.sqlDB, project, change)
}

// AdvanceApplyProgress atomically enforces batch immutability, snapshot CAS, and
// request-id idempotency. It never calls SaveMemory or changes general memory writes.
func (d *DB) AdvanceApplyProgress(request ApplyProgressAdvance) (ApplyProgressAdvanceResult, error) {
	request.Project = canonicalProjectKey(request.Project)
	request.Change = strings.TrimSpace(request.Change)
	if request.Project == "" || request.Change == "" || request.RequestID == "" || request.Snapshot.Project != request.Project || request.Snapshot.Change != request.Change || request.Batches == nil {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	if request.Snapshot.Generation != request.ExpectedGeneration+1 || request.Snapshot.Revision != request.ExpectedRevision+1 || request.Snapshot.PreviousDigest != request.ExpectedDigest {
		return ApplyProgressAdvanceResult{}, ErrApplyProgressInvalid
	}
	snapshot, snapshotBytes, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
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
		var replay ApplyProgressAdvanceResult
		if err := json.Unmarshal([]byte(storedResponse), &replay); err != nil {
			return ApplyProgressAdvanceResult{}, fmt.Errorf("decode apply progress receipt: %w", err)
		}
		return replay, nil
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

	current, err := d.applyProgressState(tx, request.Project, request.Change)
	if errors.Is(err, ErrApplyProgressNotFound) {
		current = ApplyProgressState{}
	} else if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	if current.Generation != request.ExpectedGeneration || current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return ApplyProgressAdvanceResult{Outcome: "conflict", State: current}, nil
	}
	if err := validateApplyProgressBatchRefs(tx, snapshot, batches, batchBytes, request.Project, request.Change); err != nil {
		return ApplyProgressAdvanceResult{}, err
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
	if current.Generation == 0 {
		_, err = tx.Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES (?, ?, ?, ?, ?, ?)`, request.Project, request.Change, snapshotID, snapshot.Generation, snapshot.Revision, snapshot.Digest)
	} else {
		var update sql.Result
		update, err = tx.Exec(`UPDATE sdd_apply_heads SET snapshot_memory_id = ?, generation = ?, revision = ?, digest = ? WHERE project = ? AND change_name = ? AND generation = ? AND revision = ? AND digest = ?`, snapshotID, snapshot.Generation, snapshot.Revision, snapshot.Digest, request.Project, request.Change, request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest)
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
		State:   ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot},
		Receipt: ApplyProgressReceipt{RequestID: request.RequestID, PayloadSHA256: payloadDigest},
	}
	response, err := json.Marshal(result)
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

func validateApplyProgressBatchRefs(tx *sql.Tx, snapshot applyprogress.Snapshot, batches []applyprogress.Batch, batchBytes [][]byte, project, change string) error {
	provided := make(map[string]int, len(batches))
	for i, batch := range batches {
		provided[batch.BatchID] = i
	}
	seen := make(map[string]bool, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		if seen[ref.BatchID] {
			return ErrApplyProgressInvalid
		}
		seen[ref.BatchID] = true
		if i, ok := provided[ref.BatchID]; ok {
			if batches[i].SHA256 != ref.SHA256 {
				return ErrApplyProgressInvalid
			}
			continue
		}
		var content []byte
		topic := "sdd/" + change + "/apply-evidence/" + ref.BatchID
		if err := tx.QueryRow(`SELECT content FROM memories WHERE project = ? AND topic_key = ? AND deleted_at IS NULL`, project, topic).Scan(&content); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrApplyProgressInvalid
			}
			return fmt.Errorf("read referenced apply progress batch: %w", err)
		}
		batch, err := applyprogress.DecodeCanonicalBatch(content)
		if err != nil || batch.Project != project || batch.Change != change || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 {
			return ErrApplyProgressInvalid
		}
	}
	for id := range provided {
		if !seen[id] {
			return ErrApplyProgressInvalid
		}
	}
	return nil
}

func (d *DB) applyProgressState(query interface{ QueryRow(string, ...any) *sql.Row }, project, change string) (ApplyProgressState, error) {
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

func applyProgressPayloadDigest(request ApplyProgressAdvance, snapshot []byte, batches [][]byte) string {
	payload, _ := json.Marshal(struct {
		Project    string   `json:"project"`
		Change     string   `json:"change"`
		RequestID  string   `json:"request_id"`
		Generation uint64   `json:"expected_generation"`
		Revision   uint64   `json:"expected_revision"`
		Digest     string   `json:"expected_digest"`
		Snapshot   []byte   `json:"snapshot"`
		Batches    [][]byte `json:"batches"`
	}{request.Project, request.Change, request.RequestID, request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest, snapshot, batches})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

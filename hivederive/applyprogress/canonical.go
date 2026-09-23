package applyprogress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

// SealBatch validates a batch, computes its SHA-256, and returns its canonical JSON.
func SealBatch(batch Batch) (Batch, []byte, error) {
	if err := validateBatch(batch); err != nil {
		return Batch{}, nil, err
	}
	payload, err := canonicalJSON(payloadForBatch(batch))
	if err != nil {
		return Batch{}, nil, err
	}
	batch.SHA256 = digest(payload)
	data, err := canonicalJSON(batch)
	if err != nil {
		return Batch{}, nil, err
	}
	if err := checkCapacity("evidence batch", data); err != nil {
		return Batch{}, nil, err
	}
	return batch, data, nil
}

// SealSnapshot validates a snapshot, computes its digest, and returns its canonical JSON.
func SealSnapshot(snapshot Snapshot) (Snapshot, []byte, error) {
	// Historical explicit-zero continuation fields are accepted only on decode.
	// Every new write uses the current all-or-nothing continuation encoding.
	snapshot.historicalZeroContinuation = false
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, nil, err
	}
	payload, err := canonicalJSON(payloadForSnapshot(snapshot))
	if err != nil {
		return Snapshot{}, nil, err
	}
	snapshot.Digest = digest(payload)
	data, err := canonicalJSON(snapshot)
	if err != nil {
		return Snapshot{}, nil, err
	}
	if err := checkCapacity("snapshot", data); err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, data, nil
}

// DecodeCanonicalBatch accepts only a valid, exact canonical batch document.
func DecodeCanonicalBatch(data []byte) (Batch, error) {
	var batch Batch
	if err := decodeCanonical(data, &batch); err != nil {
		return Batch{}, err
	}
	if err := VerifyBatch(batch); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

// DecodeCanonicalSnapshot accepts only a valid, exact canonical snapshot document.
func DecodeCanonicalSnapshot(data []byte) (Snapshot, error) {
	if !utf8.Valid(data) {
		return Snapshot{}, ErrNonCanonical
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("%w: decode canonical JSON: %v", ErrNonCanonical, err)
	}
	canonical, err := snapshotCanonicalJSON(snapshot)
	if err != nil || !bytes.Equal(data, canonical) {
		return Snapshot{}, ErrNonCanonical
	}
	if err := VerifySnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// VerifyBatch validates a typed batch, verifies its payload digest, and enforces the final document ceiling.
func VerifyBatch(batch Batch) error {
	if err := validateBatch(batch); err != nil {
		return err
	}
	if !validDigest(batch.SHA256) {
		return ErrHashMismatch
	}
	payload, err := canonicalJSON(payloadForBatch(batch))
	if err != nil || batch.SHA256 != digest(payload) {
		return ErrHashMismatch
	}
	data, err := canonicalJSON(batch)
	if err != nil {
		return err
	}
	return checkCapacity("evidence batch", data)
}

// VerifySnapshot validates a typed snapshot, verifies its payload digest, and enforces the final document ceiling.
func VerifySnapshot(snapshot Snapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if !validDigest(snapshot.Digest) {
		return ErrHashMismatch
	}
	payload, err := canonicalJSON(snapshotPayloadForDigest(snapshot))
	if err != nil || snapshot.Digest != digest(payload) {
		return ErrHashMismatch
	}
	data, err := snapshotCanonicalJSON(snapshot)
	if err != nil {
		return err
	}
	return checkCapacity("snapshot", data)
}

func decodeCanonical(data []byte, destination any) error {
	if !utf8.Valid(data) {
		return ErrNonCanonical
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%w: decode canonical JSON: %v", ErrNonCanonical, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrNonCanonical
	}
	canonical, err := canonicalJSON(destination)
	if err != nil || !bytes.Equal(data, canonical) {
		return ErrNonCanonical
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

func checkCapacity(document string, data []byte) error {
	if runes := utf8.RuneCount(data); runes > MaxDocumentRunes {
		return &CapacityError{Document: document, Runes: runes, Limit: MaxDocumentRunes}
	}
	return nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type batchPayload struct {
	Schema  string          `json:"schema"`
	Project string          `json:"project"`
	Change  string          `json:"change"`
	BatchID string          `json:"batch_id"`
	Entries []EvidenceEntry `json:"entries"`
}

func payloadForBatch(batch Batch) batchPayload {
	return batchPayload{batch.Schema, batch.Project, batch.Change, batch.BatchID, batch.Entries}
}

type snapshotPayload struct {
	Schema             string             `json:"schema"`
	Project            string             `json:"project"`
	Change             string             `json:"change"`
	Generation         uint64             `json:"generation"`
	Revision           uint64             `json:"revision"`
	PreviousDigest     string             `json:"previous_digest"`
	TaskManifestSHA256 string             `json:"task_manifest_sha256"`
	Status             Status             `json:"status"`
	Coverage           []Coverage         `json:"coverage"`
	Batches            []BatchRef         `json:"batches"`
	StreamSHA256       *string            `json:"stream_sha256,omitempty"`
	NextEntryIndex     *int               `json:"next_entry_index,omitempty"`
	NextEntryID        *string            `json:"next_entry_id,omitempty"`
	SealIntent         *SealIntent        `json:"seal_intent,omitempty"`
	Supersedes         *SupersedesPointer `json:"supersedes,omitempty"`
}

func payloadForSnapshot(snapshot Snapshot) snapshotPayload {
	payload := snapshotPayload{
		Schema: snapshot.Schema, Project: snapshot.Project, Change: snapshot.Change,
		Generation: snapshot.Generation, Revision: snapshot.Revision, PreviousDigest: snapshot.PreviousDigest,
		TaskManifestSHA256: snapshot.TaskManifestSHA256, Status: snapshot.Status, Coverage: snapshot.Coverage, Batches: snapshot.Batches,
	}
	if snapshot.hasContinuation() {
		payload.StreamSHA256, payload.NextEntryIndex, payload.NextEntryID = &snapshot.StreamSHA256, &snapshot.NextEntryIndex, &snapshot.NextEntryID
	}
	if snapshot.Schema == SupersessionSnapshotSchema {
		payload.SealIntent, payload.Supersedes = snapshot.SealIntent, snapshot.Supersedes
	}
	return payload
}

type historicalSnapshotPayload struct {
	Schema             string     `json:"schema"`
	Project            string     `json:"project"`
	Change             string     `json:"change"`
	Generation         uint64     `json:"generation"`
	Revision           uint64     `json:"revision"`
	PreviousDigest     string     `json:"previous_digest"`
	TaskManifestSHA256 string     `json:"task_manifest_sha256"`
	Status             Status     `json:"status"`
	Coverage           []Coverage `json:"coverage"`
	Batches            []BatchRef `json:"batches"`
	StreamSHA256       string     `json:"stream_sha256"`
	NextEntryIndex     int        `json:"next_entry_index"`
	NextEntryID        string     `json:"next_entry_id"`
}

func snapshotPayloadForDigest(snapshot Snapshot) any {
	if !snapshot.historicalZeroContinuation {
		return payloadForSnapshot(snapshot)
	}
	return historicalSnapshotPayload{
		Schema: snapshot.Schema, Project: snapshot.Project, Change: snapshot.Change,
		Generation: snapshot.Generation, Revision: snapshot.Revision, PreviousDigest: snapshot.PreviousDigest,
		TaskManifestSHA256: snapshot.TaskManifestSHA256, Status: snapshot.Status, Coverage: snapshot.Coverage, Batches: snapshot.Batches,
		StreamSHA256: snapshot.StreamSHA256, NextEntryIndex: snapshot.NextEntryIndex, NextEntryID: snapshot.NextEntryID,
	}
}

func snapshotCanonicalJSON(snapshot Snapshot) ([]byte, error) {
	if !snapshot.historicalZeroContinuation {
		return canonicalJSON(snapshot)
	}
	return canonicalJSON(struct {
		historicalSnapshotPayload
		Digest string `json:"digest"`
	}{historicalSnapshotPayload: snapshotPayloadForDigest(snapshot).(historicalSnapshotPayload), Digest: snapshot.Digest})
}

// MarshalJSON preserves the v2 field order while treating continuation as an
// atomic group. Pointers keep index zero present on the wire.
func (snapshot Snapshot) MarshalJSON() ([]byte, error) {
	// A decoded historical snapshot must retain the exact bytes that its digest
	// authenticates until an authorized upgrade reseals it in the current form.
	if snapshot.historicalZeroContinuation {
		return snapshotCanonicalJSON(snapshot)
	}
	payload := payloadForSnapshot(snapshot)
	return canonicalJSON(struct {
		snapshotPayload
		Digest string `json:"digest"`
	}{payload, snapshot.Digest})
}

// UnmarshalJSON accepts an absent continuation group for current unbound
// snapshots and preserves only the historical explicit-zero wire group for
// guarded continuation upgrade.
func (snapshot *Snapshot) UnmarshalJSON(data []byte) error {
	var wire struct {
		Schema             string             `json:"schema"`
		Project            string             `json:"project"`
		Change             string             `json:"change"`
		Generation         uint64             `json:"generation"`
		Revision           uint64             `json:"revision"`
		PreviousDigest     string             `json:"previous_digest"`
		TaskManifestSHA256 string             `json:"task_manifest_sha256"`
		Status             Status             `json:"status"`
		Coverage           []Coverage         `json:"coverage"`
		Batches            []BatchRef         `json:"batches"`
		StreamSHA256       *string            `json:"stream_sha256"`
		NextEntryIndex     *int               `json:"next_entry_index"`
		NextEntryID        *string            `json:"next_entry_id"`
		SealIntent         *SealIntent        `json:"seal_intent"`
		Supersedes         *SupersedesPointer `json:"supersedes"`
		Digest             string             `json:"digest"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrNonCanonical
	}
	snapshot.Schema, snapshot.Project, snapshot.Change = wire.Schema, wire.Project, wire.Change
	snapshot.Generation, snapshot.Revision, snapshot.PreviousDigest = wire.Generation, wire.Revision, wire.PreviousDigest
	snapshot.TaskManifestSHA256, snapshot.Status, snapshot.Coverage, snapshot.Batches, snapshot.Digest = wire.TaskManifestSHA256, wire.Status, wire.Coverage, wire.Batches, wire.Digest
	snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = "", 0, ""
	snapshot.SealIntent, snapshot.Supersedes = wire.SealIntent, wire.Supersedes
	snapshot.historicalZeroContinuation = false
	if wire.StreamSHA256 != nil || wire.NextEntryIndex != nil || wire.NextEntryID != nil {
		if wire.StreamSHA256 == nil || wire.NextEntryIndex == nil || wire.NextEntryID == nil {
			return ErrNonCanonical
		}
		snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = *wire.StreamSHA256, *wire.NextEntryIndex, *wire.NextEntryID
		// The old serializer always emitted zero values for this group. Preserve
		// its digest algorithm only for that exact decoded wire representation.
		snapshot.historicalZeroContinuation = snapshot.StreamSHA256 == "" && snapshot.NextEntryIndex == 0 && snapshot.NextEntryID == ""
	}
	return nil
}

func (snapshot Snapshot) hasContinuation() bool {
	return snapshot.StreamSHA256 != "" || snapshot.NextEntryIndex != 0 || snapshot.NextEntryID != ""
}

func validateBatch(batch Batch) error {
	if !validBatchText(batch) {
		return ErrInvalidValue
	}
	if batch.Schema != EvidenceSchema {
		return ErrInvalidSchema
	}
	if !hasValidBatchIdentity(batch) {
		return ErrInvalidID
	}
	if len(batch.Entries) == 0 {
		return ErrInvalidValue
	}
	for _, entry := range batch.Entries {
		if err := validateEvidenceEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshot(snapshot Snapshot) error {
	if !validSnapshotText(snapshot) {
		return ErrInvalidValue
	}
	if snapshot.Schema != SnapshotSchema && snapshot.Schema != SupersessionSnapshotSchema {
		return ErrInvalidSchema
	}
	if snapshot.Schema == SnapshotSchema && (snapshot.SealIntent != nil || snapshot.Supersedes != nil) {
		return ErrInvalidValue
	}
	if snapshot.Schema == SupersessionSnapshotSchema {
		if snapshot.SealIntent != nil {
			i := snapshot.SealIntent
			if snapshot.Status != StatusSuperseded || !validID(i.SuccessorProject) || !validID(i.SuccessorChange) || i.SuccessorProject != snapshot.Project || i.SuccessorChange == snapshot.Change || !validDigest(i.SuccessorManifestSHA256) || i.Actor == "" || !validText(i.Actor) || !validID(i.OperationID) || i.Reason == "" || !validText(i.Reason) || !validTimestamp(i.Timestamp) {
				return ErrInvalidValue
			}
		}
		if snapshot.Supersedes != nil {
			p := snapshot.Supersedes
			if !validID(p.Project) || !validID(p.Change) || p.Project != snapshot.Project || p.Change == snapshot.Change || !validDigest(p.SealDigest) || !validDigest(p.OriginalManifestSHA256) || p.Actor == "" || !validText(p.Actor) || p.Reason == "" || !validText(p.Reason) || !validID(p.OperationID) || !validTimestamp(p.Timestamp) {
				return ErrInvalidValue
			}
		}
		if snapshot.Status == StatusSuperseded && snapshot.SealIntent == nil {
			return ErrInvalidValue
		}
		if snapshot.Status != StatusSuperseded && snapshot.Supersedes == nil {
			return ErrInvalidValue
		}
	}
	if snapshot.Status == StatusSuperseded && snapshot.Schema != SupersessionSnapshotSchema {
		return ErrInvalidValue
	}
	if !hasValidSnapshotIdentity(snapshot) {
		return ErrInvalidID
	}
	if snapshot.Status != StatusPartial && snapshot.Status != StatusComplete && snapshot.Status != StatusSuperseded {
		return ErrInvalidValue
	}
	if (snapshot.PreviousDigest != "" && !validDigest(snapshot.PreviousDigest)) || !validDigest(snapshot.TaskManifestSHA256) {
		return ErrHashMismatch
	}
	if !validSnapshotContinuation(snapshot) {
		return ErrInvalidValue
	}
	for _, coverage := range snapshot.Coverage {
		if !validID(coverage.TaskID) || !batchIDPattern.MatchString(coverage.BatchID) || !validID(coverage.EntryID) {
			return ErrInvalidID
		}
	}
	for _, batch := range snapshot.Batches {
		if !batchIDPattern.MatchString(batch.BatchID) {
			return ErrInvalidID
		}
		if !validDigest(batch.SHA256) {
			return ErrHashMismatch
		}
	}
	return nil
}

func hasValidBatchIdentity(batch Batch) bool {
	return validID(batch.Project) && validID(batch.Change) && batchIDPattern.MatchString(batch.BatchID)
}

func hasValidSnapshotIdentity(snapshot Snapshot) bool {
	return validID(snapshot.Project) && validID(snapshot.Change) && snapshot.Coverage != nil && snapshot.Batches != nil
}

func validBatchText(batch Batch) bool {
	if !validText(batch.Schema, batch.Project, batch.Change, batch.BatchID, batch.SHA256) {
		return false
	}
	for _, entry := range batch.Entries {
		if !validText(entry.EntryID, string(entry.Kind), entry.Summary, entry.Command, string(entry.Outcome)) ||
			!validTextSlice(entry.TaskIDs) || !validTextSlice(entry.CompletesTaskIDs) || !validTextSlice(entry.Files) {
			return false
		}
	}
	return true
}

func validSnapshotText(snapshot Snapshot) bool {
	if !validText(snapshot.Schema, snapshot.Project, snapshot.Change, snapshot.PreviousDigest, snapshot.TaskManifestSHA256, snapshot.StreamSHA256, snapshot.NextEntryID, string(snapshot.Status), snapshot.Digest) {
		return false
	}
	for _, coverage := range snapshot.Coverage {
		if !validText(coverage.TaskID, coverage.BatchID, coverage.EntryID) {
			return false
		}
	}
	for _, batch := range snapshot.Batches {
		if !validText(batch.BatchID, batch.SHA256) {
			return false
		}
	}
	return true
}

func validText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func validateEvidenceEntry(entry EvidenceEntry) error {
	if !validID(entry.EntryID) || entry.TaskIDs == nil || entry.CompletesTaskIDs == nil || entry.Files == nil {
		return ErrInvalidID
	}
	for _, id := range append(append([]string{}, entry.TaskIDs...), entry.CompletesTaskIDs...) {
		if !validID(id) {
			return ErrInvalidID
		}
	}
	if !validKind(entry.Kind) || !validOutcome(entry.Outcome) {
		return ErrInvalidValue
	}
	return nil
}

func validTimestamp(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validSnapshotContinuation(snapshot Snapshot) bool {
	if snapshot.Status == StatusComplete || snapshot.Status == StatusSuperseded {
		return !snapshot.hasContinuation()
	}
	// A missing group is a valid current unbound partial snapshot. Only the
	// decoder-marked historical explicit-zero wire shape requires safe upgrade.
	if !snapshot.hasContinuation() {
		return true
	}
	return validDigest(snapshot.StreamSHA256) && snapshot.NextEntryIndex >= 0 && validID(snapshot.NextEntryID)
}

func validTextSlice(values []string) bool { return validText(values...) }
func validID(id string) bool              { return idPattern.MatchString(id) }
func validDigest(digest string) bool      { return digestPattern.MatchString(digest) }
func validKind(kind EvidenceKind) bool {
	return kind == EvidenceRed || kind == EvidenceGreen || kind == EvidenceTriangulate || kind == EvidenceRefactor || kind == EvidenceVerification || kind == EvidenceDelivery || kind == EvidenceImported
}
func validOutcome(outcome Outcome) bool {
	return outcome == OutcomePass || outcome == OutcomeFail || outcome == OutcomeNotRun
}

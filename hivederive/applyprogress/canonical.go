package applyprogress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	var snapshot Snapshot
	if err := decodeCanonical(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := VerifySnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

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

func VerifySnapshot(snapshot Snapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if !validDigest(snapshot.Digest) {
		return ErrHashMismatch
	}
	payload, err := canonicalJSON(payloadForSnapshot(snapshot))
	if err != nil || snapshot.Digest != digest(payload) {
		return ErrHashMismatch
	}
	data, err := canonicalJSON(snapshot)
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
		return &CapacityError{Document: document, Runes: runes}
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
}

func payloadForSnapshot(snapshot Snapshot) snapshotPayload {
	return snapshotPayload{
		snapshot.Schema, snapshot.Project, snapshot.Change, snapshot.Generation, snapshot.Revision,
		snapshot.PreviousDigest, snapshot.TaskManifestSHA256, snapshot.Status, snapshot.Coverage, snapshot.Batches,
	}
}

func validateBatch(batch Batch) error {
	if !validBatchText(batch) {
		return ErrInvalidValue
	}
	if batch.Schema != EvidenceSchema {
		return ErrInvalidSchema
	}
	if !validID(batch.Project) || !validID(batch.Change) || !batchIDPattern.MatchString(batch.BatchID) || batch.Entries == nil {
		return ErrInvalidID
	}
	for _, entry := range batch.Entries {
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
	}
	return nil
}

func validateSnapshot(snapshot Snapshot) error {
	if !validSnapshotText(snapshot) {
		return ErrInvalidValue
	}
	if snapshot.Schema != SnapshotSchema {
		return ErrInvalidSchema
	}
	if !validID(snapshot.Project) || !validID(snapshot.Change) || snapshot.Coverage == nil || snapshot.Batches == nil {
		return ErrInvalidID
	}
	if snapshot.Status != StatusPartial && snapshot.Status != StatusComplete {
		return ErrInvalidValue
	}
	if (snapshot.PreviousDigest != "" && !validDigest(snapshot.PreviousDigest)) || !validDigest(snapshot.TaskManifestSHA256) {
		return ErrHashMismatch
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
	if !validText(snapshot.Schema, snapshot.Project, snapshot.Change, snapshot.PreviousDigest, snapshot.TaskManifestSHA256, string(snapshot.Status), snapshot.Digest) {
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

func validTextSlice(values []string) bool { return validText(values...) }
func validID(id string) bool              { return idPattern.MatchString(id) }
func validDigest(digest string) bool      { return digestPattern.MatchString(digest) }
func validKind(kind EvidenceKind) bool {
	return kind == EvidenceRed || kind == EvidenceGreen || kind == EvidenceTriangulate || kind == EvidenceRefactor || kind == EvidenceVerification || kind == EvidenceDelivery
}
func validOutcome(outcome Outcome) bool {
	return outcome == OutcomePass || outcome == OutcomeFail || outcome == OutcomeNotRun
}

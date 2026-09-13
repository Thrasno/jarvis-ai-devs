// Package applyprogress defines the bounded v2 apply-progress wire documents.
package applyprogress

import (
	"errors"
	"fmt"
	"regexp"
)

const (
	SnapshotSchema   = "jarvis.sdd-apply-progress/v2"
	EvidenceSchema   = "jarvis.sdd-apply-evidence/v2"
	MaxDocumentRunes = 40000
)

var (
	ErrInvalidSchema = errors.New("invalid apply-progress schema")
	ErrInvalidID     = errors.New("invalid apply-progress ID")
	ErrInvalidValue  = errors.New("invalid apply-progress value")
	ErrNonCanonical  = errors.New("non-canonical apply-progress JSON")
	ErrHashMismatch  = errors.New("apply-progress hash mismatch")
	idPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	batchIDPattern   = regexp.MustCompile(`^apb-[0-9a-f]{32}$`)
	digestPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Status string

const (
	StatusPartial  Status = "partial"
	StatusComplete Status = "complete"
)

type EvidenceKind string

const (
	EvidenceRed          EvidenceKind = "red"
	EvidenceGreen        EvidenceKind = "green"
	EvidenceTriangulate  EvidenceKind = "triangulate"
	EvidenceRefactor     EvidenceKind = "refactor"
	EvidenceVerification EvidenceKind = "verification"
	EvidenceDelivery     EvidenceKind = "delivery"
)

type Outcome string

const (
	OutcomePass   Outcome = "pass"
	OutcomeFail   Outcome = "fail"
	OutcomeNotRun Outcome = "not_run"
)

type EvidenceEntry struct {
	EntryID          string       `json:"entry_id"`
	TaskIDs          []string     `json:"task_ids"`
	CompletesTaskIDs []string     `json:"completes_task_ids"`
	Kind             EvidenceKind `json:"kind"`
	Summary          string       `json:"summary"`
	Command          string       `json:"command"`
	ExitCode         int          `json:"exit_code"`
	Outcome          Outcome      `json:"outcome"`
	Files            []string     `json:"files"`
}

type Batch struct {
	Schema  string          `json:"schema"`
	Project string          `json:"project"`
	Change  string          `json:"change"`
	BatchID string          `json:"batch_id"`
	Entries []EvidenceEntry `json:"entries"`
	SHA256  string          `json:"sha256"`
}

type BatchRef struct {
	BatchID string `json:"batch_id"`
	SHA256  string `json:"sha256"`
}

type Coverage struct {
	TaskID  string `json:"task_id"`
	BatchID string `json:"batch_id"`
	EntryID string `json:"entry_id"`
}

type Snapshot struct {
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
	Digest             string     `json:"digest"`
}

// CapacityError identifies a final serialized document that exceeds the protocol limit.
type CapacityError struct {
	Document string
	Runes    int
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf("%s has %d runes; maximum is %d", e.Document, e.Runes, MaxDocumentRunes)
}

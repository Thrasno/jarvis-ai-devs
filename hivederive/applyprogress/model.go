// Package applyprogress defines the bounded v2 apply-progress wire documents.
package applyprogress

import (
	"errors"
	"fmt"
	"regexp"
)

const (
	SnapshotSchema             = "jarvis.sdd-apply-progress/v2"
	SupersessionSnapshotSchema = "jarvis.sdd-apply-progress/v3"
	EvidenceSchema             = "jarvis.sdd-apply-evidence/v2"
	// MaxDocumentRunes leaves transport and enclosing-document headroom while bounding each canonical protocol document.
	MaxDocumentRunes = 40000
	// SnapshotCapacityWarningPercent and SnapshotCapacityWarningRemainingRunes
	// leave callers time to consolidate evidence before the hard ceiling.
	SnapshotCapacityWarningPercent        = 80
	SnapshotCapacityWarningRemainingRunes = 8000
	// SnapshotCheckpointReferenceGuard starts consolidation only before a
	// continuation identity binds a new stream, while callers may still combine
	// pending evidence without changing committed stream authority.
	SnapshotCheckpointReferenceGuard = 256
	// MinNonterminalCheckpointRunes prevents tiny batches from irreversibly
	// consuming the finite snapshot reference budget near its ceiling.
	MinNonterminalCheckpointRunes = MaxDocumentRunes / 4
)

// checkpointConsolidationSafeRunes leaves a full nonterminal checkpoint's worth
// of room after an incomplete unbound result. It is a variable only so focused
// package tests can exercise the boundary without manufacturing 40,000-rune
// fixtures; production always uses the protocol-derived default.
var checkpointConsolidationSafeRunes = MaxDocumentRunes - MinNonterminalCheckpointRunes

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

// ValidID reports whether value is a protocol-safe identifier.
func ValidID(value string) bool { return validID(value) }

// ValidDigest reports whether value is a protocol SHA-256 hex digest.
func ValidDigest(value string) bool { return validDigest(value) }

type Status string

const (
	StatusPartial    Status = "partial"
	StatusComplete   Status = "complete"
	StatusSuperseded Status = "superseded"
)

type EvidenceKind string

const (
	EvidenceRed          EvidenceKind = "red"
	EvidenceGreen        EvidenceKind = "green"
	EvidenceTriangulate  EvidenceKind = "triangulate"
	EvidenceRefactor     EvidenceKind = "refactor"
	EvidenceVerification EvidenceKind = "verification"
	EvidenceDelivery     EvidenceKind = "delivery"
	// EvidenceImported records a completion imported from a legacy artifact. It makes no RED/GREEN/REFACTOR claim.
	EvidenceImported EvidenceKind = "imported"
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

// SealIntent freezes the destination and retry identity in a terminal predecessor head.
type SealIntent struct {
	SuccessorProject        string `json:"successor_project"`
	SuccessorChange         string `json:"successor_change"`
	SuccessorManifestSHA256 string `json:"successor_manifest_sha256"`
	Actor                   string `json:"actor"`
	Reason                  string `json:"reason"`
	Timestamp               string `json:"timestamp"`
	OperationID             string `json:"operation_id"`
}

// SupersedesPointer authenticates the predecessor seal from a fresh successor.
type SupersedesPointer struct {
	Project                string `json:"project"`
	Change                 string `json:"change"`
	SealDigest             string `json:"seal_digest"`
	OriginalManifestSHA256 string `json:"original_manifest_sha256"`
	Actor                  string `json:"actor"`
	Reason                 string `json:"reason"`
	OperationID            string `json:"operation_id"`
	Timestamp              string `json:"timestamp"`
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
	// StreamSHA256 and the next-entry cursor bind a partial snapshot to its complete evidence stream.
	// Snapshot JSON serializes this as an atomic group; NextEntryIndex may be zero.
	StreamSHA256   string             `json:"-"`
	NextEntryIndex int                `json:"-"`
	NextEntryID    string             `json:"-"`
	SealIntent     *SealIntent        `json:"-"`
	Supersedes     *SupersedesPointer `json:"-"`
	Digest         string             `json:"digest"`

	// historicalZeroContinuation is set only by the canonical decoder for the
	// pre-continuation v2 wire shape, which explicitly serialized all three zero
	// continuation fields and therefore used a different historical digest input.
	// New seals always clear it and emit the current all-or-nothing group.
	historicalZeroContinuation bool
}

// RequiresContinuationUpgrade reports whether this snapshot was decoded from
// the historical v2 wire shape that explicitly encoded an all-zero continuation
// group. New unbound partial snapshots intentionally omit that group and can
// start an independent later stream without a migration CAS.
func (snapshot Snapshot) RequiresContinuationUpgrade() bool {
	return snapshot.historicalZeroContinuation
}

// CapacityError identifies a final serialized document that exceeds the protocol limit.
//
// CurrentRunes, ProjectedRunes, and CeilingRunes describe snapshot capacity at
// the guarded head. They are populated for checkpoint successor diagnostics and
// warnings; Document, Runes, and Limit retain compatibility with low-level
// document validation.
type CapacityError struct {
	Document       string
	Runes          int
	Limit          int
	CurrentRunes   int `json:"current_runes"`
	ProjectedRunes int `json:"projected_runes"`
	CeilingRunes   int `json:"ceiling_runes"`
}

// Capacity is the stable checkpoint capacity diagnostic.
type Capacity = CapacityError

// SnapshotCapacityExhaustedError marks a no-write successor preflight failure.
// It deliberately unwraps to CapacityError for existing defensive validators.
type SnapshotCapacityExhaustedError struct {
	Capacity *CapacityError
}

func (e *SnapshotCapacityExhaustedError) Error() string {
	if e == nil || e.Capacity == nil {
		return "snapshot capacity exhausted"
	}
	return e.Capacity.Error()
}

func (e *SnapshotCapacityExhaustedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Capacity
}

// SnapshotCapacityRecovery is the single operator recovery for a flat snapshot
// that cannot accept another successor. It intentionally does not suggest
// compaction, rollover, or rewriting immutable evidence.
const SnapshotCapacityRecovery = "stop with apply StatusPartial without archive, list task IDs absent from coverage, and carry them into a new ordinary SDD change"

// CapacityWarning reports a successor snapshot that is approaching the hard
// document ceiling. It is advisory only; CapacityError remains fail-closed.
type CapacityWarning struct {
	Document  string `json:"document"`
	Runes     int    `json:"runes"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	Threshold int    `json:"threshold"`
}

// CheckpointFrequencyError blocks an undersized nonterminal checkpoint before
// its immutable batch reference can consume the finite snapshot budget.
type CheckpointFrequencyError struct {
	References    int
	Threshold     int
	BatchRunes    int
	MinimumRunes  int
	SnapshotRunes int
	SafeRunes     int
}

func (e *CheckpointFrequencyError) Error() string {
	if e.SafeRunes > 0 {
		return fmt.Sprintf("projected incomplete snapshot has %d runes (safe threshold %d); consolidate evidence before retrying", e.SnapshotRunes, e.SafeRunes)
	}
	return fmt.Sprintf("checkpoint has %d batch references (guard %d) and only %d batch runes; accumulate or combine pending evidence to at least %d runes before retrying", e.References, e.Threshold, e.BatchRunes, e.MinimumRunes)
}

// EffectiveLimit retains compatibility for callers that construct a capacity
// error directly while protocol producers always set the explicit limit.
func (e *CapacityError) EffectiveLimit() int {
	if e == nil || e.Limit <= 0 {
		return MaxDocumentRunes
	}
	return e.Limit
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf("%s has %d runes; maximum is %d", e.Document, e.Runes, e.EffectiveLimit())
}

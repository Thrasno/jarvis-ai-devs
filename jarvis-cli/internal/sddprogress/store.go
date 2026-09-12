// Package sddprogress persists bounded v2 apply-progress documents in OpenSpec.
package sddprogress

import (
	"errors"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var (
	ErrConflict          = errors.New("apply-progress expected state conflict")
	ErrBatchCollision    = errors.New("apply-progress batch collision")
	ErrMissingBatch      = errors.New("apply-progress missing referenced batch")
	ErrRequestConflict   = errors.New("apply-progress request ID payload conflict")
	ErrLegacyMigration   = errors.New("apply-progress legacy migration failed")
	ErrInvalidChangeRoot = errors.New("apply-progress change root is missing or invalid")
	// ErrPublicationInterrupted means write-ahead receipt lineage cannot prove one
	// append-only history ending at the authoritative head.
	ErrPublicationInterrupted = errors.New("apply-progress publication interrupted")
)

// AdvanceRequest contains canonical v2 documents for one guarded publication.
type AdvanceRequest struct {
	RequestID          string `json:"request_id"`
	ExpectedGeneration uint64 `json:"expected_generation"`
	ExpectedRevision   uint64 `json:"expected_revision"`
	ExpectedDigest     string `json:"expected_digest"`
	// LegacySourceSHA256 is compatibility-only provenance for an exact legacy
	// migration or its replay. Checkpoint JSON deliberately does not expose it.
	LegacySourceSHA256 string                 `json:"legacy_source_sha256,omitempty"`
	Batches            []applyprogress.Batch  `json:"batches"`
	Snapshot           applyprogress.Snapshot `json:"snapshot"`
}

type AdvanceResult struct {
	Generation    uint64 `json:"generation"`
	Revision      uint64 `json:"revision"`
	Digest        string `json:"digest"`
	PayloadSHA256 string `json:"payload_sha256,omitempty"`
}

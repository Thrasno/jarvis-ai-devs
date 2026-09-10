// Package sddprogress persists bounded v2 apply-progress documents in OpenSpec.
package sddprogress

import (
	"errors"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var (
	ErrConflict        = errors.New("apply-progress expected state conflict")
	ErrBatchCollision  = errors.New("apply-progress batch collision")
	ErrMissingBatch    = errors.New("apply-progress missing referenced batch")
	ErrRequestConflict = errors.New("apply-progress request ID payload conflict")
)

// AdvanceRequest contains canonical v2 documents for one guarded publication.
type AdvanceRequest struct {
	RequestID          string
	ExpectedGeneration uint64
	ExpectedRevision   uint64
	ExpectedDigest     string
	Batches            []applyprogress.Batch
	Snapshot           applyprogress.Snapshot
}

type AdvanceResult struct {
	Generation uint64
	Revision   uint64
	Digest     string
}

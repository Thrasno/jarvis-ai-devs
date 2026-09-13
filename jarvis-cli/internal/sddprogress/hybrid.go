package sddprogress

import (
	"errors"
	"slices"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var (
	ErrBackendDiverged = errors.New("apply-progress backend diverged")
	ErrMissingBackend  = errors.New("apply-progress backend missing")
)

// ResolvedProgress is a backend's independently validated v2 snapshot.
type ResolvedProgress struct{ Snapshot applyprogress.Snapshot }

type progressResolver func() (ResolvedProgress, error)

// ResolveHybrid accepts progress only when both independently validated backends
// resolve the same canonical snapshot. It intentionally never selects a winner.
func ResolveHybrid(openspec, hive progressResolver) (ResolvedProgress, error) {
	left, leftErr := openspec()
	right, rightErr := hive()
	if leftErr != nil || rightErr != nil || !sameSnapshot(left.Snapshot, right.Snapshot) {
		return ResolvedProgress{}, ErrBackendDiverged
	}
	return left, nil
}

func sameSnapshot(left, right applyprogress.Snapshot) bool {
	return left.Schema == right.Schema && left.Project == right.Project && left.Change == right.Change && left.Generation == right.Generation && left.Revision == right.Revision && left.PreviousDigest == right.PreviousDigest && left.TaskManifestSHA256 == right.TaskManifestSHA256 && left.Status == right.Status && left.Digest != "" && left.Digest == right.Digest && slices.Equal(left.Batches, right.Batches) && slices.Equal(left.Coverage, right.Coverage)
}

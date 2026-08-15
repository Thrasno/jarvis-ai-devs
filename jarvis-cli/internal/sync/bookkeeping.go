// This file holds sync's own bookkeeping: the small, compatible record replay
// leaves behind about the asset set it applied. It is written only after a run
// that measured its own outcome and found a target changed, so a converged
// machine is left byte-identical and a run that failed before that measurement
// leaves the previous digest exactly where it was (see Run), and
// only under the manifest lock, from a manifest re-read inside that lock so a
// concurrent writer is not clobbered by stale in-memory state.
package sync

import (
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// Bookkeeping records what replay applied.
type Bookkeeping struct {
	// ManagedAssetDigest identifies the embedded asset set this run replayed.
	// Empty means the caller has nothing to record, which is not a failure.
	ManagedAssetDigest string
	// Lock defaults to state.WithLock; it is a seam so the critical section can
	// be observed without a second process.
	Lock func(func() error) error
}

// record persists the record, and only the fields that differ from what is
// already on disk. A nil Bookkeeping is a run with no record to keep.
func (b *Bookkeeping) record() error {
	if b == nil {
		return nil
	}
	lock := b.Lock
	if lock == nil {
		lock = state.WithLock
	}
	return lock(func() error {
		latest, err := state.Load()
		if err != nil {
			return fmt.Errorf("re-read the desired-state manifest for sync bookkeeping: %w", err)
		}
		if b.ManagedAssetDigest == "" || latest.ManagedAssetDigest == b.ManagedAssetDigest {
			return nil
		}
		latest.ManagedAssetDigest = b.ManagedAssetDigest
		if err := state.Save(latest); err != nil {
			return fmt.Errorf("persist sync bookkeeping: %w", err)
		}
		return nil
	})
}

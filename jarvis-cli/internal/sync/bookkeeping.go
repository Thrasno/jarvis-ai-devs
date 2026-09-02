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

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// Bookkeeping records what replay applied.
type ZohoExpansion struct {
	Pack         skills.ZohoPack
	CandidateIDs []string
}

type Bookkeeping struct {
	// ManagedAssetDigest identifies the embedded asset set this run replayed.
	// Empty means the caller has nothing to record, which is not a failure.
	ManagedAssetDigest string
	ZohoExpansion      *ZohoExpansion
	// Lock defaults to state.WithLock; it is a seam so the critical section can
	// be observed without a second process.
	Lock func(func() error) error
}

// record persists verified replay facts and returns only Zoho IDs saved by this
// transaction. A nil Bookkeeping is a run with no record to keep.
func (b *Bookkeeping) record(changed, converged bool) (added []string, err error) {
	if b == nil || (!changed && (b.ZohoExpansion == nil || len(b.ZohoExpansion.CandidateIDs) == 0)) {
		return nil, nil
	}
	lock := b.Lock
	if lock == nil {
		lock = state.WithLock
	}
	err = lock(func() error {
		latest, err := state.Load()
		if err != nil {
			return fmt.Errorf("re-read the desired-state manifest for sync bookkeeping: %w", err)
		}
		dirty := false
		if changed && b.ManagedAssetDigest != "" && latest.ManagedAssetDigest != b.ManagedAssetDigest {
			latest.ManagedAssetDigest = b.ManagedAssetDigest
			dirty = true
		}
		if converged && b.ZohoExpansion != nil && b.ZohoExpansion.Pack.Selected(latest.Skills) {
			present := make(map[string]bool, len(latest.Skills))
			for _, id := range latest.Skills {
				present[id] = true
			}
			nextSkills := b.ZohoExpansion.Pack.ApplySelection(latest.Skills, true)
			for _, id := range nextSkills {
				if !present[id] {
					added = append(added, id)
				}
			}
			if len(added) > 0 {
				latest.Skills = nextSkills
				dirty = true
			}
		}
		if !dirty {
			return nil
		}
		if err := state.Save(latest); err != nil {
			return fmt.Errorf("persist sync bookkeeping: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return added, nil
}

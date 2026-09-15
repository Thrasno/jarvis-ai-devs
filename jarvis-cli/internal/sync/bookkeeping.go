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
	"reflect"

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
	// SkillAdditions and SkillRemovals are lifecycle deltas projected from the
	// manifest used for this replay. They are rebased on the manifest re-read
	// under the state lock, so unrelated concurrent selections survive.
	SkillAdditions []string
	SkillRemovals  []string
	ZohoExpansion  *ZohoExpansion
	// Lock defaults to state.WithLock; it is a seam so the critical section can
	// be observed without a second process.
	Lock func(func() error) error
}

// record persists verified replay facts and returns only Zoho IDs saved by this
// transaction. A nil Bookkeeping is a run with no record to keep.
func (b *Bookkeeping) record(changed, converged bool) (added []string, err error) {
	if b == nil || (!changed && len(b.SkillAdditions) == 0 && len(b.SkillRemovals) == 0 && (b.ZohoExpansion == nil || len(b.ZohoExpansion.CandidateIDs) == 0)) {
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
		if converged && (len(b.SkillAdditions) > 0 || len(b.SkillRemovals) > 0) {
			nextSkills := ApplySkillLifecycleDeltas(latest.Skills, b.SkillAdditions, b.SkillRemovals)
			if !reflect.DeepEqual(latest.Skills, nextSkills) {
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

// RetiredSkillIDs lists skills a past catalog shipped and this build removed on
// purpose. Retirement is the only authority for deleting a manifest-owned skill
// the catalog no longer offers: an ID the catalog merely lacks may have been
// recorded by a newer jarvis, so replay retains it instead of deleting it. It is
// exported so tests can register a retirement; production code only reads it.
var RetiredSkillIDs = map[string]bool{}

// SplitCatalogRemovals separates the manifest skills the catalog dropped into
// the ones this build retired, which replay deletes, and the ones it merely does
// not know, which replay retains: never rendered, never deleted, still recorded.
func SplitCatalogRemovals(own Ownership, manifestSkills []string) (deleted, retained []string) {
	deleted, retained = make([]string, 0), make([]string, 0)
	for _, id := range manifestSkills {
		if own.ResolveSkill(id) != SkillActionDelete {
			continue
		}
		if RetiredSkillIDs[id] {
			deleted = append(deleted, id)
		} else {
			retained = append(retained, id)
		}
	}
	return deleted, retained
}

// SkillLifecycleDeltas calculates the additions and removals that transform a
// manifest skill list into its lifecycle-resolved desired list. Order follows the
// desired list for additions and the current list for removals.
func SkillLifecycleDeltas(current, desired []string) (additions, removals []string) {
	currentSet := make(map[string]bool, len(current))
	for _, id := range current {
		currentSet[id] = true
	}
	desiredSet := make(map[string]bool, len(desired))
	for _, id := range desired {
		desiredSet[id] = true
		if !currentSet[id] {
			additions = append(additions, id)
		}
	}
	for _, id := range current {
		if !desiredSet[id] {
			removals = append(removals, id)
		}
	}
	return additions, removals
}

// ApplySkillLifecycleDeltas rebases a replay's additions and removals on the
// freshly loaded manifest. It preserves unrelated concurrent selections and adds
// each requested skill at most once.
func ApplySkillLifecycleDeltas(current, additions, removals []string) []string {
	removed := make(map[string]bool, len(removals))
	for _, id := range removals {
		removed[id] = true
	}
	present := make(map[string]bool, len(current)+len(additions))
	next := make([]string, 0, len(current)+len(additions))
	for _, id := range current {
		if removed[id] || present[id] {
			continue
		}
		present[id] = true
		next = append(next, id)
	}
	for _, id := range additions {
		if removed[id] || present[id] {
			continue
		}
		present[id] = true
		next = append(next, id)
	}
	return next
}

// This file is the post-apply verification pass. Replay measures what it
// changed, then checks that what it produced is actually on the machine: a
// component that returned success without leaving a valid managed output is a
// silent failure, and the useful thing to say is how to repair it.
package sync

import (
	"fmt"
	"strings"
)

// RecoveryCommand names the command that repairs a failed run. Replay has no
// per-agent retry: `jarvis sync` re-applies every agent the manifest records. A
// manifest with no configured agents has nothing for sync to replay at all, so
// its only actionable recovery is a reinstall.
func RecoveryCommand(targets []AgentTarget) string {
	if len(targets) == 0 {
		return "jarvis"
	}
	return "jarvis sync"
}

// verifyApplied reports the managed outputs replay failed to produce, measured
// against the same closing snapshot the changed-path report came from. Only a
// path whose desired content is known is verifiable: planning fails closed when
// it cannot render one, so an unknown digest has nothing to be checked against.
func verifyApplied(after Snapshot, tracked []TrackedPath, targets []AgentTarget) error {
	invalid := make([]string, 0)
	for _, path := range tracked {
		observed, recorded := after.states[path.Path]
		valid := recorded && observed.exists &&
			observed.digest == path.Desired && sameManagedMode(observed.mode, path.Mode)
		if path.Desired == "" || valid {
			continue
		}
		invalid = append(invalid, path.Path)
	}
	if len(invalid) == 0 {
		return nil
	}
	return fmt.Errorf(
		"sync finished with %d managed output(s) missing or invalid (%s); run `%s` to repair",
		len(invalid), strings.Join(invalid, ", "), RecoveryCommand(targets),
	)
}

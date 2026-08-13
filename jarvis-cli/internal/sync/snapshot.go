// This file measures replay instead of trusting it: a run snapshots the plan's
// own path list before applying, again after, and reports what differs. The
// comparison is content and mode, and emphatically not modification time,
// because InstallStatusline removes the script and rewrites it on every single
// run (agent/claude.go:882-885) so its 0755 mode always lands on a freshly
// created file. An mtime diff would report a change on every sync forever.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"sort"
)

// Managed modes are asserted on every tracked path, never inherited from what
// is already on disk. writeFileAtomic reuses an existing file's permission bits
// (agent/claude.go:918-923), and a path recreated after deletion takes whatever
// the process umask allows, so neither may decide the final mode.
const (
	// 0644 for instructions (claude.go:367) and skills (installer.go:86); 0755 keeps the statusline runnable (claude.go:885).
	ManagedFileMode       fs.FileMode = 0o644
	ManagedExecutableMode fs.FileMode = 0o755
)

// TrackedPath is one path replay is responsible for, with the mode Jarvis
// asserts on it and the agent it belongs to.
type TrackedPath struct {
	// Agent is the manifest agent ID that owns this path. It is recorded by the
	// planner, where the owner is already known, so the per-agent changed-path
	// report never has to guess an owner back out of a path prefix.
	Agent string
	Path  string
	Mode  fs.FileMode
	// Desired is the SHA-256 of the bytes replay would write here, recorded by
	// the planner where the content is rendered. A digest, not the bytes: the
	// only question asked of it is equality, and holding the whole managed tree
	// in memory answers nothing a 32-byte hash does not. Empty means unknown,
	// which is never a match.
	Desired string
}

// digestOf is the desired-content side of the same hash readFileState computes
// for what is on disk, so the two are directly comparable.
func digestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// fileState is the whole evidence set the diff compares; a path that does not
// exist yet is a first-class state, not an error. digest is the SHA-256 of the
// content, or of a symlink's target.
type fileState struct {
	exists bool
	digest string
	mode   fs.FileMode
}

// Snapshot is the recorded state of one path list at one moment.
type Snapshot struct {
	order  []string
	states map[string]fileState
}

// TakeSnapshot records the current content and mode of every tracked path. It
// Lstats and never follows a symlink, because neither jarvis writer does: the
// skills installer replaces the link (skills/installer.go:102-117) and
// writeFileAtomic renames over it. Resolving it would miss that replacement.
func TakeSnapshot(paths []TrackedPath) (Snapshot, error) {
	snapshot := Snapshot{
		order:  make([]string, 0, len(paths)),
		states: make(map[string]fileState, len(paths)),
	}
	for _, tracked := range paths {
		if _, recorded := snapshot.states[tracked.Path]; recorded {
			continue
		}
		state, err := readFileState(tracked.Path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("snapshot %s: %w", tracked.Path, err)
		}
		snapshot.order = append(snapshot.order, tracked.Path)
		snapshot.states[tracked.Path] = state
	}
	return snapshot, nil
}

func readFileState(path string) (fileState, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return fileState{}, nil
	}
	if err != nil {
		return fileState{}, err
	}
	var content []byte
	if info.Mode()&os.ModeSymlink != 0 {
		target, linkErr := os.Readlink(path)
		if linkErr != nil {
			return fileState{}, linkErr
		}
		content = []byte(target)
	} else if content, err = os.ReadFile(path); err != nil {
		return fileState{}, err
	}
	return fileState{exists: true, digest: digestOf(content), mode: info.Mode()}, nil
}

// Diff reports every path whose content or mode differs between the two
// snapshots, sorted for a stable, reviewable report. A path present in only one
// snapshot counts as changed; a path absent from both never does.
func Diff(before, after Snapshot) []string {
	seen := make(map[string]bool, len(before.order)+len(after.order))
	changed := make([]string, 0)
	for _, path := range append(append([]string{}, before.order...), after.order...) {
		if seen[path] || before.states[path] == after.states[path] {
			continue
		}
		seen[path] = true
		changed = append(changed, path)
	}
	sort.Strings(changed)
	return changed
}

// attributeChanges records the measured diff on the report: the whole list on
// the run, and each path on the agent that tracks it.
//
// Report.Changed stays authoritative. A changed path whose owner is absent from
// the report, which a target list narrower than the plan would produce, is still
// named there rather than dropped on the floor.
func attributeChanges(report Report, tracked []TrackedPath, changed []string) Report {
	owners := make(map[string]string, len(tracked))
	for _, path := range tracked {
		owners[path.Path] = path.Agent
	}
	byAgent := make(map[string][]string, len(report.Agents))
	for _, path := range changed {
		byAgent[owners[path]] = append(byAgent[owners[path]], path)
	}
	report.Changed = changed
	for i, result := range report.Agents {
		report.Agents[i].Changed = byAgent[result.Agent]
	}
	return report
}

// EnforceModes is the counterpart to the diff's mode comparison: the diff
// notices a mode that drifted and this puts it back. Absent paths stay absent
// (asserting a mode is not a licence to create a file) and a symlink is left
// for the writer to replace rather than chmodded through.
func EnforceModes(paths []TrackedPath) error {
	for _, tracked := range paths {
		info, err := os.Lstat(tracked.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("assert mode on %s: %w", tracked.Path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() || sameManagedMode(info.Mode(), tracked.Mode) {
			continue
		}
		if err := os.Chmod(tracked.Path, tracked.Mode.Perm()); err != nil {
			return fmt.Errorf("assert mode %04o on %s: %w", tracked.Mode.Perm(), tracked.Path, err)
		}
	}
	return nil
}

//go:build windows

package sync

import "io/fs"

// sameManagedMode reports whether an observed file carries the permission bits
// replay asserts for it. Windows has no POSIX permission bits: os.Chmod only
// toggles the read-only attribute and Perm always reads 0666 or 0444, so a
// managed file can never report 0644 or 0755 no matter what replay wrote.
//
// Treating that mismatch as real would break replay twice over: the pre-apply
// short-circuit would never match, so a converged machine would apply on every
// run, and post-apply verification would call every managed output invalid.
// Mode is simply not a verifiable property here. Content still is, and the
// digest check covers it.
func sameManagedMode(_, _ fs.FileMode) bool {
	return true
}

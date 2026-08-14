//go:build !windows

package sync

import "io/fs"

// sameManagedMode reports whether an observed file carries the permission bits
// replay asserts for it. POSIX platforms have real permission bits, so this is
// a genuine comparison.
func sameManagedMode(observed, want fs.FileMode) bool {
	return observed.Perm() == want.Perm()
}

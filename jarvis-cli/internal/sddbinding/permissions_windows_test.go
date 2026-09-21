//go:build windows

package sddbinding

import (
	"os"
	"testing"
)

// Windows reports writable/read-only regular-file state rather than Unix
// owner/group bits. Preserve both contracts: writable inputs stay writable and
// read-only inputs stay read-only after adoption.
func assertStatePermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("state mode = %v, %v; want regular file", info.Mode(), err)
	}
	writable := info.Mode().Perm()&0o222 != 0
	if want&0o222 == 0 && writable {
		t.Fatalf("state mode = %v; want read-only regular file", info.Mode())
	}
	if want&0o222 != 0 && !writable {
		t.Fatalf("state mode = %v; want writable regular file", info.Mode())
	}
}

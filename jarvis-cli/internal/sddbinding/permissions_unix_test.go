//go:build !windows

package sddbinding

import (
	"os"
	"testing"
)

func assertStatePermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != want {
		t.Fatalf("state mode = %v, %v; want %v", info.Mode().Perm(), err, want)
	}
}

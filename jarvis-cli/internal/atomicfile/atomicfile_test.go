package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteReplacesContentWithMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("ReadFile() = %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o644) {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
}

func TestRemoveDeletesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Lstat() after Remove error = %v, want not exist", err)
	}
	if err := Remove(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Remove(missing) error = %v, want not exist", err)
	}
}

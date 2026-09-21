//go:build !windows

package sddbinding

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"golang.org/x/sys/unix"
)

func TestReadOpenSpecRejectsFIFOWithoutBlocking(t *testing.T) {
	changeDir := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(changeDir, stateFileName), 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := ReadOpenSpec(changeDir)
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("ReadOpenSpec(FIFO) error = %v, want unsafe path", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("ReadOpenSpec(FIFO) blocked before rejecting the special file")
	}
}

func TestOpenSpecBindingAcceptsSymlinkedAncestor(t *testing.T) {
	parent := t.TempDir()
	physical := filepath.Join(parent, "physical")
	changeDir := filepath.Join(physical, "change")
	alias := filepath.Join(parent, "alias")
	if err := os.MkdirAll(changeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	want := mustBinding(t, sddruntime.StoreModeOpenSpec, "symlinked ancestor")
	if _, created, err := AdoptOpenSpec(filepath.Join(alias, "change"), want); err != nil || !created {
		t.Fatalf("AdoptOpenSpec() = created=%t, err=%v; want true, nil", created, err)
	}
	if binding, err := ReadOpenSpec(changeDir); err != nil || binding == nil || *binding != want {
		t.Fatalf("ReadOpenSpec() = %#v, %v; want %#v, nil", binding, err, want)
	}
}

func TestRootedReplacementCannotEscapeRenamedChangeDirectory(t *testing.T) {
	parent := t.TempDir()
	changeDir := filepath.Join(parent, "change")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(changeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := openChangeRoot(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(changeDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, changeDir); err != nil {
		t.Fatal(err)
	}
	if err := root.replaceState([]byte("bound: true\n"), stateRecord{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, stateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement escaped rooted directory: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(moved, stateFileName)); err != nil || string(data) != "bound: true\n" {
		t.Fatalf("rooted replacement = %q, %v", data, err)
	}
}

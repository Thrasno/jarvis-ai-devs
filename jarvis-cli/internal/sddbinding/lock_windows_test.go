//go:build windows

package sddbinding

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"golang.org/x/sys/windows"
)

func TestWindowsMutexUsesGlobalPhysicalIdentity(t *testing.T) {
	changeDir := t.TempDir()
	first, err := openChangeRoot(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := openChangeRoot(changeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if !strings.HasPrefix(first.mutexName, "Global\\jarvis-sddbinding-") || first.mutexName != second.mutexName {
		t.Fatalf("mutex names = %q, %q; want identical global physical identity", first.mutexName, second.mutexName)
	}
	unlock, err := first.lock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	unlock()
}

func TestWindowsReadOnlyReplacementSourceContract(t *testing.T) {
	want := uint32(windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS | windows.FILE_RENAME_IGNORE_READONLY_ATTRIBUTE)
	if got := stagedRenameFlags(); got != want || got&windows.FILE_RENAME_IGNORE_READONLY_ATTRIBUTE == 0 {
		t.Fatalf("stagedRenameFlags() = %#x, want %#x including ignore-readonly", got, want)
	}
	source, err := os.ReadFile("lock_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "r.root.Chmod(stateFileName") {
		t.Fatal("read-only replacement mutates the destination before atomic rename")
	}
	if !strings.Contains(string(source), "SetFileInformationByHandle") {
		t.Fatal("read-only replacement does not use handle-based atomic rename")
	}
	if strings.Contains(string(source), "return fmt.Errorf(\"close OpenSpec binding stage: %w\", err)") {
		t.Fatal("post-commit staged-handle close can report failure after a successful rename")
	}
	if !strings.Contains(string(source), "\t_ = file.Close()\n\t// SetFileInformationByHandle is the commit point") {
		t.Fatal("successful rename does not best-effort close the staged handle before returning success")
	}
}

func TestWindowsRootedBindingContract(t *testing.T) {
	changeDir := t.TempDir()
	want := mustBinding(t, sddruntime.StoreModeOpenSpec, "windows selection")
	if _, created, err := AdoptOpenSpec(changeDir, want); err != nil || !created {
		t.Fatalf("AdoptOpenSpec() = created=%t, err=%v; want true, nil", created, err)
	}
	if _, created, err := AdoptOpenSpec(changeDir, want); err != nil || created {
		t.Fatalf("AdoptOpenSpec(replay) = created=%t, err=%v; want false, nil", created, err)
	}
	other := mustBinding(t, sddruntime.StoreModeHive, "windows selection")
	if _, _, err := AdoptOpenSpec(changeDir, other); !errors.Is(err, ErrBindingConflict) {
		t.Fatalf("AdoptOpenSpec(conflict) error = %v, want binding conflict", err)
	}
	binding, err := ReadOpenSpec(changeDir)
	if err != nil || binding == nil || *binding != want {
		t.Fatalf("ReadOpenSpec() = %#v, %v; want %#v, nil", binding, err, want)
	}
}

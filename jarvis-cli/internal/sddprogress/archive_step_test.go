package sddprogress

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

func archiveStepFixture(t *testing.T) (OpenSpec, string) {
	t.Helper()
	root := newOpenSpecTestRoot(t)
	if _, err := (OpenSpec{Root: root}).Advance(validArchiveRequest(t, "archive-step", applyprogress.StatusComplete)); err != nil {
		t.Fatal(err)
	}
	return OpenSpec{Root: root}, filepath.Join(t.TempDir(), "archive")
}

func TestOpenSpecArchiveRunsPreMoveStepUnderLockAfterValidation(t *testing.T) {
	store, destination := archiveStepFixture(t)
	var order []string
	store.BeforeArchiveValidate = func() error { order = append(order, "locked"); return nil }
	validate := func() error { order = append(order, "validate"); return nil }
	step := func() (func() error, error) {
		order = append(order, "step")
		if _, err := os.Stat(store.Root); err != nil {
			t.Fatalf("step ran after the move: %v", err)
		}
		return func() error { order = append(order, "revert"); return nil }, nil
	}
	if err := store.ArchiveWithPreMoveStep(destination, validate, step); err != nil {
		t.Fatalf("ArchiveWithPreMoveStep() error = %v", err)
	}
	if got := strings.Join(order, ","); got != "locked,validate,step" {
		t.Fatalf("order = %s, want locked,validate,step", got)
	}
	if _, err := os.Stat(filepath.Join(destination, "apply-progress.md")); err != nil {
		t.Fatalf("topology not moved: %v", err)
	}
}

func TestOpenSpecArchivePreMoveStepErrorMovesNothing(t *testing.T) {
	store, destination := archiveStepFixture(t)
	stop := errors.New("step stopped")
	err := store.ArchiveWithPreMoveStep(destination, nil, func() (func() error, error) { return nil, stop })
	if !errors.Is(err, stop) {
		t.Fatalf("ArchiveWithPreMoveStep() error = %v, want %v", err, stop)
	}
	if _, err := os.Stat(filepath.Join(store.Root, "apply-progress.md")); err != nil {
		t.Fatalf("source moved: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists: %v", err)
	}
}

func TestOpenSpecArchiveRevertsPreMoveStepWhenMoveFails(t *testing.T) {
	store, destination := archiveStepFixture(t)
	moveErr := errors.New("injected move failure")
	revertErr := errors.New("injected revert failure")
	store.ArchiveRename = func(string, string) error { return moveErr }
	for _, tt := range []struct {
		name   string
		revert error
	}{
		{name: "revert succeeds"},
		{name: "revert fails", revert: revertErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reverted := 0
			err := store.ArchiveWithPreMoveStep(destination, nil, func() (func() error, error) {
				return func() error { reverted++; return tt.revert }, nil
			})
			if !errors.Is(err, moveErr) || reverted != 1 {
				t.Fatalf("ArchiveWithPreMoveStep() error = %v reverted = %d, want move error and one revert", err, reverted)
			}
			if tt.revert != nil && !errors.Is(err, tt.revert) {
				t.Fatalf("ArchiveWithPreMoveStep() error = %v, want revert error joined", err)
			}
			if _, err := os.Stat(filepath.Join(store.Root, "apply-progress.md")); err != nil {
				t.Fatalf("source moved: %v", err)
			}
		})
	}
}

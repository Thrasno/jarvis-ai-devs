package sddprogress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var (
	ErrBackendDiverged = errors.New("apply-progress backend diverged")
	ErrMissingBackend  = errors.New("apply-progress backend missing")
)

// ResolvedProgress is a backend's independently validated v2 snapshot.
type ResolvedProgress struct{ Snapshot applyprogress.Snapshot }

type progressResolver func() (ResolvedProgress, error)

// ResolveHybrid accepts progress only when both independently validated backends
// resolve the same canonical snapshot. It intentionally never selects a winner.
func ResolveHybrid(openspec, hive progressResolver) (ResolvedProgress, error) {
	left, leftErr := openspec()
	right, rightErr := hive()
	if leftErr != nil || rightErr != nil || !sameSnapshot(left.Snapshot, right.Snapshot) {
		return ResolvedProgress{}, ErrBackendDiverged
	}
	return left, nil
}

func sameSnapshot(left, right applyprogress.Snapshot) bool {
	return left.Schema == right.Schema && left.Project == right.Project && left.Change == right.Change && left.Generation == right.Generation && left.Revision == right.Revision && left.PreviousDigest == right.PreviousDigest && left.TaskManifestSHA256 == right.TaskManifestSHA256 && left.Status == right.Status && left.Digest != "" && left.Digest == right.Digest && slices.Equal(left.Batches, right.Batches) && slices.Equal(left.Coverage, right.Coverage)
}

type advanceBackend interface {
	Advance(AdvanceRequest) (AdvanceResult, error)
}

type currentBackend interface {
	Current(AdvanceRequest) (AdvanceResult, error)
}

type legacyBackend interface {
	UpgradeLegacy(AdvanceRequest) (AdvanceResult, bool, error)
}

type legacyRestorer interface {
	RestoreLegacy([]byte) error
}

type receiptOutcome string

const (
	receiptPending   receiptOutcome = "pending"
	receiptCommitted receiptOutcome = "committed"
	receiptFailed    receiptOutcome = "failed"
)

type hybridReceipt struct {
	Payload  string         `json:"payload"`
	OpenSpec receiptOutcome `json:"openspec"`
	Hive     receiptOutcome `json:"hive"`
}

// Hybrid uses a durable receipt to repair only the side that did not commit.
type Hybrid struct {
	Root      string
	OpenSpec  advanceBackend
	Hive      advanceBackend
	HiveFirst bool // Test seam for either interrupted publication direction.
}

func (h Hybrid) UpgradeLegacy(request AdvanceRequest) (AdvanceResult, bool, error) {
	upgrader, ok := h.OpenSpec.(legacyBackend)
	if !ok {
		return AdvanceResult{}, false, nil
	}
	legacy, readErr := os.ReadFile(filepath.Join(h.Root, "apply-progress.md"))
	if readErr != nil || isV2(legacy) {
		return AdvanceResult{}, false, nil
	}
	_, upgraded, err := upgrader.UpgradeLegacy(request)
	if !upgraded || err != nil {
		return AdvanceResult{}, upgraded, err
	}
	result, err := h.Advance(request)
	if err != nil {
		if restorer, ok := h.OpenSpec.(legacyRestorer); ok {
			if restoreErr := restorer.RestoreLegacy(legacy); restoreErr != nil {
				return AdvanceResult{}, true, fmt.Errorf("hybrid legacy upgrade: %w; restore legacy: %v", err, restoreErr)
			}
		}
	}
	return result, true, err
}

func (h Hybrid) Advance(request AdvanceRequest) (AdvanceResult, error) {
	if request.RequestID == "" || filepath.Base(request.RequestID) != request.RequestID {
		return AdvanceResult{}, ErrRequestConflict
	}
	if err := os.MkdirAll(h.Root, 0o755); err != nil {
		return AdvanceResult{}, err
	}
	unlock, err := h.lock()
	if err != nil {
		return AdvanceResult{}, err
	}
	defer unlock()
	payload := payloadDigestFor(request)
	receipt, found, err := h.receipt(request.RequestID)
	if err != nil {
		return AdvanceResult{}, err
	}
	if !found {
		receipt = hybridReceipt{Payload: payload, OpenSpec: receiptPending, Hive: receiptPending}
		if err := h.writeReceipt(request.RequestID, receipt); err != nil {
			return AdvanceResult{}, err
		}
	} else if receipt.Payload != payload {
		return AdvanceResult{}, ErrRequestConflict
	}
	sides := []struct {
		outcome *receiptOutcome
		backend advanceBackend
	}{{&receipt.OpenSpec, h.OpenSpec}, {&receipt.Hive, h.Hive}}
	if h.HiveFirst {
		sides[0], sides[1] = sides[1], sides[0]
	}
	for _, side := range sides {
		if *side.outcome == receiptCommitted {
			continue
		}
		var result AdvanceResult
		if side.backend == nil {
			err = fmt.Errorf("missing hybrid backend")
		} else {
			result, err = side.backend.Advance(request)
		}
		if err == nil {
			*side.outcome = receiptCommitted
		} else {
			*side.outcome = receiptFailed
		}
		if writeErr := h.writeReceipt(request.RequestID, receipt); writeErr != nil {
			return AdvanceResult{}, writeErr
		}
		if err != nil {
			if errors.Is(err, ErrConflict) && result.Digest == "" {
				if hive, ok := h.Hive.(currentBackend); ok {
					current, currentErr := hive.Current(request)
					if currentErr != nil {
						return AdvanceResult{}, fmt.Errorf("%w: %w", ErrBackendDiverged, currentErr)
					}
					result = current
				}
				if receipt.OpenSpec != receiptCommitted && receipt.Hive != receiptCommitted {
					dir := filepath.Join(h.Root, ".apply-progress-hybrid-receipts")
					if removeErr := os.Remove(filepath.Join(dir, request.RequestID+".json")); removeErr != nil {
						return AdvanceResult{}, removeErr
					}
					if syncErr := syncDir(dir); syncErr != nil {
						return AdvanceResult{}, syncErr
					}
				}
			}
			return result, fmt.Errorf("%w: %w", ErrBackendDiverged, err)
		}
	}
	return AdvanceResult{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest}, nil
}

func (h Hybrid) lock() (func(), error) {
	file, err := os.OpenFile(filepath.Join(h.Root, ".apply-progress-hybrid.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func payloadDigestFor(request AdvanceRequest) string {
	_, data, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		return ""
	}
	return payloadDigest(data, request)
}

func (h Hybrid) receipt(id string) (hybridReceipt, bool, error) {
	data, err := os.ReadFile(filepath.Join(h.Root, ".apply-progress-hybrid-receipts", id+".json"))
	if os.IsNotExist(err) {
		return hybridReceipt{}, false, nil
	}
	var receipt hybridReceipt
	if err != nil || json.Unmarshal(data, &receipt) != nil || !validReceipt(receipt) {
		return hybridReceipt{}, false, ErrRequestConflict
	}
	return receipt, true, nil
}

func validReceipt(receipt hybridReceipt) bool {
	valid := func(outcome receiptOutcome) bool {
		return outcome == receiptPending || outcome == receiptCommitted || outcome == receiptFailed
	}
	return receipt.Payload != "" && valid(receipt.OpenSpec) && valid(receipt.Hive)
}

func (h Hybrid) writeReceipt(id string, receipt hybridReceipt) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Root, ".apply-progress-hybrid-receipts", id+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".receipt-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err == nil {
		err = syncDir(filepath.Dir(path))
	}
	return err
}

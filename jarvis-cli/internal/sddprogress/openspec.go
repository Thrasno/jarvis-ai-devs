package sddprogress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

// OpenSpec exposes the Go-only atomic publication seam used by a later adapter.
type OpenSpec struct {
	Root                  string
	BeforeRename          func() error       // Test-only interruption point after durable staging.
	BeforeArchiveValidate func() error       // Test-only hook after archive lock acquisition.
	SyncDir               func(string) error // Test-only durability observer.
}

func (s OpenSpec) Advance(request AdvanceRequest) (AdvanceResult, error) {
	if request.RequestID == "" || filepath.Base(request.RequestID) != request.RequestID {
		return AdvanceResult{}, fmt.Errorf("invalid request ID")
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return AdvanceResult{}, err
	}
	unlock, err := s.lock()
	if err != nil {
		return AdvanceResult{}, err
	}
	defer unlock()

	snapshot, snapshotData, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		return AdvanceResult{}, err
	}
	payload := payloadDigest(snapshotData, request)
	if err := s.checkReceipt(request.RequestID, payload); err != nil {
		return AdvanceResult{}, err
	}
	current, _, err := s.current()
	if err != nil {
		return AdvanceResult{}, err
	}
	if current != nil && current.Digest == snapshot.Digest {
		return AdvanceResult{snapshot.Generation, snapshot.Revision, snapshot.Digest}, nil
	}
	if err := validateAdvance(current, request, snapshot); err != nil {
		return AdvanceResult{}, err
	}
	for _, batch := range request.Batches {
		sealed, data, err := applyprogress.SealBatch(batch)
		if err != nil {
			return AdvanceResult{}, err
		}
		if err := writeImmutable(filepath.Join(s.Root, "apply-evidence", sealed.BatchID+".json"), data, s.syncDir); err != nil {
			return AdvanceResult{}, err
		}
	}
	if err := s.ensureReferencedBatches(snapshot); err != nil {
		return AdvanceResult{}, err
	}
	if err := writeImmutable(filepath.Join(s.Root, ".apply-progress-receipts", request.RequestID+".json"), []byte(`{"payload":"`+payload+`"}`), s.syncDir); err != nil {
		return AdvanceResult{}, err
	}
	if err := s.publish(snapshotData); err != nil {
		return AdvanceResult{}, err
	}
	return AdvanceResult{snapshot.Generation, snapshot.Revision, snapshot.Digest}, nil
}

func (s OpenSpec) ensureReferencedBatches(snapshot applyprogress.Snapshot) error {
	batches := make(map[string]applyprogress.Batch, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		data, err := os.ReadFile(filepath.Join(s.Root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return ErrMissingBatch
		}
		batch, err := applyprogress.DecodeCanonicalBatch(data)
		if err != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return ErrMissingBatch
		}
		batches[batch.BatchID] = batch
	}
	if err := applyprogress.ValidateEvidenceCoverage(snapshot, batches); err != nil {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return nil
}

func (s OpenSpec) current() (*applyprogress.Snapshot, []byte, error) {
	data, err := os.ReadFile(filepath.Join(s.Root, "apply-progress.md"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(data)
	if err != nil {
		return nil, nil, err
	}
	return &snapshot, data, nil
}

func (s OpenSpec) checkReceipt(id, payload string) error {
	data, err := os.ReadFile(filepath.Join(s.Root, ".apply-progress-receipts", id+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || string(data) != `{"payload":"`+payload+`"}` {
		return ErrRequestConflict
	}
	return nil
}

func (s OpenSpec) publish(data []byte) error {
	file, err := os.CreateTemp(s.Root, ".apply-progress-")
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
	if err != nil {
		return err
	}
	if s.BeforeRename != nil {
		return s.BeforeRename()
	}
	if err := os.Rename(name, filepath.Join(s.Root, "apply-progress.md")); err != nil {
		return err
	}
	return s.syncDir(s.Root)
}

// Archive validates the authoritative snapshot before atomically moving the
// complete change topology, including immutable evidence and request receipts.
func (s OpenSpec) Archive(destination string) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if s.BeforeArchiveValidate != nil {
		if err := s.BeforeArchiveValidate(); err != nil {
			return err
		}
	}
	snapshot, data, err := s.current()
	if err != nil || snapshot == nil {
		return ErrMissingBatch
	}
	if snapshot.Status != applyprogress.StatusComplete {
		return ErrConflict
	}
	if err := s.validateArchive(*snapshot, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.Rename(s.Root, destination); err != nil {
		return err
	}
	return s.syncDir(filepath.Dir(destination))
}

func (s OpenSpec) validateArchive(snapshot applyprogress.Snapshot, data []byte) error {
	taskData, err := os.ReadFile(filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return err
	}
	var tasks []applyprogress.Task
	for _, line := range strings.Split(string(taskData), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		end := strings.Index(line, "]")
		fields := strings.Fields(line[end+1:])
		if end >= 0 && len(fields) > 1 {
			tasks = append(tasks, applyprogress.Task{ID: fields[0], Text: strings.Join(fields[1:], " ")})
		}
	}
	batches := make(map[string][]byte, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, err := os.ReadFile(filepath.Join(s.Root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return ErrMissingBatch
		}
		batches[ref.BatchID] = batch
	}
	return applyprogress.ValidateProgress(data, tasks, batches)
}

func (s OpenSpec) lock() (func(), error) {
	file, err := os.OpenFile(filepath.Join(s.Root, "apply-progress.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func (s OpenSpec) syncDir(path string) error {
	if s.SyncDir != nil {
		return s.SyncDir(path)
	}
	return syncDir(path)
}

func writeImmutable(path string, data []byte, sync func(string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		existing, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(existing, data) {
			return nil
		}
		return ErrBatchCollision
	}
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return sync(filepath.Dir(path))
}

func validateAdvance(current *applyprogress.Snapshot, request AdvanceRequest, snapshot applyprogress.Snapshot) error {
	if snapshot.Generation != request.ExpectedGeneration+1 || snapshot.Revision != request.ExpectedRevision+1 || snapshot.PreviousDigest != request.ExpectedDigest {
		return ErrConflict
	}
	if current == nil {
		if request.ExpectedGeneration != 0 || request.ExpectedRevision != 0 || request.ExpectedDigest != "" {
			return ErrConflict
		}
		return nil
	}
	if current.Generation != request.ExpectedGeneration || current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return ErrConflict
	}
	return nil
}

func payloadDigest(snapshot []byte, request AdvanceRequest) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d:%d:%s:", request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest)
	hash.Write(snapshot)
	for _, batch := range request.Batches {
		_, data, _ := applyprogress.SealBatch(batch)
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func syncDir(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

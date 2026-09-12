package sddprogress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress/filelock"
)

// OpenSpec exposes the Go-only atomic publication seam used by a later adapter.
type OpenSpec struct {
	Root                   string
	BeforeRename           func() error       // Test-only interruption point after durable staging.
	BeforeImmutablePublish func() error       // Test-only interruption point before immutable publication.
	BeforeArchiveValidate  func() error       // Test-only hook after archive lock acquisition.
	SyncDir                func(string) error // Test-only durability observer.
}

type receipt struct {
	Payload string `json:"payload"`
}

func (s OpenSpec) Advance(request AdvanceRequest) (AdvanceResult, error) {
	return s.advance(request, nil)
}

// AdvanceLegacy publishes a pre-validated legacy migration through the normal
// immutable publication path. Hybrid owns the cross-backend receipt and calls this
// only after that receipt is durable.
func (s OpenSpec) AdvanceLegacy(request AdvanceRequest, legacy []byte) (AdvanceResult, error) {
	return s.advance(request, legacy)
}

// Current resolves the validated authoritative OpenSpec snapshot, if any. It is
// a mutation-capable reader for recovery paths, so it removes only safe stale
// stages while holding the publication lock before inspecting the topology.
func (s OpenSpec) Current() (*applyprogress.Snapshot, error) {
	if err := s.validateRoot(); err != nil {
		return nil, err
	}
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return s.InspectPublication()
}

// InspectPublication resolves the authoritative publication topology without
// acquiring a lock, cleaning staging residue, or mutating the filesystem. Status
// readers use it so an interrupted receipt lineage remains observable.
func (s OpenSpec) InspectPublication() (*applyprogress.Snapshot, error) {
	if err := s.validateRoot(); err != nil {
		return nil, err
	}
	if err := s.validateStaging(nil); err != nil {
		return nil, err
	}
	snapshot, raw, err := s.current()
	if err != nil {
		if !isV2(raw) {
			// A legacy head cannot hide write-ahead receipts from an interrupted
			// publication. Inspect that immutable topology before classifying the
			// readable head as requiring a legacy upgrade.
			if receiptErr := s.validateReceiptLineage(nil, false); receiptErr != nil {
				return nil, receiptErr
			}
			return nil, ErrLegacyMigration
		}
		return nil, err
	}
	if snapshot == nil {
		if err := s.validateReceiptLineage(nil, false); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := s.validateReceiptLineage(snapshot, false); err != nil {
		return nil, err
	}
	if err := s.validateCurrentProgress(*snapshot, raw); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s OpenSpec) advance(request AdvanceRequest, legacy []byte) (AdvanceResult, error) {
	if !applyprogress.ValidID(request.RequestID) {
		return AdvanceResult{}, fmt.Errorf("%w: request_id", applyprogress.ErrInvalidID)
	}
	if err := s.ValidateRequestManifest(request); err != nil {
		return AdvanceResult{}, err
	}
	unlock, err := s.lock()
	if err != nil {
		return AdvanceResult{}, err
	}
	defer unlock()

	current, raw, currentErr := s.current()
	snapshot, snapshotData, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		var capacity *applyprogress.CapacityError
		if errors.As(err, &capacity) && capacity.Document == "snapshot" {
			currentRunes := 0
			if currentErr == nil && current != nil {
				currentRunes = utf8.RuneCount(raw)
			}
			capacity = &applyprogress.CapacityError{Document: "snapshot", Runes: capacity.Runes, Limit: applyprogress.MaxDocumentRunes, CurrentRunes: currentRunes, ProjectedRunes: capacity.Runes, CeilingRunes: applyprogress.MaxDocumentRunes}
			return AdvanceResult{}, &applyprogress.SnapshotCapacityExhaustedError{Capacity: capacity}
		}
		return AdvanceResult{}, err
	}
	payload, err := payloadDigest(snapshotData, request)
	if err != nil {
		return AdvanceResult{}, err
	}
	stages, err := s.expectedStaging(request, snapshot, snapshotData, payload)
	if err != nil {
		return AdvanceResult{}, err
	}
	if err := s.validateStaging(stages); err != nil {
		return AdvanceResult{}, err
	}
	for _, stage := range stages {
		if stage.path == filepath.Join(s.Root, "apply-progress.md") {
			continue
		}
		if err := recoverStagedImmutable(stage.path, stage.data, s.syncDir, s.BeforeImmutablePublish); err != nil {
			return AdvanceResult{}, err
		}
	}
	receipt, replay, err := s.checkReceipt(request.RequestID, payload)
	if err != nil {
		return AdvanceResult{}, err
	}
	if currentErr != nil {
		if legacy == nil || !bytes.Equal(raw, legacy) || isV2(raw) {
			return AdvanceResult{}, fmt.Errorf("%w: %v", ErrLegacyMigration, currentErr)
		}
		current = nil
	}
	if !replay {
		if _, statErr := os.Stat(filepath.Join(s.Root, ".apply-progress-receipts", request.RequestID+".json")); os.IsNotExist(statErr) {
			if err := s.validateReceiptLineage(current, false); err != nil {
				return AdvanceResult{}, err
			}
		} else if statErr != nil {
			return AdvanceResult{}, statErr
		}
	}
	if replay {
		// A matching receipt owns its original result only after its snapshot is
		// the current head or a validated direct predecessor. A durable receipt
		// staged before an interrupted rename is not a committed result.
		if !sameSnapshot(receipt.Snapshot, snapshot) {
			return AdvanceResult{}, ErrRequestConflict
		}
		committed, err := s.receiptSnapshotCommitted(receipt.Snapshot, current)
		if err != nil {
			return AdvanceResult{}, err
		}
		if committed {
			if err := s.ensureReferencedBatches(receipt.Snapshot); err != nil {
				return AdvanceResult{}, err
			}
			return AdvanceResult{Generation: receipt.Snapshot.Generation, Revision: receipt.Snapshot.Revision, Digest: receipt.Snapshot.Digest, PayloadSHA256: receipt.Payload}, nil
		}
	}
	if err := validateImportedOpenSpecEvidence(request, current, raw, legacy); err != nil {
		return AdvanceResult{}, err
	}
	if current != nil && current.Digest == snapshot.Digest {
		return AdvanceResult{}, ErrRequestConflict
	}
	batches := make(map[string]applyprogress.Batch, len(request.Batches))
	for _, batch := range request.Batches {
		sealed, _, err := applyprogress.SealBatch(batch)
		if err != nil {
			return AdvanceResult{}, err
		}
		batches[sealed.BatchID] = sealed
	}
	if current != nil {
		prior, err := s.referencedBatches(*current)
		if err != nil {
			return AdvanceResult{}, err
		}
		for id, batch := range prior {
			batches[id] = batch
		}
	}
	if err := s.validateCandidateProgress(snapshotData, snapshot, batches); err != nil {
		return AdvanceResult{}, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	if err := validateAdvance(current, request, snapshot, batches); err != nil {
		return AdvanceResult{}, err
	}
	for _, batch := range request.Batches {
		sealed, data, err := applyprogress.SealBatch(batch)
		if err != nil {
			return AdvanceResult{}, err
		}
		if err := s.rejectReferencedCorruptBatch(current, sealed.BatchID, data); err != nil {
			return AdvanceResult{}, err
		}
		if err := writeImmutable(filepath.Join(s.Root, "apply-evidence", sealed.BatchID+".json"), data, s.syncDir, s.BeforeImmutablePublish); err != nil {
			return AdvanceResult{}, err
		}
	}
	if err := s.ensureReferencedBatches(snapshot); err != nil {
		return AdvanceResult{}, err
	}
	receiptData, err := json.Marshal(immutableReceipt{Payload: payload, Snapshot: snapshot})
	if err != nil {
		return AdvanceResult{}, err
	}
	if err := writeImmutable(filepath.Join(s.Root, ".apply-progress-receipts", request.RequestID+".json"), receiptData, s.syncDir, s.BeforeImmutablePublish); err != nil {
		return AdvanceResult{}, err
	}
	if err := s.publish(snapshotData); err != nil {
		if legacy != nil {
			if restoreErr := s.restoreLegacy(legacy); restoreErr != nil {
				return AdvanceResult{}, fmt.Errorf("publish legacy upgrade: %w; restore legacy: %v", err, restoreErr)
			}
		}
		return AdvanceResult{}, err
	}
	return AdvanceResult{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, PayloadSHA256: payload}, nil
}

// ValidateRequestManifest binds every OpenSpec publication to the frozen
// authoritative tasks.md parser result before it creates evidence, receipts, or a head.
func (s OpenSpec) ValidateRequestManifest(request AdvanceRequest) error {
	manifest, strictErr := s.authoritativeTaskManifest()
	if strictErr == nil && request.Snapshot.TaskManifestSHA256 == manifest {
		return nil
	}
	// Legacy parsing is migration compatibility, not a v2 fallback. An incoming
	// conversion carries source-bound imported evidence; a continuation proves the
	// same authority from its already-published immutable predecessor.
	if _, legacyManifest, legacyErr := s.legacyTaskManifest(); legacyErr == nil && request.Snapshot.TaskManifestSHA256 == legacyManifest {
		incomingImport := applyprogress.ValidDigest(request.LegacySourceSHA256) && applyprogress.ContainsImportedEvidence(request.Batches)
		if incomingImport || s.currentLegacyImportProvenance(legacyManifest) {
			return nil
		}
	}
	if strictErr != nil {
		return strictErr
	}
	return &applyprogress.ValidationError{Code: applyprogress.CodeTaskManifestMismatch, Detail: "task_manifest_sha256"}
}

// currentLegacyImportProvenance proves that a legacy manifest belongs to an
// already-published migration rather than to a caller-selected v2 fallback.
func (s OpenSpec) currentLegacyImportProvenance(manifest string) bool {
	snapshot, _, err := s.current()
	if err != nil || snapshot == nil || snapshot.TaskManifestSHA256 != manifest {
		return false
	}
	for _, ref := range snapshot.Batches {
		data, err := readAuthoritativeBatch(filepath.Join(s.Root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return false
		}
		batch, err := applyprogress.DecodeCanonicalBatch(data)
		if err != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 {
			return false
		}
		if applyprogress.ContainsImportedEvidence([]applyprogress.Batch{batch}) {
			return true
		}
	}
	return false
}

func (s OpenSpec) authoritativeTaskManifest() (string, error) {
	if err := s.validateRoot(); err != nil {
		return "", err
	}
	tasks, err := readRegularFile(filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return "", ErrInvalidChangeRoot
	}
	parsed, err := applyprogress.ParseTasksMarkdown(string(tasks))
	if err != nil || len(parsed.Tasks) == 0 {
		return "", ErrInvalidChangeRoot
	}
	_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
	if err != nil {
		return "", ErrInvalidChangeRoot
	}
	return manifest, nil
}

func (s OpenSpec) validateChangeRoot() error {
	_, err := s.authoritativeTaskManifest()
	return err
}

func (s OpenSpec) validateRoot() error { return validateRegularDirectory(s.Root) }

func validateExistingPathComponents(path string) error {
	return ValidateExistingPathComponents(path)
}

// ValidateExistingPathComponents rejects a symlink in every existing component.
// It intentionally stops at the first missing component so writers can create a
// final path only after the already-existing directory topology is authenticated.
func ValidateExistingPathComponents(path string) error {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(path, current), string(filepath.Separator))
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (index < len(parts)-1 && !info.IsDir()) {
			return ErrInvalidChangeRoot
		}
	}
	return nil
}

func validateRegularDirectory(path string) error {
	if err := validateExistingPathComponents(path); err != nil {
		return ErrInvalidChangeRoot
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrInvalidChangeRoot
	}
	return nil
}

func ensureRegularDirectory(path string) error {
	if err := validateExistingPathComponents(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return validateRegularDirectory(path)
}

func readRegularFile(path string) ([]byte, error) {
	if err := validateExistingPathComponents(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, ErrInvalidChangeRoot
	}
	return os.ReadFile(path)
}

func readAuthoritativeDir(path string) ([]os.DirEntry, error) {
	if err := validateExistingPathComponents(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrInvalidChangeRoot
	}
	return os.ReadDir(path)
}

func readAuthoritativeBatch(path string) ([]byte, error) {
	data, err := readRegularFile(path)
	if err != nil {
		return nil, ErrMissingBatch
	}
	return data, nil
}

func (s OpenSpec) legacyTaskManifest() ([]applyprogress.Task, string, error) {
	tasksData, err := readRegularFile(filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return nil, "", err
	}
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(string(tasksData))
	if err != nil || len(parsed.Tasks) == 0 {
		return nil, "", ErrInvalidChangeRoot
	}
	return applyprogress.LegacyTaskManifest(parsed.Tasks)
}

func (s OpenSpec) authoritativeTasksForManifest(manifest string) ([]applyprogress.Task, error) {
	tasksData, err := readRegularFile(filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return nil, ErrInvalidChangeRoot
	}
	content := string(tasksData)
	if parsed, parseErr := applyprogress.ParseTasksMarkdown(content); parseErr == nil && len(parsed.Tasks) != 0 {
		if _, nativeManifest, manifestErr := applyprogress.TaskManifest(parsed.Tasks); manifestErr == nil && nativeManifest == manifest {
			return parsed.Tasks, nil
		}
	}
	legacyParsed, legacyParseErr := applyprogress.ParseLegacyTasksMarkdown(content)
	if legacyParseErr != nil || len(legacyParsed.Tasks) == 0 {
		return nil, ErrInvalidChangeRoot
	}
	legacy, legacyManifest, legacyErr := applyprogress.LegacyTaskManifest(legacyParsed.Tasks)
	if legacyErr == nil && legacyManifest == manifest {
		return legacy, nil
	}
	return nil, &applyprogress.ValidationError{Code: applyprogress.CodeTaskManifestMismatch, Detail: "task_manifest_sha256"}
}

// validateCurrentProgress validates the complete authoritative topology, rather
// than merely checking that referenced evidence files can be decoded. A valid
// coverage projection alone can still hide a task-manifest mismatch or invalid
// task attribution in immutable evidence.
// validateCandidateProgress validates the same complete snapshot-to-evidence
// topology that Hive requires before an initial or successor OpenSpec head can
// be published.
func (s OpenSpec) validateCandidateProgress(data []byte, snapshot applyprogress.Snapshot, batches map[string]applyprogress.Batch) error {
	tasks, err := s.authoritativeTasksForManifest(snapshot.TaskManifestSHA256)
	if err != nil {
		return err
	}
	raw := make(map[string][]byte, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, ok := batches[ref.BatchID]
		if !ok {
			return ErrMissingBatch
		}
		_, batchData, err := applyprogress.SealBatch(batch)
		if err != nil {
			return err
		}
		raw[ref.BatchID] = batchData
	}
	return applyprogress.ValidateProgress(data, tasks, raw)
}

func (s OpenSpec) validateCurrentProgress(snapshot applyprogress.Snapshot, data []byte) error {
	tasks, err := s.authoritativeTasksForManifest(snapshot.TaskManifestSHA256)
	if err != nil {
		return err
	}
	batches := make(map[string][]byte, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, err := readAuthoritativeBatch(filepath.Join(s.Root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return err
		}
		batches[ref.BatchID] = batch
	}
	return applyprogress.ValidateProgress(data, tasks, batches)
}

func (s OpenSpec) rejectReferencedCorruptBatch(snapshot *applyprogress.Snapshot, batchID string, incoming []byte) error {
	if snapshot == nil {
		return nil
	}
	referenced := false
	for _, ref := range snapshot.Batches {
		if ref.BatchID == batchID {
			referenced = true
			break
		}
	}
	if !referenced {
		return nil
	}
	existing, err := readRegularFile(filepath.Join(s.Root, "apply-evidence", batchID+".json"))
	if os.IsNotExist(err) || bytes.Equal(existing, incoming) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := applyprogress.DecodeCanonicalBatch(existing); err != nil {
		return ErrBatchCollision
	}
	return nil
}

func (s OpenSpec) ensureReferencedBatches(snapshot applyprogress.Snapshot) error {
	_, err := s.referencedBatches(snapshot)
	return err
}

func (s OpenSpec) receiptSnapshotCommitted(receipt applyprogress.Snapshot, current *applyprogress.Snapshot) (bool, error) {
	if current == nil {
		return false, nil
	}
	if current.Digest == receipt.Digest {
		return true, nil
	}

	// The head retains only its immediate predecessor. Receipts preserve every
	// committed snapshot, so walk that immutable chain backwards to prove that a
	// delayed replay belongs to the current lineage rather than a stale fork.
	receipts, err := s.committedReceiptSnapshots()
	if err != nil {
		return false, err
	}
	candidate := *current
	seen := map[string]bool{}
	for candidate.Digest != receipt.Digest {
		if candidate.PreviousDigest == "" || seen[candidate.Digest] {
			return false, nil
		}
		seen[candidate.Digest] = true

		batches, err := s.referencedBatches(candidate)
		if err != nil {
			return false, err
		}
		var predecessor applyprogress.Snapshot
		found := false
		for _, stored := range receipts[candidate.PreviousDigest] {
			if applyprogress.ValidateSuccessor(stored.Snapshot, candidate, batches) != nil {
				continue
			}
			if found && !sameSnapshot(predecessor, stored.Snapshot) {
				return false, nil
			}
			predecessor, found = stored.Snapshot, true
		}
		if !found {
			return false, nil
		}
		if err := s.ensureReferencedBatches(predecessor); err != nil {
			return false, err
		}
		candidate = predecessor
	}
	return true, nil
}

func (s OpenSpec) committedReceiptSnapshots() (map[string][]immutableReceipt, error) {
	dir := filepath.Join(s.Root, ".apply-progress-receipts")
	entries, err := readAuthoritativeDir(dir)
	if os.IsNotExist(err) {
		return map[string][]immutableReceipt{}, nil
	}
	if err != nil {
		return nil, err
	}
	receipts := make(map[string][]immutableReceipt, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := readRegularFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		receipt, ok := decodeReceipt(data)
		if !ok {
			// A torn receipt is not committed and cannot establish a lineage link.
			continue
		}
		receipts[receipt.Snapshot.Digest] = append(receipts[receipt.Snapshot.Digest], receipt)
	}
	return receipts, nil
}

func (s OpenSpec) referencedBatches(snapshot applyprogress.Snapshot) (map[string]applyprogress.Batch, error) {
	batches := make(map[string]applyprogress.Batch, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		data, err := readAuthoritativeBatch(filepath.Join(s.Root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return nil, err
		}
		batch, err := applyprogress.DecodeCanonicalBatch(data)
		if err != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return nil, ErrMissingBatch
		}
		batches[batch.BatchID] = batch
	}
	if err := applyprogress.ValidateEvidenceCoverage(snapshot, batches); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return batches, nil
}

func (s OpenSpec) current() (*applyprogress.Snapshot, []byte, error) {
	data, err := readRegularFile(filepath.Join(s.Root, "apply-progress.md"))
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(data)
	if err != nil {
		return nil, data, err
	}
	return &snapshot, data, nil
}

// RequestIDUsed reports whether an immutable request receipt already occupies id.
// Checkpoint capacity paths create no receipt, so they use this narrow identity
// query to preserve request-ID conflict precedence without publishing anything.
func (s OpenSpec) RequestIDUsed(id string) bool {
	if !applyprogress.ValidID(id) {
		return false
	}
	_, err := readRegularFile(filepath.Join(s.Root, ".apply-progress-receipts", id+".json"))
	return err == nil
}

// HasReceipt identifies the exact interrupted low-level publication that a
// checkpoint may safely replan and replay. It intentionally binds both the
// canonical snapshot and batches through the same payload digest as Advance.
func (s OpenSpec) HasReceipt(request AdvanceRequest) bool {
	if !applyprogress.ValidID(request.RequestID) {
		return false
	}
	snapshot, data, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		return false
	}
	request.Snapshot = snapshot
	payload, err := payloadDigest(data, request)
	if err != nil {
		return false
	}
	_, found, err := s.checkReceipt(request.RequestID, payload)
	return err == nil && found
}

func (s OpenSpec) checkReceipt(id, payload string) (immutableReceipt, bool, error) {
	data, err := readRegularFile(filepath.Join(s.Root, ".apply-progress-receipts", id+".json"))
	if os.IsNotExist(err) {
		return immutableReceipt{}, false, nil
	}
	receipt, ok := decodeReceipt(data)
	if err != nil || !ok {
		// A torn receipt has not committed an idempotency decision and can be repaired.
		return immutableReceipt{}, false, nil
	}
	if receipt.Payload != payload {
		return immutableReceipt{}, false, ErrRequestConflict
	}
	return receipt, true, nil
}

type immutableReceipt struct {
	Payload  string                 `json:"payload"`
	Snapshot applyprogress.Snapshot `json:"snapshot"`
}

func decodeReceipt(data []byte) (immutableReceipt, bool) {
	var receipt immutableReceipt
	if json.Unmarshal(data, &receipt) != nil || !applyprogress.ValidDigest(receipt.Payload) || applyprogress.VerifySnapshot(receipt.Snapshot) != nil {
		return immutableReceipt{}, false
	}
	canonical, err := json.Marshal(receipt)
	return receipt, err == nil && bytes.Equal(data, canonical)
}

func receiptPayload(data []byte) (string, bool) {
	receipt, ok := decodeReceipt(data)
	return receipt.Payload, ok
}

func (s OpenSpec) RestoreLegacy(data []byte) error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return s.restoreLegacy(data)
}

func (s OpenSpec) restoreLegacy(data []byte) error {
	file, err := os.CreateTemp(s.Root, ".apply-progress-restore-")
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
	if err := os.Rename(name, filepath.Join(s.Root, "apply-progress.md")); err != nil {
		return err
	}
	return s.syncDir(s.Root)
}

func (s OpenSpec) publish(data []byte) error {
	target := filepath.Join(s.Root, "apply-progress.md")
	staged := stagingPath(target, data)
	if err := validateExistingPathComponents(target); err != nil {
		return err
	}
	if err := validateExistingPathComponents(staged); err != nil {
		return err
	}
	info, err := os.Lstat(staged)
	switch {
	case os.IsNotExist(err):
		file, createErr := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			return createErr
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
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%w: unsafe staging entry", ErrPublicationInterrupted)
	default:
		stagedData, readErr := os.ReadFile(staged)
		if readErr != nil || !bytes.Equal(stagedData, data) {
			return fmt.Errorf("%w: changed staging entry", ErrPublicationInterrupted)
		}
	}
	if s.BeforeRename != nil {
		return s.BeforeRename()
	}
	if err := os.Rename(staged, target); err != nil {
		return err
	}
	return s.syncDir(s.Root)
}

// LegacyAuthority returns the exact local legacy source and tasks that an
// OpenSpec migration would consume. v2 and absent progress are not legacy input.
func (s OpenSpec) LegacyAuthority(AdvanceRequest) (LegacyAuthority, error) {
	progress, err := readRegularFile(filepath.Join(s.Root, "apply-progress.md"))
	if os.IsNotExist(err) || isV2(progress) {
		return LegacyAuthority{}, nil
	}
	if err != nil {
		return LegacyAuthority{}, err
	}
	tasks, err := readRegularFile(filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return LegacyAuthority{}, err
	}
	return LegacyAuthority{Tasks: tasks, Progress: progress, Found: true}, nil
}

// UpgradeLegacy converts only an unchanged, unambiguous legacy source during a mutation.
func (s OpenSpec) UpgradeLegacy(request AdvanceRequest) (AdvanceResult, bool, error) {
	legacy, err := readRegularFile(filepath.Join(s.Root, "apply-progress.md"))
	if os.IsNotExist(err) || isV2(legacy) {
		return AdvanceResult{}, false, nil
	}
	if err != nil {
		return AdvanceResult{}, false, err
	}
	if err := s.ValidateLegacyRequest(request, legacy); err != nil {
		return AdvanceResult{}, true, err
	}
	// UpgradeLegacy owns the only automatic OpenSpec migration path. Bind the
	// source after comparing the exact bytes that will be replaced, rather than
	// accepting a caller-provided digest from compatibility JSON.
	request.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(legacy)
	result, err := s.AdvanceLegacy(request, legacy)
	return result, true, err
}

// ValidateLegacyRequest proves that request is the sole v2 conversion authorized
// by legacy. It performs no mutation, so Hybrid can persist its operation receipt
// before either backend replaces the legacy source.
func (s OpenSpec) ValidateLegacyRequest(request AdvanceRequest, legacy []byte) error {
	converted, status, err := legacyConversion(legacy, filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLegacyMigration, err)
	}
	if request.Snapshot.TaskManifestSHA256 != converted.TaskManifestSHA256 {
		return fmt.Errorf("%w: initial request does not match legacy source or imported evidence", ErrLegacyMigration)
	}
	if request.ExpectedGeneration != 0 || request.ExpectedRevision != 0 || request.ExpectedDigest != "" || request.Snapshot.Generation != 1 || request.Snapshot.Revision != 1 || request.Snapshot.PreviousDigest != "" || request.Snapshot.Status != status || !sameLegacyCoverage(converted, request.Snapshot) || !hasImportedLegacyEvidence(converted, request) {
		return fmt.Errorf("%w: initial request does not match legacy source or imported evidence", ErrLegacyMigration)
	}
	return nil
}

func legacyConversion(progress []byte, tasksPath string) (applyprogress.LegacyConversion, applyprogress.Status, error) {
	tasksData, err := readRegularFile(tasksPath)
	if err != nil {
		return applyprogress.LegacyConversion{}, "", err
	}
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(string(tasksData))
	if err != nil || len(parsed.Tasks) == 0 {
		return applyprogress.LegacyConversion{}, "", ErrLegacyMigration
	}
	legacyTasks := append([]applyprogress.Task(nil), parsed.Tasks...)
	for i := range legacyTasks {
		if legacyTasks[i].Path == "" {
			legacyTasks[i].Path = legacyTasks[i].ID
		}
	}
	legacy := applyprogress.LegacyProgress{Tasks: legacyTasks, Completed: parsed.CompletedIDs}
	complete, partial := false, false
	for _, line := range strings.Split(string(progress), "\n") {
		switch strings.TrimSpace(line) {
		case "status: complete":
			complete = true
		case "status: partial":
			partial = true
		default:
			if strings.HasPrefix(strings.TrimSpace(line), "status:") {
				return applyprogress.LegacyConversion{}, "", ErrLegacyMigration
			}
		}
	}
	if complete && partial || len(legacy.Tasks) == 0 {
		return applyprogress.LegacyConversion{}, "", ErrLegacyMigration
	}
	converted, err := applyprogress.ConvertLegacy(legacy)
	if err != nil {
		return applyprogress.LegacyConversion{}, "", err
	}
	if complete && len(converted.Completed) != len(converted.Tasks) {
		return applyprogress.LegacyConversion{}, "", ErrLegacyMigration
	}
	// Completion is derived from the authoritative checkbox conversion and its
	// imported coverage, not from an optional historical status marker. A partial
	// marker alongside full coverage is normalized to complete so UpgradeLegacy
	// cannot publish an internally inconsistent partial snapshot.
	status := applyprogress.StatusPartial
	if len(converted.Completed) == len(converted.Tasks) {
		status = applyprogress.StatusComplete
	}
	return converted, status, nil
}

func sameLegacyCoverage(legacy applyprogress.LegacyConversion, snapshot applyprogress.Snapshot) bool {
	completed := map[string]bool{}
	for _, id := range legacy.Completed {
		completed[id] = true
	}
	var want []string
	for _, task := range legacy.Tasks {
		if completed[task.ID] {
			want = append(want, task.ID)
		}
	}
	if len(want) != len(snapshot.Coverage) {
		return false
	}
	for i, id := range want {
		if snapshot.Coverage[i].TaskID != id {
			return false
		}
	}
	return true
}

func hasImportedLegacyEvidence(legacy applyprogress.LegacyConversion, request AdvanceRequest) bool {
	batches := make(map[string]applyprogress.Batch, len(request.Batches))
	for _, batch := range request.Batches {
		batches[batch.BatchID] = batch
	}
	for _, completed := range legacy.Completed {
		found := false
		for _, coverage := range request.Snapshot.Coverage {
			if coverage.TaskID != completed {
				continue
			}
			batch, ok := batches[coverage.BatchID]
			if !ok {
				return false
			}
			for _, entry := range batch.Entries {
				if entry.EntryID == coverage.EntryID && entry.Kind == applyprogress.EvidenceImported && containsID(entry.CompletesTaskIDs, completed) {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func isV2(data []byte) bool {
	return bytes.HasPrefix(data, []byte(`{"schema":"jarvis.sdd-apply-progress/v2"`))
}

// Archive validates the authoritative snapshot before atomically moving the
// complete change topology, including immutable evidence and request receipts.
func (s OpenSpec) Archive(destination string) error {
	return s.archive(destination, nil)
}

// ArchiveWithLifecycleValidation rechecks lifecycle readiness under lock before moving the topology.
func (s OpenSpec) ArchiveWithLifecycleValidation(destination string, validate func() error) error {
	return s.archive(destination, validate)
}

func (s OpenSpec) archive(destination string, validate func() error) error {
	root, err := filepath.Abs(filepath.Clean(s.Root))
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return err
	}
	s.Root = root
	if root == destination {
		return ErrInvalidChangeRoot
	}
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := ensureRegularDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := validateExistingPathComponents(destination); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return ErrInvalidChangeRoot
	} else if !os.IsNotExist(err) {
		return err
	}

	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := s.validateStaging(nil); err != nil {
		return err
	}
	if s.BeforeArchiveValidate != nil {
		if err := s.BeforeArchiveValidate(); err != nil {
			return err
		}
	}
	// The hook models an attacker or concurrent writer between status and archive.
	// Re-authenticate every source and destination component while holding the
	// archive lock immediately before the irreversible directory rename.
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := validateRegularDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := validateExistingPathComponents(destination); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return ErrInvalidChangeRoot
	} else if !os.IsNotExist(err) {
		return err
	}
	snapshot, data, err := s.current()
	if err != nil {
		return err
	}
	if snapshot == nil {
		return ErrMissingBatch
	}
	if snapshot.Status != applyprogress.StatusComplete {
		return ErrConflict
	}
	if err := s.validateArchive(*snapshot, data); err != nil {
		return err
	}
	if err := s.validateArchivePublicationTopology(*snapshot); err != nil {
		return err
	}
	if err := s.validateArchiveLifecycleArtifacts(); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(); err != nil {
			return err
		}
	}
	if err := os.Rename(s.Root, destination); err != nil {
		return err
	}
	return s.syncDir(filepath.Dir(destination))
}

func (s OpenSpec) validateArchiveLifecycleArtifacts() error {
	for _, name := range []string{"proposal.md", "design.md", "tasks.md", "verify-report.md"} {
		data, err := readRegularFile(filepath.Join(s.Root, name))
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			return ErrConflict
		}
		if name == "verify-report.md" {
			readiness := applyprogress.InspectVerifyReport(string(data))
			if readiness.Code != "" {
				return fmt.Errorf("%w: %w", ErrConflict, &applyprogress.VerifyReportError{Code: readiness.Code})
			}
			if !readiness.Ready {
				return ErrConflict
			}
		}
	}
	return s.validateArchiveSpecs(filepath.Join(s.Root, "specs"))
}

func (s OpenSpec) validateArchiveSpecs(root string) error {
	if err := validateRegularDirectory(root); err != nil {
		return ErrConflict
	}
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.Type()&os.ModeSymlink != 0 {
			return ErrConflict
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() != "spec.md" {
			return nil
		}
		data, err := readRegularFile(path)
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			return ErrConflict
		}
		found = true
		return nil
	})
	if err != nil || !found {
		return ErrConflict
	}
	return nil
}

func (s OpenSpec) validateArchive(snapshot applyprogress.Snapshot, data []byte) error {
	taskData, err := readRegularFile(filepath.Join(s.Root, "tasks.md"))
	if err != nil {
		return err
	}
	content := string(taskData)
	parsed, strictErr := applyprogress.ParseTasksMarkdown(content)
	tasks := parsed.Tasks
	allDone := parsed.AllDone
	manifest := ""
	if strictErr == nil && len(tasks) > 0 {
		_, manifest, strictErr = applyprogress.TaskManifest(tasks)
	}
	if strictErr != nil || len(tasks) == 0 || manifest != snapshot.TaskManifestSHA256 {
		legacyParsed, legacyErr := applyprogress.ParseLegacyTasksMarkdown(content)
		if legacyErr != nil || len(legacyParsed.Tasks) == 0 {
			return ErrInvalidChangeRoot
		}
		legacyTasks, legacyManifest, legacyErr := applyprogress.LegacyTaskManifest(legacyParsed.Tasks)
		if legacyErr != nil || legacyManifest != snapshot.TaskManifestSHA256 {
			return &applyprogress.ValidationError{Code: applyprogress.CodeTaskManifestMismatch, Detail: "task_manifest_sha256"}
		}
		tasks, allDone = legacyTasks, legacyParsed.AllDone
	}
	if !allDone {
		return ErrConflict
	}
	batches := make(map[string][]byte, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, err := readAuthoritativeBatch(filepath.Join(s.Root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return err
		}
		batches[ref.BatchID] = batch
	}
	return applyprogress.ValidateProgress(data, tasks, batches)
}

// validateArchivePublicationTopology accepts only the same authoritative receipt
// shape that archived hybrid status accepts. A torn JSON receipt or any retained
// staging file makes the topology ambiguous and must prevent the directory rename.
func (s OpenSpec) validateArchivePublicationTopology(expected applyprogress.Snapshot) error {
	if err := s.validateReceiptLineage(&expected, true); err != nil {
		return err
	}
	receipts, err := readAuthoritativeDir(filepath.Join(s.Root, ".apply-progress-receipts"))
	if err != nil || len(receipts) == 0 {
		return ErrConflict
	}
	canonicalReceipts, headReceipt := 0, false
	for _, entry := range receipts {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".apply-progress-stage-") {
			return ErrConflict
		}
		// Replaced corrupt bytes are recovery debris, never receipt authority.
		if strings.HasPrefix(entry.Name(), ".apply-progress-corrupt-") && filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			return ErrConflict
		}
		data, readErr := readRegularFile(filepath.Join(s.Root, ".apply-progress-receipts", entry.Name()))
		receipt, ok := decodeReceipt(data)
		if readErr != nil || !ok || receipt.Snapshot.Project != expected.Project || receipt.Snapshot.Change != expected.Change {
			return ErrConflict
		}
		canonicalReceipts++
		headReceipt = headReceipt || receipt.Snapshot.Digest == expected.Digest
	}
	if canonicalReceipts == 0 || !headReceipt {
		return ErrConflict
	}

	evidence, err := readAuthoritativeDir(filepath.Join(s.Root, "apply-evidence"))
	if err != nil {
		return ErrMissingBatch
	}
	for _, entry := range evidence {
		if entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".apply-progress-stage-") {
			return ErrConflict
		}
	}
	return nil
}

// validateReceiptLineage accepts receipts only when every canonical snapshot is
// the head or a uniquely validated ancestor of that head. A write-ahead
// successor, fork, orphan, torn receipt, or staged receipt blocks normal reads
// and archive. The exact low-level Advance request remains the recovery path.
func (s OpenSpec) validateReceiptLineage(head *applyprogress.Snapshot, requireReceipt bool) error {
	dir := filepath.Join(s.Root, ".apply-progress-receipts")
	entries, err := readAuthoritativeDir(dir)
	if os.IsNotExist(err) {
		if requireReceipt {
			return ErrConflict
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: receipt directory", ErrPublicationInterrupted)
	}
	receipts := make(map[string][]immutableReceipt)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".apply-progress-stage-") {
			return fmt.Errorf("%w: unresolved receipt staging", ErrPublicationInterrupted)
		}
		if strings.HasPrefix(entry.Name(), ".apply-progress-corrupt-") && filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("%w: invalid receipt entry", ErrPublicationInterrupted)
		}
		data, readErr := readRegularFile(filepath.Join(dir, entry.Name()))
		receipt, ok := decodeReceipt(data)
		if readErr != nil || !ok || (head != nil && (receipt.Snapshot.Project != head.Project || receipt.Snapshot.Change != head.Change)) {
			return fmt.Errorf("%w: invalid receipt", ErrPublicationInterrupted)
		}
		receipts[receipt.Snapshot.Digest] = append(receipts[receipt.Snapshot.Digest], receipt)
		count++
	}
	if count == 0 {
		if requireReceipt {
			return ErrConflict
		}
		return nil
	}
	if head == nil {
		return fmt.Errorf("%w: receipt without current head", ErrPublicationInterrupted)
	}

	seen := map[string]bool{head.Digest: true}
	candidate := *head
	for candidate.PreviousDigest != "" {
		parents := receipts[candidate.PreviousDigest]
		if len(parents) == 0 {
			// Receipts were introduced after older v2 snapshots existed. A missing
			// historical receipt is therefore not itself unresolved; every receipt
			// we did find must still be on the proven head path below.
			break
		}
		batches, batchErr := s.referencedBatches(candidate)
		if batchErr != nil {
			return fmt.Errorf("%w: receipt successor evidence", ErrPublicationInterrupted)
		}
		var parent applyprogress.Snapshot
		found := false
		for _, receipt := range parents {
			if applyprogress.ValidateSuccessor(receipt.Snapshot, candidate, batches) != nil {
				continue
			}
			if found && !sameSnapshot(parent, receipt.Snapshot) {
				return fmt.Errorf("%w: receipt fork", ErrPublicationInterrupted)
			}
			parent, found = receipt.Snapshot, true
		}
		if !found || seen[parent.Digest] {
			return fmt.Errorf("%w: receipt ancestor", ErrPublicationInterrupted)
		}
		seen[parent.Digest] = true
		candidate = parent
	}
	for digest := range receipts {
		if !seen[digest] {
			return fmt.Errorf("%w: pending successor, fork, or orphan", ErrPublicationInterrupted)
		}
	}
	return nil
}

type immutableStage struct {
	path string
	data []byte
}

// expectedStaging describes every content-bound stage that this exact request may
// recover. A stage is never anonymous: its deterministic name commits to both the
// destination and the bytes it may publish.
func (s OpenSpec) expectedStaging(request AdvanceRequest, snapshot applyprogress.Snapshot, snapshotData []byte, payload string) ([]immutableStage, error) {
	stages := make([]immutableStage, 0, len(request.Batches)+2)
	for _, batch := range request.Batches {
		sealed, data, err := applyprogress.SealBatch(batch)
		if err != nil {
			return nil, err
		}
		stages = append(stages, immutableStage{path: filepath.Join(s.Root, "apply-evidence", sealed.BatchID+".json"), data: data})
	}
	receiptData, err := json.Marshal(immutableReceipt{Payload: payload, Snapshot: snapshot})
	if err != nil {
		return nil, err
	}
	stages = append(stages,
		immutableStage{path: filepath.Join(s.Root, ".apply-progress-receipts", request.RequestID+".json"), data: receiptData},
		immutableStage{path: filepath.Join(s.Root, "apply-progress.md"), data: snapshotData},
	)
	return stages, nil
}

// validateStaging keeps malformed, anonymous, changed, and symlinked residue
// fail-closed. Exact retries may carry only the deterministic stage for the same
// target bytes; they finalize it through writeImmutable after this validation.
func (s OpenSpec) validateStaging(expected []immutableStage) error {
	known := make(map[string][]byte, len(expected))
	for _, stage := range expected {
		known[stagingPath(stage.path, stage.data)] = stage.data
	}
	paths := []string{filepath.Join(s.Root, "apply-evidence"), filepath.Join(s.Root, ".apply-progress-receipts"), s.Root}
	for _, dir := range paths {
		entries, err := readAuthoritativeDir(dir)
		if os.IsNotExist(err) && dir != s.Root {
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: staging directory", ErrPublicationInterrupted)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".apply-progress-stage-") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("%w: unsafe staging entry", ErrPublicationInterrupted)
			}
			data, err := os.ReadFile(path)
			if want, ok := known[path]; err != nil || !ok || !bytes.Equal(data, want) {
				return fmt.Errorf("%w: unbound staging entry", ErrPublicationInterrupted)
			}
		}
	}
	return nil
}

func (s OpenSpec) lockPath() string {
	return filepath.Join(filepath.Dir(s.Root), "."+filepath.Base(s.Root)+".apply-progress.lock")
}

func (s OpenSpec) lock() (func() error, error) {
	return filelock.Acquire(s.lockPath())
}

func (s OpenSpec) syncDir(path string) error {
	if s.SyncDir != nil {
		return s.SyncDir(path)
	}
	return syncDir(path)
}

// writeImmutable stages and fsyncs deterministic, content-bound bytes before
// publishing them under the caller's lock. Only an exact retry may reuse a stage;
// changed or anonymous residue remains observable and fail-closed.
func writeImmutable(path string, data []byte, sync func(string) error, beforePublish func() error) error {
	dir := filepath.Dir(path)
	if err := ensureRegularDirectory(dir); err != nil {
		return err
	}
	if err := validateExistingPathComponents(path); err != nil {
		return err
	}
	staged := stagingPath(path, data)
	if err := validateExistingPathComponents(staged); err != nil {
		return err
	}
	info, err := os.Lstat(staged)
	switch {
	case os.IsNotExist(err):
		file, createErr := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			return createErr
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
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%w: unsafe staging entry", ErrPublicationInterrupted)
	default:
		stagedData, readErr := os.ReadFile(staged)
		if readErr != nil || !bytes.Equal(stagedData, data) {
			return fmt.Errorf("%w: changed staging entry", ErrPublicationInterrupted)
		}
	}
	if beforePublish != nil {
		if err := beforePublish(); err != nil {
			return err
		}
	}

	existing, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if err := os.Rename(staged, path); err != nil {
			return err
		}
	case err != nil:
		return err
	case bytes.Equal(existing, data):
		if err := os.Remove(staged); err != nil && !os.IsNotExist(err) {
			return err
		}
	case canonicalImmutable(path, existing):
		return immutableConflict(path)
	default:
		// Preserve corrupt bytes first so Windows can publish without overwriting an
		// existing path and a crash never turns debris into authoritative evidence.
		debris, err := os.CreateTemp(dir, ".apply-progress-corrupt-")
		if err != nil {
			return err
		}
		debrisPath := debris.Name()
		if err := debris.Close(); err != nil {
			return err
		}
		if err := os.Remove(debrisPath); err != nil {
			return err
		}
		if err := os.Rename(path, debrisPath); err != nil {
			return err
		}
		if err := os.Rename(staged, path); err != nil {
			return err
		}
	}
	return sync(dir)
}

func recoverStagedImmutable(path string, data []byte, sync func(string) error, beforePublish func() error) error {
	if err := validateExistingPathComponents(stagingPath(path, data)); err != nil {
		return err
	}
	if _, err := os.Lstat(stagingPath(path, data)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return writeImmutable(path, data, sync, beforePublish)
}

func stagingPath(path string, data []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(filepath.Base(path)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	return filepath.Join(filepath.Dir(path), ".apply-progress-stage-"+hex.EncodeToString(hash.Sum(nil)))
}

func canonicalImmutable(path string, data []byte) bool {
	if filepath.Base(filepath.Dir(path)) == "apply-evidence" {
		_, err := applyprogress.DecodeCanonicalBatch(data)
		return err == nil
	}
	_, ok := receiptPayload(data)
	return ok
}

func immutableConflict(path string) error {
	if filepath.Base(filepath.Dir(path)) == ".apply-progress-receipts" {
		return ErrRequestConflict
	}
	return ErrBatchCollision
}

func validateAdvance(current *applyprogress.Snapshot, request AdvanceRequest, snapshot applyprogress.Snapshot, batches map[string]applyprogress.Batch) error {
	if request.ExpectedRevision == ^uint64(0) {
		return ErrConflict
	}
	expectedGeneration := request.ExpectedGeneration
	if expectedGeneration == 0 {
		expectedGeneration = 1
	}
	// Generation identifies the epoch; Revision is the checkpoint CAS inside it.
	if snapshot.Generation != expectedGeneration || snapshot.Revision != request.ExpectedRevision+1 || snapshot.PreviousDigest != request.ExpectedDigest {
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
	if err := applyprogress.ValidateSuccessor(*current, snapshot, batches); err != nil {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return nil
}

func validateImportedOpenSpecEvidence(request AdvanceRequest, current *applyprogress.Snapshot, raw, legacy []byte) error {
	if !applyprogress.ContainsImportedEvidence(request.Batches) && request.LegacySourceSHA256 == "" {
		return nil
	}
	if current != nil || legacy == nil || !bytes.Equal(raw, legacy) || isV2(raw) || !applyprogress.ValidDigest(request.LegacySourceSHA256) || applyprogress.LegacySourceSHA256(raw) != request.LegacySourceSHA256 {
		return fmt.Errorf("%w: imported evidence requires the exact initial legacy source", ErrLegacyMigration)
	}
	return nil
}

func payloadDigest(snapshot []byte, request AdvanceRequest) (string, error) {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d:%d:%s:", request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest)
	if request.LegacySourceSHA256 != "" {
		fmt.Fprintf(hash, "legacy_source_sha256=%s:", request.LegacySourceSHA256)
	}
	_, _ = hash.Write(snapshot)
	for _, batch := range request.Batches {
		_, data, err := applyprogress.SealBatch(batch)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncDir(path string) error {
	if runtime.GOOS == "windows" {
		// Go cannot reliably fsync directory handles on Windows. Rename is still
		// atomic there; callers have already fsynced the staged regular file.
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

package sddprogress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress/filelock"
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
	return left.Schema == right.Schema && left.Project == right.Project && left.Change == right.Change && left.Generation == right.Generation && left.Revision == right.Revision && left.PreviousDigest == right.PreviousDigest && left.TaskManifestSHA256 == right.TaskManifestSHA256 && left.Status == right.Status && left.StreamSHA256 == right.StreamSHA256 && left.NextEntryIndex == right.NextEntryIndex && left.NextEntryID == right.NextEntryID && left.Digest != "" && left.Digest == right.Digest && slices.Equal(left.Batches, right.Batches) && slices.Equal(left.Coverage, right.Coverage)
}

type advanceBackend interface {
	Advance(AdvanceRequest) (AdvanceResult, error)
}

type currentBackend interface {
	Current(AdvanceRequest) (AdvanceResult, error)
}

type snapshotBackend interface {
	CurrentSnapshot(AdvanceRequest) (*applyprogress.Snapshot, error)
}

type openSpecSnapshotBackend interface {
	Current() (*applyprogress.Snapshot, error)
}

type legacyRequestValidator interface {
	ValidateLegacyRequest(AdvanceRequest, []byte) error
}

type legacyAdvanceBackend interface {
	AdvanceLegacy(AdvanceRequest, []byte) (AdvanceResult, error)
}

// LegacyAuthority is the exact legacy input that a backend would migrate.
// Hybrid migration accepts no read precedence: both authorities must present
// byte-identical tasks and progress before either side publishes v2.
type LegacyAuthority struct {
	Tasks    []byte
	Progress []byte
	Found    bool
}

type legacyAuthorityReader interface {
	LegacyAuthority(AdvanceRequest) (LegacyAuthority, error)
}

type manifestValidator interface {
	ValidateRequestManifest(AdvanceRequest) error
}

type receiptOutcome string

const (
	receiptPending   receiptOutcome = "pending"
	receiptCommitted receiptOutcome = "committed"
	receiptFailed    receiptOutcome = "failed"
)

// receiptAcknowledgement is the immutable identity returned by one backend for
// the exact candidate that Hybrid asked it to commit. A committed outcome without
// this acknowledgement is a legacy receipt and requires safe replay.
type receiptAcknowledgement struct {
	Generation uint64 `json:"generation"`
	Revision   uint64 `json:"revision"`
	Digest     string `json:"digest"`
	Payload    string `json:"payload"`
}

type hybridReceipt struct {
	Payload     string                  `json:"payload"`
	OpenSpec    receiptOutcome          `json:"openspec"`
	Hive        receiptOutcome          `json:"hive"`
	OpenSpecAck *receiptAcknowledgement `json:"openspec_ack,omitempty"`
	HiveAck     *receiptAcknowledgement `json:"hive_ack,omitempty"`
}

// Hybrid uses a durable receipt to repair only the side that did not commit.
type Hybrid struct {
	Root                     string
	OpenSpec                 advanceBackend
	Hive                     advanceBackend
	HiveFirst                bool                                            // Test seam for either interrupted publication direction.
	publishOpenSpecSuccessor func(OpenSpec, OpenSpec) (AdvanceResult, error) // Test-only returned-result seam.
}

// successorPublisher is supplied by the Hive adapter in the follow-up CLI wiring.
// It owns remote target reservation and exact idempotent publication; no target
// coordinates are accepted from a caller.
type successorPublisher interface {
	PublishSuccessorGenesis(applyprogress.Snapshot) (applyprogress.Snapshot, error)
}

// PublishSuccessorGenesis derives the target solely from the two authenticated
// predecessor heads. A retry replays both immutable publications, never adopting
// an unrelated successor already occupying either target.
func (h Hybrid) PublishSuccessorGenesis() (applyprogress.Snapshot, error) {
	open, ok := h.OpenSpec.(OpenSpec)
	if !ok {
		return applyprogress.Snapshot{}, ErrBackendDiverged
	}
	hive, ok := h.Hive.(interface {
		snapshotBackend
		successorPublisher
	})
	if !ok {
		return applyprogress.Snapshot{}, ErrBackendDiverged
	}
	unlock, err := h.lock()
	if err != nil {
		return applyprogress.Snapshot{}, err
	}
	defer unlock()
	left, leftErr := open.InspectPublication()
	if leftErr != nil || left == nil || applyprogress.VerifySnapshot(*left) != nil || filepath.Clean(h.Root) != filepath.Clean(open.Root) {
		return applyprogress.Snapshot{}, ErrBackendDiverged
	}
	// The existing CLI adapter resolves CurrentSnapshot using these coordinates.
	// They come from the authenticated predecessor, not a caller-supplied target.
	right, rightErr := hive.CurrentSnapshot(AdvanceRequest{Snapshot: applyprogress.Snapshot{Project: left.Project, Change: left.Change}})
	if rightErr != nil || right == nil || applyprogress.VerifySnapshot(*right) != nil ||
		!sameSnapshot(*left, *right) || left.Status != applyprogress.StatusSuperseded || left.SealIntent == nil ||
		left.SealIntent.SuccessorProject != left.Project || !applyprogress.ValidID(left.SealIntent.SuccessorChange) {
		return applyprogress.Snapshot{}, ErrBackendDiverged
	}
	// OpenSpec validates the occupied target and the sealed task manifest before
	// publishing. Hive validates its own target against the same sealed identity.
	target := OpenSpec{Root: filepath.Join(filepath.Dir(open.Root), left.SealIntent.SuccessorChange)}
	publish := h.publishOpenSpecSuccessor
	if publish == nil {
		publish = OpenSpec.PublishSuccessorGenesis
	}
	result, err := publish(open, target)
	if err != nil {
		return applyprogress.Snapshot{}, err
	}
	// A backend's success claim is not authority. Authenticate the published
	// target and its pair with the agreed predecessor before mutating Hive.
	published, err := target.InspectPublication()
	if err != nil || published == nil || applyprogress.ValidateSuccessorGenesisPair(*left, *published) != nil ||
		result.Generation != published.Generation || result.Revision != published.Revision || result.Digest != published.Digest {
		return applyprogress.Snapshot{}, ErrBackendDiverged
	}
	genesis, err := hive.PublishSuccessorGenesis(*left)
	if err != nil {
		return applyprogress.Snapshot{}, err
	}
	if applyprogress.ValidateSuccessorGenesisPair(*right, genesis) != nil || !sameSnapshot(*published, genesis) {
		return applyprogress.Snapshot{}, ErrBackendDiverged
	}
	return *published, nil
}

// Current accepts only independently validated matching snapshots.
func (h Hybrid) Current(request AdvanceRequest) (*applyprogress.Snapshot, error) {
	open, ok := h.OpenSpec.(openSpecSnapshotBackend)
	if !ok {
		return nil, ErrBackendDiverged
	}
	hive, ok := h.Hive.(snapshotBackend)
	if !ok {
		return nil, ErrBackendDiverged
	}
	left, leftErr := open.Current()
	right, rightErr := hive.CurrentSnapshot(request)
	if leftErr != nil || rightErr != nil || (left == nil) != (right == nil) {
		return nil, ErrBackendDiverged
	}
	if left == nil {
		return nil, nil
	}
	if !sameSnapshot(*left, *right) {
		return nil, ErrBackendDiverged
	}
	return left, nil
}

// RequestIDUsed reports whether the durable hybrid receipt already reserves id.
// Capacity outcomes never create a receipt, so callers must surface a payload
// conflict rather than allow capacity or stale state to hide that reuse.
func (h Hybrid) RequestIDUsed(id string) bool {
	if !applyprogress.ValidID(id) {
		return false
	}
	_, err := readRegularFile(filepath.Join(h.Root, ".apply-progress-hybrid-receipts", id+".json"))
	return err == nil
}

// HasReceipt identifies an exact partially committed request for missing-side recovery.
func (h Hybrid) HasReceipt(request AdvanceRequest) bool {
	if !applyprogress.ValidID(request.RequestID) {
		return false
	}
	payload, payloadErr := payloadDigestFor(request)
	receipt, found, err := h.receipt(request.RequestID)
	if payloadErr != nil || err != nil || !found || receipt.Payload != payload {
		return false
	}
	missing := func(outcome receiptOutcome, ack *receiptAcknowledgement) bool {
		return outcome != receiptCommitted || !receiptAcknowledges(ack, request, payload)
	}
	return (receipt.OpenSpec == receiptCommitted && receiptAcknowledges(receipt.OpenSpecAck, request, payload) && missing(receipt.Hive, receipt.HiveAck)) ||
		(receipt.Hive == receiptCommitted && receiptAcknowledges(receipt.HiveAck, request, payload) && missing(receipt.OpenSpec, receipt.OpenSpecAck))
}

func (h Hybrid) UpgradeLegacy(request AdvanceRequest) (AdvanceResult, bool, error) {
	openAuthority, openOK := h.OpenSpec.(legacyAuthorityReader)
	hiveAuthority, hiveOK := h.Hive.(legacyAuthorityReader)
	validator, validateOK := h.OpenSpec.(legacyRequestValidator)
	// Non-OpenSpec test and adapter seams are ordinary v2 backends; they do not
	// participate in legacy migration detection.
	if !validateOK {
		return AdvanceResult{}, false, nil
	}
	if !openOK || !hiveOK {
		return AdvanceResult{}, false, ErrBackendDiverged
	}

	// A receipt without legacy provenance may belong to an ordinary v2 operation.
	// Check its payload exactly as received before consulting a legacy authority:
	// an exact v2 match must continue through Advance's idempotent receipt repair.
	// A mismatch remains a legacy recovery attempt, whose source binding may be
	// derived only from the protected remaining authority.
	if receipt, found, err := h.receipt(request.RequestID); err != nil {
		return AdvanceResult{}, false, err
	} else if found {
		payload, payloadErr := payloadDigestFor(request)
		if payloadErr != nil {
			return AdvanceResult{}, false, payloadErr
		}
		if !applyprogress.ValidDigest(request.LegacySourceSHA256) {
			if receipt.Payload == payload {
				return AdvanceResult{}, false, nil
			}
			right, rightErr := hiveAuthority.LegacyAuthority(request)
			if rightErr != nil || !right.Found {
				return AdvanceResult{}, false, ErrBackendDiverged
			}
			request.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(right.Progress)
			payload, payloadErr = payloadDigestFor(request)
			if payloadErr != nil {
				return AdvanceResult{}, false, payloadErr
			}
		}
		if receipt.Payload != payload {
			return AdvanceResult{}, false, ErrRequestConflict
		}
		if receiptComplete(receipt, request, payload) {
			return AdvanceResult{}, false, nil
		}
		result, advanceErr := h.advance(request, nil)
		return result, true, advanceErr
	}

	left, leftErr := openAuthority.LegacyAuthority(request)
	right, rightErr := hiveAuthority.LegacyAuthority(request)
	if leftErr != nil || rightErr != nil || left.Found != right.Found {
		return AdvanceResult{}, false, ErrBackendDiverged
	}
	if !left.Found {
		return AdvanceResult{}, false, nil
	}
	if !bytes.Equal(left.Tasks, right.Tasks) || !bytes.Equal(left.Progress, right.Progress) {
		return AdvanceResult{}, false, ErrBackendDiverged
	}
	// Both independently authoritative stores agreed on the precise source
	// bytes, so this migration—not caller JSON—owns the compatibility binding.
	request.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(left.Progress)
	if err := validator.ValidateLegacyRequest(request, left.Progress); err != nil {
		return AdvanceResult{}, true, err
	}
	result, err := h.advance(request, left.Progress)
	return result, true, err
}

func (h Hybrid) Advance(request AdvanceRequest) (AdvanceResult, error) {
	return h.advance(request, nil)
}

// advance writes a hybrid receipt before either backend mutation. legacy is passed
// only to the initial OpenSpec migration; recovery replays its committed OpenSpec
// receipt normally while applying the exact same request to the missing backend.
func (h Hybrid) advance(request AdvanceRequest, legacy []byte) (AdvanceResult, error) {
	if !applyprogress.ValidID(request.RequestID) {
		return AdvanceResult{}, fmt.Errorf("%w: request_id", applyprogress.ErrInvalidID)
	}
	if err := validateRegularDirectory(h.Root); err != nil {
		return AdvanceResult{}, ErrInvalidChangeRoot
	}
	// Concrete OpenSpec is the production mutation path. Test and adapter
	// backends may not own a tasks.md root, so they retain their bounded seam.
	if validator, ok := h.OpenSpec.(manifestValidator); ok {
		if err := validator.ValidateRequestManifest(request); err != nil {
			return AdvanceResult{}, err
		}
	}
	unlock, err := h.lock()
	if err != nil {
		return AdvanceResult{}, err
	}
	defer unlock()
	payload, err := payloadDigestFor(request)
	if err != nil {
		return AdvanceResult{}, err
	}
	receipt, found, err := h.receipt(request.RequestID)
	if err != nil {
		return AdvanceResult{}, err
	}
	if !found {
		receipt = hybridReceipt{Payload: payload, OpenSpec: receiptPending, Hive: receiptPending}
	} else if receipt.Payload != payload {
		return AdvanceResult{}, ErrRequestConflict
	} else if receiptHasWrongAcknowledgement(receipt, request, payload) {
		return AdvanceResult{}, ErrBackendDiverged
	} else if receiptComplete(receipt, request, payload) {
		return candidateAdvanceResult(request, payload), nil
	}
	sides := []struct {
		outcome *receiptOutcome
		ack     **receiptAcknowledgement
		backend advanceBackend
		legacy  bool
	}{{&receipt.OpenSpec, &receipt.OpenSpecAck, h.OpenSpec, legacy != nil}, {&receipt.Hive, &receipt.HiveAck, h.Hive, false}}
	if h.HiveFirst {
		sides[0], sides[1] = sides[1], sides[0]
	}
	if !found && legacy == nil {
		for _, side := range sides {
			result, err := advanceSide(side.backend, request)
			if errors.Is(err, ErrRequestConflict) {
				// A fresh request conflict must not reserve this request ID. The
				// original valid request remains replayable through each backend's
				// idempotent Advance contract.
				return result, fmt.Errorf("%w: %w", ErrBackendDiverged, err)
			}
			if err == nil && !resultAcknowledges(result, request, payload) {
				err = ErrBackendDiverged
			}
			if err == nil {
				*side.outcome = receiptCommitted
				*side.ack = acknowledgementFor(result, request, payload)
				continue
			}
			*side.outcome = receiptFailed
			*side.ack = nil
			if writeErr := h.writeReceipt(request.RequestID, receipt); writeErr != nil {
				return AdvanceResult{}, writeErr
			}
			return h.failedAdvance(request, receipt, result, err)
		}
		if err := h.writeReceipt(request.RequestID, receipt); err != nil {
			return AdvanceResult{}, err
		}
		return candidateAdvanceResult(request, payload), nil
	}
	if !found {
		// Legacy migration must leave recovery authority durable before OpenSpec
		// replaces its source bytes.
		if err := h.writeReceipt(request.RequestID, receipt); err != nil {
			return AdvanceResult{}, err
		}
	}
	for _, side := range sides {
		// A hybrid receipt is recovery metadata, never success authority. Replay the
		// exact request through each backend's existing idempotent Advance contract
		// before accepting a recorded committed side.
		result, err := advanceReceiptSide(side.backend, request, legacy, side.legacy)
		if err == nil && !resultAcknowledges(result, request, payload) {
			err = ErrBackendDiverged
		}
		if err == nil {
			*side.outcome = receiptCommitted
			*side.ack = acknowledgementFor(result, request, payload)
		} else {
			*side.outcome = receiptFailed
			*side.ack = nil
		}
		if writeErr := h.writeReceipt(request.RequestID, receipt); writeErr != nil {
			return AdvanceResult{}, writeErr
		}
		if err != nil {
			return h.failedAdvance(request, receipt, result, err)
		}
	}
	// Exact per-side acknowledgements are durable success authority. Do not reread
	// Current here: a peer successor or a transient backend lock after both commits
	// cannot retroactively invalidate this candidate's completed publication.
	if !receiptComplete(receipt, request, payload) {
		return AdvanceResult{}, ErrBackendDiverged
	}
	return candidateAdvanceResult(request, payload), nil
}

func candidateAdvanceResult(request AdvanceRequest, payload string) AdvanceResult {
	return AdvanceResult{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, PayloadSHA256: payload}
}

func acknowledgementFor(result AdvanceResult, request AdvanceRequest, payload string) *receiptAcknowledgement {
	if resultAcknowledgesByRequest(result) {
		return &receiptAcknowledgement{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, Payload: payload}
	}
	if result.PayloadSHA256 != "" {
		payload = result.PayloadSHA256
	}
	return &receiptAcknowledgement{Generation: result.Generation, Revision: result.Revision, Digest: result.Digest, Payload: payload}
}

func resultAcknowledges(result AdvanceResult, request AdvanceRequest, payload string) bool {
	// Compatibility-only adapters can acknowledge a successful Advance only by the
	// exact request they accepted and return an empty result. Bind that successful
	// call to its sealed candidate identity. Any populated acknowledgement remains
	// an exact claim and must agree, so wrong coordinates, digest, or payload cannot
	// be normalized away.
	return resultAcknowledgesByRequest(result) ||
		(result.Generation == request.Snapshot.Generation && result.Revision == request.Snapshot.Revision && result.Digest == request.Snapshot.Digest && (result.PayloadSHA256 == "" || result.PayloadSHA256 == payload))
}

func resultAcknowledgesByRequest(result AdvanceResult) bool {
	return result.Generation == 0 && result.Revision == 0 && result.Digest == "" && result.PayloadSHA256 == ""
}

func receiptAcknowledges(ack *receiptAcknowledgement, request AdvanceRequest, payload string) bool {
	return ack != nil && ack.Generation == request.Snapshot.Generation && ack.Revision == request.Snapshot.Revision && ack.Digest == request.Snapshot.Digest && ack.Payload == payload
}

func receiptComplete(receipt hybridReceipt, request AdvanceRequest, payload string) bool {
	return receipt.OpenSpec == receiptCommitted && receipt.Hive == receiptCommitted && receiptAcknowledges(receipt.OpenSpecAck, request, payload) && receiptAcknowledges(receipt.HiveAck, request, payload)
}

func receiptHasWrongAcknowledgement(receipt hybridReceipt, request AdvanceRequest, payload string) bool {
	return (receipt.OpenSpecAck != nil && !receiptAcknowledges(receipt.OpenSpecAck, request, payload)) ||
		(receipt.HiveAck != nil && !receiptAcknowledges(receipt.HiveAck, request, payload))
}

func checkpointAdvanceResult(snapshot *applyprogress.Snapshot) AdvanceResult {
	if snapshot == nil {
		return AdvanceResult{}
	}
	return AdvanceResult{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest}
}

func advanceSide(backend advanceBackend, request AdvanceRequest) (AdvanceResult, error) {
	if backend == nil {
		return AdvanceResult{}, fmt.Errorf("missing hybrid backend")
	}
	return backend.Advance(request)
}

func advanceReceiptSide(backend advanceBackend, request AdvanceRequest, legacy []byte, useLegacy bool) (AdvanceResult, error) {
	if backend == nil {
		return AdvanceResult{}, fmt.Errorf("missing hybrid backend")
	}
	if !useLegacy {
		return backend.Advance(request)
	}
	legacyBackend, ok := backend.(legacyAdvanceBackend)
	if !ok {
		return AdvanceResult{}, ErrBackendDiverged
	}
	return legacyBackend.AdvanceLegacy(request, legacy)
}

func (h Hybrid) failedAdvance(request AdvanceRequest, receipt hybridReceipt, result AdvanceResult, err error) (AdvanceResult, error) {
	if errors.Is(err, ErrConflict) && result.Digest == "" {
		// A backend may return a bare stale error. Treat it as a normal conflict only
		// after both independently validated snapshots agree; never expose coordinates
		// from one unvalidated side.
		current, currentErr := h.Current(request)
		if currentErr != nil {
			return AdvanceResult{}, fmt.Errorf("%w: %w", ErrBackendDiverged, currentErr)
		}
		result = checkpointAdvanceResult(current)
		if receipt.OpenSpec != receiptCommitted && receipt.Hive != receiptCommitted {
			dir := filepath.Join(h.Root, ".apply-progress-hybrid-receipts")
			if removeErr := os.Remove(filepath.Join(dir, request.RequestID+".json")); removeErr != nil {
				return AdvanceResult{}, removeErr
			}
			if syncErr := syncDir(dir); syncErr != nil {
				return AdvanceResult{}, syncErr
			}
		}
		return result, err
	}
	return result, fmt.Errorf("%w: %w", ErrBackendDiverged, err)
}

func (h Hybrid) lock() (func() error, error) {
	// The lock coordinates both backends, so it must remain beside—not inside—the
	// change topology that OpenSpec archive atomically moves.
	return filelock.Acquire(filepath.Join(filepath.Dir(h.Root), "."+filepath.Base(h.Root)+".apply-progress-hybrid.lock"))
}

func payloadDigestFor(request AdvanceRequest) (string, error) {
	_, data, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		return "", err
	}
	return payloadDigest(data, request)
}

func (h Hybrid) receipt(id string) (hybridReceipt, bool, error) {
	data, err := readRegularFile(filepath.Join(h.Root, ".apply-progress-hybrid-receipts", id+".json"))
	if os.IsNotExist(err) {
		return hybridReceipt{}, false, nil
	}
	var receipt hybridReceipt
	if err != nil || json.Unmarshal(data, &receipt) != nil || !validReceipt(receipt) {
		return hybridReceipt{}, false, ErrRequestConflict
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal(data, canonical) {
		return hybridReceipt{}, false, ErrRequestConflict
	}
	return receipt, true, nil
}

func validReceipt(receipt hybridReceipt) bool {
	valid := func(outcome receiptOutcome) bool {
		return outcome == receiptPending || outcome == receiptCommitted || outcome == receiptFailed
	}
	validAcknowledgement := func(ack *receiptAcknowledgement) bool {
		return ack == nil || (applyprogress.ValidDigest(ack.Digest) && applyprogress.ValidDigest(ack.Payload))
	}
	if !applyprogress.ValidDigest(receipt.Payload) || !valid(receipt.OpenSpec) || !valid(receipt.Hive) || !validAcknowledgement(receipt.OpenSpecAck) || !validAcknowledgement(receipt.HiveAck) {
		return false
	}
	// Acknowledgements are meaningful only for a recorded committed side. Missing
	// acknowledgements remain valid solely for backward-compatible safe replay.
	return (receipt.OpenSpec == receiptCommitted || receipt.OpenSpecAck == nil) && (receipt.Hive == receiptCommitted || receipt.HiveAck == nil)
}

func (h Hybrid) writeReceipt(id string, receipt hybridReceipt) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Root, ".apply-progress-hybrid-receipts", id+".json")
	if err := ensureRegularDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := validateExistingPathComponents(path); err != nil {
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

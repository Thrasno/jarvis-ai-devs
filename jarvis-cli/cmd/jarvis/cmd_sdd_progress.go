package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress/filelock"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type progressAdvancer interface {
	Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error)
}

type legacyProgressUpgrader interface {
	UpgradeLegacy(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, bool, error)
}

type capacityOutput struct {
	Document       string `json:"document"`
	Runes          int    `json:"runes"`
	Limit          int    `json:"limit"`
	CurrentRunes   int    `json:"current_runes"`
	ProjectedRunes int    `json:"projected_runes"`
	CeilingRunes   int    `json:"ceiling_runes"`
}

// capacityWarningOutput is advisory preflight data. It tells callers to avoid
// new tiny streams and consolidate evidence before the hard ceiling.
type capacityWarningOutput struct {
	Document  string `json:"document"`
	Runes     int    `json:"runes"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	Threshold int    `json:"threshold"`
	Guidance  string `json:"guidance"`
}

type checkpointFrequencyOutput struct {
	References    int `json:"references"`
	Threshold     int `json:"threshold"`
	BatchRunes    int `json:"batch_runes"`
	MinimumRunes  int `json:"minimum_runes"`
	SnapshotRunes int `json:"snapshot_runes,omitempty"`
	SafeRunes     int `json:"safe_runes,omitempty"`
}

// progressAdvanceOutput is a machine-readable command contract. A returned nil error
// with outcome conflict is an intentional successful state query: no bytes were
// published and callers must inspect code/recovery before deciding to retry.
type progressAdvanceOutput struct {
	Outcome  string                    `json:"outcome"`
	Code     string                    `json:"code"`
	Detail   string                    `json:"detail,omitempty"`
	Capacity *capacityOutput           `json:"capacity,omitempty"`
	State    sddprogress.AdvanceResult `json:"state"`
	Recovery string                    `json:"recovery,omitempty"`
}

type checkpointInput struct {
	Project            string                        `json:"project"`
	Change             string                        `json:"change"`
	Tasks              []applyprogress.Task          `json:"tasks"`
	Base               *applyprogress.Snapshot       `json:"base"`
	ExpectedGeneration uint64                        `json:"expected_generation"`
	ExpectedRevision   uint64                        `json:"expected_revision"`
	ExpectedDigest     string                        `json:"expected_digest"`
	RequestID          string                        `json:"request_id"`
	BatchID            string                        `json:"batch_id"`
	Entries            []applyprogress.EvidenceEntry `json:"entries"`
	EntryIndex         int                           `json:"entry_index"`
	EntryID            string                        `json:"entry_id"`
	StreamSHA256       string                        `json:"stream_sha256,omitempty"`
}

type checkpointReceipt struct {
	RequestID     string `json:"request_id"`
	PayloadSHA256 string `json:"payload_sha256,omitempty"`
}

const (
	staleProgressRecovery       = "generation is a stable epoch; retry with the authoritative generation plus the current revision and digest to prepare the next revision"
	continuationUpgradeCommand  = "jarvis sdd progress upgrade-continuation"
	continuationUpgradeRecovery = "run `" + continuationUpgradeCommand + "` with the same request ID and request inputs"
)

// checkpointOutput is a machine-readable command contract. Planner capacity outcomes
// and conflicts are intentional no-publication responses; they use exit zero so an
// agent can continue from code/recovery without parsing stderr. All other outcomes
// return a non-nil error and therefore a non-zero CLI exit status.
type checkpointOutput struct {
	Outcome        string                     `json:"outcome"`
	Code           string                     `json:"code"`
	State          sddprogress.AdvanceResult  `json:"state"`
	Snapshot       *applyprogress.Snapshot    `json:"snapshot,omitempty"`
	Receipt        *checkpointReceipt         `json:"receipt,omitempty"`
	NextEntryIndex int                        `json:"next_entry_index,omitempty"`
	NextEntryID    string                     `json:"next_entry_id,omitempty"`
	StreamSHA256   string                     `json:"stream_sha256,omitempty"`
	EntryIndex     int                        `json:"entry_index,omitempty"`
	EntryID        string                     `json:"entry_id,omitempty"`
	Recovery       string                     `json:"recovery,omitempty"`
	Detail         string                     `json:"detail,omitempty"`
	Capacity       *capacityOutput            `json:"capacity,omitempty"`
	Frequency      *checkpointFrequencyOutput `json:"frequency,omitempty"`
	Warning        *capacityWarningOutput     `json:"warning,omitempty"`
}

// MarshalJSON treats the next-entry ID as cursor presence. Its index may validly
// be zero, so the wire contract must not rely on int's omitempty behavior.
func (output checkpointOutput) MarshalJSON() ([]byte, error) {
	type alias checkpointOutput
	var nextEntryIndex *int
	if output.NextEntryID != "" {
		nextEntryIndex = &output.NextEntryIndex
	}
	return json.Marshal(struct {
		alias
		NextEntryIndex *int `json:"next_entry_index,omitempty"`
	}{alias: alias(output), NextEntryIndex: nextEntryIndex})
}

type progressCheckpointStore interface {
	progressAdvancer
	Current() (*applyprogress.Snapshot, error)
}

type hiveCheckpointStore interface {
	CurrentCheckpoint(string, string) (*applyprogress.Snapshot, error)
}

// hiveCheckpointTasksStore is implemented only by the pure Hive adapter.
// OpenSpec and hybrid checkpoints retain their local tasks.md authority.
type hiveCheckpointTasksStore interface {
	FetchAuthoritativeTasks(string, string) (string, error)
}

// hiveCheckpointLegacyStore supplies one authoritative Hive artifact view for
// initial legacy migration. The marker is deliberately distinct from a missing
// v2 head so migration is never inferred from local files in pure Hive mode.
type hiveCheckpointLegacyStore interface {
	FetchAuthoritativeCheckpointArtifacts(string, string) (tasks, legacy string, found bool, err error)
}

type hybridCheckpointStore interface {
	Current(sddprogress.AdvanceRequest) (*applyprogress.Snapshot, error)
}

type checkpointReceiptStore interface {
	HasReceipt(sddprogress.AdvanceRequest) bool
}

// checkpointRequestIDStore exposes locally durable receipt identity without
// permitting the coordinator to inspect or reconstruct receipt payloads.
// A capacity response creates no receipt, so a previously used request ID
// necessarily represents a different payload and must retain conflict priority.
type checkpointRequestIDStore interface {
	RequestIDUsed(string) bool
}

// hiveCheckpointReceiptStore is implemented only by the pure Hive adapter. It
// checks a receipt's project/change binding without exposing stored responses.
type hiveCheckpointReceiptStore interface {
	ReceiptIdentity(project, change, requestID string) (bool, error)
}

func defaultOpenSpec(root string) progressAdvancer { return sddprogress.OpenSpec{Root: root} }

var newProgressOpenSpec = defaultOpenSpec

// newProgressHive is the legacy env-only factory used by focused unit seams.
// Product request handling uses newProgressHiveWithContext instead.
var newProgressHive = func() (progressAdvancer, error) {
	client, err := hiveclient.NewFromEnv()
	return hiveProgressAdvancer{client: client}, err
}

var newProgressHiveWithContext = func(ctx context.Context) (progressAdvancer, error) {
	client, err := hiveclient.NewFromEnv()
	return hiveProgressAdvancer{client: client, ctx: ctx}, err
}

type hiveProgressAdvancer struct {
	client *hiveclient.Client
	ctx    context.Context
}

func (h hiveProgressAdvancer) requestContext() context.Context {
	if h.ctx != nil {
		return h.ctx
	}
	return context.Background()
}

func (h hiveProgressAdvancer) Current(request sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	result, err := h.client.GetApplyProgress(h.requestContext(), request.Snapshot.Project, request.Snapshot.Change)
	return sddprogress.AdvanceResult{Generation: result.State.Generation, Revision: result.State.Revision, Digest: result.State.Digest, PayloadSHA256: result.Receipt.PayloadSHA256}, err
}

func (h hiveProgressAdvancer) CurrentCheckpoint(project, change string) (*applyprogress.Snapshot, error) {
	result, err := h.client.GetApplyProgress(h.requestContext(), project, change)
	var apiErr *hiveclient.ApplyProgressError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 && apiErr.Result.Code == "not_found" {
		return nil, nil
	}
	if err != nil || result.State.Snapshot.Digest == "" {
		return nil, err
	}
	return &result.State.Snapshot, nil
}

func (h hiveProgressAdvancer) CurrentSnapshot(request sddprogress.AdvanceRequest) (*applyprogress.Snapshot, error) {
	return h.CurrentCheckpoint(request.Snapshot.Project, request.Snapshot.Change)
}

func (h hiveProgressAdvancer) FetchAuthoritativeTasks(project, change string) (string, error) {
	tasks, _, _, err := h.FetchAuthoritativeCheckpointArtifacts(project, change)
	return tasks, err
}

func (h hiveProgressAdvancer) FetchAuthoritativeCheckpointArtifacts(project, change string) (tasks, legacy string, found bool, err error) {
	artifacts, err := h.client.FetchSDDArtifacts(h.requestContext(), project, change)
	if err != nil {
		return "", "", false, err
	}
	for _, artifact := range artifacts {
		switch artifact.Artifact {
		case "tasks":
			if tasks != "" {
				return "", "", false, errors.New("authoritative Hive tasks artifact is ambiguous")
			}
			tasks = artifact.Content
		case "apply-progress":
			if found {
				return "", "", false, fmt.Errorf("%w: authoritative Hive apply-progress artifact is ambiguous", sddprogress.ErrLegacyMigration)
			}
			legacy, found = artifact.Content, true
		}
	}
	if strings.TrimSpace(tasks) == "" {
		return "", "", false, errors.New("authoritative Hive tasks artifact is missing")
	}
	if !found || bytes.HasPrefix([]byte(legacy), []byte(`{"schema":"jarvis.sdd-apply-progress/v2"`)) {
		return tasks, "", false, nil
	}
	return tasks, legacy, true, nil
}

func (h hiveProgressAdvancer) LegacyAuthority(request sddprogress.AdvanceRequest) (sddprogress.LegacyAuthority, error) {
	tasks, legacy, found, err := h.FetchAuthoritativeCheckpointArtifacts(request.Snapshot.Project, request.Snapshot.Change)
	if err != nil {
		return sddprogress.LegacyAuthority{}, err
	}
	return sddprogress.LegacyAuthority{Tasks: []byte(tasks), Progress: []byte(legacy), Found: found}, nil
}

// ReceiptIdentity reports whether the pure Hive stream has already committed
// this request ID. A no-write planner result cannot be an exact committed
// payload, so an occupied ID must win over stale or capacity diagnostics; a
// normal candidate remains on Advance, where the daemon replays exact payloads.
func (h hiveProgressAdvancer) ReceiptIdentity(project, change, requestID string) (bool, error) {
	_, found, err := h.client.GetApplyProgressReceipt(h.requestContext(), project, change, requestID)
	return found, err
}

func (h hiveProgressAdvancer) Advance(request sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	result, err := h.client.AdvanceApplyProgress(h.requestContext(), hiveclient.ApplyProgressAdvanceRequest{
		Project: request.Snapshot.Project, Change: request.Snapshot.Change, RequestID: request.RequestID,
		ExpectedGeneration: request.ExpectedGeneration, ExpectedRevision: request.ExpectedRevision, ExpectedDigest: request.ExpectedDigest,
		LegacySourceSHA256: request.LegacySourceSHA256,
		Snapshot:           request.Snapshot, Batches: request.Batches,
	})
	state := sddprogress.AdvanceResult{Generation: result.State.Generation, Revision: result.State.Revision, Digest: result.State.Digest, PayloadSHA256: result.Receipt.PayloadSHA256}
	if result.Code == "stale" {
		return state, sddprogress.ErrConflict
	}
	if result.Code == "request_id_conflict" {
		return state, sddprogress.ErrRequestConflict
	}
	if err != nil {
		if result.Code == "capacity" && result.Capacity != nil {
			return state, &applyprogress.CapacityError{Document: result.Capacity.Document, Runes: result.Capacity.Runes, Limit: result.Capacity.Limit, CurrentRunes: result.Capacity.CurrentRunes, ProjectedRunes: result.Capacity.ProjectedRunes, CeilingRunes: result.Capacity.CeilingRunes}
		}
		return state, err
	}
	if result.Outcome != "committed" {
		return state, sddprogress.ErrBackendDiverged
	}
	return state, nil
}

type failedProgressAdvancer struct{ err error }

func (f failedProgressAdvancer) Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	return sddprogress.AdvanceResult{}, f.err
}

func configuredProgressStore(root string) progressAdvancer {
	contract, err := sddruntime.ResolveRuntimeStoreContract(sddruntime.StoreModeOpenSpec)
	if err != nil {
		return failedProgressAdvancer{err}
	}
	open := newProgressOpenSpec(root)
	if contract.Mode == sddruntime.StoreModeOpenSpec {
		return open
	}
	if contract.Mode == sddruntime.StoreModeNone {
		return failedProgressAdvancer{errors.New("SDD progress store is disabled")}
	}
	hive, err := newProgressHive()
	if err != nil {
		return failedProgressAdvancer{err}
	}
	if contract.Mode == sddruntime.StoreModeHive {
		return hive
	}
	return sddprogress.Hybrid{Root: root, OpenSpec: open, Hive: hive}
}

type progressStoreResolver func(context.Context, string, string, string) (progressAdvancer, error)

func init() { sddCmd.AddCommand(newBoundSddProgressCommand(resolveBoundProgressStore)) }

// newSddProgressCommand preserves the narrow store-only seam used by progress
// unit tests. Product wiring must use newBoundSddProgressCommand so every
// mutation resolves the request's authoritative binding first.
func newSddProgressCommand(open func(string) progressAdvancer) *cobra.Command {
	return newSddProgressCommandWithResolver(func(_ context.Context, root, _, _ string) (progressAdvancer, error) {
		return open(root), nil
	})
}

func newBoundSddProgressCommand(resolve progressStoreResolver) *cobra.Command {
	return newSddProgressCommandWithResolver(resolve)
}

func newSddProgressCommandWithResolver(resolve progressStoreResolver) *cobra.Command {
	var root, requestPath string
	advance := &cobra.Command{
		Use:           "advance",
		Short:         "Atomically advance bounded apply progress",
		Long:          "Atomically advance bounded apply progress. JSON conflict and capacity outcomes are handled responses and exit 0; inspect outcome, code, and recovery. Other outcomes exit non-zero.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := os.ReadFile(requestPath)
			if err != nil {
				return err
			}
			var request sddprogress.AdvanceRequest
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				return fmt.Errorf("decode advance request: %w", err)
			}
			if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				return errors.New("decode advance request: trailing JSON value")
			}
			store, err := resolve(cmd.Context(), root, request.Snapshot.Project, request.Snapshot.Change)
			if err != nil {
				return err
			}
			output, err := runSddProgressAdvance(store, root, request)
			if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(output); encodeErr != nil && err == nil {
				return encodeErr
			}
			return err
		},
	}
	advance.Flags().StringVar(&root, "root", "", "OpenSpec change root")
	advance.Flags().StringVar(&requestPath, "request", "", "canonical advance request JSON")
	// --root is needed only when the configured store writes OpenSpec (or hybrid).
	// Pure Hive checkpoints use the project/change identity in the request instead.
	_ = advance.MarkFlagRequired("request")

	checkpoint := &cobra.Command{
		Use: "checkpoint", Short: "Commit one bounded evidence prefix", Long: "Commit one bounded evidence prefix through the configured OpenSpec, Hive, or hybrid store. An unambiguous legacy Markdown artifact imports automatically on its first checkpoint. JSON continuation, conflict, and capacity outcomes are handled responses and exit 0; inspect outcome, code, and recovery. Other outcomes exit non-zero. Only the historical explicit-zero v2 continuation wire shape requires `upgrade-continuation`; an absent group starts a new unbound stream normally.", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := os.ReadFile(requestPath)
			if err != nil {
				return err
			}
			var request checkpointInput
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				return fmt.Errorf("decode checkpoint request: %w", err)
			}
			if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				return errors.New("decode checkpoint request: trailing JSON value")
			}
			store, err := resolve(cmd.Context(), root, request.Project, request.Change)
			if err != nil {
				return err
			}
			output, err := runSddProgressCheckpoint(store, request)
			if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(output); encodeErr != nil && err == nil {
				return encodeErr
			}
			return err
		},
	}
	checkpoint.Flags().StringVar(&root, "root", "", "OpenSpec change root")
	checkpoint.Flags().StringVar(&requestPath, "request", "", "canonical checkpoint request JSON")
	_ = checkpoint.MarkFlagRequired("request")

	upgrade := &cobra.Command{
		Use: "upgrade-continuation", Short: "Bind a historical explicit-zero v2 snapshot to an ordered stream", Long: "Bind a historical explicit-zero v2 snapshot to an ordered stream without rewriting its immutable evidence. The request uses the canonical checkpoint schema and must reuse the blocked checkpoint request ID with the exact current base, tasks, full ordered entries, and stream SHA-256.", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := os.ReadFile(requestPath)
			if err != nil {
				return err
			}
			var request checkpointInput
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				return fmt.Errorf("decode continuation upgrade request: %w", err)
			}
			if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
				return errors.New("decode continuation upgrade request: trailing JSON value")
			}
			store, err := resolve(cmd.Context(), root, request.Project, request.Change)
			if err != nil {
				return err
			}
			output, err := runSddProgressUpgradeContinuation(store, request)
			if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(output); encodeErr != nil && err == nil {
				return encodeErr
			}
			return err
		},
	}
	upgrade.Flags().StringVar(&root, "root", "", "OpenSpec change root")
	upgrade.Flags().StringVar(&requestPath, "request", "", "canonical continuation upgrade request JSON")
	_ = upgrade.MarkFlagRequired("request")

	progress := &cobra.Command{Use: "progress", Short: "Manage bounded apply progress"}
	progress.AddCommand(advance, checkpoint, upgrade)
	return progress
}

func runSddProgressAdvance(store progressAdvancer, root string, request sddprogress.AdvanceRequest) (progressAdvanceOutput, error) {
	if upgrader, ok := store.(legacyProgressUpgrader); ok {
		result, upgraded, err := upgrader.UpgradeLegacy(request)
		if errors.Is(err, sddprogress.ErrBackendDiverged) {
			return progressAdvanceOutput{Outcome: "blocked", Code: "backend_diverged", Recovery: "reconcile matching OpenSpec and Hive legacy authorities before retrying"}, err
		}
		if upgraded {
			if err != nil {
				return progressAdvanceOutput{Outcome: "invalid", Code: "legacy_migration_failed", Recovery: "preserve and reconcile the legacy source before retrying"}, err
			}
			return progressAdvanceOutput{Outcome: "committed", Code: "committed", State: result}, nil
		}
	}
	result, err := store.Advance(request)
	if err == nil {
		return progressAdvanceOutput{Outcome: "committed", Code: "committed", State: result}, nil
	}
	var applyErr *hiveclient.ApplyProgressError
	if errors.As(err, &applyErr) {
		output := progressAdvanceOutput{Outcome: applyErr.Result.Outcome, Code: applyErr.Result.Code, State: result, Recovery: applyErr.Result.Recovery}
		if applyErr.Result.Capacity != nil {
			output.Capacity = &capacityOutput{Document: applyErr.Result.Capacity.Document, Runes: applyErr.Result.Capacity.Runes, Limit: applyErr.Result.Capacity.Limit}
		}
		if isHandledProgressOutcome(output.Code) {
			return output, nil
		}
		return output, err
	}
	state := result
	if state.Digest == "" {
		var stateErr error
		state, stateErr = currentProgressState(root)
		if stateErr != nil {
			return progressAdvanceOutput{Outcome: "recovery", Code: "read_current_failed", Recovery: "read the authoritative snapshot before retrying"}, err
		}
	}
	var validation *applyprogress.ValidationError
	switch {
	case errors.Is(err, sddprogress.ErrBackendDiverged):
		return progressAdvanceOutput{Outcome: "blocked", Code: "backend_diverged", State: state, Recovery: "retry the identical request only after the missing backend is available"}, err
	case errors.As(err, &validation):
		recovery := "repair the named protocol field and retry"
		if validation.Code == applyprogress.CodeTaskManifestMismatch {
			recovery = "fix the authoritative tasks.md task manifest, then replan and retry"
		}
		return progressAdvanceOutput{Outcome: "invalid", Code: string(validation.Code), Detail: validation.Detail, State: state, Recovery: recovery}, err
	case isSnapshotCapacityExhausted(err):
		return progressAdvanceOutput{Outcome: string(applyprogress.PlanSnapshotCapacityExhausted), Code: string(applyprogress.PlanSnapshotCapacityExhausted), Capacity: progressCapacityOutput(err), State: state, Recovery: applyprogress.SnapshotCapacityRecovery}, nil
	case isProgressCapacityError(err):
		return progressAdvanceOutput{Outcome: "invalid", Code: "capacity", Capacity: progressCapacityOutput(err), State: state, Recovery: "split evidence at a complete entry boundary"}, nil
	case errors.Is(err, applyprogress.ErrNonCanonical):
		return progressAdvanceOutput{Outcome: "invalid", Code: "noncanonical", State: state, Recovery: "submit canonical protocol JSON"}, err
	case errors.Is(err, applyprogress.ErrHashMismatch):
		return progressAdvanceOutput{Outcome: "invalid", Code: "hash_mismatch", State: state, Recovery: "recompute protocol digests"}, err
	case errors.Is(err, applyprogress.ErrInvalidID), errors.Is(err, applyprogress.ErrInvalidSchema), errors.Is(err, applyprogress.ErrInvalidValue), errors.Is(err, sddprogress.ErrInvalidChangeRoot):
		return progressAdvanceOutput{Outcome: "invalid", Code: "validation", State: state, Recovery: "repair the request and retry"}, err
	case errors.Is(err, sddprogress.ErrBatchCollision):
		return progressAdvanceOutput{Outcome: "conflict", Code: "batch_collision", State: state, Recovery: "use a new immutable batch ID"}, nil
	case errors.Is(err, sddprogress.ErrMissingBatch):
		return progressAdvanceOutput{Outcome: "blocked", Code: "missing_batch", State: state, Recovery: "restore the referenced immutable batch before retrying"}, err
	case errors.Is(err, filelock.ErrBusy):
		return progressAdvanceOutput{Outcome: "blocked", Code: "lock_busy", State: state, Recovery: "wait for the active progress operation, then retry the identical request"}, err
	case errors.Is(err, sddprogress.ErrRequestConflict):
		return progressAdvanceOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
	case errors.Is(err, sddprogress.ErrConflict):
		return progressAdvanceOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: staleProgressRecovery}, nil
	default:
		return progressAdvanceOutput{Outcome: "recovery", Code: "publication_interrupted", State: state, Recovery: "retry the identical request ID and payload"}, err
	}
}

// isHandledProgressOutcome identifies machine-readable outcomes a caller can
// recover from without parsing stderr, so Cobra exits zero in every store mode.
func isHandledProgressOutcome(code string) bool {
	switch code {
	case "stale", "request_id_conflict", "batch_collision", "capacity", string(applyprogress.PlanSnapshotCapacityExhausted):
		return true
	default:
		return false
	}
}

func isSnapshotCapacityExhausted(err error) bool {
	var exhausted *applyprogress.SnapshotCapacityExhaustedError
	return errors.As(err, &exhausted)
}

func isProgressCapacityError(err error) bool {
	return progressCapacityOutput(err) != nil
}

func progressCapacityOutput(err error) *capacityOutput {
	var capacity *applyprogress.CapacityError
	if !errors.As(err, &capacity) {
		return nil
	}
	return capacityOutputFor(capacity)
}

func capacityOutputFor(capacity *applyprogress.CapacityError) *capacityOutput {
	if capacity == nil {
		return nil
	}
	return &capacityOutput{Document: capacity.Document, Runes: capacity.Runes, Limit: capacity.EffectiveLimit(), CurrentRunes: capacity.CurrentRunes, ProjectedRunes: capacity.ProjectedRunes, CeilingRunes: capacity.CeilingRunes}
}

func capacityWarningOutputFor(warning *applyprogress.CapacityWarning) *capacityWarningOutput {
	if warning == nil {
		return nil
	}
	return &capacityWarningOutput{Document: warning.Document, Runes: warning.Runes, Limit: warning.Limit, Remaining: warning.Remaining, Threshold: warning.Threshold, Guidance: "avoid starting new tiny streams; consolidate evidence before the hard snapshot ceiling"}
}

func checkpointFrequencyOutputFor(guard *applyprogress.CheckpointFrequencyError) *checkpointFrequencyOutput {
	if guard == nil {
		return nil
	}
	return &checkpointFrequencyOutput{References: guard.References, Threshold: guard.Threshold, BatchRunes: guard.BatchRunes, MinimumRunes: guard.MinimumRunes, SnapshotRunes: guard.SnapshotRunes, SafeRunes: guard.SafeRunes}
}

// runSddProgressUpgradeContinuation is the executable recovery for readable v2
// historical explicit-zero continuation snapshots. It preserves every existing
// immutable batch and coverage record while binding the new ordered stream.
func runSddProgressUpgradeContinuation(store progressAdvancer, request checkpointInput) (checkpointOutput, error) {
	if !applyprogress.ValidID(request.RequestID) {
		err := fmt.Errorf("%w: request_id", applyprogress.ErrInvalidID)
		return checkpointValidationOutput(err), err
	}
	if request.Base == nil {
		return checkpointOutput{Outcome: "invalid", Code: "legacy_upgrade_required", Recovery: "supply the exact historical pre-continuation v2 base snapshot"}, sddprogress.ErrLegacyMigration
	}
	current, err := checkpointCurrent(store, request)
	if err != nil {
		return checkpointCurrentErrorOutput(err), err
	}
	if !sameCheckpointSnapshot(request.Base, current) {
		return checkpointOutput{Outcome: "conflict", Code: "stale", State: checkpointState(current), Recovery: staleProgressRecovery}, nil
	}
	if !request.Base.RequiresContinuationUpgrade() {
		return checkpointOutput{Outcome: "invalid", Code: "legacy_upgrade_required", Recovery: "supply the exact historical pre-continuation v2 base snapshot"}, sddprogress.ErrLegacyMigration
	}
	state := checkpointState(current)
	if state != (sddprogress.AdvanceResult{Generation: request.ExpectedGeneration, Revision: request.ExpectedRevision, Digest: request.ExpectedDigest}) {
		return checkpointOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: staleProgressRecovery}, nil
	}
	upgraded, err := applyprogress.UpgradeLegacyContinuation(*request.Base, request.Tasks, request.Entries, request.StreamSHA256)
	if err != nil {
		return checkpointValidationOutput(err), err
	}
	advance := sddprogress.AdvanceRequest{RequestID: request.RequestID, ExpectedGeneration: request.ExpectedGeneration, ExpectedRevision: request.ExpectedRevision, ExpectedDigest: request.ExpectedDigest, Snapshot: upgraded, Batches: []applyprogress.Batch{}}
	result, err := store.Advance(advance)
	if err != nil {
		output, handled := checkpointStoreErrorOutput(err, result, state)
		if handled {
			return output, nil
		}
		return output, err
	}
	return checkpointOutput{Outcome: "committed", Code: "committed", State: result, Snapshot: &upgraded, Receipt: &checkpointReceipt{RequestID: request.RequestID, PayloadSHA256: result.PayloadSHA256}, NextEntryIndex: upgraded.NextEntryIndex, NextEntryID: upgraded.NextEntryID, StreamSHA256: upgraded.StreamSHA256}, nil
}

func runSddProgressCheckpoint(store progressAdvancer, request checkpointInput) (checkpointOutput, error) {
	if !applyprogress.ValidID(request.RequestID) {
		err := fmt.Errorf("%w: request_id", applyprogress.ErrInvalidID)
		return checkpointValidationOutput(err), err
	}
	hiveTasksValidated := false
	var current *applyprogress.Snapshot
	var currentErr error
	currentResolved := false
	if source, ok := store.(hiveCheckpointLegacyStore); ok {
		// The guarded v2 head is authoritative. Query it before reading the legacy
		// projection, so a retained partial marker cannot trigger another import on
		// a continuation checkpoint.
		current, currentErr = checkpointCurrent(store, request)
		currentResolved = true
		if currentErr != nil {
			return checkpointCurrentErrorOutput(currentErr), currentErr
		}
		tasks, legacy, found, err := source.FetchAuthoritativeCheckpointArtifacts(request.Project, request.Change)
		if err != nil {
			return checkpointValidationOutput(err), err
		}
		if found && current == nil {
			upgrade, err := automaticLegacyCheckpointFromArtifacts([]byte(legacy), tasks, request)
			if err != nil {
				return checkpointValidationOutput(err), err
			}
			result, err := store.Advance(upgrade)
			if err != nil {
				output, handled := checkpointStoreErrorOutput(err, result, sddprogress.AdvanceResult{})
				if handled {
					return output, nil
				}
				return output, err
			}
			output := checkpointOutput{Outcome: "committed", Code: "legacy_imported", State: result, Snapshot: &upgrade.Snapshot, Receipt: &checkpointReceipt{RequestID: upgrade.RequestID, PayloadSHA256: result.PayloadSHA256}}
			if upgrade.Snapshot.Status == applyprogress.StatusPartial && upgrade.Snapshot.StreamSHA256 != "" && upgrade.Snapshot.NextEntryID != "" {
				output.Outcome = string(applyprogress.PlanContinuationRequired)
				output.NextEntryIndex, output.NextEntryID, output.StreamSHA256 = upgrade.Snapshot.NextEntryIndex, upgrade.Snapshot.NextEntryID, upgrade.Snapshot.StreamSHA256
			}
			return output, nil
		}
		if err := validateHiveCheckpointTasksContent(tasks, request); err != nil {
			return checkpointValidationOutput(err), err
		}
		hiveTasksValidated = true
	}
	if upgrader, ok := store.(legacyProgressUpgrader); ok {
		upgrade, found, err := automaticLegacyCheckpoint(rootForCheckpointStore(store), request)
		if err != nil {
			return checkpointValidationOutput(err), err
		}
		if found {
			result, upgraded, err := upgrader.UpgradeLegacy(upgrade)
			if errors.Is(err, sddprogress.ErrBackendDiverged) {
				return checkpointOutput{Outcome: "blocked", Code: "backend_diverged", Recovery: "reconcile matching OpenSpec and Hive legacy authorities before retrying"}, err
			}
			if upgraded {
				if err != nil {
					return checkpointOutput{Outcome: "invalid", Code: "legacy_migration_failed", Recovery: "repair the legacy artifact and retry the same checkpoint request"}, err
				}
				output := checkpointOutput{Outcome: "committed", Code: "legacy_imported", State: result, Snapshot: &upgrade.Snapshot, Receipt: &checkpointReceipt{RequestID: upgrade.RequestID, PayloadSHA256: result.PayloadSHA256}}
				if upgrade.Snapshot.Status == applyprogress.StatusPartial && upgrade.Snapshot.StreamSHA256 != "" && upgrade.Snapshot.NextEntryID != "" {
					output.Outcome = string(applyprogress.PlanContinuationRequired)
					output.NextEntryIndex, output.NextEntryID, output.StreamSHA256 = upgrade.Snapshot.NextEntryIndex, upgrade.Snapshot.NextEntryID, upgrade.Snapshot.StreamSHA256
				}
				return output, nil
			}
		}
	}
	if !hiveTasksValidated {
		if err := validateHiveCheckpointTasks(store, request); err != nil {
			return checkpointValidationOutput(err), err
		}
	}
	// Resolve authority before planning so stale state cannot be hidden by a
	// deterministic no-write capacity result. Receipt recovery is handled after
	// planning, when the exact candidate payload is available.
	if !currentResolved {
		current, currentErr = checkpointCurrent(store, request)
	}
	// A hybrid receipt can make a failed current read recoverable for this exact
	// candidate. Other stores have no durable partial-publication path, so preserve
	// their structured read failure before planning can obscure it.
	if currentErr != nil {
		if _, canRecover := store.(checkpointReceiptStore); !canRecover {
			return checkpointCurrentErrorOutput(currentErr), currentErr
		}
	}
	// Only a decoded historical explicit-zero v2 head has no current cursor
	// binding. It must take the explicit epoch-preserving upgrade path; a current
	// absent group is an unbound snapshot that starts a later stream normally.
	var preContinuation *applyprogress.Snapshot
	if current != nil && current.RequiresContinuationUpgrade() {
		preContinuation = current
	}
	if preContinuation == nil && currentErr == nil && request.Base != nil && request.Base.RequiresContinuationUpgrade() {
		// A caller may already hold the only decoded historical v2 snapshot when a
		// backend cannot yet resolve its head. The upgrade command still rechecks the
		// authoritative current state before publishing its migration CAS.
		preContinuation = request.Base
	}
	state, expected := checkpointState(current), sddprogress.AdvanceResult{Generation: request.ExpectedGeneration, Revision: request.ExpectedRevision, Digest: request.ExpectedDigest}
	stale := currentErr == nil && state != expected
	if currentErr == nil && !stale && !sameCheckpointSnapshot(request.Base, current) {
		return checkpointOutput{Outcome: "invalid", Code: "base_mismatch", State: state}, errors.New("checkpoint base does not match current state")
	}
	// Only the decoded historical wire shape requires the explicit migration
	// command. New exhausted partial snapshots are intentionally unbound and may
	// start a later independent stream through the normal checkpoint CAS.
	if currentErr == nil && preContinuation != nil {
		return checkpointOutput{Outcome: "blocked", Code: "legacy_upgrade_required", State: checkpointState(preContinuation), Recovery: continuationUpgradeRecovery}, sddprogress.ErrLegacyMigration
	}
	// An automatic legacy import commits the supplied stream identity even though
	// it only persists imported evidence. A changed retry must not be hidden by
	// the now-stale initial CAS coordinates.
	if stale && current != nil && current.StreamSHA256 != "" && requestIDUsed(store, request.RequestID) {
		if stream, err := applyprogress.StreamSHA256(request.Entries); err == nil && stream != current.StreamSHA256 {
			return checkpointOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
		}
	}

	planTasks, planEntries := request.Tasks, request.Entries
	if request.Base != nil {
		legacyTasks, legacyManifest, legacyErr := applyprogress.LegacyTaskManifest(request.Tasks)
		if legacyErr == nil && request.Base.TaskManifestSHA256 == legacyManifest {
			planEntries, legacyErr = remapLegacyEntries(request.Tasks, legacyTasks, request.Entries)
			if legacyErr != nil {
				return checkpointValidationOutput(legacyErr), legacyErr
			}
			planTasks = legacyTasks
		}
	}
	plan, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: request.Project, Change: request.Change, Base: request.Base, Tasks: planTasks, Entries: planEntries, EntryIndex: request.EntryIndex, EntryID: request.EntryID, BatchID: request.BatchID, StreamSHA256: request.StreamSHA256})
	if err != nil {
		return checkpointValidationOutput(err), err
	}
	advance := sddprogress.AdvanceRequest{RequestID: request.RequestID, ExpectedGeneration: request.ExpectedGeneration, ExpectedRevision: request.ExpectedRevision, ExpectedDigest: request.ExpectedDigest, Batches: []applyprogress.Batch{plan.Batch}, Snapshot: plan.Snapshot}
	if currentErr != nil {
		receipt, canRecover := store.(checkpointReceiptStore)
		if canRecover && receipt.HasReceipt(advance) {
			// A receipt was durably written before the head rename. Re-enter the
			// normal low-level Advance path only for this byte-identical candidate.
			current, currentErr, state, stale = nil, nil, sddprogress.AdvanceResult{}, false
		} else if _, plainOpenSpec := store.(sddprogress.OpenSpec); plainOpenSpec && requestIDUsed(store, request.RequestID) {
			// The interrupted plain OpenSpec receipt owns the request ID even though
			// it cannot establish a head. Do not let a changed retry bypass it.
			// Hybrid receipts have independent divergence semantics and stay blocked.
			return checkpointOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
		}
	}
	if currentErr != nil {
		return checkpointCurrentErrorOutput(currentErr), currentErr
	}
	if isNoWriteCheckpointPlan(plan.Outcome) {
		conflict, err := checkpointNoWriteRequestIDConflict(store, request.Project, request.Change, request.RequestID)
		if err != nil {
			return checkpointOutput{Outcome: "recovery", Code: "receipt_identity_failed", State: state, Recovery: "read the committed request identity before retrying"}, err
		}
		if conflict {
			return checkpointOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
		}
	}
	if stale {
		switch plan.Outcome {
		case applyprogress.PlanEvidenceItemTooLarge, applyprogress.PlanSnapshotCapacityExhausted, applyprogress.PlanStreamPreflightRequired, applyprogress.PlanCheckpointConsolidationRequired:
			return checkpointOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: staleProgressRecovery}, nil
		}
	}
	switch plan.Outcome {
	case applyprogress.PlanEvidenceItemTooLarge:
		return checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: state, EntryIndex: request.EntryIndex, EntryID: request.EntryID, StreamSHA256: plan.StreamSHA256, Capacity: capacityOutputFor(plan.Capacity)}, nil
	case applyprogress.PlanSnapshotCapacityExhausted:
		return checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: state, Recovery: applyprogress.SnapshotCapacityRecovery, StreamSHA256: plan.StreamSHA256, Capacity: capacityOutputFor(plan.Capacity)}, nil
	case applyprogress.PlanStreamPreflightRequired:
		return checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: state, Recovery: "consolidate evidence or evolve the snapshot before binding this stream", StreamSHA256: plan.StreamSHA256, Capacity: capacityOutputFor(plan.Capacity)}, nil
	case applyprogress.PlanCheckpointConsolidationRequired:
		return checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: state, StreamSHA256: plan.StreamSHA256, Frequency: checkpointFrequencyOutputFor(plan.Frequency), Recovery: "accumulate or combine pending evidence into a larger nonterminal checkpoint before retrying"}, nil
	}
	result, err := store.Advance(advance)
	if err != nil {
		output, handled := checkpointStoreErrorOutput(err, result, state)
		if handled {
			return output, nil
		}
		return output, err
	}
	output := checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: result, Snapshot: &plan.Snapshot, Receipt: &checkpointReceipt{RequestID: request.RequestID, PayloadSHA256: result.PayloadSHA256}, StreamSHA256: plan.StreamSHA256, Warning: capacityWarningOutputFor(plan.Warning)}
	if plan.Outcome == applyprogress.PlanContinuationRequired {
		output.NextEntryIndex, output.NextEntryID = plan.NextEntryIndex, plan.NextEntryID
	}
	return output, nil
}

func validateHiveCheckpointTasks(store progressAdvancer, request checkpointInput) error {
	source, ok := store.(hiveCheckpointTasksStore)
	if !ok {
		return nil
	}
	content, err := source.FetchAuthoritativeTasks(request.Project, request.Change)
	if err != nil {
		return invalidHiveTasks(err)
	}
	return validateHiveCheckpointTasksContent(content, request)
}

func validateHiveCheckpointTasksContent(content string, request checkpointInput) error {
	parsed, err := applyprogress.ParseTasksMarkdown(content)
	if err != nil || len(parsed.Tasks) == 0 {
		return invalidHiveTasks(err)
	}
	_, authoritativeManifest, err := applyprogress.TaskManifest(parsed.Tasks)
	if err != nil {
		return invalidHiveTasks(err)
	}
	_, callerManifest, err := applyprogress.TaskManifest(request.Tasks)
	if err != nil {
		return err
	}
	if callerManifest != authoritativeManifest {
		return &applyprogress.ValidationError{Code: applyprogress.CodeTaskManifestMismatch, Detail: "authoritative Hive tasks"}
	}
	return nil
}

func invalidHiveTasks(err error) error {
	if err != nil {
		return &applyprogress.ValidationError{Code: applyprogress.CodeInvalidTaskManifest, Detail: "authoritative Hive tasks"}
	}
	return &applyprogress.ValidationError{Code: applyprogress.CodeInvalidTaskManifest, Detail: "authoritative Hive tasks"}
}

func rootForCheckpointStore(store progressAdvancer) string {
	switch store := store.(type) {
	case sddprogress.OpenSpec:
		return store.Root
	case sddprogress.Hybrid:
		return store.Root
	default:
		return ""
	}
}

func remapLegacyEntries(native, legacy []applyprogress.Task, entries []applyprogress.EvidenceEntry) ([]applyprogress.EvidenceEntry, error) {
	normalized, _, err := applyprogress.TaskManifest(native)
	if err != nil || len(normalized) != len(legacy) {
		return nil, sddprogress.ErrLegacyMigration
	}
	identities := make(map[string]string, len(normalized))
	for i := range normalized {
		identities[normalized[i].ID] = legacy[i].ID
	}
	mapped := append([]applyprogress.EvidenceEntry(nil), entries...)
	for i := range mapped {
		mapped[i].TaskIDs = append([]string(nil), mapped[i].TaskIDs...)
		mapped[i].CompletesTaskIDs = append([]string(nil), mapped[i].CompletesTaskIDs...)
		for j, id := range mapped[i].TaskIDs {
			mappedID, ok := identities[id]
			if !ok {
				return nil, sddprogress.ErrLegacyMigration
			}
			mapped[i].TaskIDs[j] = mappedID
		}
		for j, id := range mapped[i].CompletesTaskIDs {
			mappedID, ok := identities[id]
			if !ok {
				return nil, sddprogress.ErrLegacyMigration
			}
			mapped[i].CompletesTaskIDs[j] = mappedID
		}
	}
	return mapped, nil
}

func automaticLegacyCheckpoint(root string, input checkpointInput) (sddprogress.AdvanceRequest, bool, error) {
	if root == "" || input.Base != nil || input.ExpectedGeneration != 0 || input.ExpectedRevision != 0 || input.ExpectedDigest != "" {
		return sddprogress.AdvanceRequest{}, false, nil
	}
	legacy, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if os.IsNotExist(err) || bytes.HasPrefix(legacy, []byte(`{"schema":"jarvis.sdd-apply-progress/v2"`)) {
		return sddprogress.AdvanceRequest{}, false, nil
	}
	if err != nil {
		return sddprogress.AdvanceRequest{}, false, err
	}
	tasksData, err := os.ReadFile(filepath.Join(root, "tasks.md"))
	if err != nil {
		return sddprogress.AdvanceRequest{}, false, err
	}
	upgrade, err := automaticLegacyCheckpointFromArtifacts(legacy, string(tasksData), input)
	return upgrade, true, err
}

func automaticLegacyCheckpointFromArtifacts(legacy []byte, tasksData string, input checkpointInput) (sddprogress.AdvanceRequest, error) {
	if input.Base != nil || input.ExpectedGeneration != 0 || input.ExpectedRevision != 0 || input.ExpectedDigest != "" {
		return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
	}
	parsed, err := applyprogress.ParseTasksMarkdown(tasksData)
	if err != nil {
		parsed, err = applyprogress.ParseLegacyTasksMarkdown(tasksData)
	}
	if err != nil || len(parsed.Tasks) == 0 {
		return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
	}
	_, nativeManifest, err := applyprogress.TaskManifest(parsed.Tasks)
	if err != nil {
		return sddprogress.AdvanceRequest{}, err
	}
	_, inputManifest, err := applyprogress.TaskManifest(input.Tasks)
	if err != nil || inputManifest != nativeManifest {
		return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
	}
	legacyTasks := append([]applyprogress.Task(nil), parsed.Tasks...)
	for i := range legacyTasks {
		if legacyTasks[i].Path == "" {
			legacyTasks[i].Path = legacyTasks[i].ID
		}
	}
	converted, err := applyprogress.ConvertLegacy(applyprogress.LegacyProgress{Tasks: legacyTasks, Completed: parsed.CompletedIDs})
	if err != nil {
		return sddprogress.AdvanceRequest{}, err
	}
	legacyTerminal := parsed.AllDone
	if legacyTerminal && (len(input.Entries) != 0 || input.EntryIndex != 0 || input.EntryID != "" || input.StreamSHA256 != "") {
		return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
	}
	streamSHA256 := ""
	if !legacyTerminal {
		if len(input.Entries) == 0 || input.EntryIndex != 0 || input.EntryID != input.Entries[0].EntryID {
			return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
		}
		legacyEntries, remapErr := remapLegacyEntries(input.Tasks, converted.Tasks, input.Entries)
		if remapErr != nil {
			return sddprogress.AdvanceRequest{}, remapErr
		}
		input.Entries = legacyEntries
		input.EntryID = input.Entries[0].EntryID
		streamSHA256, err = applyprogress.StreamSHA256(input.Entries)
		if err != nil || input.StreamSHA256 != "" && input.StreamSHA256 != streamSHA256 {
			return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
		}
		// The legacy prefix already supplies completed coverage, so validating the
		// remaining stream as an initial plan would falsely reject a valid suffix.
		// Its hash is bound now; the successor planner validates it against the
		// imported snapshot on the next mutation.
	}
	if !legacyTerminal {
		existing := make([]applyprogress.Coverage, len(converted.Completed))
		for i, id := range converted.Completed {
			existing[i].TaskID = id
		}
		// Automatic migration binds the same complete future stream contract as
		// explicit continuation upgrade before it can persist a hash or cursor.
		if err := applyprogress.ValidateFutureStream(converted.Tasks, input.Entries, existing); err != nil {
			return sddprogress.AdvanceRequest{}, err
		}
	}
	complete, partial := false, false
	for _, line := range strings.Split(string(legacy), "\n") {
		switch strings.TrimSpace(line) {
		case "status: complete":
			complete = true
		case "status: partial":
			partial = true
		default:
			if strings.HasPrefix(strings.TrimSpace(line), "status:") {
				return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
			}
		}
	}
	if complete && partial || complete && len(converted.Completed) != len(converted.Tasks) {
		return sddprogress.AdvanceRequest{}, sddprogress.ErrLegacyMigration
	}
	sum := sha256.Sum256(legacy)
	batchID := "apb-" + hex.EncodeToString(sum[:16])
	batch := applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: input.Project, Change: input.Change, BatchID: batchID, Entries: []applyprogress.EvidenceEntry{}}
	completedIDs := converted.Completed
	coverageByID := map[string]applyprogress.Coverage{}
	if len(completedIDs) > 0 {
		// One deterministic imported provenance record proves the complete legacy
		// prefix. Per-task entries only duplicate the same source and make a large
		// migration exhaust the bounded evidence document unnecessarily.
		entryID := "imported-" + hex.EncodeToString(sum[:8])
		ids := append([]string(nil), completedIDs...)
		batch.Entries = append(batch.Entries, applyprogress.EvidenceEntry{EntryID: entryID, TaskIDs: ids, CompletesTaskIDs: append([]string(nil), ids...), Kind: applyprogress.EvidenceImported, Summary: "imported legacy progress sha256=" + hex.EncodeToString(sum[:]), Command: "legacy import", Outcome: applyprogress.OutcomePass, Files: []string{"apply-progress.md"}})
		for _, taskID := range ids {
			coverageByID[taskID] = applyprogress.Coverage{TaskID: taskID, BatchID: batchID, EntryID: entryID}
		}
	}
	batches := []applyprogress.Batch{}
	refs := []applyprogress.BatchRef{}
	if len(batch.Entries) > 0 {
		batch, _, err = applyprogress.SealBatch(batch)
		if err != nil {
			return sddprogress.AdvanceRequest{}, err
		}
		batches, refs = []applyprogress.Batch{batch}, []applyprogress.BatchRef{{BatchID: batchID, SHA256: batch.SHA256}}
	}
	coverage := make([]applyprogress.Coverage, 0, len(coverageByID))
	for _, task := range converted.Tasks {
		if item, ok := coverageByID[task.ID]; ok {
			coverage = append(coverage, item)
		}
	}
	status := applyprogress.StatusPartial
	if complete || len(coverage) == len(converted.Tasks) {
		status = applyprogress.StatusComplete
	}
	snapshot := applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: input.Project, Change: input.Change, Generation: 1, Revision: 1, TaskManifestSHA256: converted.TaskManifestSHA256, Status: status, Coverage: coverage, Batches: refs}
	snapshot, _, err = applyprogress.SealSnapshot(snapshot)
	if err != nil {
		return sddprogress.AdvanceRequest{}, err
	}
	if snapshot.Status == applyprogress.StatusPartial {
		if len(input.Entries) == 0 {
			return sddprogress.AdvanceRequest{}, applyprogress.ErrInvalidValue
		}
		snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = streamSHA256, 0, input.Entries[0].EntryID
		snapshot, _, err = applyprogress.SealSnapshot(snapshot)
		if err != nil {
			return sddprogress.AdvanceRequest{}, err
		}
		if err := applyprogress.PreflightCompleteStream(snapshot, converted.Tasks, input.Entries, streamSHA256); err != nil {
			return sddprogress.AdvanceRequest{}, err
		}
	}
	return sddprogress.AdvanceRequest{RequestID: input.RequestID, LegacySourceSHA256: applyprogress.LegacySourceSHA256(legacy), Snapshot: snapshot, Batches: batches}, nil
}

// checkpointStoreErrorOutput is the single typed recovery mapping for every
// guarded checkpoint mutation, including continuation identity upgrades.
// handled reports outcomes that deliberately exit zero.
func checkpointCurrentErrorOutput(err error) checkpointOutput {
	var applyErr *hiveclient.ApplyProgressError
	if errors.As(err, &applyErr) {
		return checkpointOutput{Outcome: applyErr.Result.Outcome, Code: applyErr.Result.Code, Detail: applyErr.Result.Detail, State: sddprogress.AdvanceResult{Generation: applyErr.Result.State.Generation, Revision: applyErr.Result.State.Revision, Digest: applyErr.Result.State.Digest}, Recovery: applyErr.Result.Recovery}
	}
	if errors.Is(err, sddprogress.ErrBackendDiverged) {
		return checkpointOutput{Outcome: "blocked", Code: "backend_diverged", Recovery: "retry the identical request only after the missing backend is available"}
	}
	if errors.Is(err, sddprogress.ErrLegacyMigration) {
		return checkpointOutput{Outcome: "blocked", Code: "legacy_upgrade_required", Recovery: "submit an authorized advance with imported legacy evidence before checkpointing"}
	}
	return checkpointOutput{Outcome: "recovery", Code: "read_current_failed", Recovery: "read the authoritative snapshot before retrying"}
}

func checkpointStoreErrorOutput(err error, result, fallback sddprogress.AdvanceResult) (checkpointOutput, bool) {
	state := result
	if state.Digest == "" {
		state = fallback
	}
	var applyErr *hiveclient.ApplyProgressError
	if errors.As(err, &applyErr) {
		output := checkpointOutput{Outcome: applyErr.Result.Outcome, Code: applyErr.Result.Code, Detail: applyErr.Result.Detail, State: state, Recovery: applyErr.Result.Recovery}
		if applyErr.Result.Capacity != nil {
			output.Capacity = &capacityOutput{Document: applyErr.Result.Capacity.Document, Runes: applyErr.Result.Capacity.Runes, Limit: applyErr.Result.Capacity.Limit}
		}
		return output, isHandledProgressOutcome(output.Code)
	}
	var validation *applyprogress.ValidationError
	switch {
	case errors.As(err, &validation):
		recovery := "repair the named protocol field and replan"
		if validation.Code == applyprogress.CodeTaskManifestMismatch {
			recovery = "fix the authoritative tasks.md task manifest, then replan and retry"
		}
		return checkpointOutput{Outcome: "invalid", Code: string(validation.Code), Detail: validation.Detail, State: state, Recovery: recovery}, false
	case isSnapshotCapacityExhausted(err):
		return checkpointOutput{Outcome: string(applyprogress.PlanSnapshotCapacityExhausted), Code: string(applyprogress.PlanSnapshotCapacityExhausted), Capacity: progressCapacityOutput(err), State: state, Recovery: applyprogress.SnapshotCapacityRecovery}, true
	case isProgressCapacityError(err):
		return checkpointOutput{Outcome: "invalid", Code: "capacity", Capacity: progressCapacityOutput(err), State: state, Recovery: "split evidence at a complete entry boundary"}, true
	case errors.Is(err, applyprogress.ErrNonCanonical):
		return checkpointOutput{Outcome: "invalid", Code: "noncanonical", State: state, Recovery: "submit canonical protocol JSON"}, false
	case errors.Is(err, applyprogress.ErrHashMismatch):
		return checkpointOutput{Outcome: "invalid", Code: "hash_mismatch", State: state, Recovery: "recompute protocol digests"}, false
	case errors.Is(err, applyprogress.ErrInvalidID), errors.Is(err, applyprogress.ErrInvalidSchema), errors.Is(err, applyprogress.ErrInvalidValue), errors.Is(err, sddprogress.ErrInvalidChangeRoot):
		return checkpointOutput{Outcome: "invalid", Code: "validation", State: state, Recovery: "repair the request and replan"}, false
	case errors.Is(err, sddprogress.ErrBatchCollision):
		return checkpointOutput{Outcome: "conflict", Code: "batch_collision", State: state, Recovery: "use a new immutable batch ID"}, true
	case errors.Is(err, sddprogress.ErrMissingBatch):
		return checkpointOutput{Outcome: "blocked", Code: "missing_batch", State: state, Recovery: "restore the referenced immutable batch before retrying"}, false
	case errors.Is(err, filelock.ErrBusy):
		return checkpointOutput{Outcome: "blocked", Code: "lock_busy", State: state, Recovery: "wait for the active progress operation, then retry the identical request"}, false
	case errors.Is(err, sddprogress.ErrBackendDiverged):
		return checkpointOutput{Outcome: "blocked", Code: "backend_diverged", State: state, Recovery: "retry the identical request only after the missing backend is available"}, false
	case errors.Is(err, sddprogress.ErrRequestConflict):
		return checkpointOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, true
	case errors.Is(err, sddprogress.ErrConflict):
		return checkpointOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: staleProgressRecovery}, true
	default:
		return checkpointOutput{Outcome: "recovery", Code: "publication_interrupted", State: state, Recovery: "retry the identical request ID and payload"}, false
	}
}

func checkpointValidationOutput(err error) checkpointOutput {
	if isProgressCapacityError(err) {
		return checkpointOutput{Outcome: "invalid", Code: "capacity", Capacity: progressCapacityOutput(err), Recovery: "reduce or consolidate imported legacy evidence before retrying"}
	}
	if errors.Is(err, sddprogress.ErrLegacyMigration) {
		return checkpointOutput{Outcome: "invalid", Code: "legacy_migration", Recovery: "preserve the legacy artifact and submit a new stream only after migration"}
	}
	var validation *applyprogress.ValidationError
	if errors.As(err, &validation) {
		return checkpointOutput{Outcome: "invalid", Code: string(validation.Code), Detail: validation.Detail, Recovery: "repair the named protocol field and replan"}
	}
	switch {
	case errors.Is(err, applyprogress.ErrNonCanonical):
		return checkpointOutput{Outcome: "invalid", Code: "noncanonical", Recovery: "submit canonical protocol JSON"}
	case errors.Is(err, applyprogress.ErrHashMismatch):
		return checkpointOutput{Outcome: "invalid", Code: "hash_mismatch", Recovery: "recompute protocol digests"}
	case errors.Is(err, applyprogress.ErrInvalidID), errors.Is(err, applyprogress.ErrInvalidSchema), errors.Is(err, applyprogress.ErrInvalidValue):
		return checkpointOutput{Outcome: "invalid", Code: "validation", Recovery: "repair the request and replan"}
	default:
		return checkpointOutput{Outcome: "invalid", Code: "invalid_request", Recovery: "repair the request and replan"}
	}
}

func isNoWriteCheckpointPlan(outcome applyprogress.PlanOutcome) bool {
	switch outcome {
	case applyprogress.PlanEvidenceItemTooLarge, applyprogress.PlanSnapshotCapacityExhausted, applyprogress.PlanStreamPreflightRequired, applyprogress.PlanCheckpointConsolidationRequired:
		return true
	default:
		return false
	}
}

func checkpointNoWriteRequestIDConflict(store progressAdvancer, project, change, requestID string) (bool, error) {
	if hive, ok := store.(hiveCheckpointReceiptStore); ok {
		return hive.ReceiptIdentity(project, change, requestID)
	}
	return requestIDUsed(store, requestID), nil
}

func requestIDUsed(store progressAdvancer, requestID string) bool {
	known, ok := store.(checkpointRequestIDStore)
	return ok && known.RequestIDUsed(requestID)
}

func checkpointCurrent(store progressAdvancer, request checkpointInput) (*applyprogress.Snapshot, error) {
	// Hybrid must resolve both authoritative sides before a generic Current method
	// can match; Hive likewise needs the requested project/change rather than an
	// OpenSpec-shaped zero-argument current call used by test doubles.
	if store, ok := store.(hybridCheckpointStore); ok {
		return store.Current(sddprogress.AdvanceRequest{Snapshot: applyprogress.Snapshot{Project: request.Project, Change: request.Change}})
	}
	if store, ok := store.(hiveCheckpointStore); ok {
		return store.CurrentCheckpoint(request.Project, request.Change)
	}
	if store, ok := store.(progressCheckpointStore); ok {
		return store.Current()
	}
	return nil, errors.New("checkpoint store cannot resolve current state")
}

func checkpointState(snapshot *applyprogress.Snapshot) sddprogress.AdvanceResult {
	if snapshot == nil {
		return sddprogress.AdvanceResult{}
	}
	return sddprogress.AdvanceResult{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest}
}

func sameCheckpointSnapshot(left, right *applyprogress.Snapshot) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Generation == right.Generation && left.Revision == right.Revision && left.Digest == right.Digest
}

func currentProgressState(root string) (sddprogress.AdvanceResult, error) {
	data, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if os.IsNotExist(err) {
		return sddprogress.AdvanceResult{}, nil
	}
	if err != nil {
		return sddprogress.AdvanceResult{}, err
	}
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(data)
	if err != nil {
		return sddprogress.AdvanceResult{}, err
	}
	return sddprogress.AdvanceResult{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest}, nil
}

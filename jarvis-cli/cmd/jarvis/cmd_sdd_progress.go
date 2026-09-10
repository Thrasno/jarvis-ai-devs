package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type progressAdvancer interface {
	Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error)
}

type legacyProgressUpgrader interface {
	UpgradeLegacy(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, bool, error)
}

type progressAdvanceOutput struct {
	Outcome  string                    `json:"outcome"`
	Code     string                    `json:"code"`
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
	RequestID string `json:"request_id"`
}

type checkpointOutput struct {
	Outcome        string                    `json:"outcome"`
	Code           string                    `json:"code"`
	State          sddprogress.AdvanceResult `json:"state"`
	Snapshot       *applyprogress.Snapshot   `json:"snapshot,omitempty"`
	Receipt        *checkpointReceipt        `json:"receipt,omitempty"`
	NextEntryIndex int                       `json:"next_entry_index,omitempty"`
	NextEntryID    string                    `json:"next_entry_id,omitempty"`
	StreamSHA256   string                    `json:"stream_sha256,omitempty"`
	EntryIndex     int                       `json:"entry_index,omitempty"`
	EntryID        string                    `json:"entry_id,omitempty"`
	Recovery       string                    `json:"recovery,omitempty"`
}

type progressCheckpointStore interface {
	progressAdvancer
	Current() (*applyprogress.Snapshot, error)
}

type hiveCheckpointStore interface {
	CurrentCheckpoint(string, string) (*applyprogress.Snapshot, error)
}

type hybridCheckpointStore interface {
	Current(sddprogress.AdvanceRequest) (*applyprogress.Snapshot, error)
}

type checkpointReceiptStore interface {
	HasReceipt(sddprogress.AdvanceRequest) bool
}

func defaultOpenSpec(root string) progressAdvancer { return sddprogress.OpenSpec{Root: root} }

var newProgressOpenSpec = defaultOpenSpec
var newProgressHive = func() (progressAdvancer, error) {
	client, err := hiveclient.NewFromEnv()
	return hiveProgressAdvancer{client: client}, err
}

type hiveProgressAdvancer struct{ client *hiveclient.Client }

func (h hiveProgressAdvancer) Current(request sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	result, err := h.client.GetApplyProgress(context.Background(), request.Snapshot.Project, request.Snapshot.Change)
	return sddprogress.AdvanceResult{Generation: result.State.Generation, Revision: result.State.Revision, Digest: result.State.Digest}, err
}

func (h hiveProgressAdvancer) CurrentCheckpoint(project, change string) (*applyprogress.Snapshot, error) {
	result, err := h.client.GetApplyProgress(context.Background(), project, change)
	var apiErr *hiveclient.ApplyProgressError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 422 && apiErr.Result.Code == "validation" {
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

func (h hiveProgressAdvancer) Advance(request sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	result, err := h.client.AdvanceApplyProgress(context.Background(), hiveclient.ApplyProgressAdvanceRequest{
		Project: request.Snapshot.Project, Change: request.Snapshot.Change, RequestID: request.RequestID,
		ExpectedGeneration: request.ExpectedGeneration, ExpectedRevision: request.ExpectedRevision, ExpectedDigest: request.ExpectedDigest,
		Snapshot: request.Snapshot, Batches: request.Batches,
	})
	state := sddprogress.AdvanceResult{Generation: result.State.Generation, Revision: result.State.Revision, Digest: result.State.Digest}
	if result.Code == "stale" {
		return state, sddprogress.ErrConflict
	}
	if result.Code == "request_id_conflict" {
		return state, sddprogress.ErrRequestConflict
	}
	if err != nil {
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

func init() { sddCmd.AddCommand(newSddProgressCommand(configuredProgressStore)) }

func newSddProgressCommand(open func(string) progressAdvancer) *cobra.Command {
	var root, requestPath string
	advance := &cobra.Command{
		Use:           "advance",
		Short:         "Atomically advance OpenSpec apply progress",
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
			if decoder.More() {
				return errors.New("decode advance request: trailing JSON value")
			}
			output, err := runSddProgressAdvance(open(root), root, request)
			if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(output); encodeErr != nil && err == nil {
				return encodeErr
			}
			return err
		},
	}
	advance.Flags().StringVar(&root, "root", "", "OpenSpec change root")
	advance.Flags().StringVar(&requestPath, "request", "", "canonical advance request JSON")
	_ = advance.MarkFlagRequired("root")
	_ = advance.MarkFlagRequired("request")

	checkpoint := &cobra.Command{
		Use: "checkpoint", Short: "Commit one bounded OpenSpec evidence prefix", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
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
			if decoder.More() {
				return errors.New("decode checkpoint request: trailing JSON value")
			}
			output, err := runSddProgressCheckpoint(open(root), request)
			if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(output); encodeErr != nil && err == nil {
				return encodeErr
			}
			return err
		},
	}
	checkpoint.Flags().StringVar(&root, "root", "", "OpenSpec change root")
	checkpoint.Flags().StringVar(&requestPath, "request", "", "canonical checkpoint request JSON")
	_ = checkpoint.MarkFlagRequired("root")
	_ = checkpoint.MarkFlagRequired("request")

	progress := &cobra.Command{Use: "progress", Short: "Manage bounded apply progress"}
	progress.AddCommand(advance, checkpoint)
	return progress
}

func runSddProgressAdvance(store progressAdvancer, root string, request sddprogress.AdvanceRequest) (progressAdvanceOutput, error) {
	if upgrader, ok := store.(legacyProgressUpgrader); ok {
		result, upgraded, err := upgrader.UpgradeLegacy(request)
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
	state := result
	if state.Digest == "" {
		var stateErr error
		state, stateErr = currentProgressState(root)
		if stateErr != nil {
			return progressAdvanceOutput{Outcome: "recovery", Code: "read_current_failed", Recovery: "read the authoritative snapshot before retrying"}, err
		}
	}
	switch {
	case errors.Is(err, sddprogress.ErrRequestConflict):
		return progressAdvanceOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
	case errors.Is(err, sddprogress.ErrConflict):
		return progressAdvanceOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: "retry with the current generation, revision, and digest"}, nil
	case errors.Is(err, sddprogress.ErrBackendDiverged):
		return progressAdvanceOutput{Outcome: "blocked", Code: "backend_diverged", State: state, Recovery: "retry the identical request only after the missing backend is available"}, err
	default:
		return progressAdvanceOutput{Outcome: "recovery", Code: "publication_interrupted", State: state, Recovery: "retry the identical request ID and payload"}, err
	}
}

func runSddProgressCheckpoint(store progressAdvancer, request checkpointInput) (checkpointOutput, error) {
	plan, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: request.Project, Change: request.Change, Base: request.Base, Tasks: request.Tasks, Entries: request.Entries, EntryIndex: request.EntryIndex, EntryID: request.EntryID, BatchID: request.BatchID, StreamSHA256: request.StreamSHA256})
	if err != nil {
		return checkpointOutput{Outcome: "invalid", Code: "invalid_request"}, err
	}
	advance := sddprogress.AdvanceRequest{RequestID: request.RequestID, ExpectedGeneration: request.ExpectedGeneration, ExpectedRevision: request.ExpectedRevision, ExpectedDigest: request.ExpectedDigest, Batches: []applyprogress.Batch{plan.Batch}, Snapshot: plan.Snapshot}
	current, err := checkpointCurrent(store, request)
	if err != nil && (func() bool { receipt, ok := store.(checkpointReceiptStore); return ok && receipt.HasReceipt(advance) })() {
		current, err = nil, nil
	}
	if err != nil {
		if errors.Is(err, sddprogress.ErrBackendDiverged) {
			return checkpointOutput{Outcome: "blocked", Code: "backend_diverged", Recovery: "retry the identical request only after the missing backend is available"}, err
		}
		return checkpointOutput{Outcome: "recovery", Code: "read_current_failed", Recovery: "read the authoritative snapshot before retrying"}, err
	}
	state, expected := checkpointState(current), sddprogress.AdvanceResult{Generation: request.ExpectedGeneration, Revision: request.ExpectedRevision, Digest: request.ExpectedDigest}
	baseMatches := sameCheckpointSnapshot(request.Base, current)
	if state == expected && !baseMatches {
		return checkpointOutput{Outcome: "invalid", Code: "base_mismatch", State: state}, errors.New("checkpoint base does not match current state")
	}
	switch plan.Outcome {
	case applyprogress.PlanEvidenceItemTooLarge:
		return checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: state, EntryIndex: request.EntryIndex, EntryID: request.EntryID, StreamSHA256: plan.StreamSHA256}, nil
	case applyprogress.PlanSnapshotCapacityExhausted:
		return checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: state, Recovery: "reconcile or evolve the snapshot before retrying", StreamSHA256: plan.StreamSHA256}, nil
	}
	result, err := store.Advance(advance)
	if err != nil {
		if errors.Is(err, sddprogress.ErrRequestConflict) {
			return checkpointOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
		}
		if errors.Is(err, sddprogress.ErrConflict) {
			return checkpointOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: "re-resolve and replan with new request and batch IDs"}, nil
		}
		if errors.Is(err, sddprogress.ErrBackendDiverged) {
			return checkpointOutput{Outcome: "blocked", Code: "backend_diverged", State: state, Recovery: "retry the identical request only after the missing backend is available"}, err
		}
		return checkpointOutput{Outcome: "recovery", Code: "publication_interrupted", State: state, Recovery: "retry the identical request ID and payload"}, err
	}
	output := checkpointOutput{Outcome: string(plan.Outcome), Code: string(plan.Outcome), State: result, Snapshot: &plan.Snapshot, Receipt: &checkpointReceipt{RequestID: request.RequestID}, StreamSHA256: plan.StreamSHA256}
	if plan.Outcome == applyprogress.PlanContinuationRequired {
		output.NextEntryIndex, output.NextEntryID = plan.NextEntryIndex, plan.NextEntryID
	}
	return output, nil
}

func checkpointCurrent(store progressAdvancer, request checkpointInput) (*applyprogress.Snapshot, error) {
	if store, ok := store.(progressCheckpointStore); ok {
		return store.Current()
	}
	if store, ok := store.(hiveCheckpointStore); ok {
		return store.CurrentCheckpoint(request.Project, request.Change)
	}
	if store, ok := store.(hybridCheckpointStore); ok {
		return store.Current(sddprogress.AdvanceRequest{Snapshot: applyprogress.Snapshot{Project: request.Project, Change: request.Change}})
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

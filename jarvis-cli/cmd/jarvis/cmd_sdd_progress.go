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

	progress := &cobra.Command{Use: "progress", Short: "Manage bounded apply progress"}
	progress.AddCommand(advance)
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

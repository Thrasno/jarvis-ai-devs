package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
)

type progressAdvancer interface {
	Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error)
}

type progressAdvanceOutput struct {
	Outcome  string                    `json:"outcome"`
	Code     string                    `json:"code"`
	State    sddprogress.AdvanceResult `json:"state"`
	Recovery string                    `json:"recovery,omitempty"`
}

func defaultOpenSpec(root string) progressAdvancer { return sddprogress.OpenSpec{Root: root} }

func init() { sddCmd.AddCommand(newSddProgressCommand(defaultOpenSpec)) }

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
	result, err := store.Advance(request)
	if err == nil {
		return progressAdvanceOutput{Outcome: "committed", Code: "committed", State: result}, nil
	}
	state, stateErr := currentProgressState(root)
	if stateErr != nil {
		return progressAdvanceOutput{Outcome: "recovery", Code: "read_current_failed", Recovery: "read the authoritative snapshot before retrying"}, err
	}
	switch {
	case errors.Is(err, sddprogress.ErrConflict):
		return progressAdvanceOutput{Outcome: "conflict", Code: "stale", State: state, Recovery: "retry with the current generation, revision, and digest"}, nil
	case errors.Is(err, sddprogress.ErrRequestConflict):
		return progressAdvanceOutput{Outcome: "conflict", Code: "request_id_conflict", State: state, Recovery: "use a new request ID for changed payload"}, nil
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

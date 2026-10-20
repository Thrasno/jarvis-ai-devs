package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// sddBindOutput is the --json result of jarvis sdd bind.
type sddBindOutput struct {
	Project    string `json:"project"`
	Change     string `json:"change"`
	Mode       string `json:"mode"`
	Provenance string `json:"provenance"`
	Persisted  bool   `json:"persisted"`
}

func newSddBindCommand() *cobra.Command {
	var change, mode, project string
	var asJSON bool
	command := &cobra.Command{
		Use:   "bind",
		Short: "Bind an SDD change to the artifact store chosen in preflight",
		Long: "Bind an SDD change to the artifact store chosen in the SDD session preflight, before any status call or artifact write. " +
			"The binding is persisted through its owner with provenance " + sddbinding.ProvenancePreflight + ": hive in Hive, openspec in openspec/changes/<change>/state.yaml (the change directory is created when missing), and hybrid in both. " +
			"Any spelling of the project resolves to its canonical Hive key, and a retired alias resolves to its target, so the binding is written under the effective key. " +
			"Repeating the same mode is an idempotent replay that exits 0 and keeps the stored provenance. " +
			"A different stored binding, or one that existing apply progress already selects, is a conflict: the command exits non-zero and leaves the binding unchanged. " +
			"none is never persisted, so it is not a valid --mode.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			return runSddBind(ctx, cmd.OutOrStdout(), project, change, mode, asJSON)
		},
	}
	command.Flags().StringVar(&change, "change", "", "canonical SDD change name (required)")
	command.Flags().StringVar(&mode, "mode", "", "artifact store chosen in preflight: hive, openspec, or hybrid (required)")
	command.Flags().StringVar(&project, "project", "", sddProjectFlagUsage)
	command.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return command
}

func runSddBind(ctx context.Context, out io.Writer, projectFlag, change, modeFlag string, asJSON bool) error {
	mode, err := sddruntime.ResolveStoreMode(modeFlag)
	if err != nil || mode == sddruntime.StoreModeNone {
		return fmt.Errorf("sdd bind --mode must be hive, openspec, or hybrid, got %q", modeFlag)
	}
	if strings.TrimSpace(change) == "" {
		return errors.New("sdd bind --change is required")
	}
	workspace, _, err := progressBindingWorkspace("", change)
	if err != nil {
		return err
	}
	project, err := resolveSddProject(projectFlag, workspace)
	if err != nil {
		return err
	}
	if err := validateProgressBindingCoordinates(project, change); err != nil {
		return err
	}
	hc, err := hiveclient.NewFromEnv()
	if err != nil {
		return fmt.Errorf("connect to hive-daemon: %w", err)
	}
	binding, effective, err := followSddStoreBindingRedirect(ctx, hc, project, change, func(key projectidentity.Key) (sddbinding.Resolution, error) {
		return sddStoreBindingResolver(hc, key, change, workspace).Bind(ctx, key, change, mode)
	})
	var conflict *sddbinding.ConflictError
	if errors.As(err, &conflict) {
		return fmt.Errorf("sdd bind: change %q is %s %s (provenance %q); requested %s. The binding is immutable and was left unchanged: %w", change, conflictSubject(conflict.Existing), conflict.Existing.Mode(), conflict.Existing.Provenance(), conflict.Requested.Mode(), err)
	}
	if err != nil {
		return fmt.Errorf("sdd bind: %w", err)
	}

	result := sddBindOutput{Project: effective.String(), Change: change, Mode: string(binding.Mode), Provenance: binding.Provenance, Persisted: binding.Persisted}
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	_, err = fmt.Fprintf(out, "Bound %s/%s to %s  [provenance: %s, persisted: %t]\n", result.Project, result.Change, result.Mode, result.Provenance, result.Persisted)
	return err
}

// conflictSubject distinguishes a stored binding from the binding that
// existing apply progress selects for a change that is not bound yet.
func conflictSubject(existing sddbinding.Binding) string {
	if strings.HasPrefix(existing.Provenance(), "legacy:") {
		return "bound by its existing apply progress to"
	}
	return "bound to"
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

var sddCmd = &cobra.Command{
	Use:   "sdd",
	Short: "SDD phase routing and status",
}

var sddStatusCmd = &cobra.Command{
	Use:   "status [change]",
	Short: "Show SDD phase status for a change",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		withInstructions, _ := cmd.Flags().GetBool("instructions")
		project, _ := cmd.Flags().GetString("project")
		changeName := ""
		if len(args) > 0 {
			changeName = args[0]
		}
		return runSddStatus(changeName, project, asJSON, withInstructions)
	},
}

var sddContinueCmd = &cobra.Command{
	Use:   "continue [change]",
	Short: "Print the next recommended SDD phase for a change",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		project, _ := cmd.Flags().GetString("project")
		changeName := ""
		if len(args) > 0 {
			changeName = args[0]
		}
		return runSddContinue(changeName, project, asJSON)
	},
}

func init() {
	sddStatusCmd.Flags().Bool("json", false, "emit JSON output")
	sddStatusCmd.Flags().Bool("instructions", false, "include phase instructions for next recommended phase")
	sddStatusCmd.Flags().String("project", "", "hive project name (defaults to git repo directory name)")
	sddContinueCmd.Flags().Bool("json", false, "emit JSON output")
	sddContinueCmd.Flags().String("project", "", "hive project name (defaults to git repo directory name)")
	sddCmd.AddCommand(sddStatusCmd, sddContinueCmd)
}

// detectProjectName returns the project name for Hive queries.
// It uses the git repository root's directory name, falling back to the
// current working directory name when not inside a git repo.
func detectProjectName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return filepath.Base(root)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(cwd)
}

// resolveSource returns an ArtifactSource and the active store mode label.
// projectName is used when querying Hive; it is derived from detectProjectName
// when not explicitly provided.
func resolveSource(projectName string) (sddstatus.ArtifactSource, string, error) {
	contract, err := sddruntime.ResolveRuntimeStoreContract(sddruntime.StoreModeHive)
	if err != nil {
		return nil, "", fmt.Errorf("resolve store contract: %w", err)
	}

	switch contract.Mode {
	case sddruntime.StoreModeOpenSpec:
		root, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		return sddstatus.NewOpenSpecSource(root), string(contract.Mode), nil

	case sddruntime.StoreModeHybrid:
		hc, err := hiveclient.NewFromEnv()
		if err != nil {
			return nil, "", fmt.Errorf("connect to hive-daemon: %w", err)
		}
		root, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		hiveS := sddstatus.NewHiveSource(hc, projectName)
		osS := sddstatus.NewOpenSpecSource(root)
		return sddstatus.NewHybridSource(hiveS, osS), string(contract.Mode), nil

	default: // StoreModeHive
		hc, err := hiveclient.NewFromEnv()
		if err != nil {
			return nil, "", fmt.Errorf("connect to hive-daemon: %w", err)
		}
		return sddstatus.NewHiveSource(hc, projectName), string(contract.Mode), nil
	}
}

// resolveChangeName infers the change name when not provided explicitly.
// It fails if multiple active changes exist (requires explicit selection).
func resolveChangeName(ctx context.Context, src sddstatus.ArtifactSource, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	changes, err := src.ListChanges(ctx)
	if err != nil {
		return "", fmt.Errorf("list changes: %w", err)
	}
	switch len(changes) {
	case 0:
		return "", errors.New("no SDD changes found — run sdd-explore or sdd-propose first")
	case 1:
		return changes[0], nil
	default:
		return "", fmt.Errorf("multiple active changes found (%v) — specify a change name", changes)
	}
}

func buildStatus(changeName string, src sddstatus.ArtifactSource, storeMode string) (*sddstatus.ChangeStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	arts, contents, err := src.FetchArtifacts(ctx, changeName)
	if err != nil {
		return nil, fmt.Errorf("fetch artifacts: %w", err)
	}

	return sddstatus.ComputeStatus(changeName, storeMode, sddstatus.Input{
		Artifacts: arts,
		Contents:  contents,
	}), nil
}

func runSddStatus(given, projectFlag string, asJSON, withInstructions bool) error {
	project := projectFlag
	if project == "" {
		project = detectProjectName()
	}

	src, storeMode, err := resolveSource(project)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	changeName, err := resolveChangeName(ctx, src, given)
	cancel()
	if err != nil {
		return err
	}

	status, err := buildStatus(changeName, src, storeMode)
	if err != nil {
		return err
	}

	if asJSON {
		return printJSON(status)
	}
	printStatusHuman(os.Stdout, status, withInstructions)
	return nil
}

func runSddContinue(given, projectFlag string, asJSON bool) error {
	project := projectFlag
	if project == "" {
		project = detectProjectName()
	}

	src, storeMode, err := resolveSource(project)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	changeName, err := resolveChangeName(ctx, src, given)
	cancel()
	if err != nil {
		return err
	}

	status, err := buildStatus(changeName, src, storeMode)
	if err != nil {
		return err
	}

	if asJSON {
		return printJSON(status)
	}

	if status.NextRecommended == "none" || status.NextRecommended == "" {
		if len(status.BlockedReasons) > 0 {
			fmt.Fprintf(os.Stderr, "blocked:\n")
			for _, r := range status.BlockedReasons {
				fmt.Fprintf(os.Stderr, "  • %s\n", r)
			}
			return fmt.Errorf("change %q is blocked — resolve missing artifacts first", changeName)
		}
		fmt.Printf("✓ %s — all phases complete\n", changeName)
		return nil
	}

	fmt.Println(status.NextRecommended)
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printStatusHuman(w io.Writer, s *sddstatus.ChangeStatus, withInstructions bool) {
	fmt.Fprintf(w, "SDD Status: %s  [store: %s]\n\n", s.ChangeName, s.ArtifactStore)

	fmt.Fprintln(w, "  Artifacts:")
	for _, phase := range sddstatus.PhaseOrder {
		artifact := sddstatus.PhaseOutput[phase]
		state := s.Artifacts[artifact]
		icon := artifactIcon(state)
		fmt.Fprintf(w, "    %s %-20s %s\n", icon, artifact, state)
	}

	if s.TaskProgress != nil {
		fmt.Fprintf(w, "\n  Tasks: %d/%d complete", s.TaskProgress.Completed, s.TaskProgress.Total)
		if s.TaskProgress.AllDone {
			fmt.Fprint(w, " ✓")
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "\n  Phase readiness:")
	for _, phase := range sddstatus.PhaseOrder {
		dep := s.Dependencies[phase]
		icon := depIcon(dep)
		marker := ""
		if phase == s.NextRecommended {
			marker = " ←"
		}
		fmt.Fprintf(w, "    %s %-14s %s%s\n", icon, phase, dep, marker)
	}

	fmt.Fprintf(w, "\n  Next recommended: ")
	switch {
	case s.NextRecommended != "" && s.NextRecommended != "none":
		fmt.Fprintln(w, s.NextRecommended)
	case len(s.BlockedReasons) > 0:
		fmt.Fprintln(w, "blocked (see below)")
	default:
		fmt.Fprintln(w, "all phases complete ✓")
	}

	if len(s.BlockedReasons) > 0 {
		fmt.Fprintln(w, "\n  Blocked:")
		for _, r := range s.BlockedReasons {
			fmt.Fprintf(w, "    • %s\n", r)
		}
	}

	if withInstructions && s.NextRecommended != "none" && s.NextRecommended != "" {
		fmt.Fprintf(w, "\n  Run: /%s %s\n", s.NextRecommended, s.ChangeName)
	}
}

func artifactIcon(s sddstatus.ArtifactState) string {
	switch s {
	case sddstatus.ArtifactDone:
		return "✓"
	default:
		return "✗"
	}
}

func depIcon(s sddstatus.DependencyState) string {
	switch s {
	case sddstatus.DepAllDone:
		return "✓"
	case sddstatus.DepReady:
		return "→"
	default:
		return "✗"
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
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
		workingDir, err := sddWorkingDirectory(project)
		if err != nil {
			return err
		}
		changeName := ""
		if len(args) > 0 {
			changeName = args[0]
		}
		return runSddStatus(changeName, project, workingDir, asJSON, withInstructions)
	},
}

var sddContinueCmd = &cobra.Command{
	Use:   "continue [change]",
	Short: "Print the next recommended SDD phase for a change",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		project, _ := cmd.Flags().GetString("project")
		workingDir, err := sddWorkingDirectory(project)
		if err != nil {
			return err
		}
		changeName := ""
		if len(args) > 0 {
			changeName = args[0]
		}
		return runSddContinue(changeName, project, workingDir, asJSON)
	},
}

func init() {
	sddStatusCmd.Flags().Bool("json", false, "emit JSON output")
	sddStatusCmd.Flags().Bool("instructions", false, "include phase instructions for next recommended phase")
	sddStatusCmd.Flags().String("project", "", "hive project name (overrides origin repository or working-directory basename derivation)")
	sddContinueCmd.Flags().Bool("json", false, "emit JSON output")
	sddContinueCmd.Flags().String("project", "", "hive project name (overrides origin repository or working-directory basename derivation)")
	sddCmd.AddCommand(sddStatusCmd, sddContinueCmd, newSddArchiveCommand(func(root string) sddArchiver { return sddprogress.OpenSpec{Root: root} }, archiveStatus))
}

type sddArchiver interface {
	Archive(string) error
	ArchiveWithLifecycleValidation(string, func() error) error
}
type archiveStatusResolver func(root, project string) (*sddstatus.ChangeStatus, error)

func newSddArchiveCommand(open func(string) sddArchiver, statusFor archiveStatusResolver) *cobra.Command {
	var root, destination, project string
	command := &cobra.Command{Use: "archive", Short: "Archive validated OpenSpec progress", Long: "Archive validated OpenSpec progress. Run `jarvis sdd archive --root <change-root> --destination <archive-destination>`; this command accepts no positional arguments. It exits non-zero and moves nothing when lifecycle validation blocks archive.", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true, RunE: func(_ *cobra.Command, _ []string) error {
		status, err := statusFor(root, project)
		if err != nil {
			return err
		}
		if status.ChangeRoot == "" {
			status.ChangeRoot = root
		}
		if err := validateSddArchiveStatus(status, destination); err != nil {
			return err
		}
		return open(root).ArchiveWithLifecycleValidation(destination, func() error {
			current, err := statusFor(root, project)
			if err != nil {
				return err
			}
			if current.ChangeRoot == "" {
				current.ChangeRoot = root
			}
			return validateSddArchiveStatus(current, destination)
		})
	}}
	command.Flags().StringVar(&root, "root", "", "OpenSpec change root")
	command.Flags().StringVar(&destination, "destination", "", "archive destination")
	command.Flags().StringVar(&project, "project", "", "hive project name")
	_ = command.MarkFlagRequired("root")
	_ = command.MarkFlagRequired("destination")
	return command
}

func runSddArchive(archive sddArchiver, status *sddstatus.ChangeStatus, destination string) error {
	if err := validateSddArchiveStatus(status, destination); err != nil {
		return err
	}
	return archive.Archive(destination)
}

func validateSddArchiveStatus(status *sddstatus.ChangeStatus, destination string) error {
	if (status.ArtifactStore != "openspec" && status.ArtifactStore != "hybrid") || status.Artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactDone || (status.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepReady && status.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepAllDone) {
		return errors.New("sdd archive blocked until authoritative progress is complete")
	}
	if !archivePathsAllowed(status.ActionContext.AllowedEditRoots, status.ChangeRoot, destination) {
		return errors.New("sdd archive blocked: source and destination must be inside ActionContext.AllowedEditRoots")
	}
	return nil
}

func archivePathsAllowed(roots []string, source, destination string) bool {
	if source == "" || destination == "" || len(roots) == 0 {
		return false
	}
	for _, candidate := range []string{source, destination} {
		path, err := filepath.Abs(filepath.Clean(candidate))
		if err != nil {
			return false
		}
		allowed := false
		for _, root := range roots {
			root, err := filepath.Abs(filepath.Clean(root))
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(root, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

func archiveStatus(root, project string) (*sddstatus.ChangeStatus, error) {
	canonicalRoot, err := canonicalSddWorkspaceDirectory(root)
	if err != nil {
		return nil, fmt.Errorf("validate canonical OpenSpec change root: %w", err)
	}
	workspace := filepath.Dir(filepath.Dir(filepath.Dir(canonicalRoot)))
	if canonicalRoot != filepath.Join(workspace, "openspec", "changes", filepath.Base(canonicalRoot)) {
		return nil, errors.New("sdd archive requires a canonical OpenSpec change root")
	}
	project, err = resolveSddProject(project, workspace)
	if err != nil {
		return nil, err
	}
	source, store, err := resolveSourceAt(project, workspace)
	if err != nil {
		return nil, err
	}
	status, err := buildStatus(filepath.Base(canonicalRoot), source, store, []string{workspace})
	if err != nil {
		return nil, err
	}
	status.ChangeRoot = canonicalRoot
	return status, nil
}

func sddWorkingDirectory(_ string) (string, error) {
	// --project is a Hive identity alias. It must not erase the working directory
	// that determines the edit authority for this invocation.
	return os.Getwd()
}

// resolveSddProject is the shared project identity boundary for status and continue.
func resolveSddProject(projectFlag, workingDir string) (string, error) {
	if project := strings.TrimSpace(projectFlag); project != "" {
		return project, nil
	}
	return hivederive.Derive(workingDir)
}

// resolveSource returns an ArtifactSource and the active store mode label.
// projectName is used when querying Hive and must already be resolved.
func resolveSource(projectName string) (sddstatus.ArtifactSource, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	return resolveSourceAt(projectName, workingDir)
}

func resolveSourceAt(projectName, workingDir string) (sddstatus.ArtifactSource, string, error) {
	contract, err := sddruntime.ResolveRuntimeStoreContract(sddruntime.StoreModeHive)
	if err != nil {
		return nil, "", fmt.Errorf("resolve store contract: %w", err)
	}

	switch contract.Mode {
	case sddruntime.StoreModeOpenSpec:
		return sddstatus.NewOpenSpecSource(workingDir), string(contract.Mode), nil

	case sddruntime.StoreModeHybrid:
		hc, err := hiveclient.NewFromEnv()
		if err != nil {
			return nil, "", fmt.Errorf("connect to hive-daemon: %w", err)
		}
		hiveS := sddstatus.NewHiveSource(hc, projectName)
		osS := sddstatus.NewOpenSpecSource(workingDir)
		return sddstatus.NewHybridSource(hiveS, osS), string(contract.Mode), nil

	case sddruntime.StoreModeNone:
		return noneArtifactSource{}, string(contract.Mode), nil

	default: // StoreModeHive
		hc, err := hiveclient.NewFromEnv()
		if err != nil {
			return nil, "", fmt.Errorf("connect to hive-daemon: %w", err)
		}
		return sddstatus.NewHiveSource(hc, projectName), string(contract.Mode), nil
	}
}

func resolveBoundStatusSourceAt(ctx context.Context, projectName, given, workingDir string) (string, sddstatus.ArtifactSource, sddbinding.Resolution, error) {
	hc, err := hiveclient.NewFromEnv()
	if err != nil {
		return "", nil, sddbinding.Resolution{}, fmt.Errorf("connect to hive-daemon: %w", err)
	}
	hiveSource := sddstatus.NewHiveSource(hc, projectName)
	openSpecSource := sddstatus.NewOpenSpecSource(workingDir)
	changeName, explicit, err := normalizeExplicitChangeName(given)
	if err != nil {
		return "", nil, sddbinding.Resolution{}, err
	}
	if !explicit {
		changeName, err = resolveChangeNameAcrossStores(ctx, "", hiveSource, openSpecSource)
		if err != nil {
			return "", nil, sddbinding.Resolution{}, err
		}
	}

	resolver := sddbinding.LegacyResolver{
		HiveBindings:      hc,
		HiveSource:        hiveSource,
		OpenSpecSource:    openSpecSource,
		OpenSpecChangeDir: filepath.Join(workingDir, "openspec", "changes", changeName),
		ResolutionLockDir: workingDir,
	}
	binding, err := resolver.ResolveAndAdopt(ctx, projectName, changeName, initialStoreSelection())
	if err != nil {
		return "", nil, sddbinding.Resolution{}, fmt.Errorf("resolve SDD store binding: %w", err)
	}

	var source sddstatus.ArtifactSource
	switch binding.Mode {
	case sddruntime.StoreModeOpenSpec:
		source = openSpecSource
	case sddruntime.StoreModeHybrid:
		source = sddstatus.NewHybridSource(hiveSource, openSpecSource)
	case sddruntime.StoreModeNone:
		source = noneArtifactSource{}
	default:
		source = hiveSource
	}
	return changeName, source, binding, nil
}

func initialStoreSelection() sddbinding.InitialSelection {
	mode := strings.TrimSpace(os.Getenv("JARVIS_SDD_STORE_MODE"))
	if mode == "" {
		return sddbinding.InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial:default:hive"}
	}
	return sddbinding.InitialSelection{Mode: sddruntime.StoreMode(mode), Provenance: "initial:environment:JARVIS_SDD_STORE_MODE"}
}

// normalizeExplicitChangeName canonicalizes a user-supplied coordinate before
// it reaches any artifact path, binding request, or source read.
func normalizeExplicitChangeName(given string) (changeName string, explicit bool, err error) {
	if given == "" {
		return "", false, nil
	}
	changeName = strings.TrimSpace(given)
	if changeName == "" {
		return "", true, errors.New("SDD change name cannot be empty")
	}
	return changeName, true, nil
}

func resolveChangeNameAcrossStores(ctx context.Context, given string, hiveSource, openSpecSource sddstatus.ArtifactSource) (string, error) {
	if given != "" {
		return given, nil
	}
	hiveChanges, err := hiveSource.ListChanges(ctx)
	if err != nil {
		return "", fmt.Errorf("list Hive changes: %w", err)
	}
	openSpecChanges, err := openSpecSource.ListChanges(ctx)
	if err != nil {
		return "", fmt.Errorf("list OpenSpec changes: %w", err)
	}
	unique := make(map[string]struct{}, len(hiveChanges)+len(openSpecChanges))
	for _, change := range append(hiveChanges, openSpecChanges...) {
		unique[change] = struct{}{}
	}
	changes := make([]string, 0, len(unique))
	for change := range unique {
		changes = append(changes, change)
	}
	sort.Strings(changes)
	switch len(changes) {
	case 0:
		return "", errors.New("no SDD changes found — run sdd-explore or sdd-propose first")
	case 1:
		return changes[0], nil
	default:
		return "", fmt.Errorf("multiple active changes found (%v) — specify a change name", changes)
	}
}

func bindingStatus(binding sddbinding.Resolution) *sddstatus.StoreBindingStatus {
	return &sddstatus.StoreBindingStatus{Mode: string(binding.Mode), Provenance: binding.Provenance, Persisted: binding.Persisted}
}

type noneArtifactSource struct{}

func (noneArtifactSource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	return map[string]sddstatus.ArtifactState{}, map[string]string{}, nil
}

func (noneArtifactSource) ListChanges(context.Context) ([]string, error) {
	return nil, nil
}

func buildStatus(changeName string, src sddstatus.ArtifactSource, storeMode string, allowedEditRoots []string) (*sddstatus.ChangeStatus, error) {
	return buildStatusWithBinding(changeName, src, storeMode, allowedEditRoots, nil)
}

func buildStatusWithBinding(changeName string, src sddstatus.ArtifactSource, storeMode string, allowedEditRoots []string, binding *sddstatus.StoreBindingStatus) (*sddstatus.ChangeStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	arts, contents, err := src.FetchArtifacts(ctx, changeName)
	if err != nil {
		return nil, fmt.Errorf("fetch artifacts: %w", err)
	}

	input := sddstatus.Input{
		Artifacts:        arts,
		Contents:         contents,
		AllowedEditRoots: allowedEditRoots,
		StoreBinding:     binding,
	}

	return sddstatus.ComputeStatus(changeName, storeMode, input), nil
}

func validatedEditRootsForProject(_ string, workingDir string) []string {
	root, err := resolveSddWorkspaceAuthority(workingDir)
	if err != nil {
		return nil
	}
	return []string{root}
}

func runSddStatus(given, projectFlag, workingDir string, asJSON, withInstructions bool) error {
	project, err := resolveSddProject(projectFlag, workingDir)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	changeName, src, binding, err := resolveBoundStatusSourceAt(ctx, project, given, workingDir)
	cancel()
	if err != nil {
		return err
	}

	status, err := buildStatusWithBinding(changeName, src, string(binding.Mode), validatedEditRootsForProject(project, workingDir), bindingStatus(binding))
	if err != nil {
		return err
	}

	if asJSON {
		return printJSON(status)
	}
	printStatusHuman(os.Stdout, status, withInstructions)
	return nil
}

func runSddContinue(given, projectFlag, workingDir string, asJSON bool) error {
	project, err := resolveSddProject(projectFlag, workingDir)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	changeName, src, binding, err := resolveBoundStatusSourceAt(ctx, project, given, workingDir)
	cancel()
	if err != nil {
		return err
	}

	status, err := buildStatusWithBinding(changeName, src, string(binding.Mode), validatedEditRootsForProject(project, workingDir), bindingStatus(binding))
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
	fmt.Fprintf(w, "SDD Status: %s  [store: %s]\n", s.ChangeName, s.ArtifactStore)
	if s.StoreBinding != nil {
		fmt.Fprintf(w, "  Store binding: %s  [provenance: %s, persisted: %t]\n", s.StoreBinding.Mode, s.StoreBinding.Provenance, s.StoreBinding.Persisted)
	}
	fmt.Fprintln(w)

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
	case sddstatus.ArtifactPartial:
		return "…"
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

package main

import (
	"fmt"
	"os"
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/spf13/cobra"
)

type lifecycleOpts struct {
	provider string
	snapshot string
	yes      bool
	dryRun   bool
	soft     bool
	purge    bool
}

var (
	verifyCmd    = &cobra.Command{Use: "verify", Short: "Verify managed runtime integrity", RunE: runVerify}
	doctorCmd    = &cobra.Command{Use: "doctor", Short: "Plan remediation without mutating files", RunE: runDoctor}
	reconcileCmd = &cobra.Command{Use: "reconcile", Short: "Apply managed repairs for owned drift", RunE: runReconcile}
	backupCmd    = &cobra.Command{Use: "backup", Short: "Create lifecycle backup snapshot", RunE: runBackup}
	restoreCmd   = &cobra.Command{Use: "restore", Short: "Restore managed assets from snapshot", RunE: runRestore}
	uninstallCmd = &cobra.Command{Use: "uninstall", Short: "Remove managed lifecycle assets", RunE: runUninstall}
)

var uninstallLifecycle = func(engine *lifecycle.Engine, provider, mode string) (lifecycle.UninstallResult, error) {
	return engine.Uninstall(provider, mode)
}

// lifecycleDetectProviders returns the provider names the "all" fan-out should
// target, derived from the SAME detection source newLifecycleEngine uses so the
// targets are exactly the adapters the engine can serve. agent.Detect appends
// claude before opencode, so ordering is deterministic (claude then opencode)
// with no explicit sort. Overridable in tests as a seam.
var lifecycleDetectProviders = func() []string {
	detected := agent.Detect(jarvis.TemplatesFS)
	names := make([]string, 0, len(detected))
	for _, a := range detected {
		names = append(names, a.Name())
	}
	return names
}

func initLifecycleCommands() {
	for _, cmd := range []*cobra.Command{verifyCmd, doctorCmd, reconcileCmd, backupCmd, restoreCmd, uninstallCmd} {
		cmd.Flags().String("provider", "all", "provider to target: claude|opencode|all")
	}
	restoreCmd.Flags().String("snapshot", "", "snapshot id to restore")
	reconcileCmd.Flags().Bool("yes", false, "confirm lifecycle mutation")
	restoreCmd.Flags().Bool("yes", false, "confirm lifecycle mutation")
	uninstallCmd.Flags().Bool("yes", false, "confirm lifecycle mutation")
	reconcileCmd.Flags().Bool("dry-run", false, "print plan without mutating")
	uninstallCmd.Flags().Bool("dry-run", false, "print plan without mutating")
	uninstallCmd.Flags().Bool("soft", false, "unsupported in v1")
	uninstallCmd.Flags().Bool("purge", false, "unsupported in v1")
}

func runVerify(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		result, err := engine.Verify(provider)
		if err != nil {
			return err
		}
		printVerifyResult("verify: runtime integrity", result)
		return nil
	})
}
func runDoctor(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		plan, err := engine.Doctor(provider)
		if err != nil {
			return err
		}
		printDoctorPlan("doctor: read-only diagnosis", plan)
		return nil
	})
}
func runBackup(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		snapshotID, err := engine.Backup(provider, "backup")
		if err != nil {
			return err
		}
		fmt.Printf("backup snapshot created provider=%s snapshot=%s\n", provider, snapshotID)
		return nil
	})
}

func runReconcile(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	if err := validateProvider(opts.provider); err != nil {
		return err
	}
	if opts.dryRun {
		return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
			plan, err := engine.ReconcileDryRun(provider)
			if err != nil {
				return err
			}
			printDoctorPlan("dry-run: reconcile plan generated (no mutations)", plan)
			return nil
		})
	}
	if !opts.yes {
		return fmt.Errorf("reconcile requires --yes to mutate managed assets (or use --dry-run)")
	}
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		result, err := engine.Reconcile(provider)
		if err != nil {
			return err
		}
		fmt.Printf("reconcile: provider=%s applied=%d manual_required=%d skipped_non_owned=%d\n", provider, result.Applied, result.ManualRequired, len(result.SkippedNonOwned))
		return nil
	})
}

func printDoctorPlan(header string, plan lifecycle.DoctorPlan) {
	if header != "" {
		fmt.Println(header)
	}
	fmt.Printf("provider=%s status=%s read_only=%t steps=%d\n", plan.Provider, plan.Status, plan.ReadOnly, len(plan.Steps))
	for i, step := range plan.Steps {
		fmt.Printf("step=%d check_key=%s asset_id=%s reason_code=%s class=%s safety_class=%s safe_to_auto_apply=%t backup_needed=%t next_action=%s\n",
			i+1,
			step.CheckKey,
			step.AssetID,
			step.ReasonCode,
			step.Class,
			step.SafetyClass,
			step.SafeToAutoApply,
			step.BackupNeeded,
			step.NextAction,
		)
	}
}

func printVerifyResult(header string, result lifecycle.VerifyResult) {
	if header != "" {
		fmt.Println(header)
	}
	fmt.Printf("provider=%s status=%s contract=%s checks=%d\n", result.Report.Agent, result.Status, result.Report.ContractVersion, len(result.Report.Checks))
	for i, check := range result.Report.Checks {
		if check.Status == sddruntime.StatusPass {
			continue
		}
		fmt.Printf("check=%d check_key=%s status=%s class=%s expected=%s observed=%s message=%s\n",
			i+1,
			check.Key,
			check.Status,
			check.DriftClass,
			check.Expected,
			check.Observed,
			check.Message,
		)
	}
}

func runRestore(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	if err := validateProvider(opts.provider); err != nil {
		return err
	}
	if strings.TrimSpace(opts.snapshot) == "" {
		return fmt.Errorf("restore requires --snapshot")
	}
	if !opts.yes {
		return fmt.Errorf("restore requires --yes to mutate managed assets")
	}
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		_, err := engine.Restore(provider, opts.snapshot)
		return err
	})
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	if err := validateProvider(opts.provider); err != nil {
		return err
	}
	if opts.soft || opts.purge {
		return fmt.Errorf("unsupported uninstall mode: --soft/--purge are not available in v1")
	}
	if opts.dryRun {
		fmt.Println("dry-run: uninstall plan generated (no mutations)")
		return nil
	}
	if !opts.yes {
		return fmt.Errorf("uninstall requires --yes to mutate managed assets (or use --dry-run)")
	}
	if opts.provider == "all" {
		_, err := uninstallLifecycle(newLifecycleEngine(), "all", "all")
		return err
	}
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		_, err := uninstallLifecycle(engine, provider, "provider")
		return err
	})
}

func mustLifecycleOpts(cmd *cobra.Command) lifecycleOpts {
	provider := flagStringFresh(cmd, "provider", "all")
	snapshot := flagStringFresh(cmd, "snapshot", "")
	yes := flagBoolFresh(cmd, "yes", false)
	dryRun := flagBoolFresh(cmd, "dry-run", false)
	soft := flagBoolFresh(cmd, "soft", false)
	purge := flagBoolFresh(cmd, "purge", false)
	return lifecycleOpts{provider: provider, snapshot: snapshot, yes: yes, dryRun: dryRun, soft: soft, purge: purge}
}

func flagBoolFresh(cmd *cobra.Command, name string, def bool) bool {
	if !cmd.Flags().Changed(name) {
		return def
	}
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func flagStringFresh(cmd *cobra.Command, name, def string) string {
	if !cmd.Flags().Changed(name) {
		return def
	}
	v, _ := cmd.Flags().GetString(name)
	return v
}

func runPerProvider(_ *cobra.Command, provider string, run func(*lifecycle.Engine, string) error) error {
	if err := validateProvider(provider); err != nil {
		return err
	}
	engine := newLifecycleEngine()
	targets := []string{provider}
	if provider == "all" {
		targets = lifecycleDetectProviders()
	}
	for _, target := range targets {
		if err := run(engine, target); err != nil {
			return err
		}
	}
	return nil
}

func validateProvider(provider string) error {
	switch provider {
	case "claude", "opencode", "all":
		return nil
	default:
		return fmt.Errorf("invalid provider %q: use claude|opencode|all", provider)
	}
}

func newLifecycleEngine() *lifecycle.Engine {
	adapters := map[string]lifecycle.ProviderAdapter{}
	for _, a := range agent.Detect(jarvis.TemplatesFS) {
		adapters[a.Name()] = agent.NewLifecycleAdapter(a)
	}
	cwd, _ := os.Getwd()
	return lifecycle.NewEngine(lifecycle.EngineDeps{Adapters: adapters, ProjectRoot: cwd})
}

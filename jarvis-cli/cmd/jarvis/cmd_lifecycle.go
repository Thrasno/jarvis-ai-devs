package main

import (
	"fmt"
	"strings"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/lifecycle"
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
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error { _, err := engine.Verify(provider); return err })
}
func runDoctor(cmd *cobra.Command, _ []string) error {
	opts := mustLifecycleOpts(cmd)
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error { _, err := engine.Doctor(provider); return err })
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
		fmt.Println("dry-run: reconcile plan generated (no mutations)")
		return nil
	}
	if !opts.yes {
		return fmt.Errorf("reconcile requires --yes to mutate managed assets (or use --dry-run)")
	}
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		_, err := engine.Reconcile(provider)
		return err
	})
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
	mode := "provider"
	if opts.provider == "all" {
		mode = "all"
	}
	return runPerProvider(cmd, opts.provider, func(engine *lifecycle.Engine, provider string) error {
		_, err := engine.Uninstall(provider, mode)
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
		targets = []string{"claude", "opencode"}
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
	return lifecycle.NewEngine(lifecycle.EngineDeps{Adapters: adapters})
}

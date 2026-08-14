package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sync"
)

// newSyncCommand builds `jarvis sync`.
//
// It deliberately declares no flags. --dry-run in particular is not a missing
// feature: replay is the whole command, and a run that described changes
// without making them would need a second, divergent path through the applier.
//
// Cobra rejects an unknown flag on its own, but an inherited persistent flag
// such as the root command's --no-tui parses happily and would otherwise be
// accepted silently. The guard therefore refuses any flag that was set at all,
// and it runs in PreRunE, before the run seam and so before any mutation.
func newSyncCommand(run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Replay this machine's recorded Jarvis configuration",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			supplied := make([]string, 0, 1)
			cmd.Flags().Visit(func(flag *pflag.Flag) { supplied = append(supplied, "--"+flag.Name) })
			if len(supplied) == 0 {
				return nil
			}
			return fmt.Errorf("jarvis sync accepts no flags, got %s", strings.Join(supplied, ", "))
		},
		RunE: func(*cobra.Command, []string) error { return run() },
	}
}

var syncCmd = newSyncCommand(runSync)

// hiveNotSynchronizedNotice is printed by every run. `jarvis sync` used to be a
// no-op pointing at Hive's memory sync, so saying plainly what it does not do
// is part of the report rather than a footnote.
const hiveNotSynchronizedNotice = "Hive memory data was not synchronized: `jarvis sync` replays this machine's agent configuration only."

// runSync replays the desired state the last installation recorded.
//
// The migration runs first and unconditionally, before any early return: a run
// that cannot replay anything must still leave the manifest migrated, and its
// notice is already gated on a durable write inside state.Migrate.
func runSync() error {
	migration, err := state.Migrate()
	if err != nil {
		return fmt.Errorf("migrate configuration into the desired-state manifest: %w", err)
	}
	if migration.Notice != "" {
		fmt.Println(migration.Notice)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	manifest, err := state.Load()
	if err != nil {
		return fmt.Errorf("load the desired-state manifest: %w", err)
	}
	if err := manifest.ValidateForReplay(); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	input, err := replayInput(home, manifest, cfg)
	if err != nil {
		return err
	}
	plan, err := sync.BuildPlan(sync.PlanInputFor(input))
	if err != nil {
		return err
	}
	// The cloud portion reads one file and writes nothing, so it is decided
	// before the report and never gates it: `jarvis login` owns what it names.
	cloud := sync.CloudManualAction(home, manifest.Scope)
	result, runErr := sync.Run(sync.RunInput{
		Plan:   plan,
		Apply:  sync.ApplyInput{Runner: sync.NewRunner(input), Targets: sync.TargetsFor(input)},
		Backup: lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
		// Bookkeeping stays nil until something produces the digest of the asset
		// set a run replayed. Nothing does yet, so there is nothing to record.
	})
	fmt.Print(renderSyncReport(manifest, result, cloud, runErr))
	return syncExit(result.Report, runErr)
}

// syncExit converts a completed replay into the command's exit status.
//
// It takes no cloud argument on purpose: an unusable cloud portion is reported,
// never raised, so there is no shape here through which it could abort a local
// replay that otherwise converged.
func syncExit(report sync.Report, runErr error) error {
	if runErr != nil {
		return runErr
	}
	if report.ExitCode() == 0 {
		return nil
	}
	return errors.New("jarvis sync did not converge every configured agent; see the per-agent outcomes above")
}

// replayInput builds the one value both halves of a replay pass read. Deriving
// it twice would make the planner's desired digests and the installer's writes
// disagree and report drift forever. Nothing on this path calls config.Save,
// whose bridge takes the fail-fast manifest lock and would deadlock sync
// against itself.
func replayInput(home string, manifest *state.State, cfg *config.AppConfig) (sync.ReplayInput, error) {
	skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		return sync.ReplayInput{}, fmt.Errorf("open the embedded skill tree: %w", err)
	}
	resolved, err := persona.ResolveProfile(jarvis.PersonaFS, manifest.Persona)
	if err != nil {
		return sync.ReplayInput{}, fmt.Errorf("resolve persona %q: %w", manifest.Persona, err)
	}
	catalog, err := skills.ListSkills(jarvis.SkillsFS)
	if err != nil {
		return sync.ReplayInput{}, fmt.Errorf("list the embedded skill catalog: %w", err)
	}
	// The manifest is the authority: these IDs render the instruction file's
	// Skills section and drive the planner's digests and the installer's writes.
	recorded := make(map[string]bool, len(manifest.Skills))
	for _, id := range manifest.Skills {
		recorded[id] = true
	}
	skillInfos := make([]config.SkillInfo, 0, len(manifest.Skills))
	for _, entry := range catalog {
		if recorded[entry.ID] {
			skillInfos = append(skillInfos, config.SkillInfo{Name: entry.Name, Description: entry.Description, Trigger: entry.Trigger})
		}
	}
	installed := make(map[string]agent.Agent)
	for _, detected := range agent.Detect(jarvis.TemplatesFS) {
		installed[strings.ToLower(strings.TrimSpace(detected.Name()))] = detected
	}
	return sync.ReplayInput{
		Root:      home,
		State:     manifest,
		Templates: jarvis.TemplatesFS,
		SkillsFS:  skillsSubFS,
		HooksFS:   jarvis.HooksFS,
		AgentsFS:  agentsSubFS,
		Config:    cfg,
		Skills:    skillInfos,
		Layer1:    config.Layer1Content(),
		Layer2:    persona.RenderLayer2(resolved.Preset),
		Resolve: func(id string) (agent.Agent, bool) {
			found, ok := installed[strings.ToLower(strings.TrimSpace(id))]
			return found, ok
		},
		MCPDeps: agentapply.MCPDeps{
			NewExecutor:    func() agentapply.MCPExecutor { return agent.NewProductionExecutor() },
			HiveDaemonPath: agent.HiveDaemonBinaryPath,
		},
	}, nil
}

// agentsSubFS resolves the embedded agent-definition tree for one agent ID.
// Claude installs file-based definitions from embed/agents/claude; the
// JSON-config platforms have no such tree, which is a nil FS, not an error.
func agentsSubFS(agentID string) (fs.FS, error) {
	if strings.ToLower(strings.TrimSpace(agentID)) != "claude" {
		return nil, nil
	}
	return fs.Sub(jarvis.AgentsFS, "embed/agents/claude")
}

// renderSyncReport is the whole user-visible outcome of a run: the desired
// state replayed, the recovery point, every path that moved, each agent's
// outcome, and the verification result. It never prints credentials -- the
// cloud line carries no contents of ~/.jarvis/sync.json.
func renderSyncReport(manifest *state.State, result sync.RunResult, cloud string, runErr error) string {
	var out strings.Builder
	agentIDs := make([]string, 0, len(manifest.InstalledAgents))
	for _, configured := range manifest.InstalledAgents {
		agentIDs = append(agentIDs, configured.ID)
	}
	statusline := "undecided (never asked)"
	if manifest.Statusline.Decided {
		if statusline = "disabled"; manifest.Statusline.Enabled {
			statusline = "enabled"
		}
	}
	fmt.Fprintf(&out, "jarvis %s | state.yaml schema %d\n", sddruntime.DefaultContract().JarvisVersion, manifest.SchemaVersion)
	fmt.Fprintf(&out, "agents: %s | skills %d | persona %s | statusline %s\n",
		strings.Join(agentIDs, ", "), len(manifest.Skills), manifest.Persona, statusline)
	fmt.Fprintf(&out, "model assignments: aliases %d, claude %d, opencode %d\n",
		len(manifest.PhaseModels.Aliases), len(manifest.PhaseModels.Claude), len(manifest.PhaseModels.OpenCode))
	if result.Backup.SnapshotID != "" {
		fmt.Fprintf(&out, "backup snapshot: %s\n", result.Backup.SnapshotID)
	}
	if cloud != "" {
		fmt.Fprintln(&out, cloud)
	}
	// A nil Changed means the closing measurement never ran, which every failure
	// after the applier produces. Reporting that as a measured zero would say
	// nothing changed about the one case where something just did, so the two are
	// told apart: every path that measures leaves a non-nil list behind, even an
	// empty one, so nil is a reliable marker rather than a guess.
	if result.Report.Changed == nil {
		fmt.Fprintln(&out, "changed paths: not measured (the run ended before the closing measurement; this is not evidence that nothing changed)")
	} else {
		fmt.Fprintf(&out, "changed paths: %d\n", len(result.Report.Changed))
		for _, path := range result.Report.Changed {
			fmt.Fprintf(&out, "  %s\n", path)
		}
	}
	for _, outcome := range result.Report.Agents {
		if outcome.Converged {
			fmt.Fprintf(&out, "  %s: converged\n", outcome.Agent)
			continue
		}
		fmt.Fprintf(&out, "  %s: failed at %s: %v\n", outcome.Agent, outcome.FailedAt, outcome.Err)
	}
	if runErr != nil {
		fmt.Fprintf(&out, "verification: failed: %v\n", runErr)
	} else {
		fmt.Fprintln(&out, "verification: passed")
	}
	if runErr == nil && result.Report.Converged() && len(result.Report.Changed) == 0 {
		fmt.Fprintln(&out, "this machine is already current; nothing was changed.")
	}
	fmt.Fprintln(&out, hiveNotSynchronizedNotice)
	return out.String()
}

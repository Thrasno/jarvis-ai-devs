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
// accepted silently. The guard therefore refuses any flag that was set at all.
//
// It lives inside RunE rather than in PreRunE, because PreRunE only runs for
// callers that reach the command through cobra's dispatch. RunE is a plain
// field, and this binary's own tests call it directly; a guard those callers
// skip is not a guard on the command, only on one way of invoking it. Cobra's
// parsed flag values also outlive the invocation that set them, so a later
// direct call inherits a flag nobody passed to it. Refusing first thing inside
// the run keeps the check ahead of the run seam, and so ahead of any mutation.
func newSyncCommand(run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Replay this machine's recorded Jarvis configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			supplied := make([]string, 0, 1)
			cmd.Flags().Visit(func(flag *pflag.Flag) { supplied = append(supplied, "--"+flag.Name) })
			if len(supplied) > 0 {
				return fmt.Errorf("jarvis sync accepts no flags, got %s", strings.Join(supplied, ", "))
			}
			return run()
		},
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
		// The planner already names the command that repairs a machine recording
		// no agents, but this guard blocks before BuildPlan ever runs. Surfacing
		// the bare precondition here would leave the user correctly stopped and
		// with nothing to type.
		if errors.Is(err, state.ErrNoInstalledAgents) {
			return sync.ErrNoConfiguredAgents
		}
		return err
	}
	input, expansion, err := replayInput(home, manifest)
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
		Plan:        plan,
		Apply:       sync.ApplyInput{Runner: sync.NewRunner(input), Targets: sync.TargetsFor(input)},
		Backup:      lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
		Bookkeeping: &sync.Bookkeeping{ManagedAssetDigest: sync.ManagedAssetDigest(plan), ZohoExpansion: expansion},
	})
	fmt.Print(renderSyncReport(input.State, result, cloud, runErr))
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
// disagree and report drift forever. Nothing on this path reads or writes
// config.yaml: the manifest carries every replay field, and config.Save's bridge
// takes the fail-fast manifest lock and would deadlock sync against itself.
func replayInput(home string, manifest *state.State) (sync.ReplayInput, *sync.ZohoExpansion, error) {
	skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		return sync.ReplayInput{}, nil, fmt.Errorf("open the embedded skill tree: %w", err)
	}
	resolved, err := persona.ResolveProfile(jarvis.PersonaFS, manifest.Persona)
	if err != nil {
		return sync.ReplayInput{}, nil, fmt.Errorf("resolve persona %q: %w", manifest.Persona, err)
	}
	catalog, err := skills.ListSkills(jarvis.SkillsFS)
	if err != nil {
		return sync.ReplayInput{}, nil, fmt.Errorf("list the embedded skill catalog: %w", err)
	}
	pack := skills.NewZohoPack(catalog)
	expanded, candidates, eligible := pack.Expand(manifest.Skills)
	copy := *manifest
	copy.Skills = expanded
	var expansion *sync.ZohoExpansion
	if eligible && len(candidates) > 0 {
		expansion = &sync.ZohoExpansion{Pack: pack, CandidateIDs: candidates}
	}
	// The copied state is the authority for one replay: these IDs render the
	// Skills section and drive the planner's digests and the installer's writes.
	recorded := make(map[string]bool, len(copy.Skills))
	for _, id := range copy.Skills {
		recorded[id] = true
	}
	skillInfos := make([]config.SkillInfo, 0, len(copy.Skills))
	for _, entry := range catalog {
		if recorded[entry.ID] {
			skillInfos = append(skillInfos, config.SkillInfo{Name: entry.Name, Description: entry.Description, Trigger: entry.Trigger})
		}
	}
	installed := make(map[string]agent.Agent)
	for _, detected := range agent.Detect(jarvis.TemplatesFS) {
		installed[normalizeAgentID(detected.Name())] = detected
	}
	return sync.ReplayInput{
		Root:      home,
		State:     &copy,
		Templates: jarvis.TemplatesFS,
		SkillsFS:  skillsSubFS,
		HooksFS:   jarvis.HooksFS,
		AgentsFS:  agentsSubFS,
		Skills:    skillInfos,
		Layer1:    config.Layer1Content(),
		Layer2:    persona.RenderLayer2(resolved.Preset),
		Profile:   resolved.Preset,
		Resolve: func(id string) (agent.Agent, bool) {
			found, ok := installed[normalizeAgentID(id)]
			return found, ok
		},
		MCPDeps: agentapply.MCPDeps{
			NewExecutor:    func() agentapply.MCPExecutor { return agent.NewProductionExecutor() },
			HiveDaemonPath: agent.HiveDaemonBinaryPath,
		},
	}, expansion, nil
}

// normalizeAgentID is the one spelling rule this command applies to an agent
// identifier, and it exists so that every site applies the same one. Detection
// keys its map with it and resolution looks up with it; written out twice
// instead, the two would agree only by convention, and the first change to
// either copy would silently desynchronise them -- an agent recorded in the
// manifest would resolve to nothing and be refused as not installed, with
// neither the compiler nor a type able to point at the mismatch.
func normalizeAgentID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// agentsSubFS resolves the embedded agent-definition tree for one agent ID.
// Claude installs file-based definitions from embed/agents/claude; the
// JSON-config platforms have no such tree, which is a nil FS, not an error.
func agentsSubFS(agentID string) (fs.FS, error) {
	if normalizeAgentID(agentID) != "claude" {
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
		} else {
			fmt.Fprintf(&out, "  %s: failed at %s: %v\n", outcome.Agent, outcome.FailedAt, outcome.Err)
		}
		// Only when the diff was never measured. A measured run already names
		// every path that moved, which answers the same question better; an
		// unmeasured one can name none, and the components each agent actually
		// completed are then the whole of what this run honestly knows. They are
		// not a substitute list of "possibly modified" paths, which would be a
		// fresh claim rather than a measurement.
		if result.Report.Changed == nil {
			completed := "none"
			if len(outcome.Completed) > 0 {
				completed = strings.Join(outcome.Completed, ", ")
			}
			fmt.Fprintf(&out, "    components completed: %s\n", completed)
		}
	}
	if result.Verified || runErr == nil {
		fmt.Fprintln(&out, "verification: passed")
		if result.Verified && runErr != nil {
			fmt.Fprintf(&out, "state persistence: failed: %v\n", runErr)
		}
	} else if runErr != nil {
		fmt.Fprintf(&out, "verification: failed: %v\n", runErr)
	}
	for _, id := range result.AddedSkillIDs {
		fmt.Fprintf(&out, "zoho skill added to desired state: %s\n", id)
	}
	if runErr == nil && result.Report.Converged() && len(result.Report.Changed) == 0 {
		fmt.Fprintln(&out, "this machine is already current; nothing was changed.")
	}
	fmt.Fprintln(&out, hiveNotSynchronizedNotice)
	return out.String()
}

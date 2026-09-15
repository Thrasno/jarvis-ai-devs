package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// mutatingRunner writes to a real path on its first component, so a backup taken
// after it would archive the post-mutation bytes and be caught.
type mutatingRunner struct {
	*recordingRunner
	path    string
	content []byte
}

func (r *mutatingRunner) ApplyModels(target AgentTarget) error {
	if err := os.WriteFile(r.path, r.content, 0o644); err != nil {
		return err
	}
	return r.recordingRunner.ApplyModels(target)
}

func planWithTrackedFile(t *testing.T, home, content string) (Plan, string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tracked := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(tracked, []byte(content), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	return Plan{Tracked: []TrackedPath{{Agent: "claude", Path: tracked, Mode: ManagedFileMode}}}, tracked
}

// The backup is only a recovery path if it holds what was on disk BEFORE replay
// touched it. PR 4b discards a managed CLAUDE.md carrying no Jarvis sentinels
// and renders it fresh, so the archived pre-mutation bytes are the whole answer
// to "where did my file go".
func TestRemoveManagedSkillTreeDeletesOnlyValidatedManagedTree(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	managed := filepath.Join(skillsRoot, "retired-skill", "references", "notes.md")
	unowned := filepath.Join(skillsRoot, "team-notes", "SKILL.md")
	shared := filepath.Join(skillsRoot, "_shared", "common.md")
	for _, path := range []string{managed, unowned, shared} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	evidence := snapshotOrFail(t, []TrackedPath{{Path: filepath.Join(skillsRoot, "retired-skill")}}).states[filepath.Join(skillsRoot, "retired-skill")]
	if err := removeManagedSkillTreeWithEvidence(root, skillsRoot, "retired-skill", evidence); err != nil {
		t.Fatalf("remove managed skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "retired-skill")); !os.IsNotExist(err) {
		t.Fatalf("retired tree remains: %v", err)
	}
	for _, path := range []string{unowned, shared} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unowned path %s was changed: %v", path, err)
		}
	}
	for _, id := range []string{"", "..", "a/b", "_shared"} {
		if err := removeManagedSkillTreeWithEvidence(root, skillsRoot, id, fileState{}); err == nil {
			t.Fatalf("unsafe ID %q was accepted", id)
		}
	}
}

func TestRemoveManagedSkillTreeRejectsChangesAfterItsSnapshot(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "late file", mutate: func(tree string) error { return os.WriteFile(filepath.Join(tree, "late.md"), []byte("late"), 0o644) }},
		{name: "modified file", mutate: func(tree string) error {
			return os.WriteFile(filepath.Join(tree, "SKILL.md"), []byte("changed"), 0o644)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			skillsRoot := filepath.Join(root, "skills")
			tree := filepath.Join(skillsRoot, "retired-skill")
			file := filepath.Join(tree, "SKILL.md")
			writeFile(t, file, "archived")
			before := snapshotOrFail(t, []TrackedPath{{Path: tree}})
			if err := tt.mutate(tree); err != nil {
				t.Fatalf("mutate: %v", err)
			}
			if err := removeManagedSkillTreeWithEvidence(root, skillsRoot, "retired-skill", before.states[tree]); err == nil {
				t.Fatal("deletion succeeded after the archived tree changed")
			}
			if _, err := os.Stat(file); err != nil {
				t.Fatalf("tree was mutated on rejected deletion: %v", err)
			}
		})
	}
}

func TestRemoveManagedSkillTreeRejectsASymlinkedSkillsAncestor(t *testing.T) {
	agentRoot := t.TempDir()
	skillsRoot := filepath.Join(agentRoot, ".claude", "skills")
	externalRoot := t.TempDir()
	externalSkill := filepath.Join(externalRoot, "skills", "retired-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(externalSkill), 0o755); err != nil {
		t.Fatalf("mkdir external skill: %v", err)
	}
	if err := os.WriteFile(externalSkill, []byte("unowned"), 0o644); err != nil {
		t.Fatalf("write external skill: %v", err)
	}
	if err := os.Symlink(externalRoot, filepath.Join(agentRoot, ".claude")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := removeManagedSkillTreeWithEvidence(agentRoot, skillsRoot, "retired-skill", fileState{}); err == nil {
		t.Fatal("a symlinked skills ancestor was accepted for deletion")
	}
	if _, err := os.Stat(externalSkill); err != nil {
		t.Fatalf("symlink target was changed: %v", err)
	}
}

func TestRemoveManagedSkillTreeFailsClosedWhenIntermediateAncestorChangesDuringAcquisition(t *testing.T) {
	root := t.TempDir()
	agentRoot := filepath.Join(root, "agent")
	skillsRoot := filepath.Join(agentRoot, ".claude", "skills")
	managed := filepath.Join(skillsRoot, "retired-skill", "SKILL.md")
	external := filepath.Join(agentRoot, "unowned-claude", "skills", "retired-skill", "SKILL.md")
	for _, path := range []string{managed, external} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(managed, []byte("managed"), 0o644); err != nil {
		t.Fatalf("write managed file: %v", err)
	}
	const victim = "unowned intermediate ancestor"
	if err := os.WriteFile(external, []byte(victim), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}

	err := removeManagedSkillTreeAfterAncestorLstat(agentRoot, skillsRoot, "retired-skill", func(component string) error {
		if component != ".claude" {
			return nil
		}
		if err := os.Rename(filepath.Join(agentRoot, ".claude"), filepath.Join(agentRoot, "parked-claude")); err != nil {
			return err
		}
		if err := os.Symlink("unowned-claude", filepath.Join(agentRoot, ".claude")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return nil
	})
	if err == nil {
		t.Fatal("deletion succeeded after an intermediate ancestor changed during acquisition")
	}
	if got, readErr := os.ReadFile(external); readErr != nil || string(got) != victim {
		t.Fatalf("external intermediate-ancestor victim = %q, %v; want %q", got, readErr, victim)
	}
}

func TestRemoveManagedSkillTreeFailsClosedWhenSkillsRootChangesDuringAcquisition(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	managed := filepath.Join(skillsRoot, "retired-skill", "SKILL.md")
	external := filepath.Join(root, "unowned-skills", "retired-skill", "SKILL.md")
	for _, path := range []string{managed, external} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(managed, []byte("managed"), 0o644); err != nil {
		t.Fatalf("write managed file: %v", err)
	}
	const victim = "unowned skill root"
	if err := os.WriteFile(external, []byte(victim), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}

	err := removeManagedSkillTreeAfterAcquisition(root, skillsRoot, "retired-skill", func() error {
		if err := os.Rename(skillsRoot, filepath.Join(root, "parked-skills")); err != nil {
			return err
		}
		if err := os.Symlink("unowned-skills", skillsRoot); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return nil
	}, nil)
	if err == nil {
		t.Fatal("deletion succeeded after the skills root changed during acquisition")
	}
	if got, readErr := os.ReadFile(external); readErr != nil || string(got) != victim {
		t.Fatalf("external skills-root victim = %q, %v; want %q", got, readErr, victim)
	}
}

func TestRemoveManagedSkillTreeFailsClosedWhenSkillEntryChangesDuringAcquisition(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	managedDir := filepath.Join(skillsRoot, "retired-skill")
	managed := filepath.Join(managedDir, "SKILL.md")
	external := filepath.Join(skillsRoot, "unowned-skill", "SKILL.md")
	for _, path := range []string{managed, external} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(managed, []byte("managed"), 0o644); err != nil {
		t.Fatalf("write managed file: %v", err)
	}
	const victim = "unowned skill entry"
	if err := os.WriteFile(external, []byte(victim), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}

	err := removeManagedSkillTreeAfterAcquisition(root, skillsRoot, "retired-skill", nil, func() error {
		if err := os.Rename(managedDir, filepath.Join(skillsRoot, "parked-retired-skill")); err != nil {
			return err
		}
		if err := os.Symlink("unowned-skill", managedDir); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return nil
	})
	if err == nil {
		t.Fatal("deletion succeeded after the skill entry changed during acquisition")
	}
	if got, readErr := os.ReadFile(external); readErr != nil || string(got) != victim {
		t.Fatalf("external skill-entry victim = %q, %v; want %q", got, readErr, victim)
	}
}

func TestRemoveManagedSkillTreeFailsClosedWhenAnInventoriedDirectoryBecomesAnExternalSymlink(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	skillDir := filepath.Join(skillsRoot, "retired-skill")
	inventoriedDir := filepath.Join(skillDir, "references")
	inventoriedFile := filepath.Join(inventoriedDir, "notes.md")
	externalFile := filepath.Join(skillsRoot, "unowned", "notes.md")
	parkedDir := filepath.Join(skillsRoot, "parked-references")
	for _, path := range []string{inventoriedFile, externalFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(inventoriedFile, []byte("managed"), 0o644); err != nil {
		t.Fatalf("write inventoried file: %v", err)
	}
	const victim = "unowned victim"
	if err := os.WriteFile(externalFile, []byte(victim), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}

	err := removeManagedSkillTreeAfterInventory(root, skillsRoot, "retired-skill", func() error {
		if err := os.Rename(inventoriedDir, parkedDir); err != nil {
			return err
		}
		return os.Symlink(filepath.Join("..", "unowned"), inventoriedDir)
	})
	if err == nil {
		t.Fatal("deletion succeeded after an inventoried directory became an external symlink")
	}
	got, readErr := os.ReadFile(externalFile)
	if readErr != nil {
		t.Fatalf("read external victim: %v", readErr)
	}
	if string(got) != victim {
		t.Fatalf("external victim = %q, want %q", got, victim)
	}
}

func TestRemoveManagedSkillTreeDoesNotSweepAChildAddedAfterInventory(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	skillDir := filepath.Join(skillsRoot, "retired-skill")
	managed := filepath.Join(skillDir, "SKILL.md")
	late := filepath.Join(skillDir, "late-arrival.md")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(managed, []byte("managed"), 0o644); err != nil {
		t.Fatalf("write managed file: %v", err)
	}

	err := removeManagedSkillTreeAfterInventory(root, skillsRoot, "retired-skill", func() error {
		return os.WriteFile(late, []byte("late"), 0o644)
	})
	if err == nil {
		t.Fatal("deletion swept a child introduced after inventory")
	}
	if got, readErr := os.ReadFile(late); readErr != nil || string(got) != "late" {
		t.Fatalf("late child = %q, %v; want untouched late child", got, readErr)
	}
	if _, statErr := os.Stat(skillDir); statErr != nil {
		t.Fatalf("skill directory was removed despite late child: %v", statErr)
	}
}

func TestRun_ArchivesTrackedPathsAsTheyWereBeforeTheFirstMutation(t *testing.T) {
	home := t.TempDir()
	plan, tracked := planWithTrackedFile(t, home, "hand-written notes")
	store := lifecycle.NewBackupStore(home)
	runner := &mutatingRunner{recordingRunner: &recordingRunner{}, path: tracked, content: []byte("rendered fresh")}

	result, err := Run(RunInput{
		Plan:   plan,
		Apply:  ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}},
		Backup: store.CreateSnapshotOfTargets,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got, err := os.ReadFile(tracked); err != nil || string(got) != "rendered fresh" {
		t.Fatalf("replay must still mutate the tracked path: got %q err %v", got, err)
	}
	if !result.Report.Converged() {
		t.Fatalf("report should converge: %+v", result.Report)
	}
	if result.Backup.SourceOperation != BackupSourceOperation {
		t.Fatalf("source operation = %q, want %q", result.Backup.SourceOperation, BackupSourceOperation)
	}
	if len(result.Backup.Entries) != 1 || result.Backup.Entries[0].Path != tracked {
		t.Fatalf("manifest must cover the tracked path: %#v", result.Backup.Entries)
	}
	before := sha256.Sum256([]byte("hand-written notes"))
	if got := result.Backup.Entries[0].Checksum; got != hex.EncodeToString(before[:]) {
		t.Fatalf("manifest recorded post-mutation content: checksum %q", got)
	}
	// The archive itself, not just the manifest, must hold the original bytes.
	if err := store.ValidateSnapshot(result.Backup); err != nil {
		t.Fatalf("archive does not match the pre-mutation checksums: %v", err)
	}
}

func TestRun_PartialApplyDoesNotPersistZohoExpansion(t *testing.T) {
	home := t.TempDir()
	seedZohoManifest(t, home, "zoho-deluge")

	tracked := filepath.Join(home, ".claude", "CLAUDE.md")
	const desired = "claude instructions\n"
	plan := Plan{Tracked: []TrackedPath{{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(desired))}}}
	runner := &desiredStateRunner{
		recordingRunner: &recordingRunner{failAt: map[string]error{"claude/" + ComponentMCPs: errors.New("MCP write failed")}},
		writes:          map[string][]plannedWrite{"claude": {{path: tracked, body: desired, mode: 0o644}}},
	}
	locked := 0
	pack := skills.NewZohoPack([]skills.Skill{{ID: "zoho-deluge"}, {ID: "zoho-books"}})
	result, err := Run(RunInput{
		Plan:   plan,
		Apply:  ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}},
		Backup: lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
		Bookkeeping: &Bookkeeping{
			ZohoExpansion: &ZohoExpansion{Pack: pack, CandidateIDs: []string{"zoho-books"}},
			Lock: func(critical func() error) error {
				locked++
				return critical()
			},
		},
	})
	if err == nil {
		t.Fatal("partial apply must fail final verification")
	}
	if result.Report.Converged() || result.Verified || locked != 0 || len(result.AddedSkillIDs) != 0 {
		t.Fatalf("partial apply result = %+v, verified=%t locks=%d additions=%v; want unverified non-convergence without persistence", result.Report, result.Verified, locked, result.AddedSkillIDs)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if want := []string{"zoho-deluge"}; !reflect.DeepEqual(loaded.Skills, want) {
		t.Fatalf("skills = %v, want pre-run state %v", loaded.Skills, want)
	}
}

func TestRun_BackupFailureBlocksEveryMutation(t *testing.T) {
	home := t.TempDir()
	plan, tracked := planWithTrackedFile(t, home, "hand-written notes")
	// Both agents this run would mutate are tracked, so the pair is coherent and
	// the backup is the only thing that fails.
	plan.Tracked = append(plan.Tracked, TrackedPath{
		Agent: "opencode",
		Path:  filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		Mode:  ManagedFileMode,
	})
	boom := errors.New("backup archive is not writable")
	runner := &recordingRunner{}

	result, err := Run(RunInput{
		Plan:  plan,
		Apply: ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}, {ID: "opencode"}}},
		Backup: func(string, []lifecycle.BackupTarget) (lifecycle.BackupManifest, error) {
			return lifecycle.BackupManifest{}, boom
		},
	})

	if err == nil {
		t.Fatal("a failed backup must be reported, not swallowed")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("reported failure = %v, want it to wrap %v", err, boom)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no component may run without a backup, got calls %v", runner.calls)
	}
	if len(result.Report.Agents) != 0 {
		t.Fatalf("a blocked run reports no agent outcome: %+v", result.Report.Agents)
	}
	if got, err := os.ReadFile(tracked); err != nil || string(got) != "hand-written notes" {
		t.Fatalf("tracked path was mutated after a failed backup: got %q err %v", got, err)
	}
}

// D1, over the combination that actually exercises it: the backup succeeded and
// an agent then failed. Run still returns no error, because a per-agent failure
// is the report's own content and the command derives its exit status from
// there. Raising it here would claim the replay pass itself broke, which the
// recovery point and the sibling agent's outcome both disprove.
func TestRun_ReportsAPerAgentFailureWithoutRaisingItWhenTheBackupSucceeded(t *testing.T) {
	home := t.TempDir()
	plan, _ := planWithTrackedFile(t, home, "hand-written notes")
	boom := errors.New("native MCP replacement failed")
	runner := &recordingRunner{failAt: map[string]error{"claude/" + ComponentMCPs: boom}}

	result, err := runReportingFailures(home, plan, runner, "claude")

	if err != nil {
		t.Fatalf("a per-agent failure must be reported, not raised: %v", err)
	}
	if result.Backup.SnapshotID == "" {
		t.Fatal("the backup succeeded, so the report must still name its recovery point")
	}
	if result.Report.Converged() {
		t.Fatalf("a failed agent must not converge the run: %+v", result.Report)
	}
	if len(result.Report.Agents) != 1 || result.Report.Agents[0].FailedAt != ComponentMCPs {
		t.Fatalf("the report must name the component that failed: %+v", result.Report.Agents)
	}
	if !errors.Is(result.Report.Agents[0].Err, boom) {
		t.Fatalf("agent error = %v, want it to wrap %v", result.Report.Agents[0].Err, boom)
	}
	if result.Report.ExitCode() == 0 {
		t.Fatal("the exit status is derived from the report, so it must be non-zero")
	}
}

// A failure after the applier ran keeps everything the run already knows.
//
// The changed-path diff is deliberately not among it: nothing measured it, and
// a list of paths that were "possibly modified" would be a fresh claim rather
// than a measurement. What the run does know is which agents executed which
// components, and that is what the report must still carry so an operator
// deciding whether to restore the snapshot has something to decide with.
func TestRun_KeepsTheExecutedComponentOutcomesWhenTheClosingMeasurementFails(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	plan := Plan{Tracked: []TrackedPath{{
		Agent:    "claude",
		Path:     settings,
		Mode:     ManagedFileMode,
		Semantic: &ManagedJSON{Fragments: map[string]any{"outputStyle": "neutral"}},
	}}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: settings, body: "not json at all\n", mode: 0o644}},
	}}

	result, err := runReportingFailures(home, plan, runner, "claude")

	if err == nil {
		t.Fatal("an undecodable managed JSON document must fail the closing measurement")
	}
	if result.Report.Changed != nil {
		t.Fatalf("an unmeasured diff must stay nil rather than claim paths, got %v", result.Report.Changed)
	}
	if len(result.Report.Agents) != 1 {
		t.Fatalf("the applier's outcome must survive the failure: %+v", result.Report.Agents)
	}
	if !reflect.DeepEqual(result.Report.Agents[0].Completed, orderedComponentIDs) {
		t.Fatalf("completed components = %v, want the whole order %v",
			result.Report.Agents[0].Completed, orderedComponentIDs)
	}
}

// A missing backup seam must read as "cannot proceed", never as "no backup
// needed".
func TestRun_RefusesToMutateWithoutABackupSeam(t *testing.T) {
	runner := &recordingRunner{}

	_, err := Run(RunInput{Apply: ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}}})

	if !errors.Is(err, ErrNoBackup) {
		t.Fatalf("err = %v, want %v", err, ErrNoBackup)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no component may run, got calls %v", runner.calls)
	}
}

// Plan and Apply arrive as two independent fields, and nothing about their
// shapes forces them to describe the same run. A caller that plans one agent's
// paths and applies another's defeats the archive completely: the applier still
// mutates, and the snapshot holds nothing it overwrote. The pair is therefore
// cross-checked before the backup is taken and before any component runs.
func TestRun_RefusesToMutateAnAgentThePlanDoesNotTrack(t *testing.T) {
	home := t.TempDir()
	plan, tracked := planWithTrackedFile(t, home, "hand-written notes")
	runner := &recordingRunner{}
	backups := 0

	result, err := Run(RunInput{
		Plan:  plan,
		Apply: ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}, {ID: "opencode"}}},
		Backup: func(string, []lifecycle.BackupTarget) (lifecycle.BackupManifest, error) {
			backups++
			return lifecycle.BackupManifest{}, nil
		},
	})

	if !errors.Is(err, ErrUnprotectedAgent) {
		t.Fatalf("err = %v, want %v", err, ErrUnprotectedAgent)
	}
	if !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("err = %q, want it to name the agent the plan does not track", err)
	}
	if backups != 0 || len(runner.calls) != 0 {
		t.Fatalf("a mismatched pair must not back up or apply anything: %d backups, calls %v", backups, runner.calls)
	}
	if len(result.Report.Agents) != 0 {
		t.Fatalf("a refused run reports no agent outcome: %+v", result.Report.Agents)
	}
	if got, err := os.ReadFile(tracked); err != nil || string(got) != "hand-written notes" {
		t.Fatalf("tracked path was mutated by a refused run: got %q err %v", got, err)
	}
}

// The other half of the cross-check: the pair production actually builds must
// pass it. Both halves are projected from one manifest, so a guard that refused
// them would block every real `jarvis sync` rather than a mismatched caller.
func TestRun_AcceptsThePlanAndTargetsProjectedFromOneManifest(t *testing.T) {
	in := ReplayInput{
		Root: t.TempDir(),
		State: replayableState(
			state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md"},
			state.Agent{ID: "opencode", InstructionsPath: ".config/opencode/AGENTS.md"},
		),
		Templates: jarvis.TemplatesFS,
	}

	plan, err := BuildPlan(PlanInputFor(in))
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := protectsEveryTarget(plan, TargetsFor(in)); err != nil {
		t.Fatalf("the manifest's own plan and targets must pair: %v", err)
	}
}

// The single-list rule: backup coverage and the idempotency diff read the same
// Plan.Tracked, so neither can quietly stop covering what the other measures.
func TestBackupTargets_ProjectsThePlansOwnTrackedList(t *testing.T) {
	plan := Plan{Tracked: []TrackedPath{
		{Path: "/home/dev/.claude/CLAUDE.md", Mode: ManagedFileMode},
		{Path: "/home/dev/.claude/statusline-command.sh", Mode: ManagedExecutableMode},
	}}

	want := []lifecycle.BackupTarget{
		{Path: "/home/dev/.claude/CLAUDE.md"},
		{Path: "/home/dev/.claude/statusline-command.sh"},
	}
	if got := BackupTargets(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("BackupTargets = %#v, want %#v", got, want)
	}
	if got := BackupTargets(Plan{}); len(got) != 0 {
		t.Fatalf("an empty plan tracks nothing, got %#v", got)
	}
}

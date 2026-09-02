package sync

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// seedManifest isolates HOME and seeds a manifest carrying the given digest.
func seedManifest(t *testing.T, home, digest string) string {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seeded := state.New()
	seeded.ManagedAssetDigest = digest
	if err := state.Save(seeded); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	return filepath.Join(home, ".jarvis", "state.yaml")
}

func manifestBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func bookkeptRun(t *testing.T, home string, plan Plan, runner ComponentRunner, book *Bookkeeping) (RunResult, error) {
	t.Helper()
	return Run(RunInput{
		Plan:        plan,
		Apply:       ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}},
		Backup:      lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
		Bookkeeping: book,
	})
}

func seedZohoManifest(t *testing.T, home string, skills ...string) string {
	t.Helper()
	path := seedManifest(t, home, "sha256:current")
	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	manifest.Skills = append([]string(nil), skills...)
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	return path
}

// Both halves of the rule, over the same machine, and the positive counterpart
// of the two "does not advance" tests below: the digest moves only after a
// measured run that changed something. The first run changes a
// target and writes the record through the lock, rebuilt from a manifest
// re-read inside it: the concurrent writer landing while the lock is taken
// proves the re-read, because its skill survives where a stale in-memory copy
// would have clobbered it. The second changes nothing, never takes the lock,
// leaves the record byte-identical.
func TestRun_WritesBookkeepingUnderLockOnlyWhenATargetChanged(t *testing.T) {
	home := t.TempDir()
	manifest := seedManifest(t, home, "sha256:previous")

	const body = "claude instructions\n"
	tracked := filepath.Join(home, ".claude", "CLAUDE.md")
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(body))},
	}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: tracked, body: body, mode: 0o644}},
	}}

	locked := 0
	book := &Bookkeeping{
		ManagedAssetDigest: "sha256:current",
		Lock: func(critical func() error) error {
			locked++
			concurrent := state.New()
			concurrent.ManagedAssetDigest = "sha256:previous"
			concurrent.Skills = []string{"late-arrival"}
			if err := state.Save(concurrent); err != nil {
				return err
			}
			return critical()
		},
	}

	changed, err := bookkeptRun(t, home, plan, runner, book)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if want := []string{tracked}; !reflect.DeepEqual(changed.Report.Changed, want) {
		t.Fatalf("Report.Changed = %v, want %v", changed.Report.Changed, want)
	}
	if locked != 1 {
		t.Fatalf("critical section entered %d times, want exactly one", locked)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if loaded.ManagedAssetDigest != "sha256:current" {
		t.Fatalf("managed_asset_digest = %q, want the digest this run replayed", loaded.ManagedAssetDigest)
	}
	if want := []string{"late-arrival"}; !reflect.DeepEqual(loaded.Skills, want) {
		t.Fatalf("skills = %v, want %v; the write clobbered a concurrent writer instead of re-reading", loaded.Skills, want)
	}
	recorded := manifestBytes(t, manifest)

	converged, err := bookkeptRun(t, home, plan, runner, book)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(converged.Report.Changed) != 0 {
		t.Fatalf("converged run reported %v as changed", converged.Report.Changed)
	}
	if locked != 1 {
		t.Fatalf("a run that changed nothing took the lock: entered %d times", locked)
	}
	if got := manifestBytes(t, manifest); got != recorded {
		t.Fatalf("a run that changed nothing rewrote the bookkeeping record:\n%s", got)
	}
}

// The mode-assertion half of the temporal rule. EnforceModes fails here because
// a tracked path sits under what the applier just made a regular file, so the
// run never reaches a closing measurement and the digest must stay where it was.
// The applier's own outcome is still reported: not advancing the record is not
// the same as pretending the run never happened.
func TestRun_DoesNotAdvanceManagedAssetDigestWhenModeEnforcementFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this forces the failure through ENOTDIR on a path under a regular file, which Windows does not report the same way")
	}

	home := t.TempDir()
	seedManifest(t, home, "sha256:previous")

	blocker := filepath.Join(home, ".claude", "skills")
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: blocker, Mode: ManagedFileMode},
		{Agent: "claude", Path: filepath.Join(blocker, "SKILL.md"), Mode: ManagedFileMode},
	}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: blocker, body: "not a directory\n", mode: 0o644}},
	}}

	result, err := bookkeptRun(t, home, plan, runner, &Bookkeeping{ManagedAssetDigest: "sha256:current"})
	if err == nil {
		t.Fatal("a tracked path under a regular file must fail the mode assertion")
	}
	if len(result.Report.Agents) != 1 || !result.Report.Agents[0].Converged {
		t.Fatalf("the applier ran and its outcome must still be reported: %+v", result.Report)
	}
	if loaded, loadErr := state.Load(); loadErr != nil || loaded.ManagedAssetDigest != "sha256:previous" {
		t.Fatalf("managed_asset_digest = %+v (%v); an unasserted mode is no evidence of convergence", loaded, loadErr)
	}
}

// The measurement half of the same rule, and the one that does not depend on
// POSIX modes: the applier writes a tracked JSON document the closing snapshot
// cannot decode, so the diff is never taken and the digest must not move.
func TestRun_DoesNotAdvanceManagedAssetDigestWhenClosingSnapshotFails(t *testing.T) {
	home := t.TempDir()
	seedManifest(t, home, "sha256:previous")

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

	result, err := bookkeptRun(t, home, plan, runner, &Bookkeeping{ManagedAssetDigest: "sha256:current"})
	if err == nil {
		t.Fatal("an undecodable managed JSON document must fail the closing measurement")
	}
	if len(result.Report.Agents) != 1 || !result.Report.Agents[0].Converged {
		t.Fatalf("the applier ran and its outcome must still be reported: %+v", result.Report)
	}
	if result.Report.Changed != nil {
		t.Fatalf("an unmeasured diff must stay nil, got %v", result.Report.Changed)
	}
	if loaded, loadErr := state.Load(); loadErr != nil || loaded.ManagedAssetDigest != "sha256:previous" {
		t.Fatalf("managed_asset_digest = %+v (%v); an unmeasured run proves nothing about the asset set", loaded, loadErr)
	}
}

// A failed record says nothing about what landed on disk, and the run that
// failed it just wrote to that disk. The likeliest cause is the one the lock is
// built for — a second jarvis process holding the manifest — so letting a busy
// lock swallow the post-apply check would hide the silent broken-output failure
// that check exists to catch, in exactly the runs that mutated something.
func TestRun_DoesNotPersistBeforeVerificationFails(t *testing.T) {
	home := t.TempDir()
	seedManifest(t, home, "sha256:previous")

	const desired = "claude instructions\n"
	tracked := filepath.Join(home, ".claude", "CLAUDE.md")
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(desired))},
	}}
	// The component writes something, so the run changed a path and reaches the
	// record, but writes the wrong bytes, so verification must fail too.
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: tracked, body: "stale\n", mode: 0o644}},
	}}
	busy := errors.New("state.yaml is locked by another jarvis process")

	result, err := bookkeptRun(t, home, plan, runner, &Bookkeeping{
		ManagedAssetDigest: "sha256:current",
		Lock:               func(func() error) error { return busy },
	})

	if err == nil || errors.Is(err, busy) {
		t.Fatalf("error = %v, want only the verification failure", err)
	}
	if result.Verified {
		t.Fatal("verification failure must not be reported as verified")
	}
	if !strings.Contains(err.Error(), tracked) || !strings.Contains(err.Error(), "run `jarvis sync` to repair") {
		t.Fatalf("error = %q, want the verification verdict naming %s", err, tracked)
	}
}

func TestRun_PersistsZohoExpansionAfterVerifiedNoOp(t *testing.T) {
	home := t.TempDir()
	seedManifest(t, home, "sha256:current")
	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	manifest.Skills = []string{"go-testing", "zoho-deluge"}
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	tracked := filepath.Join(home, ".claude", "CLAUDE.md")
	const desired = "claude instructions\n"
	if err := os.MkdirAll(filepath.Dir(tracked), 0o755); err != nil {
		t.Fatalf("mkdir tracked parent: %v", err)
	}
	if err := os.WriteFile(tracked, []byte(desired), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	pack := skills.NewZohoPack([]skills.Skill{{ID: "zoho-deluge"}, {ID: "zoho-books"}, {ID: "zoho-analytics"}})
	backups := 0
	locked := 0
	result, err := Run(RunInput{
		Plan:  Plan{Tracked: []TrackedPath{{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(desired))}}},
		Apply: ApplyInput{Runner: &recordingRunner{}, Targets: []AgentTarget{{ID: "claude"}}},
		Backup: func(string, []lifecycle.BackupTarget) (lifecycle.BackupManifest, error) {
			backups++
			return lifecycle.BackupManifest{}, nil
		},
		Bookkeeping: &Bookkeeping{
			ZohoExpansion: &ZohoExpansion{Pack: pack, CandidateIDs: []string{"zoho-analytics", "zoho-books"}},
			Lock: func(critical func() error) error {
				locked++
				latest, err := state.Load()
				if err != nil {
					return err
				}
				latest.Skills = append(latest.Skills, "late-arrival")
				if err := state.Save(latest); err != nil {
					return err
				}
				return critical()
			},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Verified || backups != 0 || locked != 1 {
		t.Fatalf("verified=%t backups=%d locks=%d, want verified no-op without backup and one lock", result.Verified, backups, locked)
	}
	if want := []string{"zoho-analytics", "zoho-books"}; !reflect.DeepEqual(result.AddedSkillIDs, want) {
		t.Fatalf("AddedSkillIDs = %v, want %v", result.AddedSkillIDs, want)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load persisted manifest: %v", err)
	}
	if want := []string{"go-testing", "late-arrival", "zoho-analytics", "zoho-books", "zoho-deluge"}; !reflect.DeepEqual(loaded.Skills, want) {
		t.Fatalf("skills = %v, want %v", loaded.Skills, want)
	}
}

func TestBookkeeping_DoesNotResurrectAConcurrentlyDeselectedPack(t *testing.T) {
	home := t.TempDir()
	seedManifest(t, home, "sha256:current")
	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	manifest.Skills = []string{"zoho-deluge"}
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	pack := skills.NewZohoPack([]skills.Skill{{ID: "zoho-deluge"}, {ID: "zoho-books"}})
	book := &Bookkeeping{ZohoExpansion: &ZohoExpansion{Pack: pack, CandidateIDs: []string{"zoho-books"}}}
	book.Lock = func(critical func() error) error {
		latest, err := state.Load()
		if err != nil {
			return err
		}
		latest.Skills = nil
		if err := state.Save(latest); err != nil {
			return err
		}
		return critical()
	}
	added, err := book.record(false, true)
	if err != nil || len(added) != 0 {
		t.Fatalf("record = %v, %v; want no resurrection", added, err)
	}
	loaded, err := state.Load()
	if err != nil || len(loaded.Skills) != 0 {
		t.Fatalf("skills after concurrent deselection = %v, %v", loaded.Skills, err)
	}
}

func TestBookkeeping_RecomputesPackAdditionsAfterConcurrentPartialMembershipChange(t *testing.T) {
	home := t.TempDir()
	seedZohoManifest(t, home, "zoho-deluge", "zoho-books")

	pack := skills.NewZohoPack([]skills.Skill{{ID: "zoho-deluge"}, {ID: "zoho-books"}, {ID: "zoho-analytics"}})
	book := &Bookkeeping{ZohoExpansion: &ZohoExpansion{Pack: pack, CandidateIDs: []string{"zoho-analytics"}}}
	book.Lock = func(critical func() error) error {
		latest, err := state.Load()
		if err != nil {
			return err
		}
		latest.Skills = []string{"zoho-deluge"}
		if err := state.Save(latest); err != nil {
			return err
		}
		return critical()
	}

	added, err := book.record(false, true)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if want := []string{"zoho-analytics", "zoho-books"}; !reflect.DeepEqual(added, want) {
		t.Fatalf("AddedSkillIDs = %v, want fresh-state additions %v", added, want)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatalf("load persisted manifest: %v", err)
	}
	if want := []string{"zoho-analytics", "zoho-books", "zoho-deluge"}; !reflect.DeepEqual(loaded.Skills, want) {
		t.Fatalf("skills = %v, want complete current pack %v", loaded.Skills, want)
	}
}

func TestRun_ReturnsNoAdditionsWhenPostVerificationLockFails(t *testing.T) {
	home := t.TempDir()
	seedZohoManifest(t, home, "zoho-deluge")

	tracked := filepath.Join(home, ".claude", "CLAUDE.md")
	const desired = "claude instructions\n"
	if err := os.MkdirAll(filepath.Dir(tracked), 0o755); err != nil {
		t.Fatalf("mkdir tracked parent: %v", err)
	}
	if err := os.WriteFile(tracked, []byte(desired), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	busy := errors.New("state.yaml is locked")
	pack := skills.NewZohoPack([]skills.Skill{{ID: "zoho-deluge"}, {ID: "zoho-books"}})
	result, err := Run(RunInput{
		Plan:  Plan{Tracked: []TrackedPath{{Agent: "claude", Path: tracked, Mode: ManagedFileMode, Desired: digestOf([]byte(desired))}}},
		Apply: ApplyInput{Runner: &recordingRunner{}, Targets: []AgentTarget{{ID: "claude"}}},
		Backup: func(string, []lifecycle.BackupTarget) (lifecycle.BackupManifest, error) {
			return lifecycle.BackupManifest{}, nil
		},
		Bookkeeping: &Bookkeeping{
			ZohoExpansion: &ZohoExpansion{Pack: pack, CandidateIDs: []string{"zoho-books"}},
			Lock:          func(func() error) error { return busy },
		},
	})
	if !errors.Is(err, busy) {
		t.Fatalf("Run error = %v, want post-verification lock failure", err)
	}
	if !result.Verified || len(result.AddedSkillIDs) != 0 {
		t.Fatalf("Verified=%t AddedSkillIDs=%v, want verified persistence failure without additions", result.Verified, result.AddedSkillIDs)
	}
}

func TestBookkeeping_ReturnsNoAdditionsWhenSaveFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not reliably deny atomic writes on Windows")
	}

	home := t.TempDir()
	statePath := seedZohoManifest(t, home, "zoho-deluge")
	stateDir := filepath.Dir(statePath)
	if err := os.Chmod(stateDir, 0o555); err != nil {
		t.Fatalf("make state directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) })

	pack := skills.NewZohoPack([]skills.Skill{{ID: "zoho-deluge"}, {ID: "zoho-books"}})
	added, err := (&Bookkeeping{ZohoExpansion: &ZohoExpansion{Pack: pack, CandidateIDs: []string{"zoho-books"}}}).record(false, true)
	if err == nil {
		t.Fatal("record must return the state save failure")
	}
	if len(added) != 0 {
		t.Fatalf("AddedSkillIDs = %v, want none after failed save", added)
	}
}

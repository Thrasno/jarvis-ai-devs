package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

func snapshotOrFail(t *testing.T, tracked []TrackedPath) Snapshot {
	t.Helper()
	snapshot, err := TakeSnapshot(tracked)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	return snapshot
}

func seedFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	writeFile(t, path, "#!/bin/sh\necho jarvis\n")
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
}

// InstallStatusline rewrites the script on every single run (claude.go:882-885),
// so an mtime diff would report a change on every sync forever. Every case also
// tracks a never-installed path, proving absence on both sides is a legitimate
// state rather than an error or a phantom change.
func TestDiff_ComparesContentAndModeAndNeverModificationTime(t *testing.T) {
	tests := []struct {
		name            string
		mutate          func(t *testing.T, seeded, uninstalled string)
		wantSeeded      bool
		wantUninstalled bool
		// posixModes marks a case whose only signal is a permission change.
		// Windows has no POSIX permission bits, so Perm always reads 0666 or
		// 0444 and no mode mutation is observable there.
		posixModes bool
	}{
		{
			// Exactly what InstallStatusline does: remove, then write fresh.
			name: "an unconditional rewrite with identical bytes and mode is not a change",
			mutate: func(t *testing.T, seeded, _ string) {
				mustRemove(t, seeded)
				seedFile(t, seeded, 0o755)
			},
		},
		{
			name:       "different content is a change",
			mutate:     func(t *testing.T, seeded, _ string) { writeFile(t, seeded, "echo drifted") },
			wantSeeded: true,
		},
		{
			name:       "the same content with a different mode is a change",
			mutate:     func(t *testing.T, seeded, _ string) { seedFile(t, seeded, 0o644) },
			wantSeeded: true,
			posixModes: true,
		},
		{
			name:       "deleting the file is a change",
			mutate:     func(t *testing.T, seeded, _ string) { mustRemove(t, seeded) },
			wantSeeded: true,
		},
		{
			name:            "installing a previously absent file is a change",
			mutate:          func(t *testing.T, _, uninstalled string) { seedFile(t, uninstalled, 0o755) },
			wantUninstalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.posixModes && runtime.GOOS == "windows" {
				t.Skip("Windows has no POSIX permission bits, so a mode-only mutation is not observable")
			}
			root := t.TempDir()
			seeded := filepath.Join(root, statuslineScriptName)
			uninstalled := filepath.Join(root, "never-installed.sh")
			seedFile(t, seeded, 0o755)
			tracked := []TrackedPath{
				{Path: seeded, Mode: ManagedExecutableMode},
				{Path: uninstalled, Mode: ManagedExecutableMode},
			}

			before := snapshotOrFail(t, tracked)
			tt.mutate(t, seeded, uninstalled)

			want := []string{}
			if tt.wantSeeded {
				want = append(want, seeded)
			}
			if tt.wantUninstalled {
				want = append(want, uninstalled)
			}
			if changed := Diff(before, snapshotOrFail(t, tracked)); !reflect.DeepEqual(changed, want) {
				t.Fatalf("Diff = %v, want %v", changed, want)
			}
		})
	}
}

// A managed path can be a symlink into a dotfiles repo, and both jarvis writers
// replace it rather than write through it (skills/installer.go:102-117;
// writeFileAtomic renames over the link). A snapshot that resolved the link
// would compare the target's bytes and miss the replacement entirely.
func TestTakeSnapshot_DoesNotFollowASymlinkAtAManagedPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	managed := filepath.Join(root, "managed.md")
	seedFile(t, target, 0o644)
	if err := os.Symlink(target, managed); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	tracked := []TrackedPath{{Path: managed, Mode: ManagedFileMode}}
	before := snapshotOrFail(t, tracked)
	mustRemove(t, managed)
	seedFile(t, managed, 0o644)

	if changed := Diff(before, snapshotOrFail(t, tracked)); !reflect.DeepEqual(changed, []string{managed}) {
		t.Fatalf("Diff = %v, want [%s]: replacing a symlink with a regular file is a change", changed, managed)
	}
}

// The plan owns exactly one path list: the diff measures it and the pre-apply
// backup captures it, so a second list built elsewhere would let the two drift
// apart in silence. Consent gates the statusline entry, because undecided and
// decided-against both mean "do not touch".
func TestBuildPlan_TrackedPathsCoverEveryManagedArtifactWithItsAssertedMode(t *testing.T) {
	root := t.TempDir()
	st := replayableState(
		state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md", ConfigPath: "settings.json"},
		state.Agent{ID: "opencode", InstructionsPath: ".config/opencode/AGENTS.md", ConfigPath: "opencode.json"},
	)
	st.Skills = []string{"sdd-apply"}
	st.Statusline = state.StatuslineState{Decided: true}
	in := PlanInput{Root: root, State: st, Templates: jarvis.TemplatesFS}
	want := map[string]os.FileMode{
		filepath.Join(root, ".claude", "CLAUDE.md"):                                   ManagedFileMode,
		filepath.Join(root, ".claude", "skills", "sdd-apply", "SKILL.md"):             ManagedFileMode,
		filepath.Join(root, ".config", "opencode", "AGENTS.md"):                       ManagedFileMode,
		filepath.Join(root, ".config", "opencode", "skills", "sdd-apply", "SKILL.md"): ManagedFileMode,
	}

	// Consent asked and declined: the script is nobody's business here.
	assertTrackedPaths(t, in, want)

	// Consent decided and enabled: the script joins the list as an executable.
	st.Statusline.Enabled = true
	want[filepath.Join(root, ".claude", statuslineScriptName)] = ManagedExecutableMode
	assertTrackedPaths(t, in, want)
}

func assertTrackedPaths(t *testing.T, in PlanInput, want map[string]os.FileMode) {
	t.Helper()
	plan, err := BuildPlan(in)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	got := map[string]os.FileMode{}
	for _, tracked := range plan.Tracked {
		got[tracked.Path] = tracked.Mode
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan.Tracked = %v, want %v", got, want)
	}
}

// Threat matrix, documentation-like paths: the mode is asserted, never
// inherited. writeFileAtomic reuses an existing file's permission bits
// (claude.go:918-923) and D2's reinstall-after-delete path lets the umask
// decide; neither may choose the final mode.
func TestEnforceModes_AssertsTheManagedModeInsteadOfInheritingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits: Chmod only toggles the read-only attribute and Perm always reads 0666 or 0444")
	}

	root := t.TempDir()
	instructions := filepath.Join(root, "CLAUDE.md")
	skill := filepath.Join(root, "skills", "sdd-apply", "SKILL.md")
	reinstalled := filepath.Join(root, statuslineScriptName)
	absent := filepath.Join(root, "never-installed.md")

	// Two documents that gained an executable bit they must not keep.
	seedFile(t, instructions, 0o755)
	seedFile(t, skill, 0o777)
	// Deleted, then recreated under a umask that stripped the executable bit.
	seedFile(t, reinstalled, 0o755)
	mustRemove(t, reinstalled)
	seedFile(t, reinstalled, 0o600)

	tracked := []TrackedPath{
		{Path: instructions, Mode: ManagedFileMode},
		{Path: skill, Mode: ManagedFileMode},
		{Path: reinstalled, Mode: ManagedExecutableMode},
		{Path: absent, Mode: ManagedFileMode},
	}
	if err := EnforceModes(tracked); err != nil {
		t.Fatalf("EnforceModes: %v", err)
	}

	for _, want := range tracked[:3] {
		info, err := os.Lstat(want.Path)
		if err != nil {
			t.Fatalf("lstat %s: %v", want.Path, err)
		}
		if info.Mode().Perm() != want.Mode {
			t.Fatalf("%s has mode %04o, want %04o", want.Path, info.Mode().Perm(), want.Mode)
		}
	}
	if _, err := os.Lstat(absent); !os.IsNotExist(err) {
		t.Fatalf("EnforceModes created the absent tracked path %s", absent)
	}
}

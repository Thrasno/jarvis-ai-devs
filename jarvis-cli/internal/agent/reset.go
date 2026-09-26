// Package agent's reset core implements the consented, backed-up,
// all-or-restore configuration reset for issue #767. It plans and applies
// removal of exactly the contract-owned surfaces declared by
// sddruntime.ResetInventory — nothing else.
//
// Regeneration is deliberately NOT part of this API: after ApplyReset
// returns, the wizard's existing install sequence regenerates managed
// configuration from scratch, exactly as a fresh install would. This keeps
// the reset itself small, auditable, and free of any install-ordering
// assumptions.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// ResetChange is one concrete removal a configuration reset planned or
// applied, paired with the contract surface (sddruntime.ResetSurface.ID) it
// derives from.
type ResetChange struct {
	SurfaceID string
	Path      string
	Detail    string
}

// ResetPlan is a read-only preview of what ApplyReset would remove for one
// platform's config directory. Producing a plan never mutates anything.
type ResetPlan struct {
	Platform  sddruntime.Platform
	ConfigDir string
	Changes   []ResetChange
}

// ResetResult reports what ApplyReset actually removed.
type ResetResult struct {
	Platform   sddruntime.Platform
	SnapshotID string
	Changes    []ResetChange

	// Rollback, when non-nil, undoes this exact successful reset: it restores
	// every mutation ApplyReset applied back to its pre-reset state, using
	// the same in-process journal ApplyReset's own failure path rolls back
	// from. It stays valid after ApplyReset returns, so a caller that
	// discovers a LATER failure (e.g. the subsequent install step) can still
	// undo an already-applied reset instead of leaving the reset surfaces
	// deleted with nothing reinstalled (issue #767 hardening R4-002).
	// Rollback is nil when there was nothing to reset (no mutations).
	Rollback func() error
}

// ResetRestoreError reports that ApplyReset failed AND that the automatic
// rollback did NOT fully restore prior state (at least one path in
// UnrecoveredPaths). SnapshotID names the durable lifecycle.BackupStore
// snapshot an operator can restore from by hand.
//
// When a mutation fails but rollback fully restores every applied change,
// ApplyReset returns the plain wrapped cause instead: callers can tell the
// two outcomes apart with errors.As, since only an incomplete rollback
// produces a *ResetRestoreError.
type ResetRestoreError struct {
	SnapshotID       string
	UnrecoveredPaths []string
	Cause            error
}

func (e *ResetRestoreError) Error() string {
	return fmt.Sprintf(
		"configuration reset failed and rollback could not restore everything (snapshot %s, unrecovered paths: %s): %v",
		e.SnapshotID, strings.Join(e.UnrecoveredPaths, ", "), e.Cause,
	)
}

func (e *ResetRestoreError) Unwrap() error { return e.Cause }

// resetFileOps is the injectable filesystem seam ApplyReset and PlanReset
// mutate/read through. Tests replace it to force a failure at an exact step
// without chmod tricks.
type resetFileOps struct {
	// readFile reads a regular file's content and permission bits. Callers
	// must never call it directly on a path that may itself be a symlink:
	// resolveEditableSurfacePath (for a surface edited in place) or
	// lstatKind+readLink (for a whole-file delete surface) decide that first,
	// so readFile only ever sees a real, non-symlink path. mode is the file's
	// permission bits when existed is true.
	readFile func(path string) (data []byte, existed bool, mode os.FileMode, err error)
	// writeFile (re)writes a regular file with the given permission bits. A
	// zero mode falls back to 0644.
	writeFile func(path string, data []byte, mode os.FileMode) error
	// lstatKind reports whether path exists and, if so, whether the entry
	// itself (not what it points at) is a symlink. It never follows a
	// symlink to decide this.
	lstatKind func(path string) (exists bool, isSymlink bool, err error)
	// resolveSymlinkTarget resolves path to the real file a symlink chain
	// ultimately points at (like filepath.EvalSymlinks). Callers only invoke
	// it once lstatKind has already reported path is a symlink.
	resolveSymlinkTarget func(path string) (string, error)
	// readLink reports a symlink's own, single-hop target without following
	// it or resolving further hops. Used to journal a symlink mutation for
	// rollback, never to decide where to read/write content.
	readLink func(path string) (target string, err error)
	// writeSymlink recreates a symlink pointing at target, replacing any
	// existing entry at path first.
	writeSymlink func(path, target string) error
	removeFile   func(path string) error
	listDir      func(dir string) ([]resetDirEntry, error)
}

func defaultResetFileOps() resetFileOps {
	return resetFileOps{
		readFile: func(path string) ([]byte, bool, os.FileMode, error) {
			info, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return nil, false, 0, nil
				}
				return nil, false, 0, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, false, 0, err
			}
			return data, true, info.Mode().Perm(), nil
		},
		writeFile: func(path string, data []byte, mode os.FileMode) error {
			if mode == 0 {
				mode = 0644
			}
			return writeFileAtomic(path, data, mode)
		},
		lstatKind: func(path string) (bool, bool, error) {
			info, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, err
			}
			return true, info.Mode()&os.ModeSymlink != 0, nil
		},
		resolveSymlinkTarget: filepath.EvalSymlinks,
		readLink:             os.Readlink,
		writeSymlink: func(path, target string) error {
			if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
				return rmErr
			}
			return os.Symlink(target, path)
		},
		removeFile: func(path string) error {
			err := os.Remove(path)
			if err != nil && os.IsNotExist(err) {
				return nil
			}
			return err
		},
		listDir: listResetDirEntriesRecursively,
	}
}

// resetDirEntry is one dir-relative entry found under a whole-directory reset
// surface. IsSymlink reports the entry's own Lstat type: a symlink is never
// followed to decide this, whether it points at a file or a directory.
type resetDirEntry struct {
	RelPath   string
	IsSymlink bool
}

// listResetDirEntriesRecursively returns every regular file and symlink under
// dir, as dir-relative entries, sorted by path. An absent dir reports no
// entries and no error: there is nothing to reset in a directory that was
// never created. A symlink pointing at a directory is recorded as its own
// entry and never walked into, because fs.WalkDir uses Lstat semantics and
// does not follow symlinks to descend into them.
func listResetDirEntriesRecursively(dir string) ([]resetDirEntry, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("expected directory, found file: %s", dir)
	}
	var entries []resetDirEntry
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		entries = append(entries, resetDirEntry{
			RelPath:   rel,
			IsSymlink: entry.Type()&fs.ModeSymlink != 0,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })
	return entries, nil
}

// resetFileMutation is one file-level reset step: before is the content that
// existed at that path before the reset touched it (existed reports whether
// there was anything there at all), and after is the desired content, or nil
// to delete the path. changes is the plan-facing description this mutation
// covers (a single physical file can satisfy more than one contract surface,
// e.g. Claude settings.json covers five).
//
// beforeMode is the permission bits the path had before the reset (valid when
// existed is true); a rewrite or a rollback restore reuses it instead of a
// hardcoded mode, so an executable script keeps its executable bit.
//
// isSymlink marks a path whose entry itself is a symlink: before/beforeMode
// are unused, and linkTarget holds the link's prior target instead. A
// symlink mutation is always a deletion (after is nil); rollback recreates
// the link rather than writing file bytes.
type resetFileMutation struct {
	Path       string
	before     []byte
	beforeMode os.FileMode
	existed    bool
	after      []byte // nil means delete
	changes    []ResetChange

	isSymlink  bool
	linkTarget string
}

// PlanReset computes, without mutating anything, exactly what ApplyReset
// would remove for one platform's config directory.
func PlanReset(platform sddruntime.Platform, configDir string) (ResetPlan, error) {
	mutations, err := computeResetMutations(platform, configDir, defaultResetFileOps())
	if err != nil {
		return ResetPlan{}, err
	}
	return ResetPlan{Platform: platform, ConfigDir: configDir, Changes: collectResetChanges(mutations)}, nil
}

// ApplyReset performs a consented, backed-up, all-or-restore configuration
// reset for one platform's config directory. It snapshots every affected path
// through lifecycle.BackupStore before the first write, then applies every
// mutation; a failure at any step rolls back every mutation already applied,
// in reverse order, from the in-process journal. A failure during rollback
// itself is reported as a *ResetRestoreError carrying the durable snapshot ID
// and exactly which paths could not be restored automatically.
//
// homeDir is the home directory whose ~/.jarvis/backups holds the durable
// snapshot; it is ordinarily the same home configDir is rooted under.
//
// Regeneration is NOT part of this call: the wizard's install sequence
// regenerates managed configuration after ApplyReset returns.
func ApplyReset(platform sddruntime.Platform, configDir, homeDir string) (ResetResult, error) {
	return applyResetWithOps(platform, configDir, homeDir, defaultResetFileOps())
}

func applyResetWithOps(platform sddruntime.Platform, configDir, homeDir string, ops resetFileOps) (ResetResult, error) {
	mutations, err := computeResetMutations(platform, configDir, ops)
	if err != nil {
		return ResetResult{}, err
	}
	if len(mutations) == 0 {
		return ResetResult{Platform: platform}, nil
	}

	// A symlink mutation is excluded from the durable lifecycle.BackupStore
	// snapshot: that store archives file content via os.ReadFile, which is
	// exactly the through-the-link read this reset must never perform, and
	// it would fail outright for a dangling link. The in-process journal
	// already carries everything a symlink mutation needs to roll back (its
	// prior target via linkTarget), so no external content backup is needed
	// for it.
	//
	// A resolved symlink TARGET outside BackupStore's allowed roots
	// (~/.claude, ~/.config/opencode, ~/.jarvis) is excluded the same way:
	// resolveEditableSurfacePath can land there for a dotfile manager whose
	// real files live elsewhere (e.g. ~/dotfiles/claude-settings.json
	// symlinked into ~/.claude/settings.json), and BackupStore refuses to
	// snapshot a path outside its confinement. This reset's own mid-run
	// rollback (from the in-process journal's before-bytes) still fully
	// protects that mutation for THIS run; only the separate, durable
	// snapshot an operator could restore by hand afterward does not cover
	// it, exactly as already documented for a symlink mutation above.
	backupTargets := make([]lifecycle.BackupTarget, 0, len(mutations))
	seen := make(map[string]bool, len(mutations))
	for _, m := range mutations {
		if m.isSymlink || !withinBackupAllowedRoots(homeDir, m.Path) {
			continue
		}
		if !seen[m.Path] {
			seen[m.Path] = true
			backupTargets = append(backupTargets, lifecycle.BackupTarget{Path: m.Path})
		}
	}
	store := lifecycle.NewBackupStore(homeDir)
	manifest, err := store.CreateSnapshotOfTargets("agent-reset", backupTargets)
	if err != nil {
		return ResetResult{}, fmt.Errorf("snapshot configuration before reset: %w", err)
	}

	applied := make([]resetFileMutation, 0, len(mutations))
	for _, m := range mutations {
		var mutateErr error
		if m.after == nil {
			mutateErr = ops.removeFile(m.Path)
		} else {
			mutateErr = ops.writeFile(m.Path, m.after, m.beforeMode)
		}
		if mutateErr != nil {
			unrecovered := rollbackResetMutations(applied, ops)
			if len(unrecovered) > 0 {
				return ResetResult{}, &ResetRestoreError{
					SnapshotID:       manifest.SnapshotID,
					UnrecoveredPaths: unrecovered,
					Cause:            mutateErr,
				}
			}
			return ResetResult{}, fmt.Errorf(
				"configuration reset failed, rollback restored prior state (snapshot %s): %w",
				manifest.SnapshotID, mutateErr,
			)
		}
		applied = append(applied, m)
	}

	// The durable snapshot and every mutation already succeeded; the sidecar
	// is auxiliary recovery metadata for a symlink an operator might restore
	// by hand from the snapshot, not part of the reset's own rollback path.
	// Losing it must never fail an otherwise successful reset, so its error
	// is deliberately discarded.
	_ = writeResetSymlinkSidecar(homeDir, manifest.SnapshotID, mutations)

	return ResetResult{
		Platform:   platform,
		SnapshotID: manifest.SnapshotID,
		Changes:    collectResetChanges(mutations),
		Rollback: func() error {
			unrecovered := rollbackResetMutations(mutations, ops)
			if len(unrecovered) > 0 {
				return &ResetRestoreError{
					SnapshotID:       manifest.SnapshotID,
					UnrecoveredPaths: unrecovered,
					Cause:            errors.New("rollback requested after a later failure"),
				}
			}
			return nil
		},
	}, nil
}

// resetSymlinkSidecarEntry records one symlink mutation's path and prior
// target, so an operator restoring the durable lifecycle.BackupStore snapshot
// by hand (rather than through ApplyReset's own in-process rollback) can
// still recreate a symlink the snapshot's file archive cannot carry (issue
// #767 hardening R4-003: that archive reads content via os.ReadFile, which a
// symlink mutation is deliberately excluded from).
type resetSymlinkSidecarEntry struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// writeResetSymlinkSidecar writes the symlink inventory for one snapshot next
// to that snapshot's manifest, under the same ~/.jarvis/backups directory
// lifecycle.BackupStore uses. It writes nothing when there are no symlink
// mutations to record.
func writeResetSymlinkSidecar(homeDir, snapshotID string, mutations []resetFileMutation) error {
	var entries []resetSymlinkSidecarEntry
	for _, m := range mutations {
		if !m.isSymlink {
			continue
		}
		entries = append(entries, resetSymlinkSidecarEntry{Path: m.Path, Target: m.linkTarget})
	}
	if len(entries) == 0 {
		return nil
	}
	dir := filepath.Join(homeDir, ".jarvis", "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, snapshotID+".symlinks.json"), raw, 0o644)
}

// rollbackResetMutations restores every applied mutation in reverse order
// from its journaled snapshot: previous bytes when the path existed before,
// or deletion when it did not. It returns the sorted list of paths that could
// not be restored.
func rollbackResetMutations(applied []resetFileMutation, ops resetFileOps) []string {
	var unrecovered []string
	for i := len(applied) - 1; i >= 0; i-- {
		m := applied[i]
		var err error
		switch {
		case m.existed && m.isSymlink:
			err = ops.writeSymlink(m.Path, m.linkTarget)
		case m.existed:
			err = ops.writeFile(m.Path, m.before, m.beforeMode)
		default:
			err = ops.removeFile(m.Path)
		}
		if err != nil {
			unrecovered = append(unrecovered, m.Path)
		}
	}
	sort.Strings(unrecovered)
	return unrecovered
}

func collectResetChanges(mutations []resetFileMutation) []ResetChange {
	var changes []ResetChange
	for _, m := range mutations {
		changes = append(changes, m.changes...)
	}
	return changes
}

func computeResetMutations(platform sddruntime.Platform, configDir string, ops resetFileOps) ([]resetFileMutation, error) {
	switch platform {
	case sddruntime.PlatformClaude:
		return computeClaudeResetMutations(configDir, ops)
	case sddruntime.PlatformOpenCode:
		return computeOpenCodeResetMutations(configDir, ops)
	default:
		return nil, fmt.Errorf("unsupported reset platform %q", platform)
	}
}

// withinBackupAllowedRoots reports whether path falls under one of the
// config roots lifecycle.BackupStore allows itself to snapshot. It mirrors
// that store's own root list on purpose: the two must agree, because this
// function exists only to decide, before calling BackupStore, which resolved
// paths it would refuse. path must already be an absolute, resolved
// (non-symlink) path; no further symlink resolution happens here.
func withinBackupAllowedRoots(homeDir, path string) bool {
	roots := []string{
		filepath.Join(homeDir, ".claude"),
		filepath.Join(homeDir, ".config", "opencode"),
		filepath.Join(homeDir, ".jarvis"),
	}
	cleaned := filepath.Clean(path)
	for _, root := range roots {
		canonRoot := filepath.Clean(root)
		if cleaned == canonRoot || strings.HasPrefix(cleaned, canonRoot+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// resolveEditableSurfacePath resolves path to the real file a reset must
// read and rewrite in place (a JSON settings file or a marker-block file),
// so a symlinked top-level surface is never replaced by a regular file. When
// path is itself a symlink, the returned path is its resolved target -- the
// mutation this builds journals and writes THAT path, and the symlink at the
// original path is left completely untouched. A non-symlink path (or one
// that does not exist) is returned unchanged.
func resolveEditableSurfacePath(path string, ops resetFileOps) (string, error) {
	exists, isSymlink, err := ops.lstatKind(path)
	if err != nil {
		return "", fmt.Errorf("lstat %s: %w", path, err)
	}
	if !exists || !isSymlink {
		return path, nil
	}
	resolved, err := ops.resolveSymlinkTarget(path)
	if err != nil {
		return "", fmt.Errorf("resolve symlink %s: %w", path, err)
	}
	return resolved, nil
}

func resetSurfaceByID(platform sddruntime.Platform, id string) sddruntime.ResetSurface {
	for _, s := range sddruntime.ResetInventory(platform) {
		if s.ID == id {
			return s
		}
	}
	return sddruntime.ResetSurface{}
}

// --- Claude ---

func computeClaudeResetMutations(configDir string, ops resetFileOps) ([]resetFileMutation, error) {
	var mutations []resetFileMutation

	settingsPath, err := resolveEditableSurfacePath(filepath.Join(configDir, "settings.json"), ops)
	if err != nil {
		return nil, err
	}
	settingsBefore, existed, settingsMode, err := ops.readFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if existed {
		after, changes, err := computeClaudeSettingsReset(settingsBefore)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", settingsPath, err)
		}
		if after != nil {
			mutations = append(mutations, resetFileMutation{Path: settingsPath, before: settingsBefore, beforeMode: settingsMode, existed: true, after: after, changes: changes})
		}
	}

	if m, err := computeMarkerBlockMutation(filepath.Join(configDir, "CLAUDE.md"), "claude.instructions.marker_block", ops); err != nil {
		return nil, err
	} else if m != nil {
		mutations = append(mutations, *m)
	}

	for _, wf := range []struct{ surfaceID, rel string }{
		{"claude.orchestrator.file", "sdd-orchestrator.md"},
		{"claude.statusline.script", "statusline-command.sh"},
	} {
		m, err := computeWholeFileDeleteMutation(filepath.Join(configDir, wf.rel), wf.surfaceID, ops)
		if err != nil {
			return nil, err
		}
		if m != nil {
			mutations = append(mutations, *m)
		}
	}

	agentMutations, err := computeWholeDirectoryDeleteMutations(filepath.Join(configDir, "agents"), "claude.agents.directory", claudeJarvisOwnedAgentBaseNames(), ops)
	if err != nil {
		return nil, err
	}
	mutations = append(mutations, agentMutations...)

	for _, wd := range []struct{ surfaceID, rel string }{
		{"claude.output_styles.directory", "output-styles"},
		{"claude.skills.directory", "skills"},
		{"claude.hive_hooks.directory", "hive-hooks"},
	} {
		dirMutations, err := computeWholeDirectoryDeleteMutations(filepath.Join(configDir, wd.rel), wd.surfaceID, nil, ops)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, dirMutations...)
	}

	return mutations, nil
}

// claudeJarvisOwnedAgentBaseNames lists every Claude agents/ base name (no
// extension) that a Jarvis install has ever written: current SDD phase
// agents, Judgment Day agents, and the four retired 4R reviewer agents. A
// name outside this set found under agents/ is reported as user-added.
func claudeJarvisOwnedAgentBaseNames() map[string]bool {
	owned := make(map[string]bool)
	for _, def := range SDDPhaseAgentDefinitions() {
		owned[def.Name] = true
	}
	for _, name := range openCodeJudgmentDaySubagents() {
		owned[name] = true
	}
	for _, name := range sddruntime.RetiredClaudeReviewAgentBaseNames() {
		owned[name] = true
	}
	return owned
}

// claudeManagedStatuslineCommand mirrors the literal patched into settings.json
// by (*ClaudeAgent).statusLineSettingsPatch.
const claudeManagedStatuslineCommand = "bash ~/.claude/statusline-command.sh"

// errResetSettingsUnparseable reports that an existing settings file could
// not be parsed as a JSON object, so PlanReset/ApplyReset refuse to guess at
// its content: they name the file and stop rather than silently treating it
// as nothing to reset, or attempting to repair or rewrite it.
var errResetSettingsUnparseable = errors.New("settings file is not a valid JSON object")

func computeClaudeSettingsReset(before []byte) ([]byte, []ResetChange, error) {
	if len(strings.TrimSpace(string(before))) == 0 {
		return nil, nil, nil
	}
	root, err := parseOrderedJSON(before)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errResetSettingsUnparseable, err)
	}
	if root.object == nil {
		return nil, nil, fmt.Errorf("%w: top-level value is not a JSON object", errResetSettingsUnparseable)
	}

	var changes []ResetChange
	mutated := false

	if styleName, ok := orderedStringValue(root.object, "outputStyle"); ok {
		jarvisNames, err := claudeManagedOutputStyleNames()
		if err != nil {
			return nil, nil, err
		}
		if resetContainsString(jarvisNames, styleName) {
			root.object.delete("outputStyle")
			mutated = true
			changes = append(changes, ResetChange{SurfaceID: "claude.settings.output_style", Path: "settings.json", Detail: "outputStyle: " + styleName})
		}
	}

	if statusLineVal, ok := root.object.get("statusLine"); ok && statusLineVal.object != nil {
		if cmd, ok := orderedStringValue(statusLineVal.object, "command"); ok && cmd == claudeManagedStatuslineCommand {
			root.object.delete("statusLine")
			mutated = true
			changes = append(changes, ResetChange{SurfaceID: "claude.settings.status_line", Path: "settings.json", Detail: "statusLine: Jarvis statusline command"})
		}
	}

	if hooksVal, ok := root.object.get("hooks"); ok && hooksVal.object != nil {
		tokens := resetSurfaceByID(sddruntime.PlatformClaude, "claude.settings.hooks").Matchers
		hooksChanged := false
		for i, eventPair := range hooksVal.object.pairs {
			if eventPair.value.array == nil {
				continue
			}
			filteredGroups := make([]orderedValue, 0, len(eventPair.value.array))
			for _, group := range eventPair.value.array {
				kept, groupChanged := filterHookGroupNestedCommands(group, tokens)
				if groupChanged {
					hooksChanged = true
				}
				if kept != nil {
					filteredGroups = append(filteredGroups, *kept)
				}
			}
			hooksVal.object.pairs[i].value = orderedValue{array: filteredGroups}
		}
		if hooksChanged {
			root.object.set("hooks", hooksVal)
			mutated = true
			changes = append(changes, ResetChange{SurfaceID: "claude.settings.hooks", Path: "settings.json", Detail: "managed hook entries"})
		}
	}

	if permissionsVal, ok := root.object.get("permissions"); ok && permissionsVal.object != nil {
		literalSet := stringSet(resetSurfaceByID(sddruntime.PlatformClaude, "claude.settings.permissions").Matchers)
		modeSet := stringSet(resetSurfaceByID(sddruntime.PlatformClaude, "claude.settings.default_mode").Matchers)
		permissionsChanged := false
		var removedLiterals []string

		for _, key := range []string{"allow", "deny"} {
			listVal, ok := permissionsVal.object.get(key)
			if !ok || listVal.array == nil {
				continue
			}
			filtered := make([]orderedValue, 0, len(listVal.array))
			for _, item := range listVal.array {
				if s, ok := item.scalar.(string); ok && literalSet[s] {
					permissionsChanged = true
					removedLiterals = append(removedLiterals, s)
					continue
				}
				filtered = append(filtered, item)
			}
			if len(filtered) != len(listVal.array) {
				permissionsVal.object.set(key, orderedValue{array: filtered})
			}
		}

		if mode, ok := orderedStringValue(permissionsVal.object, "defaultMode"); ok && modeSet[mode] {
			permissionsVal.object.delete("defaultMode")
			permissionsChanged = true
			changes = append(changes, ResetChange{SurfaceID: "claude.settings.default_mode", Path: "settings.json", Detail: "permissions.defaultMode: " + mode})
		}

		if permissionsChanged {
			root.object.set("permissions", permissionsVal)
			mutated = true
			if len(removedLiterals) > 0 {
				sort.Strings(removedLiterals)
				changes = append(changes, ResetChange{SurfaceID: "claude.settings.permissions", Path: "settings.json", Detail: fmt.Sprintf("%d permission entries", len(removedLiterals))})
			}
		}
	}

	if !mutated {
		return nil, nil, nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return append(out, '\n'), changes, nil
}

// claudeManagedOutputStyleNames lists every TitleCase output-style name a
// Jarvis persona has ever written, so a reset can tell a Jarvis-emitted
// outputStyle apart from a user's own style of the same key.
func claudeManagedOutputStyleNames() ([]string, error) {
	profiles, err := persona.ListProfiles(jarvis.PersonaFS)
	if err != nil {
		return nil, fmt.Errorf("list persona profiles: %w", err)
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, toTitleCase(p.Name))
	}
	return names, nil
}

// --- OpenCode ---

func computeOpenCodeResetMutations(configDir string, ops resetFileOps) ([]resetFileMutation, error) {
	var mutations []resetFileMutation

	settingsPath, err := resolveEditableSurfacePath(filepath.Join(configDir, "opencode.json"), ops)
	if err != nil {
		return nil, err
	}
	settingsBefore, existed, settingsMode, err := ops.readFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if existed {
		after, changes, err := computeOpenCodeSettingsReset(settingsBefore)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", settingsPath, err)
		}
		if after != nil {
			mutations = append(mutations, resetFileMutation{Path: settingsPath, before: settingsBefore, beforeMode: settingsMode, existed: true, after: after, changes: changes})
		}
	}

	if m, err := computeMarkerBlockMutation(filepath.Join(configDir, "AGENTS.md"), "opencode.instructions.marker_block", ops); err != nil {
		return nil, err
	} else if m != nil {
		mutations = append(mutations, *m)
	}

	if m, err := computeWholeFileDeleteMutation(filepath.Join(configDir, "sdd-orchestrator.md"), "opencode.orchestrator.file", ops); err != nil {
		return nil, err
	} else if m != nil {
		mutations = append(mutations, *m)
	}

	skillMutations, err := computeWholeDirectoryDeleteMutations(filepath.Join(configDir, "skills"), "opencode.skills.directory", nil, ops)
	if err != nil {
		return nil, err
	}
	mutations = append(mutations, skillMutations...)

	for _, wf := range []struct{ surfaceID, rel string }{
		{"opencode.plugins.hive_hook", filepath.Join("plugins", "hive.ts")},
		{"opencode.plugins.registry_hook", filepath.Join("plugins", "skill-registry.ts")},
	} {
		m, err := computeWholeFileDeleteMutation(filepath.Join(configDir, wf.rel), wf.surfaceID, ops)
		if err != nil {
			return nil, err
		}
		if m != nil {
			mutations = append(mutations, *m)
		}
	}

	return mutations, nil
}

func computeOpenCodeSettingsReset(before []byte) ([]byte, []ResetChange, error) {
	if len(strings.TrimSpace(string(before))) == 0 {
		return nil, nil, nil
	}
	root, err := parseOrderedJSON(before)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errResetSettingsUnparseable, err)
	}
	if root.object == nil {
		return nil, nil, fmt.Errorf("%w: top-level value is not a JSON object", errResetSettingsUnparseable)
	}

	var changes []ResetChange
	mutated := false

	coreSurface := resetSurfaceByID(sddruntime.PlatformOpenCode, "opencode.settings.core_keys")
	var extraAgentEntries []string
	if agentVal, ok := root.object.get("agent"); ok && agentVal.object != nil {
		owned := stringSet(append(append([]string{}, openCodeSDDSubagents()...), openCodeJudgmentDaySubagents()...))
		for _, pair := range agentVal.object.pairs {
			if !owned[pair.key] {
				extraAgentEntries = append(extraAgentEntries, pair.key)
			}
		}
		sort.Strings(extraAgentEntries)
	}
	var removedCoreKeys []string
	for _, key := range coreSurface.JSONPaths {
		if root.object.delete(key) {
			removedCoreKeys = append(removedCoreKeys, key)
			mutated = true
		}
	}
	if len(removedCoreKeys) > 0 {
		detail := strings.Join(removedCoreKeys, ", ")
		if len(extraAgentEntries) > 0 {
			detail += " (extra agent entries lost: " + strings.Join(extraAgentEntries, ", ") + ")"
		}
		changes = append(changes, ResetChange{SurfaceID: coreSurface.ID, Path: "opencode.json", Detail: detail})
	}

	mcpSurface := resetSurfaceByID(sddruntime.PlatformOpenCode, "opencode.settings.mcp")
	var removedMCPKeys []string
	for _, dotted := range mcpSurface.JSONPaths {
		if removeOrderedDottedKey(root.object, dotted) {
			removedMCPKeys = append(removedMCPKeys, dotted)
			mutated = true
		}
	}
	if len(removedMCPKeys) > 0 {
		changes = append(changes, ResetChange{SurfaceID: mcpSurface.ID, Path: "opencode.json", Detail: strings.Join(removedMCPKeys, ", ")})
	}

	if !mutated {
		return nil, nil, nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return append(out, '\n'), changes, nil
}

// --- Shared file/JSON helpers ---

// computeWholeFileDeleteMutation plans deleting one whole-file surface. When
// path is itself a symlink, the link is deleted and its target is never
// touched: the target might be a dotfile manager's real file shared with
// other links, so recreating the LINK on rollback (not rewriting the
// target's bytes) is the only safe behavior.
func computeWholeFileDeleteMutation(path, surfaceID string, ops resetFileOps) (*resetFileMutation, error) {
	exists, isSymlink, err := ops.lstatKind(path)
	if err != nil {
		return nil, fmt.Errorf("lstat %s: %w", path, err)
	}
	if !exists {
		return nil, nil
	}
	if isSymlink {
		target, err := ops.readLink(path)
		if err != nil {
			return nil, fmt.Errorf("readlink %s: %w", path, err)
		}
		return &resetFileMutation{
			Path: path, existed: true, isSymlink: true, linkTarget: target, after: nil,
			changes: []ResetChange{{SurfaceID: surfaceID, Path: path, Detail: "removed (symlink)"}},
		}, nil
	}

	before, existed, mode, err := ops.readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !existed {
		return nil, nil
	}
	return &resetFileMutation{
		Path: path, before: before, beforeMode: mode, existed: true, after: nil,
		changes: []ResetChange{{SurfaceID: surfaceID, Path: path, Detail: "removed"}},
	}, nil
}

// computeMarkerBlockMutation plans stripping a marker block from a text file
// edited in place. When path is itself a symlink (e.g. a dotfile manager's
// CLAUDE.md/AGENTS.md), it is resolved to its real target first: the
// returned mutation's Path is that target, so the edit lands on the real
// file's bytes and the symlink itself is never replaced.
func computeMarkerBlockMutation(path, surfaceID string, ops resetFileOps) (*resetFileMutation, error) {
	resolvedPath, err := resolveEditableSurfacePath(path, ops)
	if err != nil {
		return nil, err
	}
	before, existed, mode, err := ops.readFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", resolvedPath, err)
	}
	if !existed {
		return nil, nil
	}
	after, changed := stripMarkerBlock(string(before))
	if !changed {
		return nil, nil
	}
	return &resetFileMutation{
		Path: resolvedPath, before: before, beforeMode: mode, existed: true, after: []byte(after),
		changes: []ResetChange{{SurfaceID: surfaceID, Path: resolvedPath, Detail: "marker block content"}},
	}, nil
}

// stripMarkerBlock removes the entire Jarvis-managed block, from the Layer1
// start marker through the Layer2 end marker inclusive, leaving everything
// else in the file untouched. Reports false when both markers are not found
// in order, in which case content is returned unchanged.
func stripMarkerBlock(content string) (string, bool) {
	start := strings.Index(content, Layer1Start)
	end := strings.Index(content, Layer2End)
	if start == -1 || end == -1 || end < start {
		return content, false
	}
	end += len(Layer2End)

	before := strings.TrimRight(content[:start], "\n")
	after := strings.TrimLeft(content[end:], "\n")
	switch {
	case before == "" && after == "":
		return "", true
	case before == "":
		return after, true
	case after == "":
		return before + "\n", true
	default:
		return before + "\n\n" + after, true
	}
}

// computeWholeDirectoryDeleteMutations enumerates every regular file and
// symlink under dir and plans its deletion. When owned is non-nil, an entry
// whose base name (without extension) is not a key in owned is reported as
// user-added in its Detail, so a plan surfaces content a consented reset
// would otherwise silently discard.
//
// A symlink entry is never followed to read content: its target is recorded
// via readLink and restored via writeSymlink if the reset later rolls back,
// whether the symlink points at a file or at a directory.
func computeWholeDirectoryDeleteMutations(dir, surfaceID string, owned map[string]bool, ops resetFileOps) ([]resetFileMutation, error) {
	entries, err := ops.listDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	mutations := make([]resetFileMutation, 0, len(entries))
	for _, entry := range entries {
		rel := entry.RelPath
		path := filepath.Join(dir, rel)
		detail := filepath.ToSlash(rel)
		if owned != nil {
			base := strings.TrimSuffix(rel, filepath.Ext(rel))
			if !owned[base] {
				detail += " (user-added)"
			}
		}

		if entry.IsSymlink {
			target, err := ops.readLink(path)
			if err != nil {
				return nil, fmt.Errorf("readlink %s: %w", path, err)
			}
			mutations = append(mutations, resetFileMutation{
				Path: path, existed: true, isSymlink: true, linkTarget: target, after: nil,
				changes: []ResetChange{{SurfaceID: surfaceID, Path: path, Detail: detail + " (symlink)"}},
			})
			continue
		}

		before, existed, mode, err := ops.readFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if !existed {
			continue
		}
		mutations = append(mutations, resetFileMutation{
			Path: path, before: before, beforeMode: mode, existed: true, after: nil,
			changes: []ResetChange{{SurfaceID: surfaceID, Path: path, Detail: detail}},
		})
	}
	return mutations, nil
}

func orderedStringValue(obj *orderedObject, key string) (string, bool) {
	val, ok := obj.get(key)
	if !ok {
		return "", false
	}
	s, ok := val.scalar.(string)
	return s, ok
}

func removeOrderedDottedKey(root *orderedObject, dotted string) bool {
	parts := strings.Split(dotted, ".")
	obj := root
	for i := 0; i < len(parts)-1; i++ {
		val, ok := obj.get(parts[i])
		if !ok || val.object == nil {
			return false
		}
		obj = val.object
	}
	return obj.delete(parts[len(parts)-1])
}

func resetContainsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func stringSet(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, s := range list {
		set[s] = true
	}
	return set
}

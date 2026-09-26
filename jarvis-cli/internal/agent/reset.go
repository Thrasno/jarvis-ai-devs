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
}

// ResetRestoreError reports that ApplyReset failed AND that the automatic
// rollback did not fully restore prior state. SnapshotID names the durable
// lifecycle.BackupStore snapshot an operator can restore from by hand;
// UnrecoveredPaths lists exactly which paths the rollback could not put back.
type ResetRestoreError struct {
	SnapshotID       string
	UnrecoveredPaths []string
	Cause            error
}

func (e *ResetRestoreError) Error() string {
	return fmt.Sprintf(
		"configuration reset failed and rollback was incomplete (snapshot %s, unrecovered paths: %s): %v",
		e.SnapshotID, strings.Join(e.UnrecoveredPaths, ", "), e.Cause,
	)
}

func (e *ResetRestoreError) Unwrap() error { return e.Cause }

// resetFileOps is the injectable filesystem seam ApplyReset and PlanReset
// mutate/read through. Tests replace it to force a failure at an exact step
// without chmod tricks.
type resetFileOps struct {
	readFile   func(path string) (data []byte, existed bool, err error)
	writeFile  func(path string, data []byte) error
	removeFile func(path string) error
	listDir    func(dir string) ([]string, error)
}

func defaultResetFileOps() resetFileOps {
	return resetFileOps{
		readFile: func(path string) ([]byte, bool, error) {
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return nil, false, nil
				}
				return nil, false, err
			}
			return data, true, nil
		},
		writeFile: func(path string, data []byte) error {
			return writeFileAtomic(path, data, 0644)
		},
		removeFile: func(path string) error {
			err := os.Remove(path)
			if err != nil && os.IsNotExist(err) {
				return nil
			}
			return err
		},
		listDir: listRegularFilesRecursively,
	}
}

// listRegularFilesRecursively returns every regular file under dir, as
// dir-relative paths, sorted. An absent dir reports no files and no error:
// there is nothing to reset in a directory that was never created.
func listRegularFilesRecursively(dir string) ([]string, error) {
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
	var files []string
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, rel)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(files)
	return files, nil
}

// resetFileMutation is one file-level reset step: before is the content that
// existed at that path before the reset touched it (existed reports whether
// there was anything there at all), and after is the desired content, or nil
// to delete the path. changes is the plan-facing description this mutation
// covers (a single physical file can satisfy more than one contract surface,
// e.g. Claude settings.json covers five).
type resetFileMutation struct {
	Path    string
	before  []byte
	existed bool
	after   []byte // nil means delete
	changes []ResetChange
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

	backupTargets := make([]lifecycle.BackupTarget, 0, len(mutations))
	seen := make(map[string]bool, len(mutations))
	for _, m := range mutations {
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
			mutateErr = ops.writeFile(m.Path, m.after)
		}
		if mutateErr != nil {
			unrecovered := rollbackResetMutations(applied, ops)
			return ResetResult{}, &ResetRestoreError{
				SnapshotID:       manifest.SnapshotID,
				UnrecoveredPaths: unrecovered,
				Cause:            mutateErr,
			}
		}
		applied = append(applied, m)
	}

	return ResetResult{Platform: platform, SnapshotID: manifest.SnapshotID, Changes: collectResetChanges(mutations)}, nil
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
		if m.existed {
			err = ops.writeFile(m.Path, m.before)
		} else {
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

	settingsPath := filepath.Join(configDir, "settings.json")
	settingsBefore, existed, err := ops.readFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if existed {
		after, changes, err := computeClaudeSettingsReset(settingsBefore)
		if err != nil {
			return nil, err
		}
		if after != nil {
			mutations = append(mutations, resetFileMutation{Path: settingsPath, before: settingsBefore, existed: true, after: after, changes: changes})
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

func computeClaudeSettingsReset(before []byte) ([]byte, []ResetChange, error) {
	if len(strings.TrimSpace(string(before))) == 0 {
		return nil, nil, nil
	}
	root, err := parseOrderedJSON(before)
	if err != nil || root.object == nil {
		return nil, nil, nil
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

	settingsPath := filepath.Join(configDir, "opencode.json")
	settingsBefore, existed, err := ops.readFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settingsPath, err)
	}
	if existed {
		after, changes, err := computeOpenCodeSettingsReset(settingsBefore)
		if err != nil {
			return nil, err
		}
		if after != nil {
			mutations = append(mutations, resetFileMutation{Path: settingsPath, before: settingsBefore, existed: true, after: after, changes: changes})
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
	if err != nil || root.object == nil {
		return nil, nil, nil
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

func computeWholeFileDeleteMutation(path, surfaceID string, ops resetFileOps) (*resetFileMutation, error) {
	before, existed, err := ops.readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !existed {
		return nil, nil
	}
	return &resetFileMutation{
		Path: path, before: before, existed: true, after: nil,
		changes: []ResetChange{{SurfaceID: surfaceID, Path: path, Detail: "removed"}},
	}, nil
}

func computeMarkerBlockMutation(path, surfaceID string, ops resetFileOps) (*resetFileMutation, error) {
	before, existed, err := ops.readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !existed {
		return nil, nil
	}
	after, changed := stripMarkerBlock(string(before))
	if !changed {
		return nil, nil
	}
	return &resetFileMutation{
		Path: path, before: before, existed: true, after: []byte(after),
		changes: []ResetChange{{SurfaceID: surfaceID, Path: path, Detail: "marker block content"}},
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

// computeWholeDirectoryDeleteMutations enumerates every regular file under
// dir and plans its deletion. When owned is non-nil, a file whose base name
// (without extension) is not a key in owned is reported as user-added in its
// Detail, so a plan surfaces content a consented reset would otherwise
// silently discard.
func computeWholeDirectoryDeleteMutations(dir, surfaceID string, owned map[string]bool, ops resetFileOps) ([]resetFileMutation, error) {
	files, err := ops.listDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	mutations := make([]resetFileMutation, 0, len(files))
	for _, rel := range files {
		path := filepath.Join(dir, rel)
		before, existed, err := ops.readFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if !existed {
			continue
		}
		detail := filepath.ToSlash(rel)
		if owned != nil {
			base := strings.TrimSuffix(rel, filepath.Ext(rel))
			if !owned[base] {
				detail += " (user-added)"
			}
		}
		mutations = append(mutations, resetFileMutation{
			Path: path, before: before, existed: true, after: nil,
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

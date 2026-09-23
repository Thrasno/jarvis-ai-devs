package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

// optionalPreflightOpenSpecBinding treats an absent Hive-only change directory as
// unbound, without creating it. Existing directories retain the strict binding reader.
type optionalPreflightOpenSpecBinding struct{}

func (optionalPreflightOpenSpecBinding) ReadOpenSpec(dir string) (*sddbinding.Binding, error) {
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return sddbinding.ReadOpenSpec(dir)
}

func (optionalPreflightOpenSpecBinding) AdoptOpenSpec(string, sddbinding.Binding) (sddbinding.Binding, bool, error) {
	return sddbinding.Binding{}, false, fmt.Errorf("read-only supersession preflight cannot adopt bindings")
}

// newSupersessionPreflight is advisory only: publication must recheck under its locks.
type newSupersessionPreflightResult struct {
	Mode         sddruntime.StoreMode
	Predecessor  applyprogress.Snapshot
	TaskManifest string
	TasksContent string
}

func newSupersessionPreflight(ctx context.Context, workspace, project, predecessor, successor string, client *hiveclient.Client) (newSupersessionPreflightResult, error) {
	fail := func(err error) (newSupersessionPreflightResult, error) { return newSupersessionPreflightResult{}, err }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if client == nil || project == "" || hiveclient.CanonicalProjectKey(project) != project || !applyprogress.ValidID(predecessor) || !applyprogress.ValidID(successor) || predecessor == successor {
		return fail(fmt.Errorf("invalid supersession coordinates or missing Hive client"))
	}
	if !filepath.IsAbs(workspace) || filepath.Clean(workspace) != workspace {
		return fail(fmt.Errorf("workspace must be an absolute clean path"))
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil || resolved != workspace {
		return fail(fmt.Errorf("workspace must be an existing canonical path: %v", err))
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return fail(fmt.Errorf("workspace must be a directory: %v", err))
	}
	root := filepath.Join(workspace, "openspec", "changes", predecessor)
	open := sddprogress.OpenSpec{Root: root}
	hive := sddstatus.NewHiveSource(client, project)
	local := sddstatus.NewOpenSpecSourceForProject(workspace, project)
	binding, err := (sddbinding.LegacyResolver{OpenSpecChangeDir: root, HiveBindings: client, OpenSpecBindings: optionalPreflightOpenSpecBinding{}}).ResolveExisting(ctx, project, predecessor)
	if err != nil {
		return fail(fmt.Errorf("persisted binding: %w", err))
	}
	if !binding.Persisted || binding.Mode == sddruntime.StoreModeNone {
		return fail(fmt.Errorf("NEW seal requires a persisted non-none binding"))
	}
	var result newSupersessionPreflightResult
	result.Mode = binding.Mode
	var localManifest, hiveManifest, localTasks, hiveTasks string
	inspectTasks := func(source sddstatus.ArtifactSource) (string, string, error) {
		artifacts, contents, err := source.FetchArtifacts(ctx, predecessor)
		if err != nil {
			return "", "", err
		}
		// The historical PARTIAL head must disagree only with the revised task
		// manifest. Any other state may mean the head changed between reads.
		if artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedManifestMismatch {
			return "", "", fmt.Errorf("revised tasks read has unexpected progress state %q", artifacts[sddstatus.ArtifactApplyProgress])
		}
		if artifacts[sddstatus.ArtifactTasks] != sddstatus.ArtifactDone {
			return "", "", fmt.Errorf("revised tasks artifact missing")
		}
		tasks := contents[sddstatus.ArtifactTasks]
		parsed, err := applyprogress.ParseTasksMarkdown(tasks)
		if err != nil || len(parsed.Tasks) == 0 || parsed.Completed != 0 {
			return "", "", fmt.Errorf("revised tasks must contain only fresh valid tasks: %v", err)
		}
		_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
		return manifest, tasks, err
	}
	switch binding.Mode {
	case sddruntime.StoreModeOpenSpec, sddruntime.StoreModeHybrid:
		snapshot, err := open.InspectSealablePredecessor(project, predecessor)
		if err != nil {
			return fail(fmt.Errorf("OpenSpec predecessor (NEW seal requires PARTIAL): %w", err))
		}
		result.Predecessor = *snapshot
		localManifest, localTasks, err = inspectTasks(local)
		if err != nil {
			return fail(fmt.Errorf("OpenSpec revised tasks: %w", err))
		}
	case sddruntime.StoreModeHive:
	default:
		return fail(fmt.Errorf("unsupported binding mode %q", binding.Mode))
	}
	if binding.Mode == sddruntime.StoreModeHive || binding.Mode == sddruntime.StoreModeHybrid {
		snapshot, err := hive.InspectSealablePredecessor(ctx, predecessor)
		if err != nil {
			return fail(fmt.Errorf("Hive predecessor (NEW seal requires PARTIAL): %w", err))
		}
		if binding.Mode == sddruntime.StoreModeHybrid {
			if snapshot.Digest != result.Predecessor.Digest || snapshot.Generation != result.Predecessor.Generation || snapshot.Revision != result.Predecessor.Revision || snapshot.Project != result.Predecessor.Project || snapshot.Change != result.Predecessor.Change {
				return fail(fmt.Errorf("hybrid predecessor heads diverged"))
			}
		} else {
			result.Predecessor = snapshot
		}
		hiveManifest, hiveTasks, err = inspectTasks(hive)
		if err != nil {
			return fail(fmt.Errorf("Hive revised tasks: %w", err))
		}
	}
	result.TaskManifest, result.TasksContent = localManifest, localTasks
	if binding.Mode == sddruntime.StoreModeHive {
		result.TaskManifest, result.TasksContent = hiveManifest, hiveTasks
	}
	if result.TaskManifest == result.Predecessor.TaskManifestSHA256 || result.TaskManifest == "" {
		return fail(fmt.Errorf("revised task manifest must differ from historical predecessor"))
	}
	if binding.Mode == sddruntime.StoreModeHybrid && (localManifest != hiveManifest || localTasks != hiveTasks) {
		return fail(fmt.Errorf("hybrid revised tasks diverged"))
	}
	if binding.Mode != sddruntime.StoreModeHive {
		if err := open.InspectSuccessorVacancy(sddprogress.OpenSpec{Root: filepath.Join(workspace, "openspec", "changes", successor)}, successor); err != nil {
			return fail(fmt.Errorf("OpenSpec successor occupied or unsafe: %w", err))
		}
	}
	if binding.Mode != sddruntime.StoreModeOpenSpec {
		occupancy, err := client.GetApplyProgressSuccessorOccupancy(ctx, project, successor)
		if err != nil {
			return fail(fmt.Errorf("Hive successor occupancy: %w", err))
		}
		if occupancy.Occupied {
			return fail(fmt.Errorf("Hive successor occupied: %s", occupancy.Category))
		}
	}
	return result, nil
}

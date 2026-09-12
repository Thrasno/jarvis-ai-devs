package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

// ArtifactBlockedBackendDiverged means independently read v2 stores disagree.
const ArtifactBlockedBackendDiverged ArtifactState = "blocked:backend_diverged"

// ArtifactSource fetches SDD artifact states from a backing store.
type ArtifactSource interface {
	// FetchArtifacts returns the observable state and optional content for each
	// known artifact of the given change.
	FetchArtifacts(ctx context.Context, changeName string) (artifacts map[string]ArtifactState, contents map[string]string, err error)
	// ListChanges returns the names of all SDD changes known to the backing store.
	ListChanges(ctx context.Context) ([]string, error)
}

// HiveSource reads SDD artifacts from a running hive-daemon.
type HiveSource struct {
	client  *hiveclient.Client
	project string
}

// NewHiveSource returns an ArtifactSource backed by the hive-daemon.
func NewHiveSource(client *hiveclient.Client, project string) *HiveSource {
	return &HiveSource{client: client, project: project}
}

func (h *HiveSource) FetchArtifacts(ctx context.Context, changeName string) (map[string]ArtifactState, map[string]string, error) {
	rows, err := h.client.FetchSDDArtifacts(ctx, h.project, changeName)
	if err != nil {
		return nil, nil, err
	}
	artifacts := make(map[string]ArtifactState, len(rows))
	contents := make(map[string]string, len(rows))
	for _, row := range rows {
		artifacts[row.Artifact] = ArtifactDone
		if row.Content != "" {
			contents[row.Artifact] = row.Content
		}
	}
	if content, ok := contents[ArtifactApplyProgress]; ok {
		if strings.HasPrefix(content, `{"schema":"jarvis.sdd-apply-progress/v2"`) {
			progress, err := h.client.GetApplyProgress(ctx, h.project, changeName)
			if err == nil {
				artifacts[ArtifactApplyProgress] = snapshotProgressState(progress.State.Snapshot, contents[ArtifactTasks])
			} else {
				var typed *hiveclient.ApplyProgressError
				if !errors.As(err, &typed) {
					return nil, nil, err
				}
				artifacts[ArtifactApplyProgress] = typedApplyProgressState(typed.Result.Outcome)
			}
		} else {
			artifacts[ArtifactApplyProgress] = applyProgressState(content, contents[ArtifactTasks])
		}
	}
	return artifacts, contents, nil
}

func snapshotProgressState(snapshot applyprogress.Snapshot, tasksContent string) ArtifactState {
	if tasks, ok := applyProgressTasks(tasksContent); ok {
		_, manifest, err := applyprogress.TaskManifest(tasks)
		if err != nil || snapshot.TaskManifestSHA256 != manifest {
			return ArtifactBlockedManifestMismatch
		}
	}
	if snapshot.Status == applyprogress.StatusComplete {
		return ArtifactDone
	}
	return ArtifactPartial
}

func applyProgressTasks(content string) ([]applyprogress.Task, bool) {
	var tasks []applyprogress.Task
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		end := strings.Index(line, "]")
		if end < 0 {
			continue
		}
		fields := strings.Fields(line[end+1:])
		if len(fields) < 2 || fields[0][0] < '0' || fields[0][0] > '9' {
			continue
		}
		tasks = append(tasks, applyprogress.Task{ID: fields[0], Text: strings.Join(fields[1:], " ")})
	}
	return tasks, len(tasks) > 0
}

func typedApplyProgressState(outcome string) ArtifactState {
	switch outcome {
	case "continuation_required":
		return ArtifactBlockedContinuation
	case "conflict":
		return ArtifactBlockedConflict
	default:
		if outcome != "" {
			return ArtifactBlockedInvalid
		}
		return ArtifactMissing
	}
}

func (h *HiveSource) ListChanges(ctx context.Context) ([]string, error) {
	const pageSize = 100
	var changes []string
	cursor := ""
	for {
		page, err := h.client.ListSDDChanges(ctx, h.project, hiveclient.SDDPageRequest{Limit: pageSize, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		changes = append(changes, page.Changes...)
		if page.NextCursor == "" {
			return changes, nil
		}
		if page.NextCursor == cursor {
			return nil, fmt.Errorf("hive-daemon returned a repeated SDD change cursor")
		}
		cursor = page.NextCursor
	}
}

// OpenSpecSource reads SDD artifacts from an openspec directory layout:
// openspec/changes/{change}/{artifact}.md
type OpenSpecSource struct {
	root string // project root; openspec/ is resolved relative to this
}

// NewOpenSpecSource returns an ArtifactSource backed by the openspec filesystem layout.
func NewOpenSpecSource(projectRoot string) *OpenSpecSource {
	return &OpenSpecSource{root: projectRoot}
}

func (o *OpenSpecSource) changeDir(changeName string) string {
	return filepath.Join(o.root, "openspec", "changes", changeName)
}

func (o *OpenSpecSource) FetchArtifacts(_ context.Context, changeName string) (map[string]ArtifactState, map[string]string, error) {
	dir := o.changeDir(changeName)
	artifacts := make(map[string]ArtifactState)
	contents := make(map[string]string)

	artifactFiles := map[string]string{
		ArtifactExplore:       "explore.md",
		ArtifactProposal:      "proposal.md",
		ArtifactSpec:          "spec.md",
		ArtifactDesign:        "design.md",
		ArtifactTasks:         "tasks.md",
		ArtifactApplyProgress: "apply-progress.md",
		ArtifactVerifyReport:  "verify-report.md",
		ArtifactArchiveReport: "archive-report.md",
	}

	for artifact, filename := range artifactFiles {
		path := filepath.Join(dir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		content := string(data)
		artifacts[artifact] = ArtifactDone
		if content != "" {
			contents[artifact] = content
		}
	}
	if data, ok := contents[ArtifactApplyProgress]; ok {
		if isV2Progress(data) {
			artifacts[ArtifactApplyProgress] = openSpecV2ProgressState(dir, []byte(data), contents[ArtifactTasks])
		} else {
			artifacts[ArtifactApplyProgress] = applyProgressState(data, contents[ArtifactTasks])
		}
	}

	return artifacts, contents, nil
}

func openSpecV2ProgressState(dir string, data []byte, tasksContent string) ArtifactState {
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(data)
	if err != nil {
		return ArtifactBlockedInvalid
	}
	tasks, ok := applyProgressTasks(tasksContent)
	if !ok {
		return ArtifactBlockedInvalid
	}
	batches := make(map[string][]byte, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, err := os.ReadFile(filepath.Join(dir, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			return ArtifactBlockedInvalid
		}
		batches[ref.BatchID] = batch
	}
	if err := applyprogress.ValidateProgress(data, tasks, batches); err != nil {
		var validation *applyprogress.ValidationError
		if errors.As(err, &validation) && validation.Code == applyprogress.CodeTaskManifestMismatch {
			return ArtifactBlockedManifestMismatch
		}
		return ArtifactBlockedInvalid
	}
	return snapshotProgressState(snapshot, tasksContent)
}

// applyProgressState accepts only exact, line-oriented status markers. A legacy
// unmarked artifact is complete only when task checkboxes provide deterministic
// all-complete evidence. Unknown, malformed, and conflicting markers fail closed
// as partial so incomplete work cannot enable verification.
func applyProgressState(progressContent, tasksContent string) ArtifactState {
	hasCompleteMarker := false
	hasPartialMarker := false
	for _, line := range strings.Split(progressContent, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "status: partial":
			hasPartialMarker = true
		case "status: complete":
			hasCompleteMarker = true
		default:
			if strings.HasPrefix(line, "status:") {
				return ArtifactPartial
			}
		}
	}
	if hasPartialMarker {
		return ArtifactPartial
	}
	if hasCompleteMarker {
		return ArtifactDone
	}
	if progress := parseTaskProgress(tasksContent); progress != nil && progress.AllDone {
		return ArtifactDone
	}
	return ArtifactPartial
}

func (o *OpenSpecSource) ListChanges(_ context.Context) ([]string, error) {
	dir := filepath.Join(o.root, "openspec", "changes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var changes []string
	for _, e := range entries {
		if e.IsDir() {
			changes = append(changes, e.Name())
		}
	}
	sort.Strings(changes)
	return changes, nil
}

// HybridSource reads from both Hive and OpenSpec, merging results with Hive taking priority.
type HybridSource struct {
	hive     *HiveSource
	openspec *OpenSpecSource
}

// NewHybridSource returns an ArtifactSource that reads from both Hive and OpenSpec.
func NewHybridSource(hive *HiveSource, openspec *OpenSpecSource) *HybridSource {
	return &HybridSource{hive: hive, openspec: openspec}
}

func (h *HybridSource) FetchArtifacts(ctx context.Context, changeName string) (map[string]ArtifactState, map[string]string, error) {
	osArtifacts, osContents, osErr := h.openspec.FetchArtifacts(ctx, changeName)
	hiveArtifacts, hiveContents, hiveErr := h.hive.FetchArtifacts(ctx, changeName)

	// If both fail, report both errors.
	if osErr != nil && hiveErr != nil {
		return nil, nil, fmt.Errorf("openspec: %w; hive: %v", osErr, hiveErr)
	}

	merged := make(map[string]ArtifactState)
	mergedContents := make(map[string]string)

	if osErr == nil {
		for k, v := range osArtifacts {
			merged[k] = v
		}
		for k, v := range osContents {
			mergedContents[k] = v
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: openspec source unavailable: %v\n", osErr)
	}

	// Hive takes priority: overwrite openspec entries when Hive has them.
	if hiveErr == nil {
		for k, v := range hiveArtifacts {
			merged[k] = v
		}
		for k, v := range hiveContents {
			mergedContents[k] = v
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: hive source unavailable: %v\n", hiveErr)
	}

	// Legacy progress can use task evidence from either side. V2 must instead be
	// independently valid on both sides and semantically identical.
	if isV2Progress(osContents[ArtifactApplyProgress]) || isV2Progress(hiveContents[ArtifactApplyProgress]) {
		if osErr != nil || hiveErr != nil || osArtifacts[ArtifactApplyProgress] == "" || hiveArtifacts[ArtifactApplyProgress] == "" || isBlockedApplyProgress(osArtifacts[ArtifactApplyProgress]) || isBlockedApplyProgress(hiveArtifacts[ArtifactApplyProgress]) || !sameV2Progress(osContents[ArtifactApplyProgress], hiveContents[ArtifactApplyProgress]) {
			merged[ArtifactApplyProgress] = ArtifactBlockedBackendDiverged
			delete(mergedContents, ArtifactApplyProgress)
		}
	} else if state, ok := merged[ArtifactApplyProgress]; ok && !strings.HasPrefix(string(state), "blocked:") {
		merged[ArtifactApplyProgress] = applyProgressState(mergedContents[ArtifactApplyProgress], mergedContents[ArtifactTasks])
	}

	return merged, mergedContents, nil
}

func isV2Progress(data string) bool {
	return strings.HasPrefix(data, `{"schema":"jarvis.sdd-apply-progress/v2"`)
}

func sameV2Progress(left, right string) bool {
	one, err := applyprogress.DecodeCanonicalSnapshot([]byte(left))
	if err != nil {
		return false
	}
	two, err := applyprogress.DecodeCanonicalSnapshot([]byte(right))
	return err == nil && one.Generation == two.Generation && one.Revision == two.Revision && one.TaskManifestSHA256 == two.TaskManifestSHA256 && one.Digest == two.Digest && slices.Equal(one.Batches, two.Batches) && slices.Equal(one.Coverage, two.Coverage)
}

func (h *HybridSource) ListChanges(ctx context.Context) ([]string, error) {
	hiveChanges, hiveErr := h.hive.ListChanges(ctx)
	osChanges, osErr := h.openspec.ListChanges(ctx)

	if hiveErr != nil && osErr != nil {
		return nil, fmt.Errorf("hive: %w; openspec: %v", hiveErr, osErr)
	}

	seen := make(map[string]struct{}, len(hiveChanges)+len(osChanges))
	for _, c := range hiveChanges {
		seen[c] = struct{}{}
	}
	for _, c := range osChanges {
		seen[c] = struct{}{}
	}

	all := make([]string, 0, len(seen))
	for name := range seen {
		all = append(all, name)
	}
	sort.Strings(all)
	return all, nil
}

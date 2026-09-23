package sddstatus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
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

// LegacyProgressObservation is the read-only protected progress found in one
// store before an artifact-store binding exists. Planning artifacts are not
// part of this authority decision.
type LegacyProgressObservation struct {
	Present bool
	State   ArtifactState
	Content string
}

// ObserveLegacyProgress reads one source without mutating it and projects only
// its protected apply-progress state. Source failures remain failures because
// an unavailable store cannot be treated as empty safely.
func ObserveLegacyProgress(ctx context.Context, source ArtifactSource, changeName string) (LegacyProgressObservation, error) {
	artifacts, contents, err := source.FetchArtifacts(ctx, changeName)
	if err != nil {
		return LegacyProgressObservation{}, err
	}
	state, present := artifacts[ArtifactApplyProgress]
	if !present {
		return LegacyProgressObservation{}, nil
	}
	return LegacyProgressObservation{Present: true, State: state, Content: contents[ArtifactApplyProgress]}, nil
}

// LegacyProgressEquivalent reports whether two unbound stores prove the same
// protected progress. Blocked observations never prove equality. Canonical v2
// snapshots use the existing normalized equality contract; older progress must
// match exactly and cannot be blank.
func LegacyProgressEquivalent(left, right LegacyProgressObservation) bool {
	if !left.Present || !right.Present {
		return left.Present == right.Present
	}
	if isBlockedApplyProgress(left.State) || isBlockedApplyProgress(right.State) {
		return false
	}
	_, leftV2Err := applyprogress.DecodeCanonicalSnapshot([]byte(left.Content))
	_, rightV2Err := applyprogress.DecodeCanonicalSnapshot([]byte(right.Content))
	if leftV2Err == nil || rightV2Err == nil {
		return leftV2Err == nil && rightV2Err == nil && sameV2Progress(left.Content, right.Content)
	}
	if resemblesV2Progress(left.Content) || resemblesV2Progress(right.Content) {
		return false
	}
	return left.State != "" && strings.TrimSpace(left.Content) != "" && left.State == right.State && left.Content == right.Content
}

func resemblesV2Progress(content string) bool {
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "{") || strings.Contains(content, applyprogress.SnapshotSchema)
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
	// The guarded v2 head is authoritative even when no legacy general-artifact
	// projection exists. The old exact-topic row remains a compatibility fallback
	// only when an older daemon cannot provide a typed head.
	progress, progressErr := h.client.GetApplyProgress(ctx, h.project, changeName)
	if progressErr == nil && isGuardedSchema(progress.State.Snapshot.Schema) {
		if progress.State.Snapshot.Project != h.project || progress.State.Snapshot.Change != changeName {
			artifacts[ArtifactApplyProgress] = ArtifactBlockedInvalid
			delete(contents, ArtifactApplyProgress)
			return artifacts, contents, nil
		}
		if len(progress.State.Batches) == 0 && len(progress.State.Snapshot.Batches) != 0 {
			batches, err := h.fetchGuardedEvidence(ctx, changeName, progress.State.Snapshot)
			if err != nil {
				// Evidence is fetched over its own bounded endpoint. A transport or
				// availability failure says nothing about immutable bytes, so retain
				// the typed retryable fetch error instead of inventing corruption.
				return nil, nil, err
			}
			progress.State.Batches = batches
		}
		state, data := guardedHiveProgressState(progress.State, contents[ArtifactTasks])
		artifacts[ArtifactApplyProgress] = state
		if data != nil {
			contents[ArtifactApplyProgress] = string(data)
		}
		return artifacts, contents, nil
	}
	if progressErr == nil {
		if progress.State.Snapshot.Schema != "" {
			artifacts[ArtifactApplyProgress] = ArtifactBlockedInvalid
			delete(contents, ArtifactApplyProgress)
			return artifacts, contents, nil
		}
		// A pre-v2 endpoint can return unrelated success JSON. It is not a head,
		// so preserve only the exact-topic compatibility projection.
		if content, ok := contents[ArtifactApplyProgress]; ok {
			artifacts[ArtifactApplyProgress] = applyProgressState(content, contents[ArtifactTasks])
		}
		return artifacts, contents, nil
	}
	var typed *hiveclient.ApplyProgressError
	if errors.As(progressErr, &typed) {
		if typed.Result.Code == "not_found" || typed.Result.Code == "compatibility" {
			// Older daemons return either a typed 404 or a non-JSON response. In
			// both cases their exact-topic artifact is the legacy compatibility
			// contract, not malformed v2 data.
			if content := contents[ArtifactApplyProgress]; content != "" {
				artifacts[ArtifactApplyProgress] = applyProgressState(content, contents[ArtifactTasks])
				return artifacts, contents, nil
			}
		}
		if typed.Result.Code == "unavailable" {
			// A live daemon explicitly reporting unavailable is a fetch failure;
			// do not reclassify it as repairable invalid artifact content.
			return nil, nil, progressErr
		}
		artifacts[ArtifactApplyProgress] = typedApplyProgressState(typed.Result.Code)
		if typed.Result.Detail != "" {
			// ArtifactState retains the stable machine code; contents carries the
			// daemon-provided typed detail for status/rendering without converting
			// a retryable API response into malformed immutable content.
			contents[ArtifactApplyProgress] = typed.Result.Detail
		}
		return artifacts, contents, nil
	}
	if content, ok := contents[ArtifactApplyProgress]; ok {
		artifacts[ArtifactApplyProgress] = applyProgressState(content, contents[ArtifactTasks])
		return artifacts, contents, nil
	}
	return nil, nil, progressErr
}

// fetchGuardedEvidence reads each current snapshot reference independently. The
// snapshot endpoint never aggregates historical evidence bodies.
func (h *HiveSource) fetchGuardedEvidence(ctx context.Context, changeName string, snapshot applyprogress.Snapshot) ([]applyprogress.Batch, error) {
	batches := make([]applyprogress.Batch, 0, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, err := h.client.GetApplyProgressEvidence(ctx, h.project, changeName, ref.BatchID, snapshot.Digest)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, nil
}

// guardedHiveProgressState verifies every referenced evidence document fetched by
// ID from the guarded endpoint. It intentionally validates the complete protocol
// rather than inferring progress from snapshot coverage alone.
func guardedHiveProgressState(state hiveclient.ApplyProgressState, tasksContent string) (ArtifactState, []byte) {
	snapshot := state.Snapshot
	// The nested snapshot is the authenticated immutable authority. A daemon
	// response that projects different top-level CAS coordinates is inconsistent,
	// even if each field is individually well-formed.
	if state.CoordinatesPresent && (state.Generation != snapshot.Generation || state.Revision != snapshot.Revision || state.Digest != snapshot.Digest) {
		return ArtifactBlockedInvalid, nil
	}
	if snapshot.Schema == applyprogress.SupersessionSnapshotSchema && snapshot.Status == applyprogress.StatusSuperseded {
		if applyprogress.VerifySnapshot(snapshot) != nil || snapshot.SealIntent == nil || len(snapshot.Batches) != len(state.Batches) {
			return ArtifactBlockedInvalid, nil
		}
		batches := make(map[string]applyprogress.Batch, len(state.Batches))
		for i, ref := range snapshot.Batches {
			batch := state.Batches[i]
			if batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
				return ArtifactBlockedInvalid, nil
			}
			sealed, _, err := applyprogress.SealBatch(batch)
			if err != nil || sealed.SHA256 != ref.SHA256 {
				return ArtifactBlockedInvalid, nil
			}
			batches[ref.BatchID] = batch
		}
		if applyprogress.ValidateEvidenceCoverage(snapshot, batches) != nil {
			return ArtifactBlockedInvalid, nil
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return ArtifactBlockedInvalid, nil
		}
		return ArtifactSuperseded, data
	}
	strictTasks, strict := applyProgressTasks(tasksContent)
	var normalized []applyprogress.Task
	var ok bool
	if strict {
		normalized, ok = snapshotTasks(snapshot, strictTasks)
		if !ok {
			return ArtifactBlockedManifestMismatch, nil
		}
	} else {
		normalized, _, ok = authoritativeHiveSnapshotTasks(snapshot, tasksContent)
		if !ok {
			return ArtifactBlockedInvalid, nil
		}
	}
	// MarshalJSON retains decoder-confirmed historical explicit-zero fields and
	// their authenticated digest. SealSnapshot would normalize that legacy wire
	// shape and silently change the bytes used for hybrid equality.
	snapshotData, err := json.Marshal(snapshot)
	if err != nil || applyprogress.VerifySnapshot(snapshot) != nil || len(state.Batches) != len(snapshot.Batches) {
		return ArtifactBlockedInvalid, nil
	}
	batches := make(map[string][]byte, len(state.Batches))
	for i, ref := range snapshot.Batches {
		batch := state.Batches[i]
		if batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return ArtifactBlockedInvalid, nil
		}
		_, batchData, err := applyprogress.SealBatch(batch)
		if err != nil {
			return ArtifactBlockedInvalid, nil
		}
		batches[ref.BatchID] = batchData
	}
	if err := applyprogress.ValidateProgress(snapshotData, normalized, batches); err != nil {
		var validation *applyprogress.ValidationError
		if errors.As(err, &validation) && validation.Code == applyprogress.CodeTaskManifestMismatch {
			return ArtifactBlockedManifestMismatch, nil
		}
		return ArtifactBlockedInvalid, nil
	}
	return snapshotProgressState(snapshot, tasksContent), snapshotData
}

func snapshotProgressState(snapshot applyprogress.Snapshot, tasksContent string) ArtifactState {
	normalized, allDone, ok := authoritativeHiveSnapshotTasks(snapshot, tasksContent)
	if !ok {
		return ArtifactBlockedManifestMismatch
	}
	if snapshot.Status == applyprogress.StatusComplete {
		if !allDone || !completeTaskCoverage(snapshot, normalized) {
			return ArtifactBlockedInvalid
		}
		return ArtifactDone
	}
	if snapshot.Status == applyprogress.StatusPartial && completeTaskCoverage(snapshot, normalized) && snapshot.StreamSHA256 == "" {
		return ArtifactBlockedInvalid
	}
	return ArtifactPartial
}

func completeTaskCoverage(snapshot applyprogress.Snapshot, tasks []applyprogress.Task) bool {
	if len(snapshot.Coverage) != len(tasks) {
		return false
	}
	covered := make(map[string]struct{}, len(snapshot.Coverage))
	for _, coverage := range snapshot.Coverage {
		if coverage.TaskID == "" {
			return false
		}
		if _, duplicate := covered[coverage.TaskID]; duplicate {
			return false
		}
		covered[coverage.TaskID] = struct{}{}
	}
	for _, task := range tasks {
		if _, found := covered[task.ID]; !found {
			return false
		}
	}
	return true
}

func applyProgressTasks(content string) ([]applyprogress.Task, bool) {
	parsed, err := applyprogress.ParseTasksMarkdown(content)
	if err != nil || len(parsed.Tasks) == 0 {
		return nil, false
	}
	for _, task := range parsed.Tasks {
		if task.Text == "" {
			return nil, false
		}
	}
	return parsed.Tasks, true
}

// snapshotTasks accepts either the native task identity or the deterministic
// legacy identity used by an authorized v1 upgrade.
func snapshotTasks(snapshot applyprogress.Snapshot, tasks []applyprogress.Task) ([]applyprogress.Task, bool) {
	if _, manifest, err := applyprogress.TaskManifest(tasks); err == nil && snapshot.TaskManifestSHA256 == manifest {
		return tasks, true
	}
	legacy, manifest, err := applyprogress.LegacyTaskManifest(tasks)
	if err == nil && snapshot.TaskManifestSHA256 == manifest {
		return legacy, true
	}
	return nil, false
}

// authoritativeHiveSnapshotTasks keeps tolerant parsing out of ordinary v2
// checkpoints. A legacy parser is permitted only after the migrated snapshot's
// manifest proves that it uses imported deterministic legacy identities.
func authoritativeHiveSnapshotTasks(snapshot applyprogress.Snapshot, content string) ([]applyprogress.Task, bool, bool) {
	if parsed, err := applyprogress.ParseTasksMarkdown(content); err == nil && len(parsed.Tasks) > 0 {
		tasks, ok := snapshotTasks(snapshot, parsed.Tasks)
		return tasks, parsed.AllDone, ok
	}
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(content)
	if err != nil || len(parsed.Tasks) == 0 {
		return nil, false, false
	}
	legacy, manifest, err := applyprogress.LegacyTaskManifest(parsed.Tasks)
	if err != nil || snapshot.TaskManifestSHA256 != manifest {
		return nil, false, false
	}
	return legacy, parsed.AllDone, true
}

func typedApplyProgressState(code string) ArtifactState {
	switch code {
	case "not_found", "compatibility":
		return ArtifactMissing
	case "continuation_required":
		return ArtifactBlockedContinuation
	case "conflict", "stale":
		return ArtifactBlockedConflict
	case "unavailable":
		return ArtifactState("blocked:unavailable")
	case "validation", "invalid":
		return ArtifactBlockedInvalid
	default:
		// ValidationError.Code is already a stable protocol value. Keep it as
		// the typed blocked discriminator rather than flattening it to invalid.
		return ArtifactState("blocked:" + code)
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
	root    string // project root; openspec/ is resolved relative to this
	project string // optional expected project identity
}

// NewOpenSpecSource returns an ArtifactSource backed by the openspec filesystem layout.
func NewOpenSpecSource(projectRoot string) *OpenSpecSource {
	return &OpenSpecSource{root: projectRoot}
}

// NewOpenSpecSourceForProject binds guarded progress to an expected project.
func NewOpenSpecSourceForProject(projectRoot, project string) *OpenSpecSource {
	return &OpenSpecSource{root: projectRoot, project: project}
}

func (o *OpenSpecSource) changeDir(changeName string) string {
	return filepath.Join(o.root, "openspec", "changes", changeName)
}

func validateOpenSpecPath(path string) error {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	if err := sddprogress.ValidateExistingPathComponents(path); err != nil {
		return fmt.Errorf("unsafe OpenSpec path %q", path)
	}
	return nil
}

func readOpenSpecRegular(path string) ([]byte, error) {
	if err := validateOpenSpecPath(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, err
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe OpenSpec file %q", path)
	}
	return os.ReadFile(path)
}

func readOpenSpecDir(path string) ([]os.DirEntry, error) {
	if err := validateOpenSpecPath(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, err
	}
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("unsafe OpenSpec directory %q", path)
	}
	return os.ReadDir(path)
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
		data, err := readOpenSpecRegular(path)
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
	if deltaSpecs, found, err := readCanonicalDeltaSpecs(filepath.Join(dir, "specs")); err != nil {
		return nil, nil, err
	} else if found {
		artifacts[ArtifactSpec] = ArtifactDone
		contents[ArtifactSpec] = deltaSpecs
	}
	var inspectionErr error
	var inspected *applyprogress.Snapshot
	if _, dirErr := readOpenSpecDir(dir); dirErr == nil {
		inspected, inspectionErr = (sddprogress.OpenSpec{Root: dir}).InspectPublication()
	} else if !os.IsNotExist(dirErr) {
		return nil, nil, dirErr
	}
	if inspected != nil {
		if inspected.Change != changeName || (o.project != "" && inspected.Project != o.project) {
			artifacts[ArtifactApplyProgress] = ArtifactBlockedInvalid
			delete(contents, ArtifactApplyProgress)
			return artifacts, contents, nil
		}
		canonical, err := json.Marshal(inspected)
		if err != nil {
			artifacts[ArtifactApplyProgress] = ArtifactBlockedInvalid
			delete(contents, ArtifactApplyProgress)
			return artifacts, contents, nil
		}
		// Reject a changed head rather than comparing bytes from different reads.
		readback, err := readOpenSpecRegular(filepath.Join(dir, "apply-progress.md"))
		if err != nil || !bytes.Equal(readback, canonical) {
			artifacts[ArtifactApplyProgress] = ArtifactBlockedInvalid
			delete(contents, ArtifactApplyProgress)
			return artifacts, contents, nil
		}
		contents[ArtifactApplyProgress] = string(canonical)
		artifacts[ArtifactApplyProgress] = ArtifactDone
	}
	if state := inspectionErrorState(inspectionErr); state != "" {
		artifacts[ArtifactApplyProgress] = state
		delete(contents, ArtifactApplyProgress)
	} else if data, ok := contents[ArtifactApplyProgress]; ok {
		if inspected != nil && inspected.Schema == applyprogress.SupersessionSnapshotSchema && inspected.Status == applyprogress.StatusSuperseded && inspected.SealIntent != nil {
			artifacts[ArtifactApplyProgress] = ArtifactSuperseded
		} else if isV2Progress(data) {
			artifacts[ArtifactApplyProgress] = openSpecV2ProgressState(dir, []byte(data), contents[ArtifactTasks])
		} else {
			artifacts[ArtifactApplyProgress] = applyProgressState(data, contents[ArtifactTasks])
		}
	}

	return artifacts, contents, nil
}

func inspectionErrorState(err error) ArtifactState {
	var validation *applyprogress.ValidationError
	switch {
	case err == nil, errors.Is(err, sddprogress.ErrLegacyMigration):
		return ""
	case errors.Is(err, sddprogress.ErrPublicationInterrupted):
		return ArtifactBlockedPublicationInterrupted
	case errors.As(err, &validation) && validation.Code == applyprogress.CodeTaskManifestMismatch:
		return ArtifactBlockedManifestMismatch
	default:
		return ArtifactBlockedInvalid
	}
}

// readCanonicalDeltaSpecs discovers every non-empty delta specification under
// specs/<domain>/spec.md. Nested domains are canonical OpenSpec topology.
func readCanonicalDeltaSpecs(root string) (string, bool, error) {
	if _, err := readOpenSpecDir(root); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	var specs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe OpenSpec path %q", path)
		}
		if entry.IsDir() || entry.Name() != "spec.md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.Dir(rel) == "." {
			return nil
		}
		data, err := readOpenSpecRegular(path)
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return fmt.Errorf("canonical delta specification %q is empty", path)
		}
		specs = append(specs, string(data))
		return nil
	})
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	sort.Strings(specs)
	return strings.Join(specs, "\n"), len(specs) != 0, nil
}

func openSpecV2ProgressState(dir string, data []byte, tasksContent string) ArtifactState {
	snapshot, err := applyprogress.DecodeCanonicalSnapshot(data)
	if err != nil {
		return ArtifactBlockedInvalid
	}
	if snapshot.Status == applyprogress.StatusSuperseded {
		return ArtifactBlockedInvalid
	}
	// A phase-style tasks.md has no strict v2 IDs. It remains readable only when
	// the immutable snapshot proves the deterministic legacy manifest; ordinary
	// v2 manifests still fail closed through authoritativeHiveSnapshotTasks.
	tasks, _, ok := authoritativeHiveSnapshotTasks(snapshot, tasksContent)
	if !ok {
		return ArtifactBlockedManifestMismatch
	}
	batches := make(map[string][]byte, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		batch, err := readOpenSpecRegular(filepath.Join(dir, "apply-evidence", ref.BatchID+".json"))
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
	if isV2Progress(progressContent) {
		return ArtifactBlockedInvalid
	}
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
	if hasCompleteMarker && legacyTasksComplete(tasksContent) {
		return ArtifactDone
	}
	if !hasCompleteMarker && legacyTasksComplete(tasksContent) {
		return ArtifactDone
	}
	return ArtifactPartial
}

func legacyTasksComplete(content string) bool {
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(content)
	return err == nil && parsed.AllDone
}

func (o *OpenSpecSource) ListChanges(_ context.Context) ([]string, error) {
	dir := filepath.Join(o.root, "openspec", "changes")
	entries, err := readOpenSpecDir(dir)
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
	// independently valid on both sides and semantically identical, except for an
	// archived change: archive atomically removes the active OpenSpec topology while
	// immutable Hive history intentionally retains its complete guarded head.
	if isV2Progress(osContents[ArtifactApplyProgress]) || isV2Progress(hiveContents[ArtifactApplyProgress]) {
		if osArtifacts[ArtifactApplyProgress] == ArtifactBlockedPublicationInterrupted {
			// OpenSpec's read-only inspection found durable but unresolved receipt
			// lineage. This is more actionable than generic backend divergence and
			// must remain visible until exact recovery resolves it.
			merged[ArtifactApplyProgress] = ArtifactBlockedPublicationInterrupted
			delete(mergedContents, ArtifactApplyProgress)
		} else if h.archivedHybridOpenSpec(changeName, osArtifacts, osContents, hiveArtifacts, hiveContents) {
			merged[ArtifactApplyProgress] = hiveArtifacts[ArtifactApplyProgress]
			merged[ArtifactArchiveReport] = ArtifactDone
		} else if osErr == nil && hiveErr == nil && osArtifacts[ArtifactApplyProgress] == ArtifactSuperseded && hiveArtifacts[ArtifactApplyProgress] == ArtifactSuperseded && sameV2Progress(osContents[ArtifactApplyProgress], hiveContents[ArtifactApplyProgress]) {
			merged[ArtifactApplyProgress] = ArtifactSuperseded
		} else if osErr != nil || hiveErr != nil || osArtifacts[ArtifactApplyProgress] == "" || hiveArtifacts[ArtifactApplyProgress] == "" || isBlockedApplyProgress(osArtifacts[ArtifactApplyProgress]) || isBlockedApplyProgress(hiveArtifacts[ArtifactApplyProgress]) || osArtifacts[ArtifactApplyProgress] == ArtifactSuperseded || hiveArtifacts[ArtifactApplyProgress] == ArtifactSuperseded || !sameV2Progress(osContents[ArtifactApplyProgress], hiveContents[ArtifactApplyProgress]) {
			merged[ArtifactApplyProgress] = ArtifactBlockedBackendDiverged
			delete(mergedContents, ArtifactApplyProgress)
		}
	} else if state, ok := merged[ArtifactApplyProgress]; ok && !strings.HasPrefix(string(state), "blocked:") {
		merged[ArtifactApplyProgress] = applyProgressState(mergedContents[ArtifactApplyProgress], mergedContents[ArtifactTasks])
	}

	return merged, mergedContents, nil
}

// archivedHybridOpenSpec accepts retained Hive history only after OpenSpec proves
// that the active topology was archived. An absent active tree alone can mean a
// wrong workspace, an I/O race, or a deleted change and must remain divergent.
func (h *HybridSource) archivedHybridOpenSpec(changeName string, osArtifacts map[string]ArtifactState, osContents map[string]string, hiveArtifacts map[string]ArtifactState, hiveContents map[string]string) bool {
	if len(osArtifacts) != 0 || len(osContents) != 0 || hiveArtifacts[ArtifactApplyProgress] != ArtifactDone {
		return false
	}
	hiveSnapshot, err := applyprogress.DecodeCanonicalSnapshot([]byte(hiveContents[ArtifactApplyProgress]))
	if err != nil || hiveSnapshot.Status != applyprogress.StatusComplete {
		return false
	}
	archiveRoot := filepath.Join(h.openspec.root, "openspec", "changes", "archive")
	entries, err := readOpenSpecDir(archiveRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || (entry.Name() != changeName && !strings.HasSuffix(entry.Name(), "-"+changeName)) {
			continue
		}
		root := filepath.Join(archiveRoot, entry.Name())
		if !validArchivedAuthorityFiles(root) || !validArchivedDeltaSpecs(filepath.Join(root, "specs")) {
			return false
		}
		data, readErr := readOpenSpecRegular(filepath.Join(root, "apply-progress.md"))
		if readErr != nil {
			return false
		}
		snapshot, decodeErr := applyprogress.DecodeCanonicalSnapshot(data)
		if decodeErr != nil || snapshot.Status != applyprogress.StatusComplete || snapshot.Digest != hiveSnapshot.Digest || !validArchivedReceipts(filepath.Join(root, ".apply-progress-receipts"), snapshot) || !validArchivedEvidenceTopology(filepath.Join(root, "apply-evidence")) {
			return false
		}
		tasksData, tasksErr := readOpenSpecRegular(filepath.Join(root, "tasks.md"))
		if tasksErr != nil {
			return false
		}
		// Archived migration history keeps the same deterministic manifest proof
		// as a current head; never broaden strict v2 parsing to accept phase labels.
		tasks, _, ok := authoritativeHiveSnapshotTasks(snapshot, string(tasksData))
		if !ok {
			return false
		}
		batches := make(map[string][]byte, len(snapshot.Batches))
		for _, ref := range snapshot.Batches {
			batch, batchErr := readOpenSpecRegular(filepath.Join(root, "apply-evidence", ref.BatchID+".json"))
			if batchErr != nil {
				return false
			}
			batches[ref.BatchID] = batch
		}
		if applyprogress.ValidateProgress(data, tasks, batches) == nil {
			return true
		}
	}
	return false
}

func validArchivedAuthorityFiles(root string) bool {
	for _, name := range []string{"archive-report.md", "proposal.md", "design.md", "tasks.md", "apply-progress.md", "verify-report.md"} {
		if _, err := readOpenSpecRegular(filepath.Join(root, name)); err != nil {
			return false
		}
	}
	return true
}

type archivedReceipt struct {
	Payload  string                 `json:"payload"`
	Snapshot applyprogress.Snapshot `json:"snapshot"`
}

// validArchivedReceipts proves that every canonical receipt is the archived head
// or a validated append-only ancestor. A matching head receipt cannot hide an
// interrupted successor, a fork, or an orphan receipt in the archived topology.
func validArchivedReceipts(path string, expected applyprogress.Snapshot) bool {
	entries, err := readOpenSpecDir(path)
	if err != nil || len(entries) == 0 {
		return false
	}
	receipts := make(map[string][]archivedReceipt)
	headReceipt := false
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".apply-progress-stage-") {
			return false
		}
		// writeImmutable preserves a torn receipt under this exact non-JSON
		// debris prefix before replacing it with the canonical receipt. It is not
		// authority and may travel with a recovered archived topology.
		if strings.HasPrefix(entry.Name(), ".apply-progress-corrupt-") && filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			return false
		}
		data, err := readOpenSpecRegular(filepath.Join(path, entry.Name()))
		if err != nil {
			return false
		}
		var receipt archivedReceipt
		if json.Unmarshal(data, &receipt) != nil || !applyprogress.ValidDigest(receipt.Payload) || applyprogress.VerifySnapshot(receipt.Snapshot) != nil || receipt.Snapshot.Project != expected.Project || receipt.Snapshot.Change != expected.Change {
			return false
		}
		canonical, err := json.Marshal(receipt)
		if err != nil || !bytes.Equal(data, canonical) {
			return false
		}
		receipts[receipt.Snapshot.Digest] = append(receipts[receipt.Snapshot.Digest], receipt)
		headReceipt = headReceipt || receipt.Snapshot.Digest == expected.Digest
	}
	if len(receipts) == 0 || !headReceipt {
		return false
	}

	seen := map[string]bool{expected.Digest: true}
	candidate := expected
	for candidate.PreviousDigest != "" {
		parents := receipts[candidate.PreviousDigest]
		if len(parents) == 0 {
			// Receipts can begin after older v2 snapshots were already committed.
			break
		}
		batches, ok := archivedReferencedBatches(filepath.Join(filepath.Dir(path), "apply-evidence"), candidate)
		if !ok {
			return false
		}
		var parent applyprogress.Snapshot
		found := false
		for _, receipt := range parents {
			if applyprogress.ValidateSuccessor(receipt.Snapshot, candidate, batches) != nil {
				continue
			}
			if found && !sameArchivedSnapshot(receipt.Snapshot, parent) {
				return false
			}
			parent, found = receipt.Snapshot, true
		}
		if !found || seen[parent.Digest] {
			return false
		}
		seen[parent.Digest] = true
		candidate = parent
	}
	for digest := range receipts {
		if !seen[digest] {
			return false
		}
	}
	return true
}

func sameArchivedSnapshot(left, right applyprogress.Snapshot) bool {
	return left.Schema == right.Schema && left.Project == right.Project && left.Change == right.Change && left.Generation == right.Generation && left.Revision == right.Revision && left.PreviousDigest == right.PreviousDigest && left.TaskManifestSHA256 == right.TaskManifestSHA256 && left.Status == right.Status && left.StreamSHA256 == right.StreamSHA256 && left.NextEntryIndex == right.NextEntryIndex && left.NextEntryID == right.NextEntryID && left.Digest != "" && left.Digest == right.Digest && slices.Equal(left.Batches, right.Batches) && slices.Equal(left.Coverage, right.Coverage)
}

func archivedReferencedBatches(path string, snapshot applyprogress.Snapshot) (map[string]applyprogress.Batch, bool) {
	batches := make(map[string]applyprogress.Batch, len(snapshot.Batches))
	for _, ref := range snapshot.Batches {
		data, err := readOpenSpecRegular(filepath.Join(path, ref.BatchID+".json"))
		if err != nil {
			return nil, false
		}
		batch, err := applyprogress.DecodeCanonicalBatch(data)
		if err != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return nil, false
		}
		batches[batch.BatchID] = batch
	}
	if applyprogress.ValidateEvidenceCoverage(snapshot, batches) != nil {
		return nil, false
	}
	return batches, true
}

func validArchivedEvidenceTopology(path string) bool {
	entries, err := readOpenSpecDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".apply-progress-stage-") {
			return false
		}
		if _, err := readOpenSpecRegular(filepath.Join(path, entry.Name())); err != nil {
			return false
		}
	}
	return true
}

func validArchivedDeltaSpecs(path string) bool {
	_, found, err := readCanonicalDeltaSpecs(path)
	return err == nil && found
}

func isGuardedSchema(schema string) bool {
	return schema == applyprogress.SnapshotSchema || schema == applyprogress.SupersessionSnapshotSchema
}

func isV2Progress(data string) bool {
	trimmed := strings.TrimSpace(data)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func sameV2Progress(left, right string) bool {
	one, err := applyprogress.DecodeCanonicalSnapshot([]byte(left))
	if err != nil {
		return false
	}
	two, err := applyprogress.DecodeCanonicalSnapshot([]byte(right))
	return err == nil && sameArchivedSnapshot(one, two)
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

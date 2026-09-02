package sddstatus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

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
	return artifacts, contents, nil
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
		artifacts[artifact] = ArtifactDone
		if len(data) > 0 {
			contents[artifact] = string(data)
		}
	}

	return artifacts, contents, nil
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

	return merged, mergedContents, nil
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

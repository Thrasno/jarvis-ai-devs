package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// replayableState is a manifest that passes validation, so every planner test
// exercises the planner's own rules rather than state validation.
func replayableState(agents ...state.Agent) *state.State {
	st := state.New()
	st.Persona = "gentleman"
	st.ManagedAssetDigest = "sha256:embedded-assets"
	st.InstalledAgents = agents
	return st
}

// snapshotTree records every file under root with its permission bits, so a
// test can prove the read-only planner wrote nothing.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		tree[filepath.ToSlash(rel)] = fmt.Sprintf("%04o:%s", info.Mode().Perm(), data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return tree
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// An agent-less manifest must block. The planner must never recover by looking
// for agent files on disk, so both installed agents are present on the
// filesystem and neither may be planned.
func TestBuildPlan_AgentlessManifestBlocksAndNeverRedetects(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "installed claude instructions")
	writeFile(t, filepath.Join(root, ".config", "opencode", "AGENTS.md"), "installed opencode instructions")
	before := snapshotTree(t, root)

	tests := []struct {
		name  string
		state *state.State
	}{
		{name: "manifest records no agents", state: replayableState()},
		{name: "manifest is absent", state: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := BuildPlan(PlanInput{Root: root, State: tt.state, Templates: jarvis.TemplatesFS})

			if !errors.Is(err, ErrNoConfiguredAgents) {
				t.Fatalf("BuildPlan error = %v, want ErrNoConfiguredAgents", err)
			}
			if !strings.Contains(err.Error(), "jarvis") {
				t.Fatalf("block message %q does not name the recovery command %q", err, "jarvis")
			}
			if len(plan.Artifacts) != 0 {
				t.Fatalf("blocked plan carries %d artifacts, want 0", len(plan.Artifacts))
			}
			if after := snapshotTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatalf("planner mutated the filesystem:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

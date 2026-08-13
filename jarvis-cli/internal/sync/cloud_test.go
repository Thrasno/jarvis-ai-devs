package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

func TestCloudManualAction_NamesLoginOnlyForAnUnusableCloudScope(t *testing.T) {
	for _, tc := range []struct {
		name        string
		scope       state.Scope
		syncJSON    string
		wantsAction bool
	}{
		{name: "local-only scope has no cloud portion", scope: state.ScopeLocalOnly},
		{name: "local-only scope ignores unreadable credentials", scope: state.ScopeLocalOnly, syncJSON: "{"},
		{name: "cloud scope with missing credentials", scope: state.ScopeLocalCloud, wantsAction: true},
		{name: "cloud scope with unparseable credentials", scope: state.ScopeLocalCloud, syncJSON: "not json", wantsAction: true},
		{name: "cloud scope with usable credentials", scope: state.ScopeLocalCloud, syncJSON: `{"api_url":"http://x","email":"a@b.c"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".jarvis", "sync.json")
			if tc.syncJSON != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create ~/.jarvis: %v", err)
				}
				if err := os.WriteFile(path, []byte(tc.syncJSON), 0o600); err != nil {
					t.Fatalf("seed sync.json: %v", err)
				}
			}

			action := CloudManualAction(home, tc.scope)
			if tc.wantsAction && !strings.Contains(action, "jarvis login") {
				t.Fatalf("expected the cloud portion to name `jarvis login`, got %q", action)
			}
			if !tc.wantsAction && action != "" {
				t.Fatalf("expected nothing to report, got %q", action)
			}
			assertSyncJSONUntouched(t, path, tc.syncJSON)
		})
	}
}

// assertSyncJSONUntouched holds the read-only rule: sync never writes
// sync.json, and never creates one that was not there.
func assertSyncJSONUntouched(t *testing.T, path, seeded string) {
	t.Helper()
	after, err := os.ReadFile(path)
	if seeded == "" {
		if !os.IsNotExist(err) {
			t.Fatalf("sync must not create sync.json, got err=%v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("read sync.json back: %v", err)
	}
	if string(after) != seeded {
		t.Fatalf("sync must not write sync.json; got %q, want %q", after, seeded)
	}
}

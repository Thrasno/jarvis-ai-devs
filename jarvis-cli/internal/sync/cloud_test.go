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
		{name: "cloud scope with usable credentials", scope: state.ScopeLocalCloud, syncJSON: credentialsWrittenByLogin},
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

// credentialsWrittenByLogin is the shape `jarvis login` actually leaves behind:
// config.WriteSyncCredentials always writes api_url, email and password, and
// hive-daemon refuses the file when any of the three is empty. Anything short of
// that is a half-finished login, not a usable cloud portion.
const credentialsWrittenByLogin = `{"api_url":"https://hivemem.dev","email":"a@b.c","password":"s3cr3t"}`

func TestCloudManualAction_RequestsLoginForNullCredentials(t *testing.T) {
	// `null` is the sharpest syntactic-only pass: it decodes without error into
	// a nil map, so a parse check alone calls an empty file usable.
	assertCloudScopeNeedsLogin(t, "null")
}

func TestCloudManualAction_RequestsLoginForEmptyObject(t *testing.T) {
	assertCloudScopeNeedsLogin(t, "{}")
}

func TestCloudManualAction_RequestsLoginForUnrelatedJSON(t *testing.T) {
	assertCloudScopeNeedsLogin(t, `{"foo":1}`)
}

func TestCloudManualAction_RequestsLoginForPartialCredentials(t *testing.T) {
	// A truncated login leaves some fields behind, which is exactly the case a
	// parse check cannot tell apart from a complete one.
	for _, tc := range []struct {
		name     string
		syncJSON string
	}{
		{name: "password missing", syncJSON: `{"api_url":"https://hivemem.dev","email":"a@b.c"}`},
		{name: "email missing", syncJSON: `{"api_url":"https://hivemem.dev","password":"s3cr3t"}`},
		{name: "api_url missing", syncJSON: `{"email":"a@b.c","password":"s3cr3t"}`},
		{name: "email present but blank", syncJSON: `{"api_url":"https://hivemem.dev","email":"  ","password":"s3cr3t"}`},
		{name: "password present but empty", syncJSON: `{"api_url":"https://hivemem.dev","email":"a@b.c","password":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertCloudScopeNeedsLogin(t, tc.syncJSON)
		})
	}
}

func TestCloudManualAction_AcceptsTheCredentialShapeWrittenByLogin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		syncJSON string
	}{
		{name: "the three required fields", syncJSON: credentialsWrittenByLogin},
		{name: "with auto_sync preserved", syncJSON: `{"api_url":"https://hivemem.dev","email":"a@b.c","password":"s3cr3t","auto_sync":true}`},
		// A newer or daemon-written file may carry fields this build does not
		// know; unknown keys must never be read as a broken login.
		{name: "with fields this build does not know", syncJSON: `{"api_url":"https://hivemem.dev","email":"a@b.c","password":"s3cr3t","daemon_id":"d-1"}`},
		// The writer keeps password whitespace verbatim, so a whitespace-only
		// password is a real password and not an absent one.
		{name: "with a whitespace-only password", syncJSON: `{"api_url":"https://hivemem.dev","email":"a@b.c","password":"   "}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := seedSyncJSON(t, tc.syncJSON)
			if action := CloudManualAction(home, state.ScopeLocalCloud); action != "" {
				t.Fatalf("expected credentials written by `jarvis login` to need no manual step, got %q", action)
			}
			assertSyncJSONUntouched(t, filepath.Join(home, ".jarvis", "sync.json"), tc.syncJSON)
		})
	}
}

// assertCloudScopeNeedsLogin seeds sync.json and demands the manual step, since
// every caller of it is asserting the same single outcome.
func assertCloudScopeNeedsLogin(t *testing.T, syncJSON string) {
	t.Helper()
	home := seedSyncJSON(t, syncJSON)
	action := CloudManualAction(home, state.ScopeLocalCloud)
	if !strings.Contains(action, "jarvis login") {
		t.Fatalf("expected %s to be reported as an unusable cloud portion, got %q", syncJSON, action)
	}
	assertSyncJSONUntouched(t, filepath.Join(home, ".jarvis", "sync.json"), syncJSON)
}

// seedSyncJSON writes the given contents to a throwaway home and returns it.
func seedSyncJSON(t *testing.T, contents string) string {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, ".jarvis", "sync.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create ~/.jarvis: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed sync.json: %v", err)
	}
	return home
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

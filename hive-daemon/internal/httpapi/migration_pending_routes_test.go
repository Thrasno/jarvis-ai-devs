package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

type mockHealthSummaryService struct{}

func (mockHealthSummaryService) Summary(context.Context) (httpapi.HealthSummaryResponse, error) {
	return httpapi.HealthSummaryResponse{}, nil
}

// fullyWiredGatedServer builds a server with every route family the daemon
// registers, backed by a real store so no admitted route can pass the matrix by
// crashing or 404ing instead of being served.
func fullyWiredGatedServer(t *testing.T, status project.MigrationStatus) *httpapi.Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// A real project so the timeline route resolves to a served response instead
	// of the handler's own 404, which would hide whether the gate let it through.
	if err := store.CreateSession("gated-session", "alpha", "", "dev", "test"); err != nil {
		t.Fatal(err)
	}
	backups := governance.NewSQLiteBackupStore(dbPath, "", store.RawDB())
	srv := httpapi.NewServerWithAll("127.0.0.1:0", store, store,
		governance.NewServiceWithBackup(store, backups), nil, mockHealthSummaryService{}, store)
	gate := project.NewMigrationGate(status)
	srv.SetMigrationGate(gate)
	// The wizard routes are wired with the real runner so an admitted route has to
	// reach a served handler rather than the "unavailable" answer an unwired
	// service would give, which would let the matrix pass without proving anything.
	srv.SetMigrationExecution(governance.NewProjectMigrationRunner(store, backups, gate))
	return srv
}

// migrationGateBody decodes the gate's own 503 body, or reports absent when the
// response came from a real handler instead of the gate.
func migrationGateBody(t *testing.T, rr *httptest.ResponseRecorder) (project.MigrationStatus, bool) {
	t.Helper()
	if rr.Code != http.StatusServiceUnavailable {
		return project.MigrationStatus{}, false
	}
	var status project.MigrationStatus
	if err := json.NewDecoder(bytes.NewReader(rr.Body.Bytes())).Decode(&status); err != nil {
		return project.MigrationStatus{}, false
	}
	// Handlers have their own 503 shapes ("restore-unavailable", …) that also
	// carry a "state" field, so only the gate's own two states count as gated.
	gated := status.State == project.MigrationStateBlocked || status.State == project.MigrationStatePendingOperatorReview
	return status, gated
}

// TestPendingMigrationGateAdmitsExactlyTheTUISnapshotReadsAndNoWrites is the
// route matrix for the pending state. The normalization wizard lives inside the
// Hive TUI, and the TUI cannot render until LoadSnapshot completes, so every GET
// that snapshot performs has to survive the closed gate. Nothing that writes
// may, and no route that merely looks read-only-ish gets in by accident.
func TestPendingMigrationGateAdmitsExactlyTheTUISnapshotReadsAndNoWrites(t *testing.T) {
	srv := fullyWiredGatedServer(t, project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		PlanFingerprint: "fingerprint-1",
	})
	srv.SetMigrationRetry(func() {})
	srv.SetMigrationRestore(func(context.Context, governance.RestoreRequest) error { return nil })
	srv.SetMigrationIdentityResolver(func(context.Context, project.IdentityResolutionRequest) error { return nil })

	// Every route jarvis-cli/internal/hiveui.LoadSnapshot calls, plus the
	// recovery routes the wizard itself drives.
	for _, tt := range []struct{ name, method, path string }{
		{"snapshot health probe", http.MethodGet, "/governance/health"},
		{"snapshot sync summary", http.MethodGet, "/governance/health/summary"},
		{"snapshot projects", http.MethodGet, "/governance/projects"},
		{"snapshot memories", http.MethodGet, "/governance/memories"},
		{"snapshot deleted memories", http.MethodGet, "/governance/memories?deleted_only=true"},
		{"snapshot capabilities", http.MethodGet, "/governance/capabilities"},
		{"snapshot warnings", http.MethodGet, "/governance/warnings"},
		{"snapshot backups", http.MethodGet, "/governance/backups"},
		{"snapshot project timeline", http.MethodGet, "/governance/projects/alpha/timeline"},
		{"identity status", http.MethodGet, "/governance/project-identity/status"},
		// The wizard's own three routes: it cannot review a plan it cannot read,
		// approve a fold it cannot post, or report a result it cannot poll.
		{"identity plan", http.MethodGet, "/governance/project-identity/plan"},
		{"identity progress", http.MethodGet, "/governance/project-identity/progress"},
		{"identity execute", http.MethodPost, "/governance/project-identity/execute"},
		{"identity resolve", http.MethodPost, "/governance/project-identity/resolve"},
		{"restore request", http.MethodPost, "/governance/restores"},
		{"identity retry", http.MethodPost, "/governance/project-identity/retry"},
	} {
		t.Run("admits "+tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString("{}")))
			if status, gated := migrationGateBody(t, rr); gated {
				t.Fatalf("%s %s answered the gate body %#v, want the real handler", tt.method, tt.path, status)
			}
			if rr.Code == http.StatusNotFound {
				t.Fatalf("%s %s = 404; the fixture does not register this route so the matrix proves nothing", tt.method, tt.path)
			}
		})
	}

	for _, tt := range []struct{ name, method, path string }{
		{"prompt write", http.MethodPost, "/prompts"},
		{"session create", http.MethodPost, "/sessions"},
		{"session end", http.MethodPost, "/sessions/session-1/end"},
		{"passive observation", http.MethodPost, "/observations/passive"},
		{"last save read", http.MethodGet, "/projects/alpha/last-save"},
		{"project merge", http.MethodPost, "/governance/projects/merge"},
		{"project archive", http.MethodPost, "/governance/projects/alpha/archive"},
		{"project delete", http.MethodDelete, "/governance/projects/alpha"},
		{"single memory read", http.MethodGet, "/governance/memories/1"},
		{"memory delete", http.MethodDelete, "/governance/memories"},
		{"guard execute", http.MethodPost, "/governance/guards/execute"},
		{"engram import preview", http.MethodPost, "/governance/imports/engram/preview"},
		{"engram import execute", http.MethodPost, "/governance/imports/engram/execute"},
		{"backup create", http.MethodPost, "/governance/backups"},
		{"health write", http.MethodPost, "/governance/health"},
		{"warnings write", http.MethodPost, "/governance/warnings"},
		{"projects write", http.MethodPost, "/governance/projects"},
		{"timeline write", http.MethodPost, "/governance/projects/alpha/timeline"},
		{"nested timeline lookalike", http.MethodGet, "/governance/projects/alpha/beta/timeline"},
		{"empty project timeline", http.MethodGet, "/governance/projects//timeline"},
		{"config status", http.MethodGet, "/governance/config/status"},
		{"mutation receipt", http.MethodGet, "/governance/mutations/req-1"},
	} {
		t.Run("blocks "+tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString("{}")))
			status, gated := migrationGateBody(t, rr)
			if !gated {
				t.Fatalf("%s %s = %d %s, want the 503 gate body", tt.method, tt.path, rr.Code, rr.Body.String())
			}
			if status.State != project.MigrationStatePendingOperatorReview {
				t.Fatalf("gate body = %#v, want the pending status body", status)
			}
			if status.Continuation != project.MigrationPendingOperatorContinuation {
				t.Fatalf("continuation = %q, want %q", status.Continuation, project.MigrationPendingOperatorContinuation)
			}
			if status.PlanFingerprint != "fingerprint-1" {
				t.Fatalf("plan fingerprint = %q, want it on every gated response", status.PlanFingerprint)
			}
		})
	}
}

// TestFailedMigrationGateStillAdmitsOnlyTheRecoveryRoutes keeps the two closed
// states apart. The pending state proved nothing was mutated, so reading the
// database is safe; a migration that failed mid-flight proved no such thing, and
// its narrow recovery-only surface must not be widened by this change.
func TestFailedMigrationGateStillAdmitsOnlyTheRecoveryRoutes(t *testing.T) {
	srv := fullyWiredGatedServer(t, project.MigrationStatus{State: project.MigrationStateBlocked, Reason: "executor aborted"})
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/governance/health"}, {http.MethodGet, "/governance/health/summary"},
		{http.MethodGet, "/governance/projects"}, {http.MethodGet, "/governance/memories"},
		{http.MethodGet, "/governance/capabilities"}, {http.MethodGet, "/governance/warnings"},
		{http.MethodGet, "/governance/backups"}, {http.MethodGet, "/governance/projects/alpha/timeline"},
		// The wizard routes belong to the pending state only. A migration that
		// failed mid-flight proves nothing about what is on disk, so reviewing and
		// approving a fresh fold there is exactly the wrong offer.
		{http.MethodGet, "/governance/project-identity/plan"},
		{http.MethodGet, "/governance/project-identity/progress"},
		{http.MethodPost, "/governance/project-identity/execute"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString("{}")))
			if status, gated := migrationGateBody(t, rr); !gated || status.State != project.MigrationStateBlocked {
				t.Fatalf("%s %s = %d %s, want the failure gate body", tt.method, tt.path, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestPendingMigrationGateStillDrivesTheRecoveryHandlers proves the pending
// state reaches the same recovery handlers the failure state does: retry has to
// restart the daemon and resolve has to record a decision, or the wizard would
// get "not needed" back from every action it offers.
func TestPendingMigrationGateStillDrivesTheRecoveryHandlers(t *testing.T) {
	srv := httpapi.NewServer("127.0.0.1:0", &mockPromptStore{})
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStatePendingOperatorReview}))
	restarts := 0
	srv.SetMigrationRetry(func() { restarts++ })
	resolved := 0
	srv.SetMigrationIdentityResolver(func(context.Context, project.IdentityResolutionRequest) error { resolved++; return nil })

	body, err := json.Marshal(project.IdentityResolutionRequest{
		SourceProject: "Foo.Bar",
		TargetProject: "foo-bar",
		Confirmation:  project.IdentityResolutionConfirmation("Foo.Bar", "foo-bar"),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolve := httptest.NewRecorder()
	srv.ServeHTTP(resolve, httptest.NewRequest(http.MethodPost, "/governance/project-identity/resolve", bytes.NewReader(body)))
	if resolve.Code != http.StatusOK || resolved != 1 {
		t.Fatalf("resolve = %d resolved=%d, want 200 and one recorded decision: %s", resolve.Code, resolved, resolve.Body.String())
	}

	retry := httptest.NewRecorder()
	srv.ServeHTTP(retry, httptest.NewRequest(http.MethodPost, "/governance/project-identity/retry", nil))
	if retry.Code != http.StatusAccepted || restarts != 1 {
		t.Fatalf("retry = %d restarts=%d, want 202 and one restart: %s", retry.Code, restarts, retry.Body.String())
	}
}

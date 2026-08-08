package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

func TestMigrationGateBlocksHTTPMemoryAndSessionServicesButLeavesGovernanceReachable(t *testing.T) {
	prompts := &mockPromptStore{}
	sessionCalled := false
	sessions := &mockSessionStore{
		createSessionFn:          func(string, string, string, string, string) error { sessionCalled = true; return nil },
		endSessionFn:             func(string, string) error { sessionCalled = true; return nil },
		savePassiveObservationFn: func(_ context.Context, _, _, _, _ string) error { sessionCalled = true; return nil },
	}
	srv := httpapi.NewServerWithSessions("127.0.0.1:0", prompts, sessions)
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked, Reason: "duplicate canonical project", BackupID: "backup-42", Continuation: "hive project identity status"}))
	for _, tt := range []struct{ name, method, path, body string }{
		{"prompt write", http.MethodPost, "/prompts", `{"project":"alpha","content":"prompt"}`},
		{"session write", http.MethodPost, "/sessions", `{"id":"session-1","project":"alpha"}`},
		{"session mutation", http.MethodPost, "/sessions/session-1/end", ""},
		{"passive observation", http.MethodPost, "/observations/passive", `{"project":"alpha","content":"observation"}`},
		{"memory read", http.MethodGet, "/projects/alpha/last-save", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body)))
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body.String())
			}
			assertMigrationBlockedHTTP(t, rr)
		})
	}
	if prompts.called || sessionCalled {
		t.Fatalf("service was called while migration was blocked: prompts=%v sessions=%v", prompts.called, sessionCalled)
	}
	governanceServer := httpapi.NewServerWithGovernance("127.0.0.1:0", prompts, governance.NewService(nil))
	governanceServer.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked}))
	rr := httptest.NewRecorder()
	governanceServer.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/governance/capabilities", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("governance status = %d, want 503: %s", rr.Code, rr.Body.String())
	}
}

func TestMigrationGateAllowsHTTPMemoryServiceWhenReady(t *testing.T) {
	prompts := &mockPromptStore{}
	srv := httpapi.NewServer("127.0.0.1:0", prompts)
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady}))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(`{"project":"alpha","content":"prompt"}`)))
	if rr.Code != http.StatusCreated || !prompts.called {
		t.Fatalf("ready prompt request status=%d called=%v", rr.Code, prompts.called)
	}
}

func TestMigrationGateExposesIdentityStatusThroughGovernance(t *testing.T) {
	srv := httpapi.NewServer("127.0.0.1:0", &mockPromptStore{})
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked, Reason: "duplicate canonical project", BackupID: "migration-backup-1", Continuation: "hive project identity status"}))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/governance/project-identity/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var status project.MigrationStatus
	if err := json.NewDecoder(rr.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.State != project.MigrationStateBlocked || status.BackupID != "migration-backup-1" || status.Continuation == "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestMigrationGateRetryRequestsLifecycleRestartOnlyWhenBlocked(t *testing.T) {
	for _, tt := range []struct {
		name, state           string
		wantStatus, wantCalls int
	}{
		{"blocked requests one clean restart", project.MigrationStateBlocked, http.StatusAccepted, 1},
		{"ready does not restart a live daemon", project.MigrationStateReady, http.StatusConflict, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httpapi.NewServer("127.0.0.1:0", &mockPromptStore{})
			srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: tt.state}))
			srv.SetMigrationRetry(func() { calls++ })
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/governance/project-identity/retry", nil))
			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if calls != tt.wantCalls {
				t.Fatalf("restart calls = %d, want %d", calls, tt.wantCalls)
			}
			if tt.wantCalls == 1 && !strings.Contains(rr.Body.String(), `"state":"restart-requested"`) {
				t.Fatalf("retry response = %s, want deterministic restart status", rr.Body.String())
			}
		})
	}
}

func TestMigrationGateRetryIsSingleShotAndPreservesBlockedStatus(t *testing.T) {
	calls := 0
	srv := httpapi.NewServer("127.0.0.1:0", &mockPromptStore{})
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked, Reason: "duplicate canonical project", BackupID: "backup-42"}))
	srv.SetMigrationRetry(func() { calls++ })
	for _, wantStatus := range []int{http.StatusAccepted, http.StatusConflict} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/governance/project-identity/retry", nil))
		if rr.Code != wantStatus {
			t.Fatalf("retry status = %d, want %d: %s", rr.Code, wantStatus, rr.Body.String())
		}
	}
	if calls != 1 {
		t.Fatalf("restart calls = %d, want one clean daemon shutdown", calls)
	}
	status := httptest.NewRecorder()
	srv.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/governance/project-identity/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"state":"migration-blocked"`) || !strings.Contains(status.Body.String(), `"backup_id":"backup-42"`) {
		t.Fatalf("blocked status after retry = %d %s, want persisted blocked recovery state", status.Code, status.Body.String())
	}
}

func TestMigrationGateAllowsOnlyIdentityRecoveryRoutes(t *testing.T) {
	srv := httpapi.NewServer("127.0.0.1:0", nil)
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked, Reason: "identity conflict"}))
	for _, tt := range []struct {
		method, path string
		wantStatus   int
	}{{http.MethodGet, "/governance/project-identity/status", http.StatusOK}, {http.MethodPost, "/governance/project-identity/retry", http.StatusServiceUnavailable}, {http.MethodPost, "/governance/restores", http.StatusNotFound}} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, nil))
			if rr.Code != tt.wantStatus {
				t.Fatalf("recovery route status = %d, want %d: %s", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
	for _, tt := range []struct{ method, path string }{{http.MethodPost, "/governance/guards/execute"}, {http.MethodPost, "/governance/projects/alpha/archive"}, {http.MethodPost, "/governance/projects/merge"}, {http.MethodDelete, "/governance/projects/alpha"}, {http.MethodPost, "/governance/imports/engram/execute"}, {http.MethodGet, "/governance/backups"}} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, nil))
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want migration blocked", rr.Code)
			}
		})
	}
}

func TestMigrationGateRestoreReservesPendingMarkerBeforeScheduling(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("live database"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := governance.NewBackupStore(dbPath, "")
	first, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, backups))
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked}))
	entered := make(chan struct{})
	release := make(chan struct{})
	var scheduleMu sync.Mutex
	scheduled := make([]governance.RestoreRequest, 0, 1)
	srv.SetMigrationRestore(func(_ context.Context, req governance.RestoreRequest) error {
		scheduleMu.Lock()
		scheduled = append(scheduled, req)
		scheduleMu.Unlock()
		close(entered)
		<-release
		data, err := json.Marshal(req)
		if err != nil {
			return err
		}
		return os.WriteFile(governance.PendingRestorePath(dbPath), data, 0o600)
	})
	retryCalls := 0
	srv.SetMigrationRetry(func() { retryCalls++ })

	serve := func(backup governance.BackupManifest) *httptest.ResponseRecorder {
		body, err := json.Marshal(governance.RestoreRequest{BackupID: backup.ID, Confirmation: governance.RestoreConfirmation(backup.ID)})
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/governance/restores", bytes.NewReader(body)))
		return rr
	}

	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstResult <- serve(first) }()
	<-entered
	secondResult := make(chan *httptest.ResponseRecorder, 1)
	go func() { secondResult <- serve(second) }()

	scheduleMu.Lock()
	if len(scheduled) != 1 || scheduled[0].BackupID != first.ID {
		t.Fatalf("scheduled restores before release = %#v, want only %q", scheduled, first.ID)
	}
	scheduleMu.Unlock()
	close(release)

	if rr := <-firstResult; rr.Code != http.StatusAccepted {
		t.Fatalf("first restore status = %d, want 202: %s", rr.Code, rr.Body.String())
	}
	if rr := <-secondResult; rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), `"state":"restart-pending"`) {
		t.Fatalf("second restore = %d %s, want 409 restart-pending", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(governance.PendingRestorePath(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	var marker governance.RestoreRequest
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.BackupID != first.ID {
		t.Fatalf("pending marker backup = %q, want accepted backup %q", marker.BackupID, first.ID)
	}
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	if len(scheduled) != 1 || retryCalls != 1 {
		t.Fatalf("scheduled=%d retry calls=%d, want 1 and 1", len(scheduled), retryCalls)
	}
}

func TestMigrationGateRestoreReleasesReservationAfterSchedulingFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("live database"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := governance.NewBackupStore(dbPath, "")
	first, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, backups))
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked}))
	calls := 0
	srv.SetMigrationRestore(func(_ context.Context, req governance.RestoreRequest) error {
		calls++
		if calls == 1 {
			return os.ErrPermission
		}
		data, err := json.Marshal(req)
		if err != nil {
			return err
		}
		return os.WriteFile(governance.PendingRestorePath(dbPath), data, 0o600)
	})
	retryCalls := 0
	srv.SetMigrationRetry(func() { retryCalls++ })

	serve := func(backup governance.BackupManifest) *httptest.ResponseRecorder {
		body, err := json.Marshal(governance.RestoreRequest{BackupID: backup.ID, Confirmation: governance.RestoreConfirmation(backup.ID)})
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/governance/restores", bytes.NewReader(body)))
		return rr
	}

	if rr := serve(first); rr.Code != http.StatusInternalServerError {
		t.Fatalf("failed schedule status = %d, want 500: %s", rr.Code, rr.Body.String())
	}
	if rr := serve(second); rr.Code != http.StatusAccepted {
		t.Fatalf("recovered schedule status = %d, want 202: %s", rr.Code, rr.Body.String())
	}
	data, err := os.ReadFile(governance.PendingRestorePath(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	var marker governance.RestoreRequest
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.BackupID != second.ID || calls != 2 || retryCalls != 1 {
		t.Fatalf("marker=%q calls=%d retries=%d, want %q, 2, 1", marker.BackupID, calls, retryCalls, second.ID)
	}
}

func assertMigrationBlockedHTTP(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	var body project.MigrationStatus
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode blocked response: %v", err)
	}
	if body.State != project.MigrationStateBlocked || body.Reason != "duplicate canonical project" || body.BackupID != "backup-42" || body.Continuation != "hive project identity status" {
		t.Fatalf("blocked response = %#v", body)
	}
}

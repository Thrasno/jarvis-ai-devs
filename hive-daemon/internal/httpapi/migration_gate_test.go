package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

func TestMigrationGateBlocksHTTPMemoryAndSessionServicesButLeavesGovernanceReachable(t *testing.T) {
	prompts := &mockPromptStore{}
	sessionCalled := false
	sessions := &mockSessionStore{
		createSessionFn: func(string, string, string, string, string) error {
			sessionCalled = true
			return nil
		},
		endSessionFn: func(string, string) error {
			sessionCalled = true
			return nil
		},
		savePassiveObservationFn: func(_ context.Context, _, _, _, _ string) error {
			sessionCalled = true
			return nil
		},
	}
	srv := httpapi.NewServerWithSessions("127.0.0.1:0", prompts, sessions)
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{
		State:        project.MigrationStateBlocked,
		Reason:       "duplicate canonical project",
		BackupID:     "backup-42",
		Continuation: "hive project identity resolve then retry",
	}))

	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"prompt write", http.MethodPost, "/prompts", `{"project":"alpha","content":"prompt"}`},
		{"session write", http.MethodPost, "/sessions", `{"id":"session-1","project":"alpha"}`},
		{"session mutation", http.MethodPost, "/sessions/session-1/end", ""},
		{"passive observation", http.MethodPost, "/observations/passive", `{"project":"alpha","content":"observation"}`},
		{"memory read", http.MethodGet, "/projects/alpha/last-save", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)
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
	if rr.Code != http.StatusOK {
		t.Fatalf("governance status = %d, want 200: %s", rr.Code, rr.Body.String())
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

func assertMigrationBlockedHTTP(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	var body project.MigrationStatus
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode blocked response: %v", err)
	}
	if body.State != project.MigrationStateBlocked || body.Reason != "duplicate canonical project" || body.BackupID != "backup-42" || body.Continuation != "hive project identity resolve then retry" {
		t.Fatalf("blocked response = %#v", body)
	}
}

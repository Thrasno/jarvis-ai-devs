package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

type mockPromptStore struct {
	savePromptFn func(ctx context.Context, project, content string) (*models.Prompt, error)
	called       bool
}

type mockProjectStore struct {
	known                   []project.KnownProject
	createRecoveryTokenFn   func(context.Context, project.TokenRequest) (string, error)
	validateRecoveryTokenFn func(context.Context, project.TokenValidation) error
	consumeRecoveryTokenFn  func(context.Context, project.TokenValidation) error
}

func (m mockProjectStore) KnownProjects(context.Context) ([]project.KnownProject, error) {
	return m.known, nil
}

func (m mockProjectStore) SessionProject(context.Context, string) (string, error) {
	return "", project.ErrSessionNotFound
}

func (m mockProjectStore) CreateRecoveryToken(ctx context.Context, req project.TokenRequest) (string, error) {
	if m.createRecoveryTokenFn != nil {
		return m.createRecoveryTokenFn(ctx, req)
	}
	return "recovery-token", nil
}

func (m mockProjectStore) ConsumeRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	if err := m.ValidateRecoveryToken(ctx, validation); err != nil {
		return err
	}
	if m.consumeRecoveryTokenFn != nil {
		return m.consumeRecoveryTokenFn(ctx, validation)
	}
	return nil
}

func (m mockProjectStore) ValidateRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	if m.validateRecoveryTokenFn != nil {
		return m.validateRecoveryTokenFn(ctx, validation)
	}
	return nil
}

func (m mockProjectStore) ResolveAlias(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (m *mockPromptStore) SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error) {
	m.called = true
	if m.savePromptFn != nil {
		return m.savePromptFn(ctx, project, content)
	}
	return &models.Prompt{ID: 42, Project: project, Content: content, CreatedAt: time.Now()}, nil
}

func newTestServer(store *mockPromptStore) *httpapi.Server {
	return httpapi.NewServer("127.0.0.1:0", store)
}

// TS-HTTP-1: POST /prompts with valid content returns 201
func TestPostPrompts_ValidContent_Returns201(t *testing.T) {
	store := &mockPromptStore{}
	srv := httpapi.NewServer("127.0.0.1:0", store)

	body := `{"content": "fix the auth bug", "project": "jarvis-dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if _, ok := resp["id"]; !ok {
		t.Error("response missing 'id'")
	}
	if _, ok := resp["created_at"]; !ok {
		t.Error("response missing 'created_at'")
	}
}

// TS-HTTP-2: Empty content returns 400, SavePrompt NOT called
func TestPostPrompts_EmptyContent_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": ""}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT have been called for empty content")
	}
}

// TS-HTTP-3: Whitespace content returns 400
func TestPostPrompts_WhitespaceContent_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": "   ", "project": "jarvis-dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT have been called for whitespace content")
	}
}

// TS-HTTP-4: DB error returns 500, error message is "internal error" (NOT the DB error)
func TestPostPrompts_DBError_Returns500WithGenericMessage(t *testing.T) {
	store := &mockPromptStore{
		savePromptFn: func(ctx context.Context, project, content string) (*models.Prompt, error) {
			return nil, errors.New("some internal db error")
		},
	}
	srv := newTestServer(store)

	body := `{"content": "valid content", "project": "jarvis-dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	errMsg, ok := resp["error"].(string)
	if !ok {
		t.Fatal("response missing 'error' field")
	}
	if errMsg != "internal error" {
		t.Errorf("expected 'internal error', got %q (DB error must not leak)", errMsg)
	}
}

// TS-HTTP-5: Malformed JSON returns 400
func TestPostPrompts_MalformedJSON_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": `
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TS-HTTP-6: Non-POST method returns 405
func TestPostPrompts_NonPOSTMethod_Returns405(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/prompts", nil)
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405, got %d for method %s", rr.Code, method)
			}
		})
	}
}

// TS-HTTP-7: Server starts and accepts real HTTP connections
func TestServer_StartsAndAcceptsConnections(t *testing.T) {
	store := &mockPromptStore{}

	// Find a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := httpapi.NewServer(addr, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Post("http://"+addr+"/prompts", "application/json", bytes.NewBufferString(`{"content":"hello","project":"jarvis-dev"}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServer_StartRejectsNonLoopbackAddress(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"0.0.0.0:0", "[::]:0"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			store := &mockPromptStore{}
			srv := httpapi.NewServer(addr, store)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			err := srv.Start(ctx)
			if err == nil {
				t.Fatal("expected non-loopback address to be rejected")
			}
			if !strings.Contains(err.Error(), "loopback") {
				t.Fatalf("expected loopback boundary error, got %v", err)
			}
		})
	}
}

func TestServer_StartAcceptsLoopbackAddresses(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"localhost:0", "[::1]:0"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			if addr == "[::1]:0" {
				listener, err := net.Listen("tcp", addr)
				if err != nil {
					t.Skipf("IPv6 loopback is not available: %v", err)
				}
				requireNoErrorClose(t, listener)
			}

			store := &mockPromptStore{}
			srv := httpapi.NewServer(addr, store)

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			time.AfterFunc(25*time.Millisecond, cancel)

			if err := srv.Start(ctx); err != nil {
				t.Fatalf("expected loopback address %s to be accepted, got %v", addr, err)
			}
		})
	}
}

func requireNoErrorClose(t *testing.T, listener net.Listener) {
	t.Helper()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
}

// ─── T-HTTP-1: project field ───────────────────────────────────────────────

func TestPostPrompts_WithProject_PassesProjectToStore(t *testing.T) {
	var gotProject string
	store := &mockPromptStore{
		savePromptFn: func(_ context.Context, project, _ string) (*models.Prompt, error) {
			gotProject = project
			return &models.Prompt{ID: 1, Project: project, CreatedAt: time.Now()}, nil
		},
	}
	srv := newTestServer(store)

	body := `{"content": "explain goroutines", "project": "jarvis-dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	if gotProject != "jarvis-dev" {
		t.Errorf("project passed to store = %q, want 'jarvis-dev'", gotProject)
	}
}

func TestPostPrompts_UnknownProjectReturnsErrorCodeAndBlocksSave(t *testing.T) {
	t.Parallel()

	store := &mockPromptStore{}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", store, mockProjectStore{
		known: []project.KnownProject{{Name: "jarvis-dev"}},
	})

	body := `{"content": "explain goroutines", "project": "ghost-project"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if got := resp["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, resp)
	}
	if store.called {
		t.Fatal("SavePrompt must not be called after project validation failure")
	}
}

func TestPostPrompts_NormalizedCollisionReturnsAmbiguousErrorAndBlocksSave(t *testing.T) {
	t.Parallel()

	store := &mockPromptStore{}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", store, mockProjectStore{
		known: []project.KnownProject{
			{Name: "jarvis-dev", Directory: "/work/jarvis-dev"},
			{Name: "jarvis.dev", Directory: "/work/jarvis-dot-dev"},
		},
	})

	body := `{"content": "explain goroutines", "project": "jarvis dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ErrorCode  string              `json:"error_code"`
		Candidates []project.Candidate `json:"candidates"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.ErrorCode != string(project.CodeProjectAmbiguous) {
		t.Fatalf("error_code = %q, want %q", resp.ErrorCode, project.CodeProjectAmbiguous)
	}
	if len(resp.Candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2; candidates=%v", len(resp.Candidates), resp.Candidates)
	}
	if store.called {
		t.Fatal("SavePrompt must not be called after project validation failure")
	}
}

func TestPostPrompts_AmbiguousProjectReturnsRecoveryTokenAndBlocksSave(t *testing.T) {
	t.Parallel()

	store := &mockPromptStore{}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", store, mockProjectStore{
		known: []project.KnownProject{
			{Name: "jarvis-dev", Directory: "/work/jarvis-dev"},
			{Name: "jarvis.dev", Directory: "/work/jarvis-dot-dev"},
		},
		createRecoveryTokenFn: func(_ context.Context, req project.TokenRequest) (string, error) {
			if req.RequestedProject != "jarvis dev" {
				t.Fatalf("requested project = %q, want jarvis dev", req.RequestedProject)
			}
			return "retry-token", nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(`{"content":"explain goroutines","project":"jarvis dev"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ErrorCode     string `json:"error_code"`
		RecoveryToken string `json:"recovery_token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.ErrorCode != string(project.CodeProjectAmbiguous) || resp.RecoveryToken != "retry-token" {
		t.Fatalf("response = %+v, want ambiguous with retry-token", resp)
	}
	if store.called {
		t.Fatal("SavePrompt must not be called after ambiguity")
	}
}

func TestPostPrompts_RecoveryTokenRetryConsumesTokenAndSavesChosenProject(t *testing.T) {
	t.Parallel()

	var consumed project.TokenValidation
	var savedProject string
	store := &mockPromptStore{savePromptFn: func(_ context.Context, projectName, _ string) (*models.Prompt, error) {
		savedProject = projectName
		return &models.Prompt{ID: 7, Project: projectName, CreatedAt: time.Now()}, nil
	}}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", store, mockProjectStore{
		known: []project.KnownProject{{Name: "jarvis-dev"}},
		consumeRecoveryTokenFn: func(_ context.Context, validation project.TokenValidation) error {
			consumed = validation
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(`{"content":"chosen","project":"jarvis-dev","recovery_token":"retry-token","project_choice_reason":"jarvis dev"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if consumed.Token != "retry-token" || consumed.SelectedProject != "jarvis-dev" {
		t.Fatalf("consumed = %+v, want retry-token for jarvis-dev", consumed)
	}
	if savedProject != "jarvis-dev" {
		t.Fatalf("saved project = %q, want jarvis-dev", savedProject)
	}
}

func TestPostPrompts_WithoutProject_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": "explain goroutines"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT be called when project is missing")
	}
}

func TestPostPrompts_WhitespaceOnlyProject_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": "some content", "project": "   \t\n  "}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace-only project, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT be called when project is whitespace-only")
	}
}

func TestPostPrompts_EmptyProjectString_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": "some content", "project": ""}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty project string, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT be called when project is empty")
	}
}

// ─── T-06b: /prompts HTTP endpoint private-tag stripping ─────────────────────

// T-06b-1: POST /prompts with private tag in content → stored stripped, response includes stripped/stripped_count
func TestPostPrompts_PrivateTagInContent_StripsAndReturnsCount(t *testing.T) {
	var savedContent string
	store := &mockPromptStore{
		savePromptFn: func(_ context.Context, _, content string) (*models.Prompt, error) {
			savedContent = content
			return &models.Prompt{ID: 9, Content: content, CreatedAt: time.Now()}, nil
		},
	}
	srv := newTestServer(store)

	body := `{"content": "token <private>secret</private> end", "project": "jarvis-dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if savedContent != "token [REDACTED] end" {
		t.Errorf("stored content = %q, want %q", savedContent, "token [REDACTED] end")
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if stripped := resp["stripped"]; stripped != true {
		t.Errorf("stripped = %v, want true", stripped)
	}
	if count := resp["stripped_count"]; count != float64(1) {
		t.Errorf("stripped_count = %v, want 1", count)
	}
}

// T-06b-2: POST /prompts with no private tags → stripped: false, stripped_count: 0 (always present)
func TestPostPrompts_NoPrivateTags_ReturnsStrippedFalse(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": "plain content", "project": "jarvis-dev"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if stripped, ok := resp["stripped"]; !ok || stripped != false {
		t.Errorf("stripped = %v, want false", resp["stripped"])
	}
	if count, ok := resp["stripped_count"]; !ok || count != float64(0) {
		t.Errorf("stripped_count = %v, want 0", resp["stripped_count"])
	}
}

func TestGovernanceGETEndpointsReturnReadOnlyViews(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "Active", Content: "content", SessionID: "sess-alpha"}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := store.RecordSyncFailure("alpha", time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC), 2, time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC), errors.New("sync failed (503): upstream body")); err != nil {
		t.Fatalf("RecordSyncFailure: %v", err)
	}
	before := readHTTPGovernanceCounters(t, store)

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", store, governance.NewService(store))

	for _, tt := range []struct {
		path      string
		wantField string
	}{
		{path: "/governance/projects", wantField: "projects"},
		{path: "/governance/projects/alpha", wantField: "project"},
		{path: "/governance/memories?project=alpha", wantField: "memories"},
		{path: "/governance/health", wantField: "projects"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
			}
			var resp map[string]any
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("response not valid JSON: %v", err)
			}
			if _, ok := resp[tt.wantField]; !ok {
				t.Fatalf("response missing %q: %v", tt.wantField, resp)
			}
			if tt.path == "/governance/health" {
				projects := resp["projects"].([]any)
				health := projects[0].(map[string]any)
				if _, ok := health["consecutive_failures"]; !ok {
					t.Fatalf("health response missing snake_case field: %v", health)
				}
				if _, ok := health["ConsecutiveFailures"]; ok {
					t.Fatalf("health response leaked Go field name: %v", health)
				}
			}
			if tt.path == "/governance/memories?project=alpha" {
				memories := resp["memories"].([]any)
				memory := memories[0].(map[string]any)
				if _, ok := memory["deleted_at"]; ok {
					t.Fatalf("active memory response must omit deleted_at: %v", memory)
				}
			}
		})
	}

	after := readHTTPGovernanceCounters(t, store)
	if before != after {
		t.Fatalf("GET governance endpoints mutated state: before=%+v after=%+v", before, after)
	}
}

func TestGovernanceWarningsEndpointReturnsPersistedWarnings(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	warning, err := store.SaveHiveWarning(db.HiveWarningInput{
		Severity: "warning",
		Source:   "startup",
		Message:  "daemon started with degraded sync config",
	})
	if err != nil {
		t.Fatalf("SaveHiveWarning: %v", err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", store, governance.NewService(store))
	req := httptest.NewRequest(http.MethodGet, "/governance/warnings", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	rawBody := rr.Body.Bytes()
	var resp struct {
		Warnings []governance.Warning `json:"warnings"`
	}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; body: %s", len(resp.Warnings), rr.Body.String())
	}
	got := resp.Warnings[0]
	if got.ID != warning.ID || got.Severity != "warning" || got.Source != "startup" || got.Message != "daemon started with degraded sync config" || got.ResolutionState != "active" {
		t.Fatalf("warning = %+v, want persisted warning row", got)
	}
	var raw map[string][]map[string]any
	if err := json.Unmarshal(rawBody, &raw); err != nil {
		t.Fatalf("response not valid raw JSON: %v", err)
	}
	if _, ok := raw["warnings"][0]["resolution_state"]; !ok {
		t.Fatalf("warning response missing snake_case resolution_state field: %v", raw)
	}
	if _, ok := raw["warnings"][0]["ResolutionState"]; ok {
		t.Fatalf("warning response leaked Go field name: %v", raw)
	}
}

func TestGovernanceWarningsEndpointMapsStoreErrorsSafely(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", store, governance.NewService(store))
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/governance/warnings", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp["error"] != "internal error" {
		t.Fatalf("error = %q, want generic internal error", resp["error"])
	}
}

func TestGovernanceEndpointStatusMapping(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", store, governance.NewService(store))

	for _, tt := range []struct {
		name string
		path string
		want int
	}{
		{name: "project detail rejects blank name", path: "/governance/projects/%20", want: http.StatusBadRequest},
		{name: "project detail returns not found for unknown project", path: "/governance/projects/missing", want: http.StatusNotFound},
		{name: "memories require project query", path: "/governance/memories", want: http.StatusBadRequest},
		{name: "memories reject blank project query", path: "/governance/memories?project=+", want: http.StatusBadRequest},
		{name: "memories return not found for unknown project", path: "/governance/memories?project=missing", want: http.StatusNotFound},
		{name: "memories reject malformed limit", path: "/governance/memories?project=alpha&limit=bogus", want: http.StatusBadRequest},
		{name: "memories reject huge limit", path: "/governance/memories?project=alpha&limit=501", want: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Fatalf("expected %d, got %d — body: %s", tt.want, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestGovernanceMemoriesLimitContract(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "Memory", Content: "content", SessionID: "sess-alpha"}); err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
	}

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", store, governance.NewService(store))

	for _, tt := range []struct {
		name string
		path string
		want int
	}{
		{name: "explicit positive limit is honored", path: "/governance/memories?project=alpha&limit=1", want: 1},
		{name: "zero limit uses default", path: "/governance/memories?project=alpha&limit=0", want: 2},
		{name: "negative limit uses default", path: "/governance/memories?project=alpha&limit=-1", want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
			}
			var resp struct {
				Memories []map[string]any `json:"memories"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("response not valid JSON: %v", err)
			}
			if len(resp.Memories) != tt.want {
				t.Fatalf("memories len = %d, want %d; body: %s", len(resp.Memories), tt.want, rr.Body.String())
			}
		})
	}
}

func TestGovernanceBackupHTTPContracts(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("live http backup"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	backupStore := governance.NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, backupStore))

	createReq := httptest.NewRequest(http.MethodPost, "/governance/backups", nil)
	createRR := httptest.NewRecorder()
	srv.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", createRR.Code, createRR.Body.String())
	}
	var createResp struct {
		Backup governance.BackupManifest `json:"backup"`
	}
	if err := json.NewDecoder(createRR.Body).Decode(&createResp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if createResp.Backup.ID == "" {
		t.Fatal("backup response missing id")
	}
	if rel, err := filepath.Rel(dbDir, createResp.Backup.ArchivePath); err != nil || rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
		t.Fatalf("backup archive must be outside db dir; rel=%q err=%v", rel, err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/governance/backups", nil)
	listRR := httptest.NewRecorder()
	srv.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Backups []governance.BackupManifest `json:"backups"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&listResp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(listResp.Backups) != 1 || listResp.Backups[0].ID != createResp.Backup.ID {
		t.Fatalf("backups = %+v, want one backup %q", listResp.Backups, createResp.Backup.ID)
	}
}

func TestGovernanceBackupHTTPCreatesSQLiteSnapshotIncludingWALData(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateSession("sess-http-backup", "alpha", dbDir, "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "HTTP WAL snapshot", Content: "committed through daemon", SessionID: "sess-http-backup"}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	backupStore := governance.NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), store.RawDB())
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(store, backupStore))
	req := httptest.NewRequest(http.MethodPost, "/governance/backups", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Backup governance.BackupManifest `json:"backup"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	backupDB, err := sql.Open("sqlite", resp.Backup.ArchivePath)
	if err != nil {
		t.Fatalf("Open backup: %v", err)
	}
	t.Cleanup(func() { _ = backupDB.Close() })
	var got string
	if err := backupDB.QueryRow(`SELECT content FROM memories WHERE title = 'HTTP WAL snapshot'`).Scan(&got); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if got != "committed through daemon" {
		t.Fatalf("backup content = %q, want committed through daemon", got)
	}
}

func TestGovernanceBackupsMethodNotAllowedIncludesAllowHeader(t *testing.T) {
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, nil))
	req := httptest.NewRequest(http.MethodPut, "/governance/backups", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if got, want := rr.Header().Get("Allow"), "GET, POST"; got != want {
		t.Fatalf("Allow header = %q, want %q", got, want)
	}
}

func TestGovernanceRestoresMethodNotAllowedIncludesAllowHeader(t *testing.T) {
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/governance/restores", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if got, want := rr.Header().Get("Allow"), http.MethodPost; got != want {
		t.Fatalf("Allow header = %q, want %q", got, want)
	}
}

func TestGovernanceRestoreHTTPRequiresExplicitSelectionAndConfirmation(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("restorable http db"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	backupStore := governance.NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := backupStore.Create(context.Background())
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("corrupted http db"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, backupStore))

	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "missing backup selection", body: fmt.Sprintf(`{"confirmation":%q}`, governance.RestoreConfirmation(backup.ID))},
		{name: "confirmation mismatch", body: fmt.Sprintf(`{"backup_id":%q,"confirmation":"RESTORE wrong-backup"}`, backup.ID)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/governance/restores", bytes.NewBufferString(tt.body))
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
			}
			contents, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(contents) != "corrupted http db" {
				t.Fatalf("restore should not mutate db on rejected request; got %q", contents)
			}
		})
	}

	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q}`, backup.ID, governance.RestoreConfirmation(backup.ID))
	req := httptest.NewRequest(http.MethodPost, "/governance/restores", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Restore governance.RestoreResult `json:"restore"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.Restore.Status != governance.RestoreStatusCoordinationRequired {
		t.Fatalf("restore status = %q, want %q", resp.Restore.Status, governance.RestoreStatusCoordinationRequired)
	}
	if !resp.Restore.RequiresDaemonRestart {
		t.Fatal("restore response must require daemon stop/restart coordination")
	}
	contents, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(contents) != "corrupted http db" {
		t.Fatalf("HTTP restore plan must not mutate live db; got %q", contents)
	}
}

func TestGovernanceRestoreHTTPMapsArchiveIntegrityFailuresToConflict(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("restorable http db"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	backupStore := governance.NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := backupStore.Create(context.Background())
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	if err := os.WriteFile(backup.ArchivePath, []byte("tampered"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(nil, backupStore))

	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q}`, backup.ID, governance.RestoreConfirmation(backup.ID))
	req := httptest.NewRequest(http.MethodPost, "/governance/restores", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestGovernanceGuardExecuteHTTPDeletesMemoryThroughDaemonService(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateSession("sess-guard-delete", "alpha", tempDir, "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	memoryID, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "Guarded delete", Content: "delete me", SessionID: "sess-guard-delete"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	backupStore := governance.NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), store.RawDB())
	backup, err := backupStore.Create(context.Background())
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(store, backupStore))
	body := fmt.Sprintf(`{"operation":"delete","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"cleanup"}`, memoryID, backup.ID, governance.GuardConfirmation(governance.GuardOperationDelete, governance.GuardTargetMemory, memoryID))
	req := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result governance.GuardResult `json:"result"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.Result.Mutated || resp.Result.TargetID != memoryID || resp.Result.BackupID != backup.ID {
		t.Fatalf("guard result = %+v, want mutated memory %d with backup %s", resp.Result, memoryID, backup.ID)
	}
	var deletedAt sql.NullString
	if err := store.RawDB().QueryRow(`SELECT deleted_at FROM memories WHERE id = ?`, memoryID).Scan(&deletedAt); err != nil {
		t.Fatalf("query memory: %v", err)
	}
	if !deletedAt.Valid {
		t.Fatal("memory was not deleted by guarded HTTP execution")
	}
}

func TestGovernanceGuardExecuteHTTPMismatchDoesNotMutate(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateSession("sess-guard-mismatch", "alpha", tempDir, "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	memoryID, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "Guarded mismatch", Content: "keep me", SessionID: "sess-guard-mismatch"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	backupStore := governance.NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), store.RawDB())
	backup, err := backupStore.Create(context.Background())
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	before := readHTTPGovernanceCounters(t, store)
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(store, backupStore))
	body := fmt.Sprintf(`{"operation":"delete","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":" DELETE memory %d ","reason":"cleanup"}`, memoryID, backup.ID, memoryID)
	req := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	after := readHTTPGovernanceCounters(t, store)
	if before != after {
		t.Fatalf("mismatch mutated state: before=%+v after=%+v", before, after)
	}
}

func TestGovernanceGuardExecuteHTTPRejectsMissingDeleteReasonBeforeMutation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		reasonJSON string
	}{
		{name: "missing reason", reasonJSON: ""},
		{name: "blank reason", reasonJSON: `,"reason":"  \t  "`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, backup, srv := newHTTPGuardTestServer(t)
			memoryID := saveHTTPGuardMemory(t, store, "reason-required")
			before := readHTTPGovernanceCounters(t, store)
			body := fmt.Sprintf(`{"operation":"delete","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":%q%s}`, memoryID, backup.ID, governance.GuardConfirmation(governance.GuardOperationDelete, governance.GuardTargetMemory, memoryID), tt.reasonJSON)
			req := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "delete reason is required") {
				t.Fatalf("body = %s, want delete reason validation error", rr.Body.String())
			}
			after := readHTTPGovernanceCounters(t, store)
			if before != after {
				t.Fatalf("missing reason mutated state: before=%+v after=%+v", before, after)
			}
			var deletedAt sql.NullString
			if err := store.RawDB().QueryRow(`SELECT deleted_at FROM memories WHERE id = ?`, memoryID).Scan(&deletedAt); err != nil {
				t.Fatalf("query memory: %v", err)
			}
			if deletedAt.Valid {
				t.Fatal("memory was deleted despite missing reason")
			}
		})
	}
}

func TestGovernanceMemoriesHTTPDefaultListExcludesTombstones(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	activeID := saveHTTPGuardMemoryInProject(t, store, "normal-list", "normal-list-active")
	deletedID := saveHTTPGuardMemoryInProject(t, store, "normal-list", "normal-list-deleted")
	body := fmt.Sprintf(`{"operation":"delete","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":%q,"reason":"duplicate"}`, deletedID, backup.ID, governance.GuardConfirmation(governance.GuardOperationDelete, governance.GuardTargetMemory, deletedID))
	deleteReq := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
	deleteRR := httptest.NewRecorder()
	srv.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d — body: %s", deleteRR.Code, deleteRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/governance/memories?project=normal-list", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Memories []governance.Memory `json:"memories"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(resp.Memories) != 1 || resp.Memories[0].ID != activeID || resp.Memories[0].ID == deletedID {
		t.Fatalf("memories = %+v, want only active memory %d", resp.Memories, activeID)
	}
}

func TestGovernanceGuardExecuteHTTPInvalidBackupArchiveDoesNotMutate(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateSession("sess-guard-invalid-backup", "alpha", tempDir, "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	memoryID, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "Guarded invalid backup", Content: "keep me", SessionID: "sess-guard-invalid-backup"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	backupStore := governance.NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), store.RawDB())
	backup, err := backupStore.Create(context.Background())
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	if err := os.Remove(backup.ArchivePath); err != nil {
		t.Fatalf("Remove backup archive: %v", err)
	}
	before := readHTTPGovernanceCounters(t, store)
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(store, backupStore))
	body := fmt.Sprintf(`{"operation":"delete","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":%q,"reason":"cleanup"}`, memoryID, backup.ID, governance.GuardConfirmation(governance.GuardOperationDelete, governance.GuardTargetMemory, memoryID))
	req := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "backup archive integrity check failed") {
		t.Fatalf("body = %s, want archive integrity error", rr.Body.String())
	}
	after := readHTTPGovernanceCounters(t, store)
	if before != after {
		t.Fatalf("invalid backup archive mutated state: before=%+v after=%+v", before, after)
	}
	var deletedAt sql.NullString
	if err := store.RawDB().QueryRow(`SELECT deleted_at FROM memories WHERE id = ?`, memoryID).Scan(&deletedAt); err != nil {
		t.Fatalf("query memory: %v", err)
	}
	if deletedAt.Valid {
		t.Fatal("memory was deleted despite invalid backup archive")
	}
}

func TestGovernanceGuardExecuteHTTPMapsDeleteTargetStateErrors(t *testing.T) {
	tests := []struct {
		name       string
		prepareID  func(t *testing.T, store *db.DB) int64
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing memory",
			prepareID:  func(t *testing.T, store *db.DB) int64 { return 9999 },
			wantStatus: http.StatusNotFound,
			wantError:  "memory not found",
		},
		{
			name: "already deleted memory",
			prepareID: func(t *testing.T, store *db.DB) int64 {
				id := saveHTTPGuardMemory(t, store, "delete-state")
				if err := store.DeleteMemory(id, "tester", "already deleted"); err != nil {
					t.Fatalf("DeleteMemory: %v", err)
				}
				return id
			},
			wantStatus: http.StatusConflict,
			wantError:  "memory already deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, backup, srv := newHTTPGuardTestServer(t)
			memoryID := tt.prepareID(t, store)
			body := fmt.Sprintf(`{"operation":"delete","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":%q,"reason":"cleanup"}`, memoryID, backup.ID, governance.GuardConfirmation(governance.GuardOperationDelete, governance.GuardTargetMemory, memoryID))
			req := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d — body: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.wantError) {
				t.Fatalf("body = %s, want %q", rr.Body.String(), tt.wantError)
			}
		})
	}
}

func TestGovernanceGuardExecuteHTTPMapsRestoreTargetStateErrors(t *testing.T) {
	tests := []struct {
		name       string
		prepareID  func(t *testing.T, store *db.DB) int64
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing memory",
			prepareID:  func(t *testing.T, store *db.DB) int64 { return 9999 },
			wantStatus: http.StatusNotFound,
			wantError:  "memory not found",
		},
		{
			name:       "active memory",
			prepareID:  func(t *testing.T, store *db.DB) int64 { return saveHTTPGuardMemory(t, store, "restore-state") },
			wantStatus: http.StatusConflict,
			wantError:  "memory is not deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, backup, srv := newHTTPGuardTestServer(t)
			memoryID := tt.prepareID(t, store)
			body := fmt.Sprintf(`{"operation":"restore","target_type":"memory","target_id":%d,"backup_id":%q,"confirmation":%q}`, memoryID, backup.ID, governance.GuardConfirmation(governance.GuardOperationRestore, governance.GuardTargetMemory, memoryID))
			req := httptest.NewRequest(http.MethodPost, "/governance/guards/execute", bytes.NewBufferString(body))
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d — body: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.wantError) {
				t.Fatalf("body = %s, want %q", rr.Body.String(), tt.wantError)
			}
		})
	}
}

func TestGovernanceProjectArchiveHTTPArchivesLocalProjectWithCloudHandoffNote(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "alpha", "project-archive-alpha")
	saveHTTPGuardMemoryInProject(t, store, "beta", "project-archive-beta")
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"cleanup"}`, backup.ID, governance.ProjectArchiveConfirmation("alpha"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/alpha/archive", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result governance.ProjectArchiveResult `json:"result"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.Result.Mutated || resp.Result.Project != "alpha" || resp.Result.BackupID != backup.ID {
		t.Fatalf("archive result = %+v, want mutated alpha with backup %s", resp.Result, backup.ID)
	}
	if !strings.Contains(resp.Result.CloudHandoffNote, "No cloud project mutation") {
		t.Fatalf("cloud handoff note = %q, want explicit no-cloud mutation note", resp.Result.CloudHandoffNote)
	}
	alpha, err := store.GetGovernanceProject(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("GetGovernanceProject alpha: %v", err)
	}
	beta, err := store.GetGovernanceProject(context.Background(), "beta")
	if err != nil {
		t.Fatalf("GetGovernanceProject beta: %v", err)
	}
	if !alpha.Archived || beta.Archived {
		t.Fatalf("archive state alpha=%t beta=%t, want selected local project only", alpha.Archived, beta.Archived)
	}
}

func TestGovernanceProjectDetailHTTPAllowsEscapedSlashProjectName(t *testing.T) {
	store, _, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "alpha/archive", "project-detail-alpha-archive")
	req := httptest.NewRequest(http.MethodGet, "/governance/projects/alpha%2Farchive", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Project governance.Project `json:"project"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.Project.Name != "alpha/archive" {
		t.Fatalf("project name = %q, want alpha/archive", resp.Project.Name)
	}
}

func TestGovernanceProjectArchiveHTTPDecodesEscapedProjectNameOnce(t *testing.T) {
	for _, tt := range []struct {
		name        string
		projectName string
		escapedPath string
	}{
		{name: "slash project name", projectName: "alpha/archive", escapedPath: "alpha%2Farchive"},
		{name: "literal percent escape text", projectName: "literal%2Ftext", escapedPath: "literal%252Ftext"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, backup, srv := newHTTPGuardTestServer(t)
			saveHTTPGuardMemoryInProject(t, store, tt.projectName, "project-archive-"+strings.ReplaceAll(tt.name, " ", "-"))
			body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"cleanup"}`, backup.ID, governance.ProjectArchiveConfirmation(tt.projectName))
			req := httptest.NewRequest(http.MethodPost, "/governance/projects/"+tt.escapedPath+"/archive", bytes.NewBufferString(body))
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
			}
			var resp struct {
				Result governance.ProjectArchiveResult `json:"result"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("response not valid JSON: %v", err)
			}
			if resp.Result.Project != tt.projectName || !resp.Result.Mutated {
				t.Fatalf("archive result = %+v, want mutated project %q", resp.Result, tt.projectName)
			}
			project, err := store.GetGovernanceProject(context.Background(), tt.projectName)
			if err != nil {
				t.Fatalf("GetGovernanceProject %q: %v", tt.projectName, err)
			}
			if !project.Archived {
				t.Fatalf("project %q was not archived", tt.projectName)
			}
		})
	}
}

func TestGovernanceProjectArchiveHTTPMismatchPreservesExactConfirmationAndDoesNotMutate(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "alpha", "project-archive-mismatch")
	before := readProjectGovernanceRowsHTTP(t, store)
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":"%s "}`, backup.ID, governance.ProjectArchiveConfirmation("alpha"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/alpha/archive", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "confirmation mismatch") {
		t.Fatalf("body = %s, want confirmation mismatch", rr.Body.String())
	}
	after := readProjectGovernanceRowsHTTP(t, store)
	if after != before {
		t.Fatalf("mismatched archive confirmation mutated governance rows: before=%d after=%d", before, after)
	}
}

func TestGovernanceProjectMergeHTTPRecordsLocalMetadataWithCloudHandoffNote(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	sourceMemoryID := saveHTTPGuardMemoryInProject(t, store, "alpha", "project-merge-alpha")
	targetMemoryID := saveHTTPGuardMemoryInProject(t, store, "beta", "project-merge-beta")
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"dedupe"}`, backup.ID, governance.ProjectMergeConfirmation("alpha", "beta"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/alpha/merge/beta", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result governance.ProjectMergeResult `json:"result"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.Result.Mutated || resp.Result.SourceProject != "alpha" || resp.Result.TargetProject != "beta" || resp.Result.BackupID != backup.ID {
		t.Fatalf("merge result = %+v, want mutated alpha into beta with backup %s", resp.Result, backup.ID)
	}
	if !strings.Contains(resp.Result.CloudHandoffNote, "No cloud project mutation") {
		t.Fatalf("cloud handoff note = %q, want explicit no-cloud mutation note", resp.Result.CloudHandoffNote)
	}
	// After physical migration alpha has no rows — read governance record directly.
	var alphaMergeTarget string
	var alphaIsMerged bool
	if govErr := store.RawDB().QueryRowContext(context.Background(), `
SELECT merge_target != '', COALESCE(merge_target,'')
FROM hive_project_governance WHERE project = 'alpha'`).Scan(&alphaIsMerged, &alphaMergeTarget); govErr != nil {
		t.Fatalf("read alpha governance: %v", govErr)
	}
	if !alphaIsMerged || alphaMergeTarget != "beta" {
		t.Fatalf("alpha governance: merged=%v target=%q, want merged into beta", alphaIsMerged, alphaMergeTarget)
	}
	beta, err := store.GetGovernanceProject(context.Background(), "beta")
	if err != nil {
		t.Fatalf("GetGovernanceProject beta: %v", err)
	}
	if beta.Merged {
		t.Fatalf("target project beta must not be merged: %+v", beta)
	}
	// Physical migration: source memory is now under beta.
	if got := requireHTTPMemoryProject(t, store, sourceMemoryID); got != "beta" {
		t.Fatalf("source memory project = %q, want beta (physical migration)", got)
	}
	if got := requireHTTPMemoryProject(t, store, targetMemoryID); got != "beta" {
		t.Fatalf("target memory project = %q, want beta", got)
	}
}

func TestGovernanceProjectMergeHTTPDecodesEscapedProjectNamesOnce(t *testing.T) {
	for _, tt := range []struct {
		name          string
		source        string
		target        string
		escapedSource string
		escapedTarget string
	}{
		{name: "slash project names", source: "alpha/source", target: "beta/target", escapedSource: "alpha%2Fsource", escapedTarget: "beta%2Ftarget"},
		{name: "literal percent escape text", source: "literal%2Fsource", target: "literal%2Ftarget", escapedSource: "literal%252Fsource", escapedTarget: "literal%252Ftarget"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, backup, srv := newHTTPGuardTestServer(t)
			saveHTTPGuardMemoryInProject(t, store, tt.source, "project-merge-source-"+strings.ReplaceAll(tt.name, " ", "-"))
			saveHTTPGuardMemoryInProject(t, store, tt.target, "project-merge-target-"+strings.ReplaceAll(tt.name, " ", "-"))
			body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q}`, backup.ID, governance.ProjectMergeConfirmation(tt.source, tt.target))
			req := httptest.NewRequest(http.MethodPost, "/governance/projects/"+tt.escapedSource+"/merge/"+tt.escapedTarget, bytes.NewBufferString(body))
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
			}
			var resp struct {
				Result governance.ProjectMergeResult `json:"result"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("response not valid JSON: %v", err)
			}
			if resp.Result.SourceProject != tt.source || resp.Result.TargetProject != tt.target || !resp.Result.Mutated {
				t.Fatalf("merge result = %+v, want %q into %q", resp.Result, tt.source, tt.target)
			}
		})
	}
}

func TestGovernanceProjectMergeHTTPMismatchDoesNotMutate(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "alpha", "project-merge-mismatch-alpha")
	saveHTTPGuardMemoryInProject(t, store, "beta", "project-merge-mismatch-beta")
	before := readProjectGovernanceRowsHTTP(t, store)
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":"%s "}`, backup.ID, governance.ProjectMergeConfirmation("alpha", "beta"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/alpha/merge/beta", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "confirmation mismatch") {
		t.Fatalf("body = %s, want confirmation mismatch", rr.Body.String())
	}
	if after := readProjectGovernanceRowsHTTP(t, store); after != before {
		t.Fatalf("mismatched merge confirmation mutated governance rows: before=%d after=%d", before, after)
	}
}

func TestGovernanceEndpointsRejectNonGETMethods(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", store, governance.NewService(store))

	req := httptest.NewRequest(http.MethodPost, "/governance/projects", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func newHTTPGuardTestServer(t *testing.T) (*db.DB, governance.BackupManifest, *httpapi.Server) {
	t.Helper()
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	backupStore := governance.NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), store.RawDB())
	backup, err := backupStore.Create(context.Background())
	if err != nil {
		t.Fatalf("Create backup: %v", err)
	}
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewServiceWithBackup(store, backupStore))
	return store, backup, srv
}

func saveHTTPGuardMemory(t *testing.T, store *db.DB, sessionID string) int64 {
	return saveHTTPGuardMemoryInProject(t, store, "alpha", sessionID)
}

func saveHTTPGuardMemoryInProject(t *testing.T, store *db.DB, projectName, sessionID string) int64 {
	t.Helper()
	if err := store.CreateSession(sessionID, projectName, t.TempDir(), "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	memoryID, err := store.SaveMemory(&models.Memory{Project: projectName, Title: "Guarded state", Content: "state", SessionID: sessionID})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	return memoryID
}

func readProjectGovernanceRowsHTTP(t *testing.T, store *db.DB) int {
	t.Helper()
	var count int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance`).Scan(&count); err != nil {
		t.Fatalf("count project governance rows: %v", err)
	}
	return count
}

func readHTTPGovernanceCounters(t *testing.T, d *db.DB) governanceCountersHTTP {
	t.Helper()
	var got governanceCountersHTTP
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&got.MemoryCount); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memory_mutations`).Scan(&got.MutationCount); err != nil {
		t.Fatalf("count mutations: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state`).Scan(&got.SyncRows); err != nil {
		t.Fatalf("count sync_state: %v", err)
	}
	return got
}

func requireHTTPMemoryProject(t *testing.T, d *db.DB, id int64) string {
	t.Helper()
	var projectName string
	if err := d.RawDB().QueryRow(`SELECT project FROM memories WHERE id = ?`, id).Scan(&projectName); err != nil {
		t.Fatalf("query memory project: %v", err)
	}
	return projectName
}

type governanceCountersHTTP struct {
	MemoryCount   int
	MutationCount int
	SyncRows      int
}

// ─── T13: Config endpoints ────────────────────────────────────────────────────

// fakeConfigService is a test double for httpapi.ConfigService.
type fakeConfigService struct {
	statusFn func(ctx context.Context) (httpapi.ConfigServiceStatus, error)
	updateFn func(ctx context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error)
	testFn   func(ctx context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceTestResult, error)
}

func (f *fakeConfigService) Status(ctx context.Context) (httpapi.ConfigServiceStatus, error) {
	if f.statusFn != nil {
		return f.statusFn(ctx)
	}
	return httpapi.ConfigServiceStatus{
		Configured:     true,
		Source:         "file",
		APIURL:         "https://hive.example.com",
		Email:          "user@example.com",
		PasswordSet:    true,
		PasswordMasked: "********",
		AutoSync:       false,
		EnvActive:      false,
	}, nil
}

func (f *fakeConfigService) Update(ctx context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, req)
	}
	return httpapi.ConfigServiceStatus{
		Configured:     true,
		Source:         "file",
		APIURL:         req.APIURL,
		Email:          req.Email,
		PasswordSet:    req.Password != "",
		PasswordMasked: "********",
		AutoSync:       req.AutoSync,
		RestartHint:    "Saved. Restart hive-daemon for the new configuration to take effect.",
	}, nil
}

func (f *fakeConfigService) Test(ctx context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceTestResult, error) {
	if f.testFn != nil {
		return f.testFn(ctx, req)
	}
	return httpapi.ConfigServiceTestResult{OK: true, Message: "Connection succeeded"}, nil
}

func newConfigTestServer(svc httpapi.ConfigService) *httpapi.Server {
	return httpapi.NewServerWithConfig("127.0.0.1:0", &mockPromptStore{}, svc)
}

// TestConfigStatus_200_MaskedPassword verifies GET /governance/config/status
// returns masked password and never the raw secret.
func TestConfigStatus_200_MaskedPassword(t *testing.T) {
	const rawSecret = "supersecret"

	svc := &fakeConfigService{
		statusFn: func(_ context.Context) (httpapi.ConfigServiceStatus, error) {
			return httpapi.ConfigServiceStatus{
				Configured:     true,
				Source:         "file",
				APIURL:         "https://hive.example.com",
				Email:          "user@example.com",
				PasswordSet:    true,
				PasswordMasked: "********",
				AutoSync:       false,
				EnvActive:      false,
			}, nil
		},
	}
	srv := newConfigTestServer(svc)

	req := httptest.NewRequest(http.MethodGet, "/governance/config/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.Bytes()

	// No-secret-leak scan.
	if strings.Contains(string(body), rawSecret) {
		t.Errorf("raw secret %q leaked into response body: %s", rawSecret, body)
	}

	var resp httpapi.ConfigStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.PasswordMasked != "********" {
		t.Errorf("PasswordMasked = %q, want sentinel", resp.PasswordMasked)
	}
	if !resp.PasswordSet {
		t.Error("PasswordSet = false, want true")
	}
	if resp.EnvActive {
		t.Error("EnvActive = true, want false for file source")
	}
}

// TestConfigStatus_200_Unconfigured verifies GET /governance/config/status
// returns PasswordSet=false and PasswordMasked="" when not configured.
func TestConfigStatus_200_Unconfigured(t *testing.T) {
	svc := &fakeConfigService{
		statusFn: func(_ context.Context) (httpapi.ConfigServiceStatus, error) {
			return httpapi.ConfigServiceStatus{
				Configured:     false,
				Source:         "none",
				PasswordSet:    false,
				PasswordMasked: "",
			}, nil
		},
	}
	srv := newConfigTestServer(svc)

	req := httptest.NewRequest(http.MethodGet, "/governance/config/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp httpapi.ConfigStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.PasswordSet {
		t.Error("PasswordSet = true, want false")
	}
	if resp.PasswordMasked != "" {
		t.Errorf("PasswordMasked = %q, want empty", resp.PasswordMasked)
	}
}

// TestConfigUpdate_200_NewPassword verifies POST /governance/config with a new
// password returns RestartRequired=true and the file is written correctly.
func TestConfigUpdate_200_NewPassword(t *testing.T) {
	const rawSecret = "newpass"
	var gotUpdate httpapi.ConfigServiceUpdate

	svc := &fakeConfigService{
		updateFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error) {
			gotUpdate = req
			return httpapi.ConfigServiceStatus{
				Configured:     true,
				Source:         "file",
				APIURL:         req.APIURL,
				Email:          req.Email,
				PasswordSet:    true,
				PasswordMasked: "********",
				RestartHint:    "Saved. Restart hive-daemon for the new configuration to take effect.",
			}, nil
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"user@example.com","password":"newpass","auto_sync":true}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	responseBytes := rr.Body.Bytes()

	// No-secret-leak scan.
	if strings.Contains(string(responseBytes), rawSecret) {
		t.Errorf("raw secret leaked into response: %s", responseBytes)
	}

	var resp httpapi.ConfigUpdateResponse
	if err := json.Unmarshal(responseBytes, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.RestartRequired {
		t.Error("RestartRequired = false, want true")
	}
	if gotUpdate.Password != "newpass" {
		t.Errorf("password passed to service = %q, want newpass", gotUpdate.Password)
	}
}

// TestConfigUpdate_200_Sentinel verifies that sending the masked sentinel
// passes it through to the service unchanged.
func TestConfigUpdate_200_Sentinel(t *testing.T) {
	var gotUpdate httpapi.ConfigServiceUpdate

	svc := &fakeConfigService{
		updateFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error) {
			gotUpdate = req
			return httpapi.ConfigServiceStatus{
				Configured:     true,
				PasswordSet:    true,
				PasswordMasked: "********",
				RestartHint:    "Saved. Restart hive-daemon for the new configuration to take effect.",
			}, nil
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"user@example.com","password":"********","auto_sync":false}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if gotUpdate.Password != "********" {
		t.Errorf("sentinel was modified before reaching service: got %q", gotUpdate.Password)
	}
}

// TestConfigUpdate_200_EnvActive verifies that env-active path returns
// RestartHint mentioning env and EnvActive=true.
func TestConfigUpdate_200_EnvActive(t *testing.T) {
	svc := &fakeConfigService{
		updateFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error) {
			return httpapi.ConfigServiceStatus{
				Configured:     true,
				Source:         "env",
				EnvActive:      true,
				PasswordSet:    true,
				PasswordMasked: "********",
				RestartHint:    "env variable overrides are active; restart to use file values",
			}, nil
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"user@example.com","password":"pass","auto_sync":false}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp httpapi.ConfigUpdateResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.EnvActive {
		t.Error("EnvActive = false, want true")
	}
	if !strings.Contains(resp.RestartHint, "env") {
		t.Errorf("RestartHint = %q, want env mention", resp.RestartHint)
	}
}

// TestConfigUpdate_400_InvalidURL verifies POST /governance/config returns 400
// when the service returns ErrConfigInvalidURL.
func TestConfigUpdate_400_InvalidURL(t *testing.T) {
	svc := &fakeConfigService{
		updateFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error) {
			return httpapi.ConfigServiceStatus{}, httpapi.ErrConfigInvalidURL
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"not-a-url","email":"user@example.com","password":"pass","auto_sync":false}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestConfigUpdate_400_EmptyEmail verifies POST /governance/config returns 400
// when the service returns ErrConfigEmailRequired.
func TestConfigUpdate_400_EmptyEmail(t *testing.T) {
	svc := &fakeConfigService{
		updateFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceStatus, error) {
			return httpapi.ConfigServiceStatus{}, httpapi.ErrConfigEmailRequired
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"","password":"pass","auto_sync":false}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestConfigTest_200_Success verifies POST /governance/config/test returns
// 200 with ok=true on success.
func TestConfigTest_200_Success(t *testing.T) {
	const rawSecret = "realpass"

	svc := &fakeConfigService{
		testFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceTestResult, error) {
			return httpapi.ConfigServiceTestResult{OK: true, Message: "Connection succeeded"}, nil
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"user@example.com","password":"realpass"}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config/test", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	responseBytes := rr.Body.Bytes()
	if strings.Contains(string(responseBytes), rawSecret) {
		t.Errorf("raw secret leaked: %s", responseBytes)
	}

	var resp httpapi.ConfigTestResult
	if err := json.Unmarshal(responseBytes, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.OK {
		t.Errorf("OK = false, want true; message: %s", resp.Message)
	}
}

// TestConfigTest_200_Failure verifies POST /governance/config/test returns
// 200 with ok=false (NOT an HTTP error) on auth failure, without leaking raw password.
func TestConfigTest_200_Failure(t *testing.T) {
	const rawSecret = "wrongpass"

	svc := &fakeConfigService{
		testFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceTestResult, error) {
			return httpapi.ConfigServiceTestResult{OK: false, Message: "Connection failed: login failed (401)"}, nil
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"user@example.com","password":"wrongpass"}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config/test", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (test failure is ok=false, not HTTP error), got %d — body: %s", rr.Code, rr.Body.String())
	}

	responseBytes := rr.Body.Bytes()
	if strings.Contains(string(responseBytes), rawSecret) {
		t.Errorf("raw secret leaked: %s", responseBytes)
	}

	var resp httpapi.ConfigTestResult
	if err := json.Unmarshal(responseBytes, &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp.OK {
		t.Error("OK = true, want false for auth failure")
	}
}

// TestConfigTest_200_Sentinel verifies POST /governance/config/test with
// the masked sentinel passes it through to the service.
func TestConfigTest_200_Sentinel(t *testing.T) {
	var gotReq httpapi.ConfigServiceUpdate

	svc := &fakeConfigService{
		testFn: func(_ context.Context, req httpapi.ConfigServiceUpdate) (httpapi.ConfigServiceTestResult, error) {
			gotReq = req
			return httpapi.ConfigServiceTestResult{OK: true, Message: "Connection succeeded"}, nil
		},
	}
	srv := newConfigTestServer(svc)

	body := `{"api_url":"https://hive.example.com","email":"user@example.com","password":"********"}`
	req := httptest.NewRequest(http.MethodPost, "/governance/config/test", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if gotReq.Password != "********" {
		t.Errorf("sentinel modified before reaching service: got %q", gotReq.Password)
	}
}

// TestConfigEndpoints_RejectNonLoopback verifies all config endpoints return
// 403 for non-loopback remote addresses.
func TestConfigEndpoints_RejectNonLoopback(t *testing.T) {
	svc := &fakeConfigService{}
	srv := newConfigTestServer(svc)

	for _, tt := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/governance/config/status"},
		{http.MethodPost, "/governance/config"},
		{http.MethodPost, "/governance/config/test"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var body *bytes.Buffer
			if tt.method == http.MethodPost {
				body = bytes.NewBufferString(`{"api_url":"https://hive.example.com","email":"x@x.com","password":"p"}`)
			} else {
				body = &bytes.Buffer{}
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.RemoteAddr = "192.168.1.100:1234" // non-loopback
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for non-loopback, got %d — body: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

// Task 3.1 — GET /governance/memories/{id}

func TestHandleGovernanceMemory_200(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	id, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "detail memory", Content: "rich content", SessionID: "sess-alpha"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewService(store))
	path := fmt.Sprintf("/governance/memories/%d", id)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if _, ok := resp["memory"]; !ok {
		t.Fatalf("response missing 'memory' field: %v", resp)
	}
	memory := resp["memory"].(map[string]any)
	if memory["title"] != "detail memory" {
		t.Fatalf("title = %v, want 'detail memory'", memory["title"])
	}
	if memory["content"] != "rich content" {
		t.Fatalf("content = %v, want 'rich content'", memory["content"])
	}
}

func TestHandleGovernanceMemory_400NonNumeric(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewService(store))
	req := httptest.NewRequest(http.MethodGet, "/governance/memories/abc", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp["error"] == "" {
		t.Fatal("expected error field in response")
	}
}

func TestHandleGovernanceMemory_404NotFound(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewService(store))
	req := httptest.NewRequest(http.MethodGet, "/governance/memories/99999", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp["error"] == "" {
		t.Fatal("expected error field in response")
	}
}

// Task 3.1 — POST /governance/projects/merge (batch)

func TestGovernanceProjectMergeBatchHTTPReturnsPerSourceResults(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "alpha", "batch-merge-alpha")
	saveHTTPGuardMemoryInProject(t, store, "gamma", "batch-merge-gamma")
	saveHTTPGuardMemoryInProject(t, store, "beta", "batch-merge-beta")
	body := fmt.Sprintf(`{"sources":["alpha","gamma"],"target":"beta","backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"dedupe"}`, backup.ID, governance.ProjectMergeBatchConfirmation("beta"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/merge", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result governance.ProjectMergeBatchResult `json:"result"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if len(resp.Result.Results) != 2 {
		t.Fatalf("results len = %d, want 2; body: %s", len(resp.Result.Results), rr.Body.String())
	}
	for _, r := range resp.Result.Results {
		if r.ErrMsg != "" {
			t.Fatalf("result %q has error: %s", r.Source, r.ErrMsg)
		}
		if !r.Mutated {
			t.Fatalf("result %q not mutated", r.Source)
		}
	}
	if resp.Result.Target != "beta" {
		t.Fatalf("target = %q, want beta", resp.Result.Target)
	}
	if resp.Result.BackupID != backup.ID {
		t.Fatalf("backup_id = %q, want %s", resp.Result.BackupID, backup.ID)
	}
}

// Task 3.2 — POST /governance/projects/merge with empty sources returns 400

func TestGovernanceProjectMergeBatchHTTPEmptySourcesReturns400(t *testing.T) {
	_, backup, srv := newHTTPGuardTestServer(t)
	body := fmt.Sprintf(`{"sources":[],"target":"beta","backup_id":%q,"confirmation":%q}`, backup.ID, governance.ProjectMergeBatchConfirmation("beta"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/merge", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleGovernanceMemory_404Deleted(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	id, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "deleted memory", Content: "deleted content", SessionID: "sess-alpha"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := store.DeleteMemory(id, "tester", "stale"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewService(store))
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/governance/memories/%d", id), nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp["error"] != "memory not found" {
		t.Fatalf("error = %q, want memory not found", resp["error"])
	}
}

// ─── Phase 3: POST /governance/projects/{name}/delete ────────────────────────

// TestHandleGovernanceProjectDelete_HappyPath verifies that POST .../delete with
// a valid body returns 200 and a DeleteProjectResult JSON.
func TestHandleGovernanceProjectDelete_HappyPath(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "purge-target", "session-purge-target")
	// Archive the project first.
	if _, err := store.ArchiveGovernanceProject(context.Background(), "purge-target", "actor", "test", time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("ArchiveGovernanceProject: %v", err)
	}
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"cleanup"}`,
		backup.ID, governance.ProjectDeleteConfirmation("purge-target"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/purge-target/delete", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result governance.DeleteProjectResult `json:"result"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if !resp.Result.Mutated || resp.Result.Project != "purge-target" {
		t.Fatalf("delete result = %+v, want mutated purge-target", resp.Result)
	}
	if !strings.Contains(resp.Result.CloudHandoffNote, "Cloud data not removed") {
		t.Fatalf("cloud handoff note = %q, want cloud-data-not-removed note", resp.Result.CloudHandoffNote)
	}
}

// TestHandleGovernanceProjectDelete_NotArchived verifies that POST .../delete
// on a non-archived project returns 409.
func TestHandleGovernanceProjectDelete_NotArchived(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "live-project", "session-live-project")
	// Do NOT archive — project is active.
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":%q,"actor_id":"tester","reason":"cleanup"}`,
		backup.ID, governance.ProjectDeleteConfirmation("live-project"))
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/live-project/delete", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleGovernanceProjectDelete_BadConfirmation verifies that POST .../delete
// with a wrong phrase returns 400.
func TestHandleGovernanceProjectDelete_BadConfirmation(t *testing.T) {
	store, backup, srv := newHTTPGuardTestServer(t)
	saveHTTPGuardMemoryInProject(t, store, "alpha-del", "session-alpha-del")
	body := fmt.Sprintf(`{"backup_id":%q,"confirmation":"WRONG PHRASE","actor_id":"tester","reason":"cleanup"}`,
		backup.ID)
	req := httptest.NewRequest(http.MethodPost, "/governance/projects/alpha-del/delete", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "confirmation mismatch") {
		t.Fatalf("body = %s, want confirmation mismatch", rr.Body.String())
	}
}

// ─── T16: GET /governance/projects/{name}/timeline ────────────────────────────

func newTimelineTestServer(t *testing.T) (*db.DB, *httpapi.Server) {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := httpapi.NewServerWithGovernance("127.0.0.1:0", &mockPromptStore{}, governance.NewService(store))
	return store, srv
}

func insertTimelineMemory(t *testing.T, store *db.DB, project, title, category, createdAt string) {
	t.Helper()
	if _, err := store.EnsureManualSaveSession(project); err != nil {
		t.Fatalf("EnsureManualSaveSession: %v", err)
	}
	_, err := store.RawDB().Exec(`
INSERT INTO memories (sync_id, project, category, title, content, created_by, session_id, created_at, updated_at)
VALUES (?, ?, ?, ?, 'content', 'tester', ?, ?, ?)`,
		"sync-"+title+"-"+project, project, category, title,
		"manual-save-"+project, createdAt, createdAt)
	if err != nil {
		t.Fatalf("insertTimelineMemory: %v", err)
	}
}

// TestHandleGovernanceTimeline_HappyPath verifies that the timeline endpoint
// returns 200 with only the 5 timeline categories, ordered oldest-first.
func TestHandleGovernanceTimeline_HappyPath(t *testing.T) {
	store, srv := newTimelineTestServer(t)

	insertTimelineMemory(t, store, "atlas", "Old decision", "decision", "2026-01-01 10:00:00")
	insertTimelineMemory(t, store, "atlas", "Middle note", "note", "2026-01-02 10:00:00")
	insertTimelineMemory(t, store, "atlas", "New bugfix", "bugfix", "2026-01-03 10:00:00")

	req := httptest.NewRequest(http.MethodGet, "/governance/projects/atlas/timeline", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Memories []map[string]any `json:"memories"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Only decision and bugfix (not note) should be returned.
	if len(resp.Memories) != 2 {
		t.Fatalf("expected 2 timeline memories, got %d: %v", len(resp.Memories), resp.Memories)
	}
	// First must be the oldest (ASC order).
	if resp.Memories[0]["title"] != "Old decision" {
		t.Fatalf("first memory title = %v, want Old decision (ASC order)", resp.Memories[0]["title"])
	}
	if resp.Memories[1]["title"] != "New bugfix" {
		t.Fatalf("second memory title = %v, want New bugfix", resp.Memories[1]["title"])
	}
}

// TestHandleGovernanceTimeline_ProjectNotFound verifies that the timeline
// endpoint returns 404 for an unknown project.
func TestHandleGovernanceTimeline_ProjectNotFound(t *testing.T) {
	_, srv := newTimelineTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/governance/projects/ghost/timeline", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleGovernanceTimeline_WrongMethod verifies that non-GET methods
// return 405 for the timeline endpoint.
func TestHandleGovernanceTimeline_WrongMethod(t *testing.T) {
	store, srv := newTimelineTestServer(t)
	insertTimelineMemory(t, store, "atlas2", "A decision", "decision", "2026-01-01 10:00:00")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/governance/projects/atlas2/timeline", nil)
			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405, got %d for method %s", rr.Code, method)
			}
		})
	}
}

// TestHandleGovernanceTimeline_EmptyResult verifies that a project with no
// timeline-category memories returns 200 with an empty array.
func TestHandleGovernanceTimeline_EmptyResult(t *testing.T) {
	store, srv := newTimelineTestServer(t)
	insertTimelineMemory(t, store, "empty-proj", "A note", "note", "2026-01-01 10:00:00")

	req := httptest.NewRequest(http.MethodGet, "/governance/projects/empty-proj/timeline", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Memories []any `json:"memories"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Memories) != 0 {
		t.Fatalf("expected empty memories array, got %d entries", len(resp.Memories))
	}
}

// TestHandleGovernanceMemories_RegressionDescOrder verifies that the existing
// /governance/memories endpoint is unaffected by the timeline changes:
// it still returns all types in DESC order.
func TestHandleGovernanceMemories_RegressionDescOrder(t *testing.T) {
	store, srv := newTimelineTestServer(t)
	insertTimelineMemory(t, store, "regress", "Old memory", "decision", "2026-01-01 10:00:00")
	insertTimelineMemory(t, store, "regress", "Middle memory", "note", "2026-01-02 10:00:00")
	insertTimelineMemory(t, store, "regress", "New memory", "architecture", "2026-01-03 10:00:00")

	req := httptest.NewRequest(http.MethodGet, "/governance/memories?project=regress", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Memories []map[string]any `json:"memories"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// All 3 types must be present.
	if len(resp.Memories) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(resp.Memories))
	}
	// First must be newest (DESC order).
	if resp.Memories[0]["title"] != "New memory" {
		t.Fatalf("first memory = %v, want New memory (DESC default for /governance/memories)", resp.Memories[0]["title"])
	}
}

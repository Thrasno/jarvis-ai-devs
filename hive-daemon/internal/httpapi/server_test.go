package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

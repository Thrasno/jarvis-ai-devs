package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── mock SessionStore ────────────────────────────────────────────────────────

type mockSessionStore struct {
	createSessionFn          func(id, project, directory, devID, client string) error
	ensureSessionFn          func(context.Context, models.SessionInput) (*models.Session, error)
	endSessionFn             func(id, summary string) error
	savePassiveObservationFn func(ctx context.Context, sessionID, project, source, content string) error
}

func (m *mockSessionStore) CreateSession(id, project, directory, devID, client string) error {
	if m.createSessionFn != nil {
		return m.createSessionFn(id, project, directory, devID, client)
	}
	return nil
}

func (m *mockSessionStore) EnsureSession(ctx context.Context, in models.SessionInput) (*models.Session, error) {
	if m.ensureSessionFn != nil {
		return m.ensureSessionFn(ctx, in)
	}
	if err := m.CreateSession(in.ID, in.Project, in.Directory, in.DevID, in.Client); err != nil {
		return nil, err
	}
	return &models.Session{ID: in.ID, Project: in.Project}, nil
}

func (m *mockSessionStore) EndSession(id, summary string) error {
	if m.endSessionFn != nil {
		return m.endSessionFn(id, summary)
	}
	return nil
}

func (m *mockSessionStore) SavePassiveObservation(ctx context.Context, sessionID, project, source, content string) error {
	if m.savePassiveObservationFn != nil {
		return m.savePassiveObservationFn(ctx, sessionID, project, source, content)
	}
	return nil
}

// ─── helper ──────────────────────────────────────────────────────────────────

func newServerWithSessions(sessions *mockSessionStore) *httpapi.Server {
	prompts := &mockPromptStore{}
	return httpapi.NewServerWithSessions("127.0.0.1:0", prompts, sessions)
}

func postJSON(srv *httpapi.Server, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

// ─── POST /sessions ───────────────────────────────────────────────────────────

func TestPostSessions_ValidBody_Returns200(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	body := `{"id":"sess-1","project":"jarvis-dev","directory":"/home/dev","dev_id":"andres","client":"claude"}`
	rr := postJSON(srv, "/sessions", body)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, true, resp["ok"])
}

func TestPostSessions_DuplicateID_Returns200_Idempotent(t *testing.T) {
	// Use a real DB so CreateSession returns a genuine *sqlite.Error on duplicate.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	realDB, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = realDB.Close() })

	srv := httpapi.NewServerWithSessions("127.0.0.1:0", &mockPromptStore{}, realDB)

	body := `{"id":"sess-dup","project":"jarvis-dev","directory":"/tmp","dev_id":"","client":""}`
	rr1 := postJSON(srv, "/sessions", body)
	require.Equal(t, http.StatusOK, rr1.Code, "first call should return 200")

	rr2 := postJSON(srv, "/sessions", body)
	assert.Equal(t, http.StatusOK, rr2.Code, "duplicate id (UNIQUE constraint) should return 200 (idempotent)")
}

func TestPostSessions_ConcurrentValidStartsShareOneEnsureAndExcludeInvalidOrBlocked(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls int
	var mu sync.Mutex
	store := &mockSessionStore{ensureSessionFn: func(ctx context.Context, in models.SessionInput) (*models.Session, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		close(entered)
		select {
		case <-release:
			return &models.Session{ID: in.ID, Project: in.Project}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	gate := project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady})
	srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, mockProjectStore{known: []project.KnownProject{{Name: "alpha"}}}, nil, nil, nil, store)
	srv.SetMigrationGate(gate)
	body := `{"id":"session-1","project":"alpha","directory":"/repo","dev_id":"dev","client":"hook"}`
	results := make(chan *httptest.ResponseRecorder, 2)
	go func() { results <- postJSON(srv, "/sessions", body) }()
	select {
	case <-entered:
	case result := <-results:
		t.Fatalf("start bypassed EnsureSession with status %d", result.Code)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("start did not reach EnsureSession")
	}
	go func() { results <- postJSON(srv, "/sessions", body) }()
	select {
	case result := <-results:
		t.Fatalf("start completed before release: %d", result.Code)
	case <-time.After(100 * time.Millisecond):
	}
	if result := postJSON(srv, "/sessions", `{"project":"alpha"}`); result.Code != http.StatusBadRequest {
		t.Fatalf("invalid start status = %d, want 400", result.Code)
	}
	gate.Adopt(project.MigrationStatus{State: project.MigrationStateBlocked})
	if result := postJSON(srv, "/sessions", body); result.Code != http.StatusServiceUnavailable {
		t.Fatalf("blocked start status = %d, want 503", result.Code)
	}
	close(release)
	for range 2 {
		if result := <-results; result.Code != http.StatusOK {
			t.Fatalf("valid start status = %d, want 200", result.Code)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("EnsureSession calls = %d, want 1", calls)
	}
}

func TestPostSessions_EmptyDevIDAndClient_Returns200(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	// Hook context: dev_id and client are empty — must be accepted without validation error.
	body := `{"id":"sess-hook","project":"my-project","directory":"/repo","dev_id":"","client":""}`
	rr := postJSON(srv, "/sessions", body)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPostSessions_EmptyProjectAndDirectory_Returns400_NoPersist(t *testing.T) {
	var createCalled bool
	store := &mockSessionStore{
		createSessionFn: func(id, project, directory, devID, client string) error {
			createCalled = true
			return nil
		},
	}
	srv := newServerWithSessions(store)

	// No project and no directory to derive from: the effective project stays
	// empty. Persisting a session row with project="" corrupts the sessions
	// table (project is TEXT NOT NULL but admits empty string), so the handler
	// must reject before touching the store.
	body := `{"id":"sess-empty","project":"","directory":"","dev_id":"","client":""}`
	rr := postJSON(srv, "/sessions", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.False(t, createCalled, "CreateSession must not be called when the effective project is empty")

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "project is required", resp["error"])
}

func TestPostSessions_MissingID_Returns400(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	body := `{"project":"jarvis-dev"}`
	rr := postJSON(srv, "/sessions", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPostSessions_DBError_Returns500(t *testing.T) {
	store := &mockSessionStore{
		createSessionFn: func(id, project, directory, devID, client string) error {
			return errors.New("disk full")
		},
	}
	srv := newServerWithSessions(store)

	body := `{"id":"sess-err","project":"jarvis-dev","directory":"/tmp","dev_id":"","client":""}`
	rr := postJSON(srv, "/sessions", body)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPostSessions_WrongMethod_Returns405(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// ─── POST /sessions/{id}/end ──────────────────────────────────────────────────

func TestPostSessionsEnd_ExistingSession_Returns200(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	rr := postJSON(srv, "/sessions/sess-1/end", "")

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, true, resp["ok"])
}

func TestPostSessionsEnd_NotFound_Returns404(t *testing.T) {
	store := &mockSessionStore{
		endSessionFn: func(id, summary string) error {
			return db.ErrSessionNotFound
		},
	}
	srv := newServerWithSessions(store)

	rr := postJSON(srv, "/sessions/ghost/end", "")

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPostSessionsEnd_DBError_Returns500(t *testing.T) {
	store := &mockSessionStore{
		endSessionFn: func(id, summary string) error {
			return errors.New("database locked")
		},
	}
	srv := newServerWithSessions(store)

	rr := postJSON(srv, "/sessions/sess-1/end", "")

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPostSessionsEnd_PassesSummaryAsEmpty(t *testing.T) {
	var capturedSummary string
	store := &mockSessionStore{
		endSessionFn: func(id, summary string) error {
			capturedSummary = summary
			return nil
		},
	}
	srv := newServerWithSessions(store)

	postJSON(srv, "/sessions/sess-1/end", "")

	assert.Equal(t, "", capturedSummary, "hook-initiated end must pass empty summary")
}

// ─── POST /observations/passive ───────────────────────────────────────────────

func TestPostObservationsPassive_ValidBody_Returns202(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	body := `{"content":"agent output","session_id":"sess-1","project":"jarvis-dev","source":"subagent-stop"}`
	rr := postJSON(srv, "/observations/passive", body)

	assert.Equal(t, http.StatusAccepted, rr.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, true, resp["ok"])
}

func TestPostObservationsPassive_MissingContent_Returns400(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	body := `{"session_id":"sess-1","project":"jarvis-dev","source":"subagent-stop"}`
	rr := postJSON(srv, "/observations/passive", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPostObservationsPassive_EmptyContent_Returns400(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	body := `{"content":"","session_id":"sess-1","project":"jarvis-dev","source":"subagent-stop"}`
	rr := postJSON(srv, "/observations/passive", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPostObservationsPassive_MissingOptionalFields_Returns202(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	// session_id, project, and source are optional — only content is required.
	body := `{"content":"agent output"}`
	rr := postJSON(srv, "/observations/passive", body)

	assert.Equal(t, http.StatusAccepted, rr.Code)
}

func TestPostObservationsPassive_DBError_Returns500(t *testing.T) {
	store := &mockSessionStore{
		savePassiveObservationFn: func(ctx context.Context, sessionID, project, source, content string) error {
			return errors.New("disk full")
		},
	}
	srv := newServerWithSessions(store)

	body := `{"content":"agent output","session_id":"sess-1","project":"jarvis-dev","source":"subagent-stop"}`
	rr := postJSON(srv, "/observations/passive", body)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPostObservationsPassive_WrongMethod_Returns405(t *testing.T) {
	store := &mockSessionStore{}
	srv := newServerWithSessions(store)

	req := httptest.NewRequest(http.MethodGet, "/observations/passive", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

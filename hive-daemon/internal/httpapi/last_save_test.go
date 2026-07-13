package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

// newLastSaveServer builds a Server backed by a real in-memory *db.DB so the
// unconditionally-registered latest-save route resolves against MemoryStore.
func newLastSaveServer(t *testing.T) (*db.DB, *httpapi.Server) {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := httpapi.NewServerWithAll("127.0.0.1:0", store, store, nil, nil, nil, store)
	return store, srv
}

func saveMemoryForProject(t *testing.T, store *db.DB, project string) {
	t.Helper()
	if _, err := store.EnsureManualSaveSession(project); err != nil {
		t.Fatalf("ensure manual-save session: %v", err)
	}
	_, err := store.SaveMemory(&models.Memory{
		Project:   project,
		Title:     "Title",
		Content:   "Content",
		SessionID: "manual-save-" + project,
	})
	if err != nil {
		t.Fatalf("save memory: %v", err)
	}
}

func TestHandleProjectLastSave_Found(t *testing.T) {
	store, srv := newLastSaveServer(t)
	saveMemoryForProject(t, store, "projA")

	req := httptest.NewRequest(http.MethodGet, "/projects/projA/last-save", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		LastSaveAt string `json:"last_save_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if body.LastSaveAt == "" {
		t.Fatalf("last_save_at should be populated when a save exists")
	}
	if _, err := time.Parse(time.RFC3339, body.LastSaveAt); err != nil {
		t.Fatalf("last_save_at should be RFC3339: %v (%q)", err, body.LastSaveAt)
	}
}

func TestHandleProjectLastSave_EmptyProjectReachable(t *testing.T) {
	_, srv := newLastSaveServer(t)

	req := httptest.NewRequest(http.MethodGet, "/projects/never-saved/last-save", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body struct {
		LastSaveAt string `json:"last_save_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.LastSaveAt != "" {
		t.Fatalf("last_save_at should be empty for a never-saved project, got %q", body.LastSaveAt)
	}
}

func TestHandleProjectLastSave_WrongMethod(t *testing.T) {
	_, srv := newLastSaveServer(t)

	req := httptest.NewRequest(http.MethodPost, "/projects/projA/last-save", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want 405", rec.Code)
	}
}

func TestHandleProjectLastSave_MissingProject(t *testing.T) {
	_, srv := newLastSaveServer(t)

	// A whitespace-only project segment must be rejected with 400.
	req := httptest.NewRequest(http.MethodGet, "/projects/%20/last-save", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleProjectLastSave_StoreNil_ServiceUnavailable(t *testing.T) {
	// A prompt store that does NOT satisfy MemoryStore leaves s.memories nil,
	// but the route must still be registered (nil-safe → 503, never 404).
	srv := httpapi.NewServerWithGovernanceAndHealth("127.0.0.1:0", &mockPromptStore{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/projects/projA/last-save", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want 503 (route must be registered, nil-safe); body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleProjectLastSave_ProjectNameWithSpaces(t *testing.T) {
	store, srv := newLastSaveServer(t)
	saveMemoryForProject(t, store, "my proj")

	req := httptest.NewRequest(http.MethodGet, "/projects/my%20proj/last-save", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		LastSaveAt string `json:"last_save_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.LastSaveAt == "" {
		t.Fatalf("last_save_at should be populated for spaced project name")
	}
}

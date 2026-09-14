package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/require"
)

func postPromptMaterialization(srv *httpapi.Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

func TestPostPrompts_ExplicitSessionMaterializesAbsentAndReopensEnded(t *testing.T) {
	for _, state := range []string{"absent", "ended"} {
		t.Run(state, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hive.db")
			store, err := db.Open(path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			const directory = "/work/alpha"
			require.NoError(t, store.CreateSession("seed", "alpha", directory, "dev", "seed"))
			if state == "ended" {
				require.NoError(t, store.CreateSession("capture", "alpha", directory, "dev", "seed"))
				require.NoError(t, store.EndSession("capture", "done"))
			}
			srv := httpapi.NewServerWithAll("127.0.0.1:0", store, store, nil, nil, nil, store)
			rr := postPromptMaterialization(srv, `{"content":"capture","project":"alpha","directory":"/work/alpha","session_id":"capture","client":"http-test"}`)
			require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
			check, err := sql.Open("sqlite", path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = check.Close() })
			var endedAt sql.NullString
			var prompts int
			require.NoError(t, check.QueryRow(`SELECT ended_at FROM sessions WHERE id = 'capture'`).Scan(&endedAt))
			require.NoError(t, check.QueryRow(`SELECT COUNT(*) FROM user_prompts WHERE session_id = 'capture'`).Scan(&prompts))
			require.False(t, endedAt.Valid)
			require.Equal(t, 1, prompts)
		})
	}
}

func TestPostPrompts_ExplicitSessionUsesAtomicCanonicalInputAndMapsStoreValidation(t *testing.T) {
	var got models.PromptWrite
	store := &mockPromptStore{savePromptWithSessionFn: func(_ context.Context, in models.PromptWrite) (*models.Prompt, error) {
		got = in
		return &models.Prompt{ID: 1, Project: in.Session.Project, SessionID: in.Session.ID, CreatedAt: time.Now()}, nil
	}}
	projects := mockProjectStore{known: []project.KnownProject{{Name: "alpha", Directory: "/work/alpha"}}}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", store, projects)
	require.Equal(t, http.StatusCreated, postPromptMaterialization(srv, `{"content":"capture","project":"alpha","directory":"/work/alpha","session_id":"capture","client":"caller"}`).Code)
	require.Equal(t, models.PromptWrite{Session: models.SessionInput{ID: "capture", Project: "alpha", Directory: "/work/alpha", Client: "caller"}, Content: "capture"}, got)

	store.savePromptWithSessionFn = func(context.Context, models.PromptWrite) (*models.Prompt, error) {
		return nil, &project.ValidationError{Code: project.CodeProjectSessionMismatch, Message: "mismatch"}
	}
	rr := postPromptMaterialization(srv, `{"content":"capture","project":"alpha","directory":"/work/alpha","session_id":"capture"}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	require.Equal(t, string(project.CodeProjectSessionMismatch), body["error_code"])
}

func TestPostPrompts_ExplicitSessionValidatesBeforeStoreAndDoesNotJoinStartFlight(t *testing.T) {
	called := false
	store := &mockPromptStore{savePromptWithSessionFn: func(_ context.Context, _ models.PromptWrite) (*models.Prompt, error) {
		called = true
		return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
	}}
	projects := mockProjectStore{known: []project.KnownProject{{Name: "alpha"}, {Name: "beta", Directory: "/work/beta"}}}
	srv := httpapi.NewServerWithProjectStore("127.0.0.1:0", store, projects)
	require.Equal(t, http.StatusBadRequest, postPromptMaterialization(srv, `{"content":"capture","project":"alpha","directory":"/work/beta","session_id":"capture"}`).Code)
	require.False(t, called)

	entered, release := make(chan struct{}), make(chan struct{})
	sessions := &mockSessionStore{ensureSessionFn: func(context.Context, models.SessionInput) (*models.Session, error) {
		close(entered)
		<-release
		return &models.Session{ID: "start", Project: "alpha"}, nil
	}}
	srv = httpapi.NewServerWithAll("127.0.0.1:0", store, projects, nil, nil, nil, sessions)
	go postJSON(srv, "/sessions", `{"id":"start","project":"alpha"}`)
	<-entered
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- postPromptMaterialization(srv, `{"content":"capture","project":"alpha","session_id":"start"}`)
	}()
	select {
	case rr := <-result:
		require.Equal(t, http.StatusCreated, rr.Code)
	case <-time.After(time.Second):
		t.Fatal("prompt capture waited for the start flight")
	}
	close(release)
}

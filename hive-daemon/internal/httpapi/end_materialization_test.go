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

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/require"
)

func postEndEvidence(srv *httpapi.Server, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+id+"/end", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

func TestPostSessionsEnd_MissingSessionMaterializesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	store, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.CreateSession("seed-alpha", "alpha", "", "dev", "test"))
	srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, store, nil, nil, nil, store)
	rr := postEndEvidence(srv, "missing", `{"project":"alpha","dev_id":"dev","client":"hook"}`)

	require.Equal(t, http.StatusOK, rr.Code)

	check, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = check.Close() })
	var endedAt sql.NullString
	require.NoError(t, check.QueryRow(`SELECT ended_at FROM sessions WHERE id = 'missing'`).Scan(&endedAt))
	require.True(t, endedAt.Valid)
}

func TestPostSessionsEnd_ActiveDuplicateAndFailureAreAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	store, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	check, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = check.Close() })
	require.NoError(t, store.CreateSession("seed-alpha", "alpha", "", "dev", "test"))
	srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, store, nil, nil, nil, store)
	body := `{"project":"alpha","dev_id":"dev","client":"hook"}`

	require.NoError(t, store.CreateSession("active", "alpha", "", "dev", "test"))
	require.Equal(t, http.StatusOK, postEndEvidence(srv, "active", body).Code)
	var endedAt sql.NullString
	require.NoError(t, check.QueryRow(`SELECT ended_at FROM sessions WHERE id = 'active'`).Scan(&endedAt))
	require.True(t, endedAt.Valid)

	require.NoError(t, store.CreateSession("duplicate", "alpha", "", "dev", "test"))
	_, err = check.Exec(`UPDATE sessions SET ended_at = CURRENT_TIMESTAMP, summary = 'prior summary' WHERE id = 'duplicate'`)
	require.NoError(t, err)
	var beforeEndedAt string
	require.NoError(t, check.QueryRow(`SELECT ended_at FROM sessions WHERE id = 'duplicate'`).Scan(&beforeEndedAt))
	require.Equal(t, http.StatusOK, postEndEvidence(srv, "duplicate", body).Code)
	var afterEndedAt, summary string
	require.NoError(t, check.QueryRow(`SELECT ended_at, summary FROM sessions WHERE id = 'duplicate'`).Scan(&afterEndedAt, &summary))
	require.Equal(t, beforeEndedAt, afterEndedAt)
	require.Equal(t, "prior summary", summary)

	_, err = check.Exec(`CREATE TRIGGER abort_end BEFORE UPDATE OF ended_at ON sessions WHEN NEW.id = 'triggered' BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, postEndEvidence(srv, "triggered", body).Code)
	var count int
	require.NoError(t, check.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'triggered'`).Scan(&count))
	require.Zero(t, count, "failed end must roll back materialization")
}

type endProjectStore struct {
	known    []project.KnownProject
	sessions map[string]string
}

func (s endProjectStore) KnownProjects(context.Context) ([]project.KnownProject, error) {
	return s.known, nil
}
func (s endProjectStore) SessionProject(_ context.Context, id string) (string, error) {
	if projectName, ok := s.sessions[id]; ok {
		return projectName, nil
	}
	return "", project.ErrSessionNotFound
}
func (endProjectStore) CreateRecoveryToken(context.Context, project.TokenRequest) (string, error) {
	return "", nil
}
func (endProjectStore) ValidateRecoveryToken(context.Context, project.TokenValidation) error {
	return nil
}
func (endProjectStore) ConsumeRecoveryToken(context.Context, project.TokenValidation) error {
	return nil
}
func (endProjectStore) ResolveAlias(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func TestPostSessionsEnd_ValidatesEvidenceAndMigrationGate(t *testing.T) {
	var calls int
	blocked := false
	sessions := &mockSessionStore{ensureAndEndSessionFn: func(_ context.Context, in models.SessionEndInput) (*models.Session, error) {
		calls++
		if blocked {
			return nil, db.ErrProjectBlocked
		}
		return &models.Session{ID: in.Session.ID, Project: in.Session.Project}, nil
	}}
	projects := endProjectStore{
		known:    []project.KnownProject{{Name: "alpha"}, {Name: "beta", Directory: "/beta"}},
		sessions: map[string]string{"mismatch": "beta"},
	}
	srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, projects, nil, nil, nil, sessions)
	for _, tt := range []struct {
		name, id, body, code string
	}{
		{"mismatch", "mismatch", `{"project":"alpha"}`, string(project.CodeProjectSessionMismatch)},
		{"invalid evidence", "new", `{"project":"alpha","directory":"/beta"}`, string(project.CodeProjectIdentityMismatch)},
		{"unresolved", "new", `{"project":"unknown"}`, string(project.CodeProjectUnknown)},
		{"no evidence", "new", `{}`, string(project.CodeProjectUnknown)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := postEndEvidence(srv, tt.id, tt.body)
			require.Equal(t, http.StatusBadRequest, rr.Code)
			var response map[string]any
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
			require.Equal(t, tt.code, response["error_code"])
		})
	}
	require.Zero(t, calls)
	emptyID := postEndEvidence(srv, "%20", `{}`)
	require.Equal(t, http.StatusBadRequest, emptyID.Code)
	var emptyResponse map[string]string
	require.NoError(t, json.NewDecoder(emptyID.Body).Decode(&emptyResponse))
	require.Equal(t, "id is required", emptyResponse["error"])

	blocked = true
	require.Equal(t, http.StatusLocked, postEndEvidence(srv, "blocked", `{"project":"alpha"}`).Code)
	srv.SetMigrationGate(project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateBlocked}))
	require.Equal(t, http.StatusServiceUnavailable, postEndEvidence(srv, "new", `{"project":"alpha"}`).Code)
}

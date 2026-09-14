package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/require"
)

func (m *mockSessionStore) SavePassiveObservationWithSession(ctx context.Context, in models.PassiveObservationWrite) error {
	return m.SavePassiveObservation(ctx, in.Session.ID, in.Session.Project, in.Source, in.Content)
}

type passiveObservationStore struct {
	legacy func(context.Context, string, string, string, string) error
	atomic func(context.Context, models.PassiveObservationWrite) error
}

func (s *passiveObservationStore) EnsureSession(context.Context, models.SessionInput) (*models.Session, error) {
	return nil, nil
}
func (s *passiveObservationStore) EnsureAndEndSession(context.Context, models.SessionEndInput) (*models.Session, error) {
	return nil, nil
}
func (s *passiveObservationStore) CreateSession(string, string, string, string, string) error {
	return nil
}
func (s *passiveObservationStore) EndSession(string, string) error { return nil }
func (s *passiveObservationStore) SavePassiveObservation(ctx context.Context, id, project, source, content string) error {
	return s.legacy(ctx, id, project, source, content)
}
func (s *passiveObservationStore) SavePassiveObservationWithSession(ctx context.Context, in models.PassiveObservationWrite) error {
	return s.atomic(ctx, in)
}

func TestPostPassiveObservation_ExplicitSessionUsesAtomicStore(t *testing.T) {
	var got models.PassiveObservationWrite
	store := &passiveObservationStore{
		legacy: func(context.Context, string, string, string, string) error { return errors.New("legacy path used") },
		atomic: func(_ context.Context, in models.PassiveObservationWrite) error { got = in; return nil },
	}
	srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, mockProjectStore{known: []project.KnownProject{{Name: "alpha"}}}, nil, nil, nil, store)
	rr := postJSON(srv, "/observations/passive", `{"session_id":"session","project":"alpha","directory":"/repo","source":"subagent","content":"capture"}`)
	require.Equal(t, http.StatusAccepted, rr.Code)
	require.Equal(t, models.PassiveObservationWrite{
		Session: models.SessionInput{ID: "session", Project: "alpha", Directory: "/repo", Client: "unknown"},
		Source:  "subagent",
		Content: "capture",
	}, got)
}

func TestPostPassiveObservation_MapsAtomicValidationAndGateErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{"mismatch", &project.ValidationError{Code: project.CodeProjectSessionMismatch}, http.StatusBadRequest},
		{"blocked", db.ErrProjectBlocked, http.StatusLocked},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &passiveObservationStore{
				legacy: func(context.Context, string, string, string, string) error { return errors.New("legacy path used") },
				atomic: func(context.Context, models.PassiveObservationWrite) error { return tt.err },
			}
			srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, mockProjectStore{known: []project.KnownProject{{Name: "alpha"}}}, nil, nil, nil, store)
			rr := postJSON(srv, "/observations/passive", `{"session_id":"session","project":"alpha","content":"capture"}`)
			require.Equal(t, tt.want, rr.Code)
		})
	}
}

func TestPostPassiveObservation_RejectsInvalidExplicitAttributionBeforeStore(t *testing.T) {
	called := false
	store := &passiveObservationStore{
		legacy: func(context.Context, string, string, string, string) error { called = true; return nil },
		atomic: func(context.Context, models.PassiveObservationWrite) error { called = true; return nil },
	}
	srv := httpapi.NewServerWithAll("127.0.0.1:0", &mockPromptStore{}, mockProjectStore{known: []project.KnownProject{{Name: "alpha"}}}, nil, nil, nil, store)
	rr := postJSON(srv, "/observations/passive", `{"session_id":"session","project":"missing","content":"capture"}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.False(t, called)
}

func TestPostPassiveObservation_EmptyAndNullSessionStayRaw(t *testing.T) {
	for _, body := range []string{
		`{"content":"capture"}`,
		`{"session_id":"","content":"capture"}`,
		`{"session_id":null,"content":"capture"}`,
	} {
		t.Run(body, func(t *testing.T) {
			atomic := false
			store := &passiveObservationStore{
				legacy: func(_ context.Context, id, _, _, _ string) error { require.Empty(t, id); return nil },
				atomic: func(context.Context, models.PassiveObservationWrite) error { atomic = true; return nil },
			}
			srv := httpapi.NewServerWithSessions("127.0.0.1:0", &mockPromptStore{}, store)
			rr := postJSON(srv, "/observations/passive", body)
			require.Equal(t, http.StatusAccepted, rr.Code)
			require.False(t, atomic)
		})
	}
}

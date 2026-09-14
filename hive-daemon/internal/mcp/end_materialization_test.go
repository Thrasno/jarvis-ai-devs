package mcp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

func TestMemSessionEnd_MissingSessionMaterializesAtomically(t *testing.T) {
	session, store := connectRealServer(t)
	directory := filepath.Join(t.TempDir(), "mcp-end")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	result := callTool(t, session, "mem_session_end", map[string]any{
		"id": "missing-end", "project": "mcp-end", "directory": directory,
		"dev_id": "developer", "summary": "finished",
	})
	if result.IsError {
		t.Fatalf("mem_session_end error: %s", textContent(t, result))
	}

	ended, err := store.GetSession("missing-end")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if ended.Project != "mcp-end" || ended.EndedAt == nil || ended.Summary != "finished" {
		t.Fatalf("materialized session = %#v, want ended mcp-end session with summary", ended)
	}

	duplicate := callTool(t, session, "mem_session_end", map[string]any{"id": "missing-end", "project": "mcp-end", "summary": "replacement"})
	if !duplicate.IsError || !strings.Contains(textContent(t, duplicate), hivedb.ErrSessionAlreadyEnded.Error()) {
		t.Fatalf("duplicate result = error:%v body:%s", duplicate.IsError, textContent(t, duplicate))
	}
	ended, err = store.GetSession("missing-end")
	if err != nil || ended.Summary != "finished" {
		t.Fatalf("duplicate changed stored summary: session=%#v err=%v", ended, err)
	}
}

func TestMemSessionEnd_ActiveSessionEndsAtomically(t *testing.T) {
	session, store := connectRealServer(t)
	directory := filepath.Join(t.TempDir(), "active-end")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureSession(context.Background(), models.SessionInput{ID: "active-end", Project: "active-end", Directory: directory, DevID: "developer", Client: "mcp"}); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	result := callTool(t, session, "mem_session_end", map[string]any{"id": "active-end", "project": "active-end", "directory": directory, "summary": "finished"})
	if result.IsError {
		t.Fatalf("mem_session_end error: %s", textContent(t, result))
	}
	ended, err := store.GetSession("active-end")
	if err != nil || ended.EndedAt == nil || ended.Summary != "finished" {
		t.Fatalf("ended session = %#v err=%v", ended, err)
	}
}

func TestMemSessionEnd_UsesAtomicStoreAndRejectsDuplicate(t *testing.T) {
	var input models.SessionEndInput
	store := &mockStore{
		ensureAndEndSessionFn: func(_ context.Context, got models.SessionEndInput) (*models.Session, error) {
			input = got
			return &models.Session{ID: got.Session.ID, Project: got.Session.Project}, nil
		},
	}
	session := connectTestServer(t, store)

	result := callTool(t, session, "mem_session_end", map[string]any{
		"id": "atomic-end", "project": "proj", "summary": "finished",
	})
	if result.IsError {
		t.Fatalf("mem_session_end error: %s", textContent(t, result))
	}
	if input.Session.ID != "atomic-end" || input.Session.Project != "proj" || input.Session.Client != "mcp" || !input.RejectAlreadyEnded || input.Summary != "finished" {
		t.Fatalf("EnsureAndEndSession input = %#v", input)
	}

	store.ensureAndEndSessionFn = func(context.Context, models.SessionEndInput) (*models.Session, error) {
		return nil, hivedb.ErrSessionAlreadyEnded
	}
	result = callTool(t, session, "mem_session_end", map[string]any{"id": "atomic-end", "project": "proj"})
	if !result.IsError || !strings.Contains(textContent(t, result), hivedb.ErrSessionAlreadyEnded.Error()) {
		t.Fatalf("duplicate result = error:%v body:%s", result.IsError, textContent(t, result))
	}
}

func TestMemSessionEnd_RejectsUnresolvedOrMismatchedProjectEvidenceBeforeMaterialization(t *testing.T) {
	t.Run("unresolved directory evidence", func(t *testing.T) {
		called := false
		store := &mockStore{ensureAndEndSessionFn: func(context.Context, models.SessionEndInput) (*models.Session, error) {
			called = true
			return nil, nil
		}}
		result := callTool(t, connectTestServer(t, store), "mem_session_end", map[string]any{
			"id": "unresolved", "directory": filepath.Join(t.TempDir(), "missing"),
		})
		body := decodeJSONResponse(t, result)
		if !result.IsError || body["error_code"] != string(project.CodeProjectUnknown) || called {
			t.Fatalf("unresolved result = error:%v called:%v body:%#v", result.IsError, called, body)
		}
	})

	t.Run("canonical directory mismatch", func(t *testing.T) {
		session, store := connectRealServer(t)
		directory := t.TempDir()
		if _, err := store.EnsureSession(context.Background(), models.SessionInput{
			ID: "canonical-source", Project: "canonical-project", Directory: directory, DevID: "developer", Client: "mcp",
		}); err != nil {
			t.Fatalf("EnsureSession canonical source: %v", err)
		}

		result := callTool(t, session, "mem_session_end", map[string]any{
			"id": "mismatched-end", "project": "other-project", "directory": directory,
		})
		body := decodeJSONResponse(t, result)
		if !result.IsError || body["error_code"] != string(project.CodeProjectIdentityMismatch) {
			t.Fatalf("mismatch result = error:%v body:%#v", result.IsError, body)
		}
		if _, err := store.GetSession("mismatched-end"); !errors.Is(err, hivedb.ErrSessionNotFound) {
			t.Fatalf("mismatched project materialized a session: %v", err)
		}
	})
}

func TestMemSessionEnd_PreservesProjectValidationAndRollback(t *testing.T) {
	t.Run("no project evidence", func(t *testing.T) {
		called := false
		session := connectTestServer(t, &mockStore{ensureAndEndSessionFn: func(context.Context, models.SessionEndInput) (*models.Session, error) {
			called = true
			return nil, nil
		}})
		result := callTool(t, session, "mem_session_end", map[string]any{"id": "no-evidence"})
		if !result.IsError || !strings.Contains(textContent(t, result), "project") || called {
			t.Fatalf("no-evidence result = error:%v called:%v body:%s", result.IsError, called, textContent(t, result))
		}
	})

	t.Run("canonical mismatch", func(t *testing.T) {
		called := false
		store := &mockStore{
			knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
				return []project.KnownProject{{Name: "proj"}}, nil
			},
			ensureAndEndSessionFn: func(context.Context, models.SessionEndInput) (*models.Session, error) {
				called = true
				return nil, nil
			},
		}
		result := callTool(t, connectTestServer(t, store), "mem_session_end", map[string]any{"id": "mismatch", "project": "other"})
		body := decodeJSONResponse(t, result)
		if !result.IsError || body["error_code"] != string(project.CodeProjectUnknown) || called {
			t.Fatalf("mismatch result = error:%v called:%v body:%#v", result.IsError, called, body)
		}
	})

	t.Run("typed mismatch remains structured", func(t *testing.T) {
		store := &mockStore{ensureAndEndSessionFn: func(context.Context, models.SessionEndInput) (*models.Session, error) {
			return nil, &project.ValidationError{Code: project.CodeProjectSessionMismatch, Message: "session belongs to another project"}
		}}
		result := callTool(t, connectTestServer(t, store), "mem_session_end", map[string]any{"id": "mismatch", "project": "proj"})
		body := decodeJSONResponse(t, result)
		if !result.IsError || body["error_code"] != string(project.CodeProjectSessionMismatch) {
			t.Fatalf("mismatch result = %#v", body)
		}
	})

	t.Run("trigger failure rolls back missing session", func(t *testing.T) {
		session, store := connectRealServer(t)
		if _, err := store.RawDB().Exec(`CREATE TRIGGER abort_end BEFORE UPDATE OF ended_at ON sessions BEGIN SELECT RAISE(ABORT, 'end trigger failed'); END`); err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(t.TempDir(), "rollback-project")
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		result := callTool(t, session, "mem_session_end", map[string]any{"id": "rolled-back", "project": "rollback-project", "directory": directory})
		if !result.IsError || !strings.Contains(textContent(t, result), "end trigger failed") {
			t.Fatalf("trigger result = error:%v body:%s", result.IsError, textContent(t, result))
		}
		if _, err := store.GetSession("rolled-back"); !errors.Is(err, hivedb.ErrSessionNotFound) {
			t.Fatalf("GetSession after rollback = %v, want ErrSessionNotFound", err)
		}
	})
}

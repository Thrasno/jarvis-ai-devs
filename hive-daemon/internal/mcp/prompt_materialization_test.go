package mcp_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestMemSavePrompt_ExplicitSessionMaterializesAbsentAndReopensEnded(t *testing.T) {
	for _, state := range []string{"absent", "ended"} {
		t.Run(state, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hive.db")
			store, err := db.Open(path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			require.NoError(t, store.CreateSession("seed", "alpha", "/work/alpha", "dev", "seed"))
			if state == "ended" {
				require.NoError(t, store.CreateSession("capture", "alpha", "/work/alpha", "dev", "seed"))
				require.NoError(t, store.EndSession("capture", "done"))
			}
			result := callTool(t, connectTestServerWithConfigAndPrompts(t, store, nil, nil, store), "mem_save_prompt", map[string]any{"content": "capture", "project": "alpha", "directory": "/work/alpha", "session_id": "capture"})
			require.False(t, result.IsError, textContent(t, result))
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

func TestMemSavePrompt_ExplicitSessionUsesAtomicCanonicalInputAndMapsStoreValidation(t *testing.T) {
	var got models.PromptWrite
	store := &mockStore{savePromptWithSessionFn: func(_ context.Context, in models.PromptWrite) (*models.Prompt, error) {
		got = in
		return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
	}}
	session := connectWithPrompts(t, store)
	result := callTool(t, session, "mem_save_prompt", map[string]any{"content": "capture", "project": "proj", "directory": "/work/proj", "session_id": "capture"})
	require.False(t, result.IsError, textContent(t, result))
	require.Equal(t, models.PromptWrite{Session: models.SessionInput{ID: "capture", Project: "proj", Directory: "/work/proj", Client: "mcp"}, Content: "capture"}, got)

	store.savePromptWithSessionFn = func(context.Context, models.PromptWrite) (*models.Prompt, error) {
		return nil, &project.ValidationError{Code: project.CodeProjectSessionMismatch, Message: "mismatch"}
	}
	result = callTool(t, session, "mem_save_prompt", map[string]any{"content": "capture", "project": "proj", "session_id": "capture"})
	require.True(t, result.IsError)
	require.Equal(t, string(project.CodeProjectSessionMismatch), decodeJSONResponse(t, result)["error_code"])
}

func TestMemSavePrompt_ExplicitSessionValidatesBeforeStoreAndDoesNotJoinStartFlight(t *testing.T) {
	called := false
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "alpha"}, {Name: "beta", Directory: "/work/beta"}}, nil
		},
		savePromptWithSessionFn: func(context.Context, models.PromptWrite) (*models.Prompt, error) {
			called = true
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	startSession, promptSession := connectPromptClients(t, store)
	result := callTool(t, promptSession, "mem_save_prompt", map[string]any{"content": "capture", "project": "alpha", "directory": "/work/beta", "session_id": "capture"})
	require.True(t, result.IsError)
	require.False(t, called)

	entered, release := make(chan struct{}), make(chan struct{})
	store.knownProjectsFn = func(context.Context) ([]project.KnownProject, error) {
		return []project.KnownProject{{Name: "alpha"}}, nil
	}
	store.ensureSessionFn = func(context.Context, models.SessionInput) (*models.Session, error) {
		close(entered)
		<-release
		return &models.Session{ID: "start", Project: "alpha"}, nil
	}
	startDone := make(chan error, 1)
	go func() {
		_, err := startSession.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "mem_session_start", Arguments: map[string]any{"id": "start", "project": "alpha", "dev_id": "dev", "client": "caller"}})
		startDone <- err
	}()
	<-entered
	resultCh := make(chan *sdkmcp.CallToolResult, 1)
	go func() {
		result, _ := promptSession.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "mem_save_prompt", Arguments: map[string]any{"content": "capture", "project": "alpha", "session_id": "start"}})
		resultCh <- result
	}()
	select {
	case result := <-resultCh:
		require.NotNil(t, result)
		require.False(t, result.IsError)
	case <-time.After(time.Second):
		t.Fatal("prompt capture waited for the start flight")
	}
	close(release)
	require.NoError(t, <-startDone)
}

func connectPromptClients(t *testing.T, store *mockStore) (*sdkmcp.ClientSession, *sdkmcp.ClientSession) {
	t.Helper()
	server := hivemcp.NewServer(store, nil, nil, nil, store)
	connect := func() *sdkmcp.ClientSession {
		t1, t2 := sdkmcp.NewInMemoryTransports()
		_, err := server.Connect(context.Background(), t1, nil)
		require.NoError(t, err)
		client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1"}, nil)
		session, err := client.Connect(context.Background(), t2, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	return connect(), connect()
}

package mcp_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestMCPExplicitMemoryCapturesMaterializeAndReopen(t *testing.T) {
	for _, tool := range []string{"mem_save", "mem_session_summary"} {
		for _, state := range []string{"absent", "ended"} {
			t.Run(tool+"/"+state, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "hive.db")
				store, err := db.Open(path)
				require.NoError(t, err)
				t.Cleanup(func() { _ = store.Close() })
				require.NoError(t, store.CreateSession("seed", "alpha", "/work/alpha", "dev", "seed"))
				if state == "ended" {
					require.NoError(t, store.CreateSession("capture", "alpha", "/work/alpha", "dev", "seed"))
					require.NoError(t, store.EndSession("capture", "prior end"))
				}
				args := map[string]any{"project": "alpha", "directory": "/work/alpha", "session_id": "capture"}
				if tool == "mem_save" {
					args["title"], args["content"], args["type"] = "capture", "content", "decision"
				} else {
					args["content"] = "## Goal\ncapture"
				}
				result := callTool(t, connectTestServer(t, store), tool, args)
				require.False(t, result.IsError, textContent(t, result))
				session, err := store.GetSession("capture")
				require.NoError(t, err)
				require.Nil(t, session.EndedAt)
				if state == "absent" {
					require.Equal(t, "mcp", session.Client)
				} else {
					require.Equal(t, "prior end", session.Summary)
				}
			})
		}
	}
}

func TestMCPExplicitMemoryCaptureMapsTransactionalErrors(t *testing.T) {
	for _, tool := range []string{"mem_save", "mem_session_summary"} {
		for _, tt := range []struct {
			name       string
			err        error
			structured bool
		}{
			{name: "validation", err: &project.ValidationError{Code: project.CodeProjectSessionMismatch, Message: "mismatch"}, structured: true},
			{name: "store", err: context.Canceled},
		} {
			t.Run(tool+"/"+tt.name, func(t *testing.T) {
				transactionCalled := false
				store := &mockStore{
					knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
						return []project.KnownProject{{Name: "alpha"}}, nil
					},
					saveMemoryWithSessionFn: func(context.Context, *models.Memory, models.SessionInput) (int64, error) {
						transactionCalled = true
						return 0, tt.err
					},
				}
				args := map[string]any{"project": "alpha", "session_id": "capture"}
				if tool == "mem_save" {
					args["title"], args["content"], args["type"] = "capture", "content", "decision"
				} else {
					args["content"] = "## Goal\ncapture"
				}

				result := callTool(t, connectTestServer(t, store), tool, args)

				require.True(t, transactionCalled, "transactional store must run after prevalidation")
				require.True(t, result.IsError)
				if tt.structured {
					require.Equal(t, string(project.CodeProjectSessionMismatch), decodeJSONResponse(t, result)["error_code"])
				} else {
					require.Contains(t, textContent(t, result), "save failed: context canceled")
				}
			})
		}
	}
}

func TestMCPExplicitMemoryCaptureDoesNotJoinStartFlight(t *testing.T) {
	for _, tool := range []string{"mem_save", "mem_session_summary"} {
		t.Run(tool, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			store := &mockStore{
				ensureSessionFn: func(context.Context, models.SessionInput) (*models.Session, error) {
					close(entered)
					<-release
					return &models.Session{ID: "start", Project: "alpha"}, nil
				},
				saveMemoryWithSessionFn: func(context.Context, *models.Memory, models.SessionInput) (int64, error) { return 1, nil },
				knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
					return []project.KnownProject{{Name: "alpha"}}, nil
				},
			}
			start, capture := connectPromptClients(t, store)
			startDone := make(chan error, 1)
			go func() {
				_, err := start.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: "mem_session_start", Arguments: map[string]any{"id": "start", "project": "alpha", "dev_id": "dev", "client": "caller"}})
				startDone <- err
			}()
			<-entered
			args := map[string]any{"project": "alpha", "session_id": "start"}
			if tool == "mem_save" {
				args["title"], args["content"], args["type"] = "capture", "content", "decision"
			} else {
				args["content"] = "## Goal\ncapture"
			}
			captured := make(chan *sdkmcp.CallToolResult, 1)
			go func() {
				result, _ := capture.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: tool, Arguments: args})
				captured <- result
			}()
			select {
			case result := <-captured:
				require.False(t, result.IsError, textContent(t, result))
			case <-time.After(time.Second):
				t.Fatalf("%s waited for the start flight", tool)
			}
			close(release)
			require.NoError(t, <-startDone)
		})
	}
}

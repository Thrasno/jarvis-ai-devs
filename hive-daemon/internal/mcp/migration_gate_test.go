package mcp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMigrationGateBlocksEveryMCPMemoryToolWithStructuredRecovery(t *testing.T) {
	gate := project.NewMigrationGate(project.MigrationStatus{
		State:        project.MigrationStateBlocked,
		Reason:       "duplicate canonical project",
		BackupID:     "backup-42",
		Continuation: "hive project identity status",
	})
	session := connectMigrationGateServer(t, gate)

	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"mem_session_start", map[string]any{"id": "session-1", "project": "alpha", "directory": "/repo", "dev_id": "dev", "client": "test"}},
		{"mem_session_end", map[string]any{"id": "session-1"}},
		{"mem_save", map[string]any{"title": "title", "content": "content", "type": "discovery", "project": "alpha"}},
		{"mem_suggest_topic_key", map[string]any{"title": "title", "type": "discovery"}},
		{"mem_search", map[string]any{"query": "query"}},
		{"mem_get_observation", map[string]any{"id": 1}},
		{"mem_session_summary", map[string]any{"content": "summary", "project": "alpha"}},
		{"mem_context", map[string]any{}},
		{"mem_sync", map[string]any{"project": "alpha"}},
		{"mem_save_prompt", map[string]any{"content": "prompt", "project": "alpha"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := callTool(t, session, tt.name, tt.args)
			if !res.IsError {
				t.Fatalf("%s unexpectedly succeeded", tt.name)
			}
			body := decodeJSONResponse(t, res)
			if body["state"] != project.MigrationStateBlocked || body["reason"] != "duplicate canonical project" || body["backup_id"] != "backup-42" || body["continuation"] != "hive project identity status" {
				t.Fatalf("blocked response = %#v", body)
			}
		})
	}
}

// TestMigrationGatePendingOperatorReviewBlocksMCPToolsWithTheTUIContinuation
// keeps the MCP surface gated while the operator has not decided yet, and sends
// the assistant's own error text to the wizard that can decide — the CLI status
// command cannot resolve anything.
func TestMigrationGatePendingOperatorReviewBlocksMCPToolsWithTheTUIContinuation(t *testing.T) {
	gate := project.NewMigrationGate(project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		Reason:          "two spellings of one project",
		PlanFingerprint: "fingerprint-1",
	})
	session := connectMigrationGateServer(t, gate)
	res := callTool(t, session, "mem_search", map[string]any{"query": "query"})
	if !res.IsError {
		t.Fatal("mem_search succeeded while the migration was pending operator review")
	}
	body := decodeJSONResponse(t, res)
	if body["state"] != project.MigrationStatePendingOperatorReview {
		t.Fatalf("state = %v, want %q", body["state"], project.MigrationStatePendingOperatorReview)
	}
	if body["continuation"] != project.MigrationPendingOperatorContinuation {
		t.Fatalf("continuation = %v, want %q", body["continuation"], project.MigrationPendingOperatorContinuation)
	}
	if body["plan_fingerprint"] != "fingerprint-1" {
		t.Fatalf("plan fingerprint = %v, want it surfaced to the assistant", body["plan_fingerprint"])
	}
	if _, present := body["backup_id"]; present {
		t.Fatalf("backup id present in %#v, want it absent when nothing was backed up", body)
	}
}

func TestMemSessionStart_ConcurrentValidCallsShareOneEnsureAndExcludeInvalidOrBlocked(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls int
	var mu sync.Mutex
	store := &mockStore{ensureSessionFn: func(ctx context.Context, in models.SessionInput) (*models.Session, error) {
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
	session := connectMigrationGateServerWithStore(t, store, gate)
	args := map[string]any{"id": "session-1", "project": "proj", "directory": "/repo", "dev_id": "dev", "client": "caller"}
	results := make(chan *sdkmcp.CallToolResult, 2)
	go func() { results <- callTool(t, session, "mem_session_start", args) }()
	select {
	case <-entered:
	case result := <-results:
		t.Fatalf("start bypassed EnsureSession: %#v", result)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("start did not reach EnsureSession")
	}
	go func() { results <- callTool(t, session, "mem_session_start", args) }()
	select {
	case result := <-results:
		t.Fatalf("start completed before release: %#v", result)
	case <-time.After(100 * time.Millisecond):
	}
	if result := callTool(t, session, "mem_session_start", map[string]any{"project": "proj", "directory": "/repo", "dev_id": "dev", "client": "caller"}); !result.IsError {
		t.Fatal("missing ID start unexpectedly succeeded")
	}
	gate.Adopt(project.MigrationStatus{State: project.MigrationStateBlocked})
	if result := callTool(t, session, "mem_session_start", args); !result.IsError {
		t.Fatal("blocked start unexpectedly succeeded")
	}
	close(release)
	for range 2 {
		if result := <-results; result.IsError {
			t.Fatalf("valid start failed: %s", textContent(t, result))
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("EnsureSession calls = %d, want 1", calls)
	}
}

func TestMigrationGateAllowsMCPMemoryToolsWhenReady(t *testing.T) {
	session := connectMigrationGateServer(t, project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady}))
	res := callTool(t, session, "mem_suggest_topic_key", map[string]any{"title": "Gate wiring", "type": "discovery"})
	if res.IsError {
		t.Fatalf("ready gate returned error: %s", textContent(t, res))
	}
}

func connectMigrationGateServer(t *testing.T, gate *project.MigrationGate) *sdkmcp.ClientSession {
	t.Helper()
	return connectMigrationGateServerWithStore(t, &mockStore{}, gate)
}

func connectMigrationGateServerWithStore(t *testing.T, store hivemcp.MemoryStore, gate *project.MigrationGate) *sdkmcp.ClientSession {
	t.Helper()
	server := hivemcp.NewServerWithMigrationGate(store, nil, nil, nil, &mockStore{}, gate)
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

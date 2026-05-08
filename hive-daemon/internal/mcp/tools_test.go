package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	hivemcp "github.com/Thrasno/jarvis-dev/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	hivesync "github.com/Thrasno/jarvis-dev/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type scriptedSyncer struct {
	mu      sync.Mutex
	result  *hivesync.Result
	err     error
	project string
	calls   int
}

func (s *scriptedSyncer) Sync(context.Context, string) (*hivesync.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.result != nil {
		return s.result, s.err
	}
	return &hivesync.Result{Project: s.project}, s.err
}

func (s *scriptedSyncer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func callTool(t *testing.T, session *sdkmcp.ClientSession, name string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%q) returned unexpected error: %v", name, err)
	}
	return res
}

func textContent(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("expected at least one content item, got none")
	}
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected *TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func decodeJSONResponse(t *testing.T, res *sdkmcp.CallToolResult) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(textContent(t, res)), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	return body
}

// ─── mem_save ──────────────────────────────────────────────────────────────

func TestMemSave_ValidParams_CallsSaveMemory(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 42, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Auth Design",
		"content": "JWT-based authentication",
		"type":    "architecture",
		"project": "jarvis-dev",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.Title != "Auth Design" {
		t.Errorf("Title = %q, want 'Auth Design'", saved.Title)
	}
	if saved.Category != "architecture" {
		t.Errorf("Category = %q, want 'architecture'", saved.Category)
	}
	if saved.Project != "jarvis-dev" {
		t.Errorf("Project = %q, want 'jarvis-dev'", saved.Project)
	}

	// Response should include the ID
	body := textContent(t, res)
	var resp map[string]any
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", err, body)
	}
	if resp["id"] == nil {
		t.Error("response should contain 'id' field")
	}
}

func TestMemSave_WithTopicKey_PassesItToStore(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	callTool(t, session, "mem_save", map[string]any{
		"title":     "Spec",
		"content":   "content",
		"type":      "architecture",
		"project":   "proj",
		"topic_key": "sdd/hive/spec",
	})

	if saved.TopicKey == nil || *saved.TopicKey != "sdd/hive/spec" {
		t.Errorf("TopicKey = %v, want sdd/hive/spec", saved.TopicKey)
	}
}

func TestMemSave_MissingTitle_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_save", map[string]any{
		"content": "content",
		"type":    "architecture",
		"project": "proj",
		// no title
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing title")
	}
}

func TestMemSave_MissingProject_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "title",
		"content": "content",
		"type":    "architecture",
		// no project
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing project")
	}
}

func TestMemSave_StoreError_ReturnsError(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(*models.Memory) (int64, error) {
			return 0, errors.New("db failure")
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "title",
		"content": "content",
		"type":    "architecture",
		"project": "proj",
	})

	if !res.IsError {
		t.Error("expected IsError=true on store failure")
	}
}

func TestMemSave_ContentTooLong_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	// 50001 runes — one over the limit
	content := strings.Repeat("a", 50001)
	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "title",
		"content": content,
		"type":    "architecture",
		"project": "proj",
	})

	if !res.IsError {
		t.Error("expected IsError=true for content exceeding 50000 runes")
	}
	body := textContent(t, res)
	if !strings.Contains(body, "50001 runes (max 50000)") {
		t.Errorf("error should mention rune count, got: %s", body)
	}
}

func TestMemSave_ContentAtLimit_IsAccepted(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	// Exactly 50000 runes — at the boundary, should be accepted
	content := strings.Repeat("x", 50000)
	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "title",
		"content": content,
		"type":    "architecture",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("content at exactly 50000 runes should be accepted, got error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Error("SaveMemory should have been called for content at limit")
	}
}

// ─── Auto-Sync Tests ───────────────────────────────────────────────────────

func TestMemSave_WithAutoSyncDisabled_DoesNotCallSync(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 1, nil
		},
	}
	syncer := &mockSyncer{}
	cfg := &hivesync.Config{
		APIURL:   "https://test.com",
		Email:    "test@test.com",
		Password: "pass",
		AutoSync: false, // Auto-sync DISABLED
	}

	session := connectTestServerWithSync(t, store, cfg, syncer)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Test Memory",
		"content": "content",
		"type":    "architecture",
		"project": "test-proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	// Wait a bit to ensure no background goroutine was spawned
	time.Sleep(100 * time.Millisecond)

	if syncer.callCount() != 0 {
		t.Errorf("syncer.Sync should NOT have been called when AutoSync=false, got %d calls", syncer.callCount())
	}
}

func TestMemSave_WithAutoSyncEnabled_CallsSyncInBackground(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 1, nil
		},
	}
	syncer := &mockSyncer{}
	cfg := &hivesync.Config{
		APIURL:   "https://test.com",
		Email:    "test@test.com",
		Password: "pass",
		AutoSync: true, // Auto-sync ENABLED
	}

	session := connectTestServerWithSync(t, store, cfg, syncer)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Test Memory",
		"content": "content",
		"type":    "architecture",
		"project": "test-proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	// Wait for background goroutine to complete (with reasonable timeout)
	time.Sleep(200 * time.Millisecond)

	if syncer.callCount() != 1 {
		t.Errorf("syncer.Sync should have been called exactly once when AutoSync=true, got %d calls", syncer.callCount())
	}

	if syncer.lastProject() != "test-proj" {
		t.Errorf("syncer.Sync called with project=%q, want %q", syncer.lastProject(), "test-proj")
	}
}

func TestMemSave_WithAutoSyncEnabled_ReturnsImmediately(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 1, nil
		},
	}
	// Slow syncer that takes 1 second to complete
	slowSyncer := &mockSyncer{}
	cfg := &hivesync.Config{
		APIURL:   "https://test.com",
		Email:    "test@test.com",
		Password: "pass",
		AutoSync: true,
	}

	session := connectTestServerWithSync(t, store, cfg, slowSyncer)

	start := time.Now()
	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Test Memory",
		"content": "content",
		"type":    "architecture",
		"project": "test-proj",
	})
	elapsed := time.Since(start)

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	// Should return in less than 50ms (fire-and-forget)
	if elapsed > 50*time.Millisecond {
		t.Errorf("mem_save with auto-sync should return immediately, took %v", elapsed)
	}
}

func TestMemSave_WithAutoSyncEnabled_ButNilSyncer_DoesNotCrash(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 1, nil
		},
	}
	cfg := &hivesync.Config{
		APIURL:   "https://test.com",
		Email:    "test@test.com",
		Password: "pass",
		AutoSync: true, // AutoSync enabled but syncer is nil
	}

	session := connectTestServerWithSync(t, store, cfg, nil) // nil syncer

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Test Memory",
		"content": "content",
		"type":    "architecture",
		"project": "test-proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	// Should not crash — the nil check should prevent goroutine spawn
	time.Sleep(50 * time.Millisecond)
}

func TestMemSave_WithNilConfig_DoesNotCallSync(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 1, nil
		},
	}
	syncer := &mockSyncer{}

	// nil config — AutoSync should be treated as disabled
	session := connectTestServerWithSync(t, store, nil, syncer)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Test Memory",
		"content": "content",
		"type":    "architecture",
		"project": "test-proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	time.Sleep(100 * time.Millisecond)

	if syncer.callCount() != 0 {
		t.Errorf("syncer.Sync should NOT have been called when cfg is nil, got %d calls", syncer.callCount())
	}
}

func TestMemSave_WithAutoSyncEnabled_SwallowsTypedBlockersQuietly(t *testing.T) {
	t.Parallel()

	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 1, nil
		},
	}
	cases := []struct {
		name   string
		syncer *scriptedSyncer
	}{
		{
			name:   "in flight skip",
			syncer: &scriptedSyncer{err: fmt.Errorf("skip: %w", hivesync.ErrSyncInFlight)},
		},
		{
			name: "backoff skip",
			syncer: &scriptedSyncer{err: &hivesync.BackoffError{
				Project: "test-proj",
				RetryAt: time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC),
			}},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &hivesync.Config{AutoSync: true}
			session := connectTestServerWithSync(t, store, cfg, tt.syncer)

			res := callTool(t, session, "mem_save", map[string]any{
				"title":   "Test Memory",
				"content": "content",
				"type":    "architecture",
				"project": "test-proj",
			})

			if res.IsError {
				t.Fatalf("expected quiet autosync skip, got error: %s", textContent(t, res))
			}

			time.Sleep(50 * time.Millisecond)

			if got := tt.syncer.callCount(); got != 1 {
				t.Fatalf("syncer.Sync call count = %d, want 1", got)
			}
		})
	}
}

func TestMemSync_ReturnsStructuredStatuses(t *testing.T) {
	t.Parallel()

	retryAt := time.Date(2026, time.May, 8, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name       string
		syncer     *scriptedSyncer
		wantError  bool
		wantStatus string
		wantRetry  string
	}{
		{
			name: "ok",
			syncer: &scriptedSyncer{result: &hivesync.Result{
				Pushed:    2,
				Pulled:    1,
				Conflicts: 0,
				Project:   "test-proj",
			}},
			wantStatus: "ok",
		},
		{
			name:       "in flight",
			syncer:     &scriptedSyncer{err: fmt.Errorf("blocked: %w", hivesync.ErrSyncInFlight)},
			wantStatus: "in_flight",
		},
		{
			name: "backoff",
			syncer: &scriptedSyncer{err: &hivesync.BackoffError{
				Project: "test-proj",
				RetryAt: retryAt,
			}},
			wantStatus: "backoff",
			wantRetry:  retryAt.Format(time.RFC3339),
		},
		{
			name:      "sync failure",
			syncer:    &scriptedSyncer{err: errors.New("boom")},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := connectTestServerWithSync(t, &mockStore{}, nil, tt.syncer)

			res := callTool(t, session, "mem_sync", map[string]any{"project": "test-proj"})

			if res.IsError != tt.wantError {
				t.Fatalf("IsError = %v, want %v; body=%s", res.IsError, tt.wantError, textContent(t, res))
			}

			if tt.wantError {
				return
			}

			body := decodeJSONResponse(t, res)
			if got := body["status"]; got != tt.wantStatus {
				t.Fatalf("status = %v, want %q", got, tt.wantStatus)
			}
			if got := body["project"]; got != "test-proj" {
				t.Fatalf("project = %v, want test-proj", got)
			}
			if tt.wantRetry != "" {
				if got := body["retry_at"]; got != tt.wantRetry {
					t.Fatalf("retry_at = %v, want %q", got, tt.wantRetry)
				}
			} else if _, ok := body["retry_at"]; ok {
				t.Fatalf("retry_at should be omitted, got %v", body["retry_at"])
			}
		})
	}
}

// ─── mem_search ────────────────────────────────────────────────────────────

func TestMemSearch_CallsSearch_ReturnsResults(t *testing.T) {
	store := &mockStore{
		searchFn: func(query, project, category string, limit int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "Auth Design", Content: "jwt", Project: project},
			}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_search", map[string]any{
		"query":   "auth",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	body := textContent(t, res)
	// Response is now markdown, not JSON
	if !strings.Contains(body, "### [1]") {
		t.Errorf("response should contain markdown header with ID, got: %s", body)
	}
	if !strings.Contains(body, "Auth Design") {
		t.Errorf("response should contain the memory title, got: %s", body)
	}
	if !strings.Contains(body, "results for") {
		t.Errorf("response should contain the footer with result count, got: %s", body)
	}
}

func TestMemSearch_DefaultLimit_Is10(t *testing.T) {
	var gotLimit int
	store := &mockStore{
		searchFn: func(_, _, _ string, limit int) ([]*models.Memory, error) {
			gotLimit = limit
			return nil, nil
		},
	}
	session := connectTestServer(t, store)

	callTool(t, session, "mem_search", map[string]any{"query": "anything"})

	if gotLimit != 10 {
		t.Errorf("default limit = %d, want 10", gotLimit)
	}
}

func TestMemSearch_ProjectFilter_PassedToStore(t *testing.T) {
	var gotProject string
	store := &mockStore{
		searchFn: func(_, project, _ string, _ int) ([]*models.Memory, error) {
			gotProject = project
			return nil, nil
		},
	}
	session := connectTestServer(t, store)

	callTool(t, session, "mem_search", map[string]any{
		"query":   "auth",
		"project": "my-project",
	})

	if gotProject != "my-project" {
		t.Errorf("project = %q, want 'my-project'", gotProject)
	}
}

func TestMemSearch_FiltersByCategory(t *testing.T) {
	var gotCategory string
	store := &mockStore{
		searchFn: func(_, _, category string, _ int) ([]*models.Memory, error) {
			gotCategory = category
			return []*models.Memory{
				{ID: 1, Title: "Arch Note", Content: "content", Project: "proj", Category: "architecture"},
			}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_search", map[string]any{
		"query":   "design",
		"project": "proj",
		"type":    "architecture",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if gotCategory != "architecture" {
		t.Errorf("category passed to store = %q, want 'architecture'", gotCategory)
	}
}

func TestMemSearch_NoResults_ReturnsNoResultsMessage(t *testing.T) {
	store := &mockStore{
		searchFn: func(_, _, _ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_search", map[string]any{
		"query": "nonexistent topic xyz",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if !strings.Contains(body, "No results found") {
		t.Errorf("expected no-results message, got: %s", body)
	}
}

// ─── mem_get_observation ───────────────────────────────────────────────────

func TestMemGetObservation_ValidID_ReturnsMemory(t *testing.T) {
	store := &mockStore{
		getMemoryFn: func(id int64) (*models.Memory, error) {
			return &models.Memory{ID: id, Title: "Found", Content: "content", Project: "proj"}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_get_observation", map[string]any{"id": 42})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	var mem models.Memory
	if err := json.Unmarshal([]byte(textContent(t, res)), &mem); err != nil {
		t.Fatalf("response not valid Memory JSON: %v", err)
	}
	if mem.Title != "Found" {
		t.Errorf("Title = %q, want 'Found'", mem.Title)
	}
}

func TestMemGetObservation_NotFound_ReturnsError(t *testing.T) {
	store := &mockStore{
		getMemoryFn: func(int64) (*models.Memory, error) {
			return nil, errors.New("memory not found: id=999")
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_get_observation", map[string]any{"id": 999})

	if !res.IsError {
		t.Error("expected IsError=true for not-found memory")
	}
}

func TestMemGetObservation_MissingID_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_get_observation", map[string]any{})

	if !res.IsError {
		t.Error("expected IsError=true for missing id")
	}
}

// ─── mem_session_summary ───────────────────────────────────────────────────

func TestMemSessionSummary_CreatesMemoryWithCorrectType(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 10, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": "## Goal\nImplement hive-daemon\n\n## Done\n- Phase 1",
		"project": "jarvis-dev",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.Category != "session_summary" {
		t.Errorf("Category = %q, want 'session_summary'", saved.Category)
	}
	if saved.Project != "jarvis-dev" {
		t.Errorf("Project = %q, want 'jarvis-dev'", saved.Project)
	}
	if saved.Title == "" {
		t.Error("Title should be extracted from content")
	}
}

func TestMemSessionSummary_MissingContent_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"project": "proj",
		// no content
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing content")
	}
}

func TestMemSessionSummary_ContentTooLong_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	// 60000 runes — well over the limit
	content := strings.Repeat("x", 60000)
	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": content,
		"project": "proj",
	})

	if !res.IsError {
		t.Error("expected IsError=true for content exceeding 50000 runes")
	}
	body := textContent(t, res)
	if !strings.Contains(body, "60000 runes (max 50000)") {
		t.Errorf("error should mention rune count, got: %s", body)
	}
}

// ─── mem_save_prompt ──────────────────────────────────────────────────────

// connectWithPrompts creates a server+client pair using the given PromptStore.
func connectWithPrompts(t *testing.T, ps *mockStore) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := hivemcp.NewServer(&mockStore{}, nil, nil, nil, ps)
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TS-MCP-1: valid content returns id and created_at
func TestMemSavePrompt_ValidContent_ReturnsIDAndCreatedAt(t *testing.T) {
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, content string) (*models.Prompt, error) {
			return &models.Prompt{ID: 7, Content: content, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{"content": "explain goroutines", "project": "jarvis-dev"})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	var resp map[string]any
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if id, ok := resp["id"]; !ok || id == nil {
		t.Error("response missing 'id'")
	}
	if ca, ok := resp["created_at"]; !ok || ca == nil {
		t.Error("response missing 'created_at'")
	}
}

// TS-MCP-2: empty content returns IsError=true, SavePrompt NOT called
func TestMemSavePrompt_EmptyContent_ReturnsError(t *testing.T) {
	called := false
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, _ string) (*models.Prompt, error) {
			called = true
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{"content": "", "project": "jarvis-dev"})

	if !res.IsError {
		t.Error("expected IsError=true for empty content")
	}
	if called {
		t.Error("SavePrompt should NOT be called for empty content")
	}
}

// TS-MCP-3: whitespace content returns IsError=true
func TestMemSavePrompt_WhitespaceContent_ReturnsError(t *testing.T) {
	session := connectWithPrompts(t, &mockStore{})

	res := callTool(t, session, "mem_save_prompt", map[string]any{"content": "\n\t  ", "project": "jarvis-dev"})

	if !res.IsError {
		t.Error("expected IsError=true for whitespace content")
	}
}

// TS-MCP-4: store error returns IsError=true with "save failed"
func TestMemSavePrompt_StoreError_ReturnsError(t *testing.T) {
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, _ string) (*models.Prompt, error) {
			return nil, errors.New("db connection closed")
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{"content": "valid content", "project": "jarvis-dev"})

	if !res.IsError {
		t.Error("expected IsError=true when store returns error")
	}
	body := textContent(t, res)
	if !strings.Contains(body, "save failed") {
		t.Errorf("error message should contain 'save failed', got: %s", body)
	}
}

// ─── mem_context ───────────────────────────────────────────────────────────

func TestMemContext_DefaultLimit_Is20(t *testing.T) {
	var gotLimit int
	store := &mockStore{
		listMemoriesFn: func(_ string, limit int) ([]*models.Memory, error) {
			gotLimit = limit
			return nil, nil
		},
	}
	session := connectTestServer(t, store)

	callTool(t, session, "mem_context", map[string]any{})

	if gotLimit != 20 {
		t.Errorf("default limit = %d, want 20", gotLimit)
	}
}

func TestMemContext_WithProject_PassedToStore(t *testing.T) {
	var gotProject string
	store := &mockStore{
		listMemoriesFn: func(project string, _ int) ([]*models.Memory, error) {
			gotProject = project
			return nil, nil
		},
	}
	session := connectTestServer(t, store)

	callTool(t, session, "mem_context", map[string]any{"project": "jarvis-dev"})

	if gotProject != "jarvis-dev" {
		t.Errorf("project = %q, want 'jarvis-dev'", gotProject)
	}
}

func TestMemContext_ReturnsFormattedMarkdown(t *testing.T) {
	store := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "Recent Memory", Project: "proj", Content: "c", Category: "decision"},
				{ID: 2, Title: "Older Memory", Project: "proj", Content: "c", Category: "bugfix"},
			}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)

	// Must be markdown, not JSON
	if strings.HasPrefix(strings.TrimSpace(body), "[") {
		t.Error("response should not be a JSON array")
	}
	if !strings.Contains(body, "### [") {
		t.Errorf("response should contain markdown headers, got: %s", body)
	}
	if !strings.Contains(body, "---") {
		t.Errorf("response should contain --- separators, got: %s", body)
	}
	if !strings.Contains(body, "memories shown") {
		t.Errorf("response should contain footer with count, got: %s", body)
	}
	// Both memories should be present
	if !strings.Contains(body, "Recent Memory") {
		t.Errorf("response should contain 'Recent Memory', got: %s", body)
	}
	if !strings.Contains(body, "Older Memory") {
		t.Errorf("response should contain 'Older Memory', got: %s", body)
	}
}

func TestMemContext_NoResults_ReturnsNoMemoriesMessage(t *testing.T) {
	store := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_context", map[string]any{"project": "empty-proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if !strings.Contains(body, "No memories found") {
		t.Errorf("expected no-memories message, got: %s", body)
	}
}

// connectTestServerFull creates a server+client pair with separate memory and prompt stores.
func connectTestServerFull(t *testing.T, mem hivemcp.MemoryStore, ps hivemcp.PromptStore) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := hivemcp.NewServer(mem, nil, nil, nil, ps)
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// ─── T-MCP-2: memContextHandler with recent prompts ───────────────────────

func TestMemContext_WithRecentPrompts_IncludesPromptsSection(t *testing.T) {
	memStore := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "A memory", Project: "proj", Content: "mem content", Category: "decision"},
			}, nil
		},
	}
	promptStore := &mockStore{
		listRecentPromptsFn: func(_ context.Context, _ string, _ int) ([]*models.Prompt, error) {
			return []*models.Prompt{
				{ID: 1, Content: "explain goroutines", CreatedAt: time.Now()},
			}, nil
		},
	}
	session := connectTestServerFull(t, memStore, promptStore)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if !strings.Contains(body, "### Recent User Prompts") {
		t.Errorf("response should contain '### Recent User Prompts', got: %s", body)
	}
	if !strings.Contains(body, "explain goroutines") {
		t.Errorf("response should contain prompt content, got: %s", body)
	}
}

func TestMemContext_EmptyPrompts_NoPromptsSection(t *testing.T) {
	memStore := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "A memory", Project: "proj", Content: "mem content", Category: "decision"},
			}, nil
		},
	}
	promptStore := &mockStore{
		listRecentPromptsFn: func(_ context.Context, _ string, _ int) ([]*models.Prompt, error) {
			return []*models.Prompt{}, nil
		},
	}
	session := connectTestServerFull(t, memStore, promptStore)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if strings.Contains(body, "### Recent User Prompts") {
		t.Errorf("response should NOT contain '### Recent User Prompts' when no prompts, got: %s", body)
	}
}

func TestMemContext_ListRecentPromptsError_ContinuesWithMemories(t *testing.T) {
	memStore := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "A memory", Project: "proj", Content: "mem content", Category: "decision"},
			}, nil
		},
	}
	promptStore := &mockStore{
		listRecentPromptsFn: func(_ context.Context, _ string, _ int) ([]*models.Prompt, error) {
			return nil, errors.New("db error")
		},
	}
	session := connectTestServerFull(t, memStore, promptStore)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("prompt store error should not fail the handler, got: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if !strings.Contains(body, "A memory") {
		t.Errorf("memories should still be present when prompts fail, got: %s", body)
	}
}

func TestMemContext_PromptsBeforeMemories(t *testing.T) {
	memStore := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "A memory", Project: "proj", Content: "mem content", Category: "decision"},
			}, nil
		},
	}
	promptStore := &mockStore{
		listRecentPromptsFn: func(_ context.Context, _ string, _ int) ([]*models.Prompt, error) {
			return []*models.Prompt{
				{ID: 1, Content: "user prompt content", CreatedAt: time.Now()},
			}, nil
		},
	}
	session := connectTestServerFull(t, memStore, promptStore)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)

	promptsIdx := strings.Index(body, "### Recent User Prompts")
	memoryIdx := strings.Index(body, "### [1]")
	if promptsIdx == -1 {
		t.Fatal("missing '### Recent User Prompts' section")
	}
	if memoryIdx == -1 {
		t.Fatal("missing memory section '### [1]'")
	}
	if promptsIdx >= memoryIdx {
		t.Errorf("prompts section (idx %d) should appear before memories (idx %d)", promptsIdx, memoryIdx)
	}
}

func TestMemContext_PassesMaxRecentPromptsAsLimit(t *testing.T) {
	var gotLimit int
	memStore := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{}, nil
		},
	}
	promptStore := &mockStore{
		listRecentPromptsFn: func(_ context.Context, _ string, limit int) ([]*models.Prompt, error) {
			gotLimit = limit
			return nil, nil
		},
	}
	session := connectTestServerFull(t, memStore, promptStore)

	callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if gotLimit != hivemcp.MaxRecentPrompts {
		t.Errorf("limit passed to ListRecentPrompts = %d, want MaxRecentPrompts (%d)", gotLimit, hivemcp.MaxRecentPrompts)
	}
}

func TestMemContext_PromptContentTruncatedAt200Runes(t *testing.T) {
	longContent := strings.Repeat("a", 250)
	memStore := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{}, nil
		},
	}
	promptStore := &mockStore{
		listRecentPromptsFn: func(_ context.Context, _ string, _ int) ([]*models.Prompt, error) {
			return []*models.Prompt{
				{ID: 1, Content: longContent, CreatedAt: time.Now()},
			}, nil
		},
	}
	session := connectTestServerFull(t, memStore, promptStore)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if !strings.Contains(body, "...") {
		t.Errorf("long prompt content should be truncated with '...', got: %s", body)
	}
	// The full 250-char content should NOT appear
	if strings.Contains(body, longContent) {
		t.Errorf("full 250-rune content should not appear in response, got: %s", body)
	}
}

// TS-MCP-5: nil store returns IsError=true immediately
func TestMemSavePrompt_NilStore_ReturnsError(t *testing.T) {
	ctx := context.Background()
	server := hivemcp.NewServer(&mockStore{}, nil, nil, nil, nil)
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res := callTool(t, session, "mem_save_prompt", map[string]any{"content": "any content", "project": "jarvis-dev"})

	if !res.IsError {
		t.Error("expected IsError=true when prompts store is nil")
	}
}

// TS-MCP-6: content exceeding 50000 runes returns error
func TestMemSavePrompt_ContentTooLong_ReturnsError(t *testing.T) {
	session := connectWithPrompts(t, &mockStore{})

	content := strings.Repeat("a", 50001)
	res := callTool(t, session, "mem_save_prompt", map[string]any{"content": content, "project": "jarvis-dev"})

	if !res.IsError {
		t.Error("expected IsError=true for content exceeding 50000 runes")
	}
	body := textContent(t, res)
	if !strings.Contains(body, "50001 runes (max 50000)") {
		t.Errorf("error should mention rune count, got: %s", body)
	}
}

// ─── T-MCP-3: mem_save_prompt with project ────────────────────────────────

func TestMemSavePrompt_WithProject_PassesProjectToStore(t *testing.T) {
	var gotProject string
	ps := &mockStore{
		savePromptFn: func(_ context.Context, project, _ string) (*models.Prompt, error) {
			gotProject = project
			return &models.Prompt{ID: 1, Project: project, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "explain goroutines",
		"project": "jarvis-dev",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if gotProject != "jarvis-dev" {
		t.Errorf("project passed to store = %q, want 'jarvis-dev'", gotProject)
	}
}

func TestMemSavePrompt_WhitespaceProject_ReturnsError(t *testing.T) {
	called := false
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, _ string) (*models.Prompt, error) {
			called = true
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "explain goroutines",
		"project": "   \t\n  ",
	})

	if !res.IsError {
		t.Error("expected IsError=true when project is whitespace-only")
	}
	if called {
		t.Error("SavePrompt should NOT be called when project is whitespace-only")
	}
}

func TestMemSavePrompt_WithoutProject_ReturnsError(t *testing.T) {
	called := false
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, _ string) (*models.Prompt, error) {
			called = true
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "explain goroutines",
		// no project
	})

	if !res.IsError {
		t.Error("expected IsError=true when project is omitted")
	}
	if called {
		t.Error("SavePrompt should NOT be called when project is missing")
	}
}

// ─── T-06: Private-tag stripping — handler integration tests ─────────────────

// T-06a-1: memSaveHandler — no private tags → stripped: false, stripped_count: 0
func TestMemSave_NoPrivateTags_ReturnsStrippedFalse(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 42, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Clean Title",
		"content": "No private tags here",
		"type":    "decision",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := decodeJSONResponse(t, res)
	if stripped, ok := body["stripped"]; !ok || stripped != false {
		t.Errorf("stripped = %v, want false", body["stripped"])
	}
	if count, ok := body["stripped_count"]; !ok || count != float64(0) {
		t.Errorf("stripped_count = %v, want 0", body["stripped_count"])
	}
}

// T-06a-2: memSaveHandler — private tag in content → stored stripped, response stripped: true, stripped_count: 1
func TestMemSave_PrivateTagInContent_StripsAndReturnsCount(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "My Note",
		"content": "token: <private>secret123</private> end",
		"type":    "decision",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.Content != "token: [REDACTED] end" {
		t.Errorf("stored content = %q, want %q", saved.Content, "token: [REDACTED] end")
	}
	body := decodeJSONResponse(t, res)
	if stripped := body["stripped"]; stripped != true {
		t.Errorf("stripped = %v, want true", stripped)
	}
	if count := body["stripped_count"]; count != float64(1) {
		t.Errorf("stripped_count = %v, want 1", count)
	}
}

// T-06a-3: memSaveHandler — private blocks in title (1) AND content (2) → stripped_count: 3
func TestMemSave_PrivateTagsInTitleAndContent_AggregatesCounts(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "title <private>X</private>",
		"content": "<private>A</private> and <private>B</private>",
		"type":    "decision",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.Title != "title [REDACTED]" {
		t.Errorf("stored title = %q, want %q", saved.Title, "title [REDACTED]")
	}
	if saved.Content != "[REDACTED] and [REDACTED]" {
		t.Errorf("stored content = %q, want %q", saved.Content, "[REDACTED] and [REDACTED]")
	}
	body := decodeJSONResponse(t, res)
	if stripped := body["stripped"]; stripped != true {
		t.Errorf("stripped = %v, want true", stripped)
	}
	if count := body["stripped_count"]; count != float64(3) {
		t.Errorf("stripped_count = %v, want 3", count)
	}
}

// T-06a-4: memSaveHandler — topic_key containing <private> → NOT stripped, stored verbatim
func TestMemSave_TopicKeyWithPrivateTag_NotStripped(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	topicKey := "sdd/<private>foo</private>/key"
	res := callTool(t, session, "mem_save", map[string]any{
		"title":     "title",
		"content":   "clean content",
		"type":      "decision",
		"project":   "proj",
		"topic_key": topicKey,
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.TopicKey == nil || *saved.TopicKey != topicKey {
		t.Errorf("stored topic_key = %v, want %q (must not be modified)", saved.TopicKey, topicKey)
	}
	// Content has no tags → stripped_count: 0
	body := decodeJSONResponse(t, res)
	if count := body["stripped_count"]; count != float64(0) {
		t.Errorf("stripped_count = %v, want 0 (topic_key must not be counted)", count)
	}
}

// T-06a-5: memSavePromptHandler — private tag in content → stripped on persistence, response has stripped/stripped_count
func TestMemSavePrompt_PrivateTagInContent_StripsAndReturnsCount(t *testing.T) {
	var savedContent string
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, content string) (*models.Prompt, error) {
			savedContent = content
			return &models.Prompt{ID: 5, Content: content, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "my token <private>tok123</private> here",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if savedContent != "my token [REDACTED] here" {
		t.Errorf("stored content = %q, want %q", savedContent, "my token [REDACTED] here")
	}
	body := decodeJSONResponse(t, res)
	if stripped := body["stripped"]; stripped != true {
		t.Errorf("stripped = %v, want true", stripped)
	}
	if count := body["stripped_count"]; count != float64(1) {
		t.Errorf("stripped_count = %v, want 1", count)
	}
}

// T-06a-6: memSavePromptHandler — no private tags → stripped: false, stripped_count: 0
func TestMemSavePrompt_NoPrivateTags_ReturnsStrippedFalse(t *testing.T) {
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, content string) (*models.Prompt, error) {
			return &models.Prompt{ID: 1, Content: content, CreatedAt: time.Now()}, nil
		},
	}
	session := connectWithPrompts(t, ps)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "plain content with no tags",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := decodeJSONResponse(t, res)
	if stripped := body["stripped"]; stripped != false {
		t.Errorf("stripped = %v, want false", stripped)
	}
	if count := body["stripped_count"]; count != float64(0) {
		t.Errorf("stripped_count = %v, want 0", count)
	}
}

// T-06a-7: memSessionSummaryHandler — private tag in content → stripped, response has stripped/stripped_count
func TestMemSessionSummary_PrivateTagInContent_StripsAndReturnsCount(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 10, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": "## Goal\nFixed <private>my-token</private> bug",
		"project": "jarvis-dev",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.Content != "## Goal\nFixed [REDACTED] bug" {
		t.Errorf("stored content = %q, want %q", saved.Content, "## Goal\nFixed [REDACTED] bug")
	}

	// The first content item should be parseable JSON containing stripped fields.
	raw := textContent(t, res)
	// The response is JSON + optional footer from SessionStats. Parse just the JSON prefix.
	var jsonPart string
	for i, c := range raw {
		if c == '\n' && i > 0 {
			jsonPart = raw[:i]
			break
		}
	}
	if jsonPart == "" {
		jsonPart = raw
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &body); err != nil {
		// Try the full text as JSON (no footer case)
		if err2 := json.Unmarshal([]byte(raw), &body); err2 != nil {
			t.Fatalf("response is not valid JSON: %v — raw: %s", err, raw)
		}
	}
	if stripped := body["stripped"]; stripped != true {
		t.Errorf("stripped = %v, want true", stripped)
	}
	if count := body["stripped_count"]; count != float64(1) {
		t.Errorf("stripped_count = %v, want 1", count)
	}
}

// T-06a-8: memSessionSummaryHandler — no private tags → stripped: false, stripped_count: 0 (always present)
func TestMemSessionSummary_NoPrivateTags_ReturnsStrippedFalse(t *testing.T) {
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			return 10, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": "## Goal\nImplemented feature",
		"project": "jarvis-dev",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	raw := textContent(t, res)
	var jsonPart string
	for i, c := range raw {
		if c == '\n' && i > 0 {
			jsonPart = raw[:i]
			break
		}
	}
	if jsonPart == "" {
		jsonPart = raw
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &body); err != nil {
		if err2 := json.Unmarshal([]byte(raw), &body); err2 != nil {
			t.Fatalf("response is not valid JSON: %v — raw: %s", err, raw)
		}
	}
	if stripped, ok := body["stripped"]; !ok || stripped != false {
		t.Errorf("stripped = %v, want false", body["stripped"])
	}
	if count, ok := body["stripped_count"]; !ok || count != float64(0) {
		t.Errorf("stripped_count = %v, want 0", body["stripped_count"])
	}
}

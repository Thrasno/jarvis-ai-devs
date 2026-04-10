package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

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

// ─── mem_search ────────────────────────────────────────────────────────────

func TestMemSearch_CallsSearch_ReturnsResults(t *testing.T) {
	store := &mockStore{
		searchFn: func(query, project string, limit int) ([]*models.Memory, error) {
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

	var results []any
	if err := json.Unmarshal([]byte(textContent(t, res)), &results); err != nil {
		t.Fatalf("response not valid JSON array: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestMemSearch_DefaultLimit_Is10(t *testing.T) {
	var gotLimit int
	store := &mockStore{
		searchFn: func(_, _ string, limit int) ([]*models.Memory, error) {
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
		searchFn: func(_, project string, _ int) ([]*models.Memory, error) {
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

func TestMemContext_ReturnsResultsAsJSON(t *testing.T) {
	store := &mockStore{
		listMemoriesFn: func(_ string, _ int) ([]*models.Memory, error) {
			return []*models.Memory{
				{ID: 1, Title: "Recent Memory", Project: "proj", Content: "c"},
				{ID: 2, Title: "Older Memory", Project: "proj", Content: "c"},
			}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_context", map[string]any{"project": "proj"})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	var results []any
	if err := json.Unmarshal([]byte(textContent(t, res)), &results); err != nil {
		t.Fatalf("response not valid JSON array: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

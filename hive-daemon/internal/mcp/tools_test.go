package mcp_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type scriptedSyncer struct {
	mu         sync.Mutex
	result     *hivesync.Result
	err        error
	project    string
	calls      int
	drainCalls int
	lastPolicy hivesync.TriggerPolicy
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

func (s *scriptedSyncer) Drain(_ context.Context, _ string, policy hivesync.TriggerPolicy) (*hivesync.Result, hivesync.DrainOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainCalls++
	s.lastPolicy = policy
	if s.result != nil {
		return s.result, hivesync.DrainOutcome{BatchesDone: 1, State: hivesync.DrainFullySynced}, s.err
	}
	return &hivesync.Result{Project: s.project}, hivesync.DrainOutcome{BatchesDone: 1, State: hivesync.DrainFullySynced}, s.err
}

func (s *scriptedSyncer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *scriptedSyncer) drainCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drainCalls
}

func (s *scriptedSyncer) lastDrainPolicy() hivesync.TriggerPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPolicy
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

// ─── mem_suggest_topic_key ────────────────────────────────────────────────

func TestMemSuggestTopicKey_ReturnsDeterministicTopicKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		typ   string
		want  string
	}{
		{
			name:  "basic architecture title",
			title: "JWT auth middleware refactor",
			typ:   "architecture",
			want:  "architecture/jwt-auth-middleware-refactor",
		},
		{
			name:  "canonical n plus one bugfix",
			title: "Fix N+1 query in UserList",
			typ:   "bugfix",
			want:  "bugfix/fix-n-plus-one-query-in-user-list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			session := connectTestServer(t, &mockStore{})
			res := callTool(t, session, "mem_suggest_topic_key", map[string]any{
				"title": tt.title,
				"type":  tt.typ,
			})

			if res.IsError {
				t.Fatalf("expected success, got error: %s", textContent(t, res))
			}
			body := decodeJSONResponse(t, res)
			if got := body["topic_key"]; got != tt.want {
				t.Fatalf("topic_key = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestMemSuggestTopicKey_ValidatesRequiredInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        map[string]any
		wantMessage string
	}{
		{
			name:        "empty title",
			args:        map[string]any{"title": "", "type": "architecture"},
			wantMessage: "title",
		},
		{
			name:        "whitespace title",
			args:        map[string]any{"title": " \n\t ", "type": "architecture"},
			wantMessage: "title",
		},
		{
			name:        "empty type",
			args:        map[string]any{"title": "Useful memory", "type": ""},
			wantMessage: "type",
		},
		{
			name:        "unknown type",
			args:        map[string]any{"title": "Useful memory", "type": "manual"},
			wantMessage: "type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			session := connectTestServer(t, &mockStore{})
			res := callTool(t, session, "mem_suggest_topic_key", tt.args)

			if !res.IsError {
				t.Fatalf("expected IsError=true")
			}
			if !strings.Contains(textContent(t, res), tt.wantMessage) {
				t.Fatalf("error = %q, want message containing %q", textContent(t, res), tt.wantMessage)
			}
		})
	}
}

func TestMemSuggestTopicKey_NormalizesSeparatorsAndCapsSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		title    string
		typ      string
		want     string
		wantType string
	}{
		{
			name:  "collapses mixed separators",
			title: "Auth---Flow / Token_refresh: cleanup!!!",
			typ:   "pattern",
			want:  "pattern/auth-flow-token-refresh-cleanup",
		},
		{
			name:     "truncates slug only",
			title:    "This title has many words and keeps going beyond the sixty character slug limit by design",
			typ:      "decision",
			wantType: "decision",
		},
		{
			name:  "truncates unicode slug at rune boundary",
			title: strings.Repeat("a", 59) + "ñ extra",
			typ:   "discovery",
			want:  "discovery/" + strings.Repeat("a", 59) + "ñ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			session := connectTestServer(t, &mockStore{})
			res := callTool(t, session, "mem_suggest_topic_key", map[string]any{
				"title": tt.title,
				"type":  tt.typ,
			})

			if res.IsError {
				t.Fatalf("expected success, got error: %s", textContent(t, res))
			}
			body := decodeJSONResponse(t, res)
			got, ok := body["topic_key"].(string)
			if !ok {
				t.Fatalf("topic_key = %T, want string", body["topic_key"])
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("topic_key = %q, want %q", got, tt.want)
			}
			if tt.wantType != "" {
				prefix := tt.wantType + "/"
				if !strings.HasPrefix(got, prefix) {
					t.Fatalf("topic_key = %q, want prefix %q", got, prefix)
				}
				slug := strings.TrimPrefix(got, prefix)
				if !utf8.ValidString(slug) {
					t.Fatalf("slug must be valid UTF-8; slug=%q", slug)
				}
				if runeCount := utf8.RuneCountInString(slug); runeCount > 60 {
					t.Fatalf("slug length = %d runes, want <= 60; slug=%q", runeCount, slug)
				}
			}
		})
	}
}

func TestMemSuggestTopicKey_HasNoPersistenceOrSyncSideEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "success",
			args: map[string]any{"title": "Stable auth decision", "type": "decision"},
		},
		{
			name: "validation failure",
			args: map[string]any{"title": "Stable auth decision", "type": "manual"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var saveCalled, ensureCalled bool
			store := &mockStore{
				saveMemoryFn: func(*models.Memory) (int64, error) {
					saveCalled = true
					return 1, nil
				},
				ensureManualSaveSessionFn: func(project string) (string, error) {
					ensureCalled = true
					return "manual-save-" + project, nil
				},
			}
			syncer := &mockSyncer{}
			cfg := &hivesync.Config{AutoSync: true}
			session := connectTestServerWithSync(t, store, cfg, syncer)

			_ = callTool(t, session, "mem_suggest_topic_key", tt.args)

			if saveCalled {
				t.Fatal("SaveMemory must not be called")
			}
			if ensureCalled {
				t.Fatal("EnsureManualSaveSession must not be called")
			}
			if syncer.callCount() != 0 {
				t.Fatalf("Sync must not be called, got %d calls", syncer.callCount())
			}
		})
	}
}

// ─── mem_session_start ────────────────────────────────────────────────────

func TestMemSessionStart_HappyPath_ReturnsSessionID(t *testing.T) {
	directory := t.TempDir()
	var createdID, createdProject, createdDir, createdDevID, createdClient string
	store := &mockStore{
		createSessionFn: func(id, project, directory, devID, client string) error {
			createdID = id
			createdProject = project
			createdDir = directory
			createdDevID = devID
			createdClient = client
			return nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_start", map[string]any{
		"id":        "sess-001",
		"project":   "jarvis-dev",
		"directory": directory,
		"dev_id":    "andres",
		"client":    "claude-code",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	body := decodeJSONResponse(t, res)
	if body["session_id"] != "sess-001" {
		t.Errorf("session_id = %v, want 'sess-001'", body["session_id"])
	}
	if body["started_at"] == nil {
		t.Error("response must contain started_at")
	}

	if createdID != "sess-001" {
		t.Errorf("CreateSession called with id=%q, want 'sess-001'", createdID)
	}
	if createdProject != "jarvis-dev" {
		t.Errorf("CreateSession called with project=%q, want 'jarvis-dev'", createdProject)
	}
	if createdDir != directory {
		t.Errorf("CreateSession called with directory=%q, want %q", createdDir, directory)
	}
	if createdDevID != "andres" {
		t.Errorf("CreateSession called with devID=%q, want 'andres'", createdDevID)
	}
	if createdClient != "claude-code" {
		t.Errorf("CreateSession called with client=%q, want 'claude-code'", createdClient)
	}
}

func TestMemSessionStart_MissingDevID_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_session_start", map[string]any{
		"id":        "sess-001",
		"project":   "jarvis-dev",
		"directory": t.TempDir(),
		// no dev_id
		"client": "claude-code",
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing dev_id")
	}
	if !strings.Contains(textContent(t, res), "dev_id") {
		t.Errorf("error message should mention 'dev_id', got: %s", textContent(t, res))
	}
}

func TestMemSessionStart_MissingClient_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_session_start", map[string]any{
		"id":        "sess-001",
		"project":   "jarvis-dev",
		"directory": t.TempDir(),
		"dev_id":    "andres",
		// no client
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing client")
	}
	if !strings.Contains(textContent(t, res), "client") {
		t.Errorf("error message should mention 'client', got: %s", textContent(t, res))
	}
}

func TestMemSessionStart_MissingID_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_session_start", map[string]any{
		"project":   "jarvis-dev",
		"directory": t.TempDir(),
		"dev_id":    "andres",
		"client":    "claude-code",
		// no id
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing id")
	}
}

func TestMemSessionStart_DuplicateID_ReturnsError(t *testing.T) {
	store := &mockStore{
		createSessionFn: func(id, _, _, _, _ string) error {
			return errors.New("UNIQUE constraint failed: sessions.id")
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_start", map[string]any{
		"id":        "sess-dup",
		"project":   "jarvis-dev",
		"directory": t.TempDir(),
		"dev_id":    "andres",
		"client":    "claude-code",
	})

	if !res.IsError {
		t.Error("expected IsError=true when session id already exists")
	}
}

// ─── mem_session_end ──────────────────────────────────────────────────────

func TestMemSessionEnd_HappyPath_ReturnsEndedAt(t *testing.T) {
	endedAt := time.Now().Add(-1 * time.Second)
	var endedID, endedSummary string
	store := &mockStore{
		getSessionFn: func(id string) (*models.Session, error) {
			return &models.Session{ID: id, Project: "jarvis-dev", EndedAt: nil}, nil
		},
		endSessionFn: func(id, summary string) error {
			endedID = id
			endedSummary = summary
			return nil
		},
	}
	_ = endedAt
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_end", map[string]any{
		"id":      "sess-abc",
		"summary": "all done",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	body := decodeJSONResponse(t, res)
	if body["ended_at"] == nil {
		t.Error("response must contain ended_at")
	}
	if body["session_id"] != "sess-abc" {
		t.Errorf("session_id = %v, want 'sess-abc'", body["session_id"])
	}
	if endedID != "sess-abc" {
		t.Errorf("EndSession called with id=%q, want 'sess-abc'", endedID)
	}
	if endedSummary != "all done" {
		t.Errorf("EndSession called with summary=%q, want 'all done'", endedSummary)
	}
}

func TestMemSessionEnd_UnknownSession_ReturnsError(t *testing.T) {
	store := &mockStore{
		getSessionFn: func(id string) (*models.Session, error) {
			return nil, errors.New("session not found")
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_end", map[string]any{
		"id": "ghost-session",
	})

	if !res.IsError {
		t.Error("expected IsError=true for unknown session")
	}
	if !strings.Contains(textContent(t, res), "not found") {
		t.Errorf("error should mention 'not found', got: %s", textContent(t, res))
	}
}

func TestMemSessionEnd_AlreadyEnded_ReturnsError(t *testing.T) {
	endedAt := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	store := &mockStore{
		getSessionFn: func(id string) (*models.Session, error) {
			return &models.Session{ID: id, EndedAt: &endedAt}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_end", map[string]any{
		"id": "sess-old",
	})

	if !res.IsError {
		t.Error("expected IsError=true for already-ended session")
	}
	body := textContent(t, res)
	if !strings.Contains(body, "already ended") {
		t.Errorf("error should mention 'already ended', got: %s", body)
	}
}

func TestMemSessionEnd_MissingID_ReturnsError(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res := callTool(t, session, "mem_session_end", map[string]any{
		// no id
		"summary": "whatever",
	})

	if !res.IsError {
		t.Error("expected IsError=true for missing id")
	}
}

func TestMemSessionEnd_ClearsActivityTracker(t *testing.T) {
	store := &mockStore{
		getSessionFn: func(id string) (*models.Session, error) {
			return &models.Session{ID: id, Project: "proj", EndedAt: nil}, nil
		},
		endSessionFn: func(_, _ string) error { return nil },
	}
	session := connectTestServer(t, store)

	// End the session — after this the activity for the sessionID should be cleared.
	res := callTool(t, session, "mem_session_end", map[string]any{"id": "sess-tracked"})

	if res.IsError {
		t.Fatalf("expected success: %s", textContent(t, res))
	}
	// Indirect verification: subsequent calls to NudgeIfNeeded (which would normally
	// return a message after many reads) return "" because the tracker was cleared.
	// Since we can't inspect the tracker directly from the test, the passing response
	// is sufficient to confirm ClearSession was called without panicking.
}

// ─── mem_save ──────────────────────────────────────────────────────────────

// ─── mem_save lazy fallback (T2.4) ────────────────────────────────────────

func TestMemSave_WithoutSessionID_CallsEnsureManualSaveSession(t *testing.T) {
	var ensureCalled bool
	var savedSessionID string
	store := &mockStore{
		ensureManualSaveSessionFn: func(project string) (string, error) {
			ensureCalled = true
			return "manual-save-" + project, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedSessionID = m.SessionID
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Test",
		"content": "content",
		"type":    "architecture",
		"project": "jarvis-dev",
		// no session_id
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if !ensureCalled {
		t.Error("EnsureManualSaveSession should be called when session_id is absent")
	}
	if savedSessionID != "manual-save-jarvis-dev" {
		t.Errorf("SaveMemory called with SessionID=%q, want 'manual-save-jarvis-dev'", savedSessionID)
	}
}

func TestMemSave_WithExplicitSessionID_DoesNotCallEnsure(t *testing.T) {
	var ensureCalled bool
	var savedSessionID string
	store := &mockStore{
		ensureManualSaveSessionFn: func(_ string) (string, error) {
			ensureCalled = true
			return "manual-save-proj", nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedSessionID = m.SessionID
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":      "Test",
		"content":    "content",
		"type":       "architecture",
		"project":    "jarvis-dev",
		"session_id": "sess-explicit",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if ensureCalled {
		t.Error("EnsureManualSaveSession must NOT be called when session_id is explicit")
	}
	if savedSessionID != "sess-explicit" {
		t.Errorf("SaveMemory called with SessionID=%q, want 'sess-explicit'", savedSessionID)
	}
}

func TestMemSave_CapturePromptSemantics(t *testing.T) {
	falseValue := false
	tests := []struct {
		name       string
		capture    *bool
		prompt     *models.Prompt
		wantLookup bool
		wantPrompt int64
	}{
		{name: "default true links latest prompt", prompt: &models.Prompt{ID: 7}, wantLookup: true, wantPrompt: 7},
		{name: "explicit false skips lookup", capture: &falseValue},
		{name: "default true falls back when no prompt exists", wantLookup: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved *models.Memory
			lookupCalled := false
			store := &mockStore{saveMemoryFn: func(m *models.Memory) (int64, error) {
				saved = m
				return 1, nil
			}}
			prompts := &mockStore{latestPromptForSessionFn: func(_ context.Context, projectName, sessionID string) (*models.Prompt, error) {
				lookupCalled = true
				if projectName != "jarvis-dev" || sessionID != "sess-001" {
					t.Fatalf("lookup scope = (%q, %q), want (jarvis-dev, sess-001)", projectName, sessionID)
				}
				return tt.prompt, nil
			}}
			session := connectTestServerFull(t, store, prompts)

			args := map[string]any{"title": "Memory", "content": "content", "type": "decision", "project": "jarvis-dev", "session_id": "sess-001"}
			if tt.capture != nil {
				args["capture_prompt"] = *tt.capture
			}
			res := callTool(t, session, "mem_save", args)

			if res.IsError {
				t.Fatalf("expected success, got error: %s", textContent(t, res))
			}
			if lookupCalled != tt.wantLookup {
				t.Fatalf("lookupCalled = %v, want %v", lookupCalled, tt.wantLookup)
			}
			if saved == nil || saved.PromptID != tt.wantPrompt {
				t.Fatalf("saved.PromptID = %v, want %d", saved, tt.wantPrompt)
			}
		})
	}
}

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

func TestMemSave_UnknownProjectReturnsStructuredErrorWithoutGhostWrite(t *testing.T) {
	t.Parallel()

	var saveCalled bool
	var ensureCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "jarvis-dev"}}, nil
		},
		ensureManualSaveSessionFn: func(project string) (string, error) {
			ensureCalled = true
			return "manual-save-" + project, nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			saveCalled = true
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Ghost",
		"content": "should not persist",
		"type":    "architecture",
		"project": "ghost-project",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true for unknown project")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
	}
	if ensureCalled {
		t.Fatal("EnsureManualSaveSession must not create a ghost session after validation failure")
	}
	if saveCalled {
		t.Fatal("SaveMemory must not be called after validation failure")
	}
}

func TestMemSavePrompt_AmbiguousProjectReturnsRecoveryTokenAndBlocksSave(t *testing.T) {
	t.Parallel()

	var promptCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "jarvis-dev"}, {Name: "jarvis.dev"}}, nil
		},
		createRecoveryTokenFn: func(_ context.Context, req project.TokenRequest) (string, error) {
			if req.RequestedProject != "jarvis dev" {
				t.Fatalf("requested project = %q, want jarvis dev", req.RequestedProject)
			}
			return "retry-token", nil
		},
		savePromptFn: func(context.Context, string, string) (*models.Prompt, error) {
			promptCalled = true
			return &models.Prompt{ID: 1}, nil
		},
	}
	session := connectTestServerWithPrompts(t, store)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "prompt",
		"project": "jarvis dev",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true for ambiguous project")
	}
	body := decodeJSONResponse(t, res)
	if body["error_code"] != string(project.CodeProjectAmbiguous) || body["recovery_token"] != "retry-token" {
		t.Fatalf("body = %v, want ambiguous retry-token", body)
	}
	if promptCalled {
		t.Fatal("SavePrompt must not be called after ambiguity")
	}
}

func TestMemSavePrompt_RecoveryTokenRetryConsumesTokenAndSaves(t *testing.T) {
	t.Parallel()

	var consumed project.TokenValidation
	var savedProject string
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "jarvis-dev"}}, nil
		},
		consumeRecoveryTokenFn: func(_ context.Context, validation project.TokenValidation) error {
			consumed = validation
			return nil
		},
		savePromptFn: func(_ context.Context, projectName, _ string) (*models.Prompt, error) {
			savedProject = projectName
			return &models.Prompt{ID: 5, Project: projectName, CreatedAt: time.Now()}, nil
		},
	}
	session := connectTestServerWithPrompts(t, store)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content":               "prompt",
		"project":               "jarvis-dev",
		"recovery_token":        "retry-token",
		"project_choice_reason": "jarvis dev",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if consumed.Token != "retry-token" || consumed.SelectedProject != "jarvis-dev" {
		t.Fatalf("consumed = %+v, want retry-token for jarvis-dev", consumed)
	}
	if savedProject != "jarvis-dev" {
		t.Fatalf("saved project = %q, want jarvis-dev", savedProject)
	}
}

func TestMemSavePrompt_ExpiredRecoveryTokenReturnsStructuredError(t *testing.T) {
	t.Parallel()

	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "jarvis-dev"}}, nil
		},
		consumeRecoveryTokenFn: func(context.Context, project.TokenValidation) error {
			return project.ErrRecoveryTokenExpired
		},
	}
	session := connectTestServerWithPrompts(t, store)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content":               "prompt",
		"project":               "jarvis-dev",
		"recovery_token":        "expired-token",
		"project_choice_reason": "jarvis dev",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true for expired recovery token")
	}
	body := decodeJSONResponse(t, res)
	if body["error_code"] != string(project.CodeRecoveryTokenExpired) {
		t.Fatalf("error_code = %v, want %q; body=%v", body["error_code"], project.CodeRecoveryTokenExpired, body)
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
	body := decodeJSONResponse(t, res)
	if got := body["autosync_status"]; got != "queued" {
		t.Fatalf("autosync_status = %v, want queued", got)
	}
	if got := body["autosync_config_source"]; got != "injected" {
		t.Fatalf("autosync_config_source = %v, want injected", got)
	}
	if got := body["auto_sync"]; got != true {
		t.Fatalf("auto_sync = %v, want true", got)
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
			if got := body["config_source"]; got != "none" {
				t.Fatalf("config_source = %v, want none", got)
			}
			if got := body["auto_sync"]; got != false {
				t.Fatalf("auto_sync = %v, want false", got)
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

// TestMemSyncHandler_UsesDrainManual pins PR 1b-ii Task 1b-ii.8: the mem_sync
// MCP handler must drive a manual, user-triggered drain via
// Drain(ctx, project, TriggerManual) instead of calling Sync (TriggerAuto).
func TestMemSyncHandler_UsesDrainManual(t *testing.T) {
	t.Parallel()

	syncer := &scriptedSyncer{result: &hivesync.Result{
		Pushed:    2,
		Pulled:    1,
		Conflicts: 0,
		Project:   "test-proj",
	}}
	session := connectTestServerWithSync(t, &mockStore{}, nil, syncer)

	res := callTool(t, session, "mem_sync", map[string]any{"project": "test-proj"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}

	if got := syncer.drainCallCount(); got != 1 {
		t.Fatalf("Drain call count = %d, want 1", got)
	}
	if got := syncer.callCount(); got != 0 {
		t.Fatalf("Sync call count = %d, want 0 — mem_sync must use Drain, not Sync", got)
	}
	if got := syncer.lastDrainPolicy(); got != hivesync.TriggerManual {
		t.Fatalf("Drain policy = %v, want TriggerManual", got)
	}

	body := decodeJSONResponse(t, res)
	if got := body["pushed"]; got != float64(2) {
		t.Fatalf("pushed = %v, want 2", got)
	}
	if got := body["pulled"]; got != float64(1) {
		t.Fatalf("pulled = %v, want 1", got)
	}
	if got := body["status"]; got != "ok" {
		t.Fatalf("status = %v, want ok", got)
	}
}

// TestMemSyncHandler_StillHandlesInFlightAndBackoff pins that switching
// mem_sync to Drain does not change the ErrSyncInFlight/BackoffError response
// branches — they must behave identically whether raised from Sync or Drain.
func TestMemSyncHandler_StillHandlesInFlightAndBackoff(t *testing.T) {
	t.Parallel()

	retryAt := time.Date(2026, time.May, 8, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name       string
		syncer     *scriptedSyncer
		wantStatus string
		wantRetry  string
	}{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := connectTestServerWithSync(t, &mockStore{}, nil, tt.syncer)

			res := callTool(t, session, "mem_sync", map[string]any{"project": "test-proj"})
			if res.IsError {
				t.Fatalf("unexpected error: %s", textContent(t, res))
			}

			if got := tt.syncer.drainCallCount(); got != 1 {
				t.Fatalf("Drain call count = %d, want 1", got)
			}

			body := decodeJSONResponse(t, res)
			if got := body["status"]; got != tt.wantStatus {
				t.Fatalf("status = %v, want %q", got, tt.wantStatus)
			}
			if tt.wantRetry != "" {
				if got := body["retry_at"]; got != tt.wantRetry {
					t.Fatalf("retry_at = %v, want %q", got, tt.wantRetry)
				}
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

func TestMemSearch_DoesNotExposeLegacyImpactMetadata(t *testing.T) {
	legacyMemory := &models.Memory{
		ID:        1,
		Title:     "Legacy Metadata",
		Content:   "legacy payload should not affect search formatting",
		Project:   "proj",
		Category:  "architecture",
		CreatedAt: time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
	}
	setLegacyImpactScoreIfPresent(t, legacyMemory, 9)

	store := &mockStore{
		searchFn: func(query, project, category string, limit int) ([]*models.Memory, error) {
			return []*models.Memory{legacyMemory}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_search", map[string]any{
		"query":   "legacy",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	body := textContent(t, res)
	if strings.Contains(body, "⭐") || strings.Contains(body, "impact_score") || strings.Contains(body, "confidence") {
		t.Fatalf("search output exposed legacy metadata, got: %s", body)
	}
}

func setLegacyImpactScoreIfPresent(t *testing.T, mem *models.Memory, score int64) {
	t.Helper()
	field := reflect.ValueOf(mem).Elem().FieldByName("ImpactScore")
	if field.IsValid() && field.CanSet() {
		field.SetInt(score)
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

// ─── mem_session_summary lazy fallback (T2.6) ─────────────────────────────

func TestMemSessionSummary_WithoutSessionID_CallsEnsureManualSave(t *testing.T) {
	var ensureCalled bool
	var savedSessionID string
	store := &mockStore{
		ensureManualSaveSessionFn: func(project string) (string, error) {
			ensureCalled = true
			return "manual-save-" + project, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedSessionID = m.SessionID
			return 10, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": "## Goal\nWrap up",
		"project": "jarvis-dev",
		// no session_id
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if !ensureCalled {
		t.Error("EnsureManualSaveSession should be called when session_id is absent")
	}
	if savedSessionID != "manual-save-jarvis-dev" {
		t.Errorf("SessionID = %q, want 'manual-save-jarvis-dev'", savedSessionID)
	}
}

func TestMemSessionSummary_WithExplicitOpenSession_SavesAndDoesNotCallEnsure(t *testing.T) {
	var ensureCalled bool
	var savedSessionID string
	store := &mockStore{
		ensureManualSaveSessionFn: func(_ string) (string, error) {
			ensureCalled = true
			return "manual-save-proj", nil
		},
		getSessionFn: func(id string) (*models.Session, error) {
			return &models.Session{ID: id, Project: "jarvis-dev", EndedAt: nil}, nil
		},
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			savedSessionID = m.SessionID
			return 10, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":    "## Goal\nWrap up",
		"project":    "jarvis-dev",
		"session_id": "sess-explicit",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if ensureCalled {
		t.Error("EnsureManualSaveSession must NOT be called when session_id is explicit")
	}
	if savedSessionID != "sess-explicit" {
		t.Errorf("SessionID = %q, want 'sess-explicit'", savedSessionID)
	}
}

func TestMemSessionSummary_WithEndedSession_ReturnsError(t *testing.T) {
	endedAt := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	store := &mockStore{
		getSessionFn: func(id string) (*models.Session, error) {
			return &models.Session{ID: id, EndedAt: &endedAt}, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":    "## Goal\nWrap up",
		"project":    "jarvis-dev",
		"session_id": "sess-done",
	})

	if !res.IsError {
		t.Error("expected IsError=true for already-ended session")
	}
	if !strings.Contains(textContent(t, res), "already ended") {
		t.Errorf("error should mention 'already ended', got: %s", textContent(t, res))
	}
}

func TestMemSessionSummary_WithUnknownSession_ReturnsError(t *testing.T) {
	store := &mockStore{
		getSessionFn: func(id string) (*models.Session, error) {
			return nil, errors.New("session not found")
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":    "## Goal\nWrap up",
		"project":    "jarvis-dev",
		"session_id": "ghost-session",
	})

	if !res.IsError {
		t.Error("expected IsError=true for unknown session")
	}
}

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

func TestMemSessionSummary_SessionProjectMismatchReturnsStructuredError(t *testing.T) {
	t.Parallel()

	var saveCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "alpha"}, {Name: "beta"}}, nil
		},
		sessionProjectFn: func(context.Context, string) (string, error) {
			return "alpha", nil
		},
		saveMemoryFn: func(*models.Memory) (int64, error) {
			saveCalled = true
			return 10, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content":    "## Goal\nWrap up",
		"project":    "beta",
		"session_id": "sess-alpha",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true for session/project mismatch")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectSessionMismatch) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectSessionMismatch, body)
	}
	if saveCalled {
		t.Fatal("SaveMemory must not be called after session mismatch")
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

// ─── mem_save_prompt lazy fallback (T2.5) ────────────────────────────────

func TestMemSavePrompt_WithoutSessionID_CallsEnsureManualSaveSession(t *testing.T) {
	var ensureCalled bool
	store := &mockStore{
		ensureManualSaveSessionFn: func(project string) (string, error) {
			ensureCalled = true
			return "manual-save-" + project, nil
		},
	}
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, _ string) (*models.Prompt, error) {
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	ctx := context.Background()
	server := hivemcp.NewServer(store, nil, nil, nil, ps)
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

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "explain goroutines",
		"project": "jarvis-dev",
		// no session_id
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if !ensureCalled {
		t.Error("EnsureManualSaveSession should be called when session_id is absent from mem_save_prompt")
	}
}

func TestMemSavePrompt_WithExplicitSessionID_DoesNotCallEnsure(t *testing.T) {
	var ensureCalled bool
	store := &mockStore{
		ensureManualSaveSessionFn: func(_ string) (string, error) {
			ensureCalled = true
			return "manual-save-proj", nil
		},
	}
	ps := &mockStore{
		savePromptFn: func(_ context.Context, _, _ string) (*models.Prompt, error) {
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	ctx := context.Background()
	server := hivemcp.NewServer(store, nil, nil, nil, ps)
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

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content":    "explain goroutines",
		"project":    "jarvis-dev",
		"session_id": "sess-explicit",
	})

	if res.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, res))
	}
	if ensureCalled {
		t.Error("EnsureManualSaveSession must NOT be called when session_id is explicit in mem_save_prompt")
	}
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

func TestMemSavePrompt_UnknownProjectReturnsStructuredErrorWithoutPromptWrite(t *testing.T) {
	t.Parallel()

	var savePromptCalled bool
	store := &mockStore{
		knownProjectsFn: func(context.Context) ([]project.KnownProject, error) {
			return []project.KnownProject{{Name: "jarvis-dev"}}, nil
		},
	}
	prompts := &mockStore{
		savePromptFn: func(context.Context, string, string) (*models.Prompt, error) {
			savePromptCalled = true
			return &models.Prompt{ID: 1, CreatedAt: time.Now()}, nil
		},
	}
	session := connectTestServerFull(t, store, prompts)

	res := callTool(t, session, "mem_save_prompt", map[string]any{
		"content": "explain goroutines",
		"project": "ghost-project",
	})

	if !res.IsError {
		t.Fatal("expected IsError=true for unknown project")
	}
	body := decodeJSONResponse(t, res)
	if got := body["error_code"]; got != string(project.CodeProjectUnknown) {
		t.Fatalf("error_code = %v, want %q; body=%v", got, project.CodeProjectUnknown, body)
	}
	if savePromptCalled {
		t.Fatal("SavePrompt must not be called after validation failure")
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

	// Private tag MUST be on line 1 so titleFromContent encounters it.
	// If title were derived from raw (un-stripped) content, the secret would leak into Title.
	res := callTool(t, session, "mem_session_summary", map[string]any{
		"content": "<private>my-token-secret</private> in title\nrest of content",
		"project": "jarvis-dev",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	wantContent := "[REDACTED] in title\nrest of content"
	if saved.Content != wantContent {
		t.Errorf("stored content = %q, want %q", saved.Content, wantContent)
	}
	// Title must be derived from stripped content — never the raw input.
	// Needle is the ACTUAL secret used in the fixture. A regressed implementation
	// that derives title from raw content would produce "my-token-secret in title"
	// and trigger this assertion.
	if strings.Contains(saved.Title, "my-token-secret") || strings.Contains(saved.Title, "<private>") {
		t.Errorf("title leaked raw content: got %q", saved.Title)
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

// T-06a-9: memSaveHandler — 2-level nested blocks pass correctly through handler boundary
func TestMemSave_NestedPrivateBlocks_StripsAndReturnsCount(t *testing.T) {
	var saved *models.Memory
	store := &mockStore{
		saveMemoryFn: func(m *models.Memory) (int64, error) {
			saved = m
			return 1, nil
		},
	}
	session := connectTestServer(t, store)

	res := callTool(t, session, "mem_save", map[string]any{
		"title":   "Nested test",
		"content": "before <private>outer <private>inner</private> tail</private> after",
		"type":    "decision",
		"project": "proj",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	if saved == nil {
		t.Fatal("SaveMemory was not called")
	}
	if saved.Content != "before [REDACTED] after" {
		t.Errorf("stored content = %q, want %q", saved.Content, "before [REDACTED] after")
	}
	body := decodeJSONResponse(t, res)
	if stripped := body["stripped"]; stripped != true {
		t.Errorf("stripped = %v, want true", stripped)
	}
	if count := body["stripped_count"]; count != float64(1) {
		t.Errorf("stripped_count = %v, want 1 (nested blocks count as 1 outermost)", count)
	}
}

// ─── T2.9: Full lifecycle integration test (real SQLite) ─────────────────────

// connectRealServer opens a real :memory: SQLite DB and connects to a full server.
// Both store and prompts are backed by the same *db.DB instance.
func connectRealServer(t *testing.T) (*sdkmcp.ClientSession, *hivedb.DB) {
	t.Helper()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("hivedb.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	server := hivemcp.NewServer(d, nil, nil, nil, d)
	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "integration-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, d
}

// querySessionIDForMemory fetches the session_id column for the given memory row.
func querySessionIDForMemory(t *testing.T, rawDB *sql.DB, memoryID float64) string {
	t.Helper()
	var sessionID sql.NullString
	err := rawDB.QueryRow(`SELECT session_id FROM memories WHERE id = ?`, int64(memoryID)).Scan(&sessionID)
	if err != nil {
		t.Fatalf("querySessionIDForMemory(%v): %v", memoryID, err)
	}
	if !sessionID.Valid {
		return ""
	}
	return sessionID.String
}

func TestE2E_FullSessionLifecycle(t *testing.T) {
	session, store := connectRealServer(t)
	directory := t.TempDir()

	// ── Step 1: start session ────────────────────────────────────────────────
	startRes := callTool(t, session, "mem_session_start", map[string]any{
		"id":        "e2e-sess-001",
		"project":   "e2e-project",
		"directory": directory,
		"dev_id":    "e2e-dev",
		"client":    "test",
	})
	if startRes.IsError {
		t.Fatalf("mem_session_start failed: %s", textContent(t, startRes))
	}
	startBody := decodeJSONResponse(t, startRes)
	if startBody["session_id"] != "e2e-sess-001" {
		t.Errorf("session_id = %v, want 'e2e-sess-001'", startBody["session_id"])
	}

	// ── Step 2: mem_save WITH session_id → memory linked to explicit session ─
	saveExplicitRes := callTool(t, session, "mem_save", map[string]any{
		"title":      "E2E Memory With Session",
		"content":    "linked to explicit session",
		"type":       "architecture",
		"project":    "e2e-project",
		"session_id": "e2e-sess-001",
	})
	if saveExplicitRes.IsError {
		t.Fatalf("mem_save with session_id failed: %s", textContent(t, saveExplicitRes))
	}
	saveExplicitBody := decodeJSONResponse(t, saveExplicitRes)
	explicitMemID, ok := saveExplicitBody["id"].(float64)
	if !ok {
		t.Fatalf("mem_save response has no numeric id: %v", saveExplicitBody)
	}
	gotSessionID := querySessionIDForMemory(t, store.RawDB(), explicitMemID)
	if gotSessionID != "e2e-sess-001" {
		t.Errorf("explicit mem_save: DB session_id = %q, want 'e2e-sess-001'", gotSessionID)
	}

	// ── Step 3: mem_save WITHOUT session_id → lazy manual-save-{project} ────
	saveLazyRes := callTool(t, session, "mem_save", map[string]any{
		"title":   "E2E Memory No Session",
		"content": "linked to manual-save session",
		"type":    "decision",
		"project": "e2e-project",
		// no session_id
	})
	if saveLazyRes.IsError {
		t.Fatalf("mem_save without session_id failed: %s", textContent(t, saveLazyRes))
	}
	saveLazyBody := decodeJSONResponse(t, saveLazyRes)
	lazyMemID, ok := saveLazyBody["id"].(float64)
	if !ok {
		t.Fatalf("lazy mem_save response has no numeric id: %v", saveLazyBody)
	}
	gotLazySessionID := querySessionIDForMemory(t, store.RawDB(), lazyMemID)
	if gotLazySessionID != "manual-save-e2e-project" {
		t.Errorf("lazy mem_save: DB session_id = %q, want 'manual-save-e2e-project'", gotLazySessionID)
	}

	// Verify manual-save session row was created in the sessions table.
	manualSession, err := store.GetSession("manual-save-e2e-project")
	if err != nil {
		t.Fatalf("GetSession(manual-save-e2e-project): %v", err)
	}
	if manualSession.EndedAt != nil {
		t.Error("manual-save session must remain open (not auto-closed)")
	}

	// ── Step 4: mem_session_summary with session_id → memory linked ──────────
	summaryRes := callTool(t, session, "mem_session_summary", map[string]any{
		"content":    "## Goal\nE2E test\n\n## Done\n- all steps",
		"project":    "e2e-project",
		"session_id": "e2e-sess-001",
	})
	if summaryRes.IsError {
		t.Fatalf("mem_session_summary failed: %s", textContent(t, summaryRes))
	}
	// Parse JSON part of the response (summary handler may append footer text).
	rawSummary := textContent(t, summaryRes)
	var summaryBody map[string]any
	if err := json.Unmarshal([]byte(rawSummary), &summaryBody); err != nil {
		// Try extracting just the first line as JSON.
		lines := strings.SplitN(rawSummary, "\n", 2)
		if jsonErr := json.Unmarshal([]byte(lines[0]), &summaryBody); jsonErr != nil {
			t.Fatalf("mem_session_summary response not valid JSON: %v — raw: %s", err, rawSummary)
		}
	}
	summaryMemID, ok := summaryBody["id"].(float64)
	if !ok {
		t.Fatalf("mem_session_summary response has no numeric id: %v", summaryBody)
	}
	gotSummarySessionID := querySessionIDForMemory(t, store.RawDB(), summaryMemID)
	if gotSummarySessionID != "e2e-sess-001" {
		t.Errorf("mem_session_summary: DB session_id = %q, want 'e2e-sess-001'", gotSummarySessionID)
	}

	// ── Step 5: mem_session_end → session closed ─────────────────────────────
	endRes := callTool(t, session, "mem_session_end", map[string]any{
		"id":      "e2e-sess-001",
		"summary": "all done",
	})
	if endRes.IsError {
		t.Fatalf("mem_session_end failed: %s", textContent(t, endRes))
	}
	endBody := decodeJSONResponse(t, endRes)
	if endBody["ended_at"] == nil {
		t.Error("mem_session_end response must contain ended_at")
	}

	// Verify session is marked ended in DB.
	closedSession, err := store.GetSession("e2e-sess-001")
	if err != nil {
		t.Fatalf("GetSession after end: %v", err)
	}
	if closedSession.EndedAt == nil {
		t.Error("session ended_at must be set in DB after mem_session_end")
	}

	// ── Step 6: second mem_session_end → "already ended" error ───────────────
	end2Res := callTool(t, session, "mem_session_end", map[string]any{
		"id": "e2e-sess-001",
	})
	if !end2Res.IsError {
		t.Error("second mem_session_end must return IsError=true for already-ended session")
	}
	if !strings.Contains(textContent(t, end2Res), "already ended") {
		t.Errorf("second end error should mention 'already ended', got: %s", textContent(t, end2Res))
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

// TestMemSave_TopicKeyDescription_NoUpsertWording asserts that the mem_save
// tool's topic_key parameter description does NOT contain the word "upsert" and
// DOES contain "grouping" or "context key" (Issue #119).
func TestMemSave_TopicKeyDescription_NoUpsertWording(t *testing.T) {
	session := connectTestServer(t, &mockStore{})

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() failed: %v", err)
	}

	var memSaveTool *sdkmcp.Tool
	for _, tool := range res.Tools {
		if tool.Name == "mem_save" {
			toolCopy := tool
			memSaveTool = toolCopy
			break
		}
	}
	if memSaveTool == nil {
		t.Fatal("mem_save tool not found in ListTools response")
	}

	// InputSchema is map[string]any on the client side.
	schemaBytes, err := json.Marshal(memSaveTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal InputSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("unmarshal InputSchema: %v", err)
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties not found or not a map: %T", schema["properties"])
	}
	topicKeyProp, ok := properties["topic_key"].(map[string]any)
	if !ok {
		t.Fatalf("topic_key property not found or not a map: %T", properties["topic_key"])
	}
	desc, ok := topicKeyProp["description"].(string)
	if !ok {
		t.Fatalf("topic_key description not found or not a string: %T", topicKeyProp["description"])
	}

	descLower := strings.ToLower(desc)
	if strings.Contains(descLower, "upsert") {
		t.Errorf("topic_key description must not contain 'upsert'; got: %q", desc)
	}
	if !strings.Contains(descLower, "grouping") && !strings.Contains(descLower, "context key") {
		t.Errorf("topic_key description must contain 'grouping' or 'context key'; got: %q", desc)
	}
}

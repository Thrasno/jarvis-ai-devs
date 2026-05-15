package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Ensure mockStore implements hivemcp.PromptStore at compile time.
var _ hivemcp.PromptStore = (*mockStore)(nil)

// Ensure mockStore implements hivemcp.MemoryStore at compile time.
// This compile-time check is the RED trigger for T2.1: adding new interface methods
// here causes compilation failure until the interface and all callers are updated.
var _ hivemcp.MemoryStore = (*mockStore)(nil)

// mockStore implements hivemcp.MemoryStore and hivemcp.PromptStore for testing.
type mockStore struct {
	saveMemoryFn              func(*models.Memory) (int64, error)
	getMemoryFn               func(int64) (*models.Memory, error)
	listMemoriesFn            func(string, int) ([]*models.Memory, error)
	searchFn                  func(string, string, string, int) ([]*models.Memory, error)
	savePromptFn              func(context.Context, string, string) (*models.Prompt, error)
	listRecentPromptsFn       func(context.Context, string, int) ([]*models.Prompt, error)
	createSessionFn           func(id, project, directory, devID, client string) error
	endSessionFn              func(id, summary string) error
	getSessionFn              func(id string) (*models.Session, error)
	ensureManualSaveSessionFn func(project string) (string, error)
	knownProjectsFn           func(context.Context) ([]project.KnownProject, error)
	sessionProjectFn          func(context.Context, string) (string, error)
	createRecoveryTokenFn     func(context.Context, project.TokenRequest) (string, error)
	validateRecoveryTokenFn   func(context.Context, project.TokenValidation) error
	consumeRecoveryTokenFn    func(context.Context, project.TokenValidation) error
}

func (m *mockStore) SaveMemory(mem *models.Memory) (int64, error) {
	if m.saveMemoryFn != nil {
		return m.saveMemoryFn(mem)
	}
	return 1, nil
}

func (m *mockStore) GetMemory(id int64) (*models.Memory, error) {
	if m.getMemoryFn != nil {
		return m.getMemoryFn(id)
	}
	return &models.Memory{ID: id, Title: "mock", Content: "mock content", Project: "proj"}, nil
}

func (m *mockStore) ListMemories(project string, limit int) ([]*models.Memory, error) {
	if m.listMemoriesFn != nil {
		return m.listMemoriesFn(project, limit)
	}
	return []*models.Memory{}, nil
}

func (m *mockStore) Search(query, project, category string, limit int) ([]*models.Memory, error) {
	if m.searchFn != nil {
		return m.searchFn(query, project, category, limit)
	}
	return []*models.Memory{}, nil
}

func (m *mockStore) SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error) {
	if m.savePromptFn != nil {
		return m.savePromptFn(ctx, project, content)
	}
	return &models.Prompt{ID: 1, Project: project, CreatedAt: time.Now()}, nil
}

func (m *mockStore) ListRecentPrompts(ctx context.Context, project string, limit int) ([]*models.Prompt, error) {
	if m.listRecentPromptsFn != nil {
		return m.listRecentPromptsFn(ctx, project, limit)
	}
	return nil, nil
}

func (m *mockStore) CreateSession(id, project, directory, devID, client string) error {
	if m.createSessionFn != nil {
		return m.createSessionFn(id, project, directory, devID, client)
	}
	return nil
}

func (m *mockStore) EndSession(id, summary string) error {
	if m.endSessionFn != nil {
		return m.endSessionFn(id, summary)
	}
	return nil
}

func (m *mockStore) GetSession(id string) (*models.Session, error) {
	if m.getSessionFn != nil {
		return m.getSessionFn(id)
	}
	return nil, errors.New("session not found")
}

func (m *mockStore) EnsureManualSaveSession(project string) (string, error) {
	if m.ensureManualSaveSessionFn != nil {
		return m.ensureManualSaveSessionFn(project)
	}
	return "manual-save-" + project, nil
}

func (m *mockStore) KnownProjects(ctx context.Context) ([]project.KnownProject, error) {
	if m.knownProjectsFn != nil {
		return m.knownProjectsFn(ctx)
	}
	return []project.KnownProject{
		{Name: "proj"},
		{Name: "jarvis-dev"},
		{Name: "test-proj"},
		{Name: "e2e-project"},
	}, nil
}

func (m *mockStore) SessionProject(ctx context.Context, sessionID string) (string, error) {
	if m.sessionProjectFn != nil {
		return m.sessionProjectFn(ctx, sessionID)
	}
	return "", project.ErrSessionNotFound
}

func (m *mockStore) CreateRecoveryToken(ctx context.Context, req project.TokenRequest) (string, error) {
	if m.createRecoveryTokenFn != nil {
		return m.createRecoveryTokenFn(ctx, req)
	}
	return "recovery-token", nil
}

func (m *mockStore) ConsumeRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	if err := m.ValidateRecoveryToken(ctx, validation); err != nil {
		return err
	}
	if m.consumeRecoveryTokenFn != nil {
		return m.consumeRecoveryTokenFn(ctx, validation)
	}
	return nil
}

func (m *mockStore) ValidateRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	if m.validateRecoveryTokenFn != nil {
		return m.validateRecoveryTokenFn(ctx, validation)
	}
	return nil
}

// connectTestServer creates a server+client pair using in-memory transport.
func connectTestServer(t *testing.T, store hivemcp.MemoryStore) *sdkmcp.ClientSession {
	t.Helper()
	return connectTestServerWithSync(t, store, nil, nil)
}

// connectTestServerWithSync creates a server+client pair with optional sync config and syncer.
func connectTestServerWithSync(t *testing.T, store hivemcp.MemoryStore, cfg *hivesync.Config, syncer hivemcp.SyncRunner) *sdkmcp.ClientSession {
	t.Helper()
	return connectTestServerWithConfigAndPrompts(t, store, cfg, syncer, &mockStore{})
}

func connectTestServerWithPrompts(t *testing.T, store *mockStore) *sdkmcp.ClientSession {
	t.Helper()
	return connectTestServerWithConfigAndPrompts(t, store, nil, nil, store)
}

func connectTestServerWithConfigAndPrompts(t *testing.T, store hivemcp.MemoryStore, cfg *hivesync.Config, syncer hivemcp.SyncRunner, prompts hivemcp.PromptStore) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := hivemcp.NewServer(store, nil, syncer, cfg, prompts)

	t1, t2 := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect failed: %v", err)
	}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// mockSyncer implements hivemcp.SyncRunner for testing.
type mockSyncer struct {
	mu        sync.Mutex
	syncCalls []syncCall
}

type syncCall struct {
	project string
	time    time.Time
}

func (m *mockSyncer) Sync(ctx context.Context, project string) (*hivesync.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCalls = append(m.syncCalls, syncCall{
		project: project,
		time:    time.Now(),
	})
	return &hivesync.Result{
		Pushed:  0,
		Pulled:  0,
		Project: project,
	}, nil
}

func (m *mockSyncer) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.syncCalls)
}

func (m *mockSyncer) lastProject() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.syncCalls) == 0 {
		return ""
	}
	return m.syncCalls[len(m.syncCalls)-1].project
}

func TestNewServer_RegistersTenTools(t *testing.T) {
	session := connectTestServer(t, &mockStore{})
	ctx := context.Background()

	expectedTools := map[string]bool{
		"mem_save":              false,
		"mem_search":            false,
		"mem_get_observation":   false,
		"mem_session_summary":   false,
		"mem_context":           false,
		"mem_sync":              false,
		"mem_save_prompt":       false,
		"mem_session_start":     false,
		"mem_session_end":       false,
		"mem_suggest_topic_key": false,
	}

	var total int
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() iteration error: %v", err)
		}
		expectedTools[tool.Name] = true
		total++
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}

	if total != 10 {
		t.Errorf("total registered tools = %d, want 10", total)
	}
}

func TestMemSuggestTopicKey_RequiresTitleAndTypeInSchema(t *testing.T) {
	session := connectTestServer(t, &mockStore{})
	ctx := context.Background()

	var found bool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() iteration error: %v", err)
		}
		if tool.Name != "mem_suggest_topic_key" {
			continue
		}
		found = true

		schemaBytes, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("cannot marshal InputSchema: %v", err)
		}
		var schema struct {
			Required   []string               `json:"required"`
			Properties map[string]interface{} `json:"properties"`
		}
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			t.Fatalf("InputSchema is not valid JSON: %v", err)
		}

		required := map[string]bool{}
		for _, name := range schema.Required {
			required[name] = true
		}
		for _, name := range []string{"title", "type"} {
			if !required[name] {
				t.Errorf("required[] missing %q", name)
			}
			if _, ok := schema.Properties[name]; !ok {
				t.Errorf("properties missing %q", name)
			}
		}

		typeProp, ok := schema.Properties["type"].(map[string]interface{})
		if !ok {
			t.Fatalf("properties.type = %T, want object", schema.Properties["type"])
		}
		enumValues, ok := typeProp["enum"].([]interface{})
		if !ok {
			t.Fatalf("properties.type.enum = %T, want array", typeProp["enum"])
		}
		wantEnum := []string{"architecture", "bugfix", "decision", "pattern", "config", "discovery", "preference"}
		if len(enumValues) != len(wantEnum) {
			t.Fatalf("properties.type.enum length = %d, want %d", len(enumValues), len(wantEnum))
		}
		for i, want := range wantEnum {
			if enumValues[i] != want {
				t.Errorf("properties.type.enum[%d] = %v, want %q", i, enumValues[i], want)
			}
		}
	}

	if !found {
		t.Fatal("mem_suggest_topic_key not found during schema check")
	}
}

// TestMemSave_SessionIDIsOptionalInSchema verifies session_id is declared as a property
// but NOT in required[] for mem_save, mem_save_prompt, and mem_session_summary.
// Design Decision 7: session_id must be discoverable by MCP clients inspecting the schema,
// while remaining optional so legacy clients without session_id still work.
func TestMemSave_SessionIDIsOptionalInSchema(t *testing.T) {
	session := connectTestServer(t, &mockStore{})
	ctx := context.Background()

	targetTools := map[string]bool{
		"mem_save":            false,
		"mem_save_prompt":     false,
		"mem_session_summary": false,
	}

	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() iteration error: %v", err)
		}
		if _, ok := targetTools[tool.Name]; !ok {
			continue
		}
		targetTools[tool.Name] = true

		// InputSchema arrives as map[string]any from the client (JSON-decoded).
		// We re-encode and decode to extract required[] and properties.
		schemaBytes, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("tool %q: cannot marshal InputSchema: %v", tool.Name, err)
		}
		var schema struct {
			Required   []string               `json:"required"`
			Properties map[string]interface{} `json:"properties"`
		}
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			t.Fatalf("tool %q: InputSchema is not valid JSON: %v", tool.Name, err)
		}

		for _, req := range schema.Required {
			if req == "session_id" {
				t.Errorf("tool %q: session_id must NOT be in required[], but it is", tool.Name)
			}
		}

		// session_id must be declared in properties so MCP clients can discover it
		if _, declared := schema.Properties["session_id"]; !declared {
			t.Errorf("tool %q: session_id must be declared in properties, but it is absent", tool.Name)
		}
	}

	for name, found := range targetTools {
		if !found {
			t.Errorf("tool %q not found during schema check", name)
		}
	}
}

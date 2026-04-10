package mcp_test

import (
	"context"
	"testing"

	hivemcp "github.com/Thrasno/jarvis-dev/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mockStore implements hivemcp.MemoryStore for testing.
type mockStore struct {
	saveMemoryFn   func(*models.Memory) (int64, error)
	getMemoryFn    func(int64) (*models.Memory, error)
	listMemoriesFn func(string, int) ([]*models.Memory, error)
	searchFn       func(string, string, int) ([]*models.Memory, error)
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

func (m *mockStore) Search(query, project string, limit int) ([]*models.Memory, error) {
	if m.searchFn != nil {
		return m.searchFn(query, project, limit)
	}
	return []*models.Memory{}, nil
}

// connectTestServer creates a server+client pair using in-memory transport.
func connectTestServer(t *testing.T, store hivemcp.MemoryStore) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := hivemcp.NewServer(store)

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

func TestNewServer_RegistersFiveTools(t *testing.T) {
	session := connectTestServer(t, &mockStore{})
	ctx := context.Background()

	expectedTools := map[string]bool{
		"mem_save":            false,
		"mem_search":          false,
		"mem_get_observation": false,
		"mem_session_summary": false,
		"mem_context":         false,
	}

	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() iteration error: %v", err)
		}
		expectedTools[tool.Name] = true
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
}

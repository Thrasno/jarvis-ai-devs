package mcp

import (
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

// MemoryStore is the interface the MCP handlers use to access the database.
// The concrete implementation is *db.DB; tests use a mock.
type MemoryStore interface {
	SaveMemory(mem *models.Memory) (int64, error)
	GetMemory(id int64) (*models.Memory, error)
	ListMemories(project string, limit int) ([]*models.Memory, error)
	Search(query, project string, limit int) ([]*models.Memory, error)
}

// NewServer creates and configures the MCP server with all 5 hive tools.
func NewServer(store MemoryStore) *sdkmcp.Server {
	s := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "hive-daemon",
		Version: "1.0.0",
	}, nil)

	registerTools(s, store)

	logger.Log.Printf("hive-daemon MCP server ready (5 tools registered)")
	return s
}

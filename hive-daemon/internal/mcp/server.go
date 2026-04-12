package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/sync"
)

// MemoryStore es la interfaz que usan los handlers para acceder a la BD local.
type MemoryStore interface {
	SaveMemory(mem *models.Memory) (int64, error)
	GetMemory(id int64) (*models.Memory, error)
	ListMemories(project string, limit int) ([]*models.Memory, error)
	Search(query, project, category string, limit int) ([]*models.Memory, error)
}

// SyncRunner es la interfaz que usa el tool mem_sync.
// *sync.Syncer la implementa; nil = sync no configurado.
type SyncRunner interface {
	Sync(ctx context.Context, project string) (*sync.Result, error)
}

// NewServer crea y configura el servidor MCP con todas las herramientas Hive.
// syncStore puede ser nil — en ese caso mem_sync no puede hacer lazy init.
// syncer puede ser nil — se inicializa lazy en la primera llamada a mem_sync.
func NewServer(store MemoryStore, syncStore sync.SyncStore, syncer SyncRunner) *sdkmcp.Server {
	s := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "hive-daemon",
		Version: "1.0.0",
	}, nil)

	activity := NewActivityTracker()
	registerTools(s, store, syncStore, syncer, activity)

	syncStatus := "sin sync"
	if syncer != nil {
		syncStatus = "sync activo"
	}
	logger.Log.Printf("hive-daemon MCP server ready (6 tools, %s)", syncStatus)
	return s
}

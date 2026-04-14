package mcp

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	hivesync "github.com/Thrasno/jarvis-dev/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MemoryStore es la interfaz que usan los handlers para acceder a la BD local.
type MemoryStore interface {
	SaveMemory(mem *models.Memory) (int64, error)
	GetMemory(id int64) (*models.Memory, error)
	ListMemories(project string, limit int) ([]*models.Memory, error)
	Search(query, project, category string, limit int) ([]*models.Memory, error)
}

// SyncRunner es la interfaz que usa el tool mem_sync.
// *hivesync.Syncer la implementa; nil = sync no configurado.
type SyncRunner interface {
	Sync(ctx context.Context, project string) (*hivesync.Result, error)
}

// NewServer crea y configura el servidor MCP con todas las herramientas Hive.
// syncStore puede ser nil — en ese caso mem_sync no puede hacer lazy init.
// syncer puede ser nil — se inicializa lazy en la primera llamada a mem_sync.
// cfg puede ser nil — en ese caso AutoSync está deshabilitado.
func NewServer(store MemoryStore, syncStore hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config) *sdkmcp.Server {
	return NewServerWithConfig(store, syncStore, syncer, cfg)
}

// NewServerWithConfig crea un servidor con configuración personalizada para testing.
// cfg puede ser nil — en ese caso AutoSync está deshabilitado.
func NewServerWithConfig(store MemoryStore, syncStore hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config) *sdkmcp.Server {
	s := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "hive-daemon",
		Version: "1.0.0",
	}, nil)

	activity := NewActivityTracker()
	registerTools(s, store, syncStore, syncer, cfg, activity)

	syncStatus := "sin sync"
	if syncer != nil {
		syncStatus = "sync activo"
	}
	logger.Log.Printf("hive-daemon MCP server ready (6 tools, %s)", syncStatus)
	return s
}

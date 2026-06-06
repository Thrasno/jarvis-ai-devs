package mcp

import (
	"context"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// PromptStore is the interface used by mem_save_prompt to persist user prompts
// and by mem_context to list recent prompts for a project.
type PromptStore interface {
	SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error)
	ListRecentPrompts(ctx context.Context, project string, limit int) ([]*models.Prompt, error)
}

// MemoryStore es la interfaz que usan los handlers para acceder a la BD local.
type MemoryStore interface {
	SaveMemory(mem *models.Memory) (int64, error)
	GetMemory(id int64) (*models.Memory, error)
	ListMemories(project string, limit int) ([]*models.Memory, error)
	Search(query, project, category string, limit int) ([]*models.Memory, error)

	// Session lifecycle — added in Slice 2
	CreateSession(id, project, directory, devID, client string) error
	EndSession(id, summary string) error
	GetSession(id string) (*models.Session, error)
	EnsureManualSaveSession(project string) (string, error)

	KnownProjects(ctx context.Context) ([]project.KnownProject, error)
	SessionProject(ctx context.Context, sessionID string) (string, error)
	CreateRecoveryToken(ctx context.Context, req project.TokenRequest) (string, error)
	ValidateRecoveryToken(ctx context.Context, validation project.TokenValidation) error
	ConsumeRecoveryToken(ctx context.Context, validation project.TokenValidation) error
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
// prompts puede ser nil — en ese caso mem_save_prompt devuelve error.
func NewServer(store MemoryStore, syncStore hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config, prompts PromptStore) *sdkmcp.Server {
	return NewServerWithConfig(store, syncStore, syncer, cfg, prompts)
}

// NewServerWithConfig crea un servidor con configuración personalizada para testing.
// cfg puede ser nil — en ese caso AutoSync está deshabilitado.
// prompts puede ser nil — en ese caso mem_save_prompt devuelve error.
func NewServerWithConfig(store MemoryStore, syncStore hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config, prompts PromptStore) *sdkmcp.Server {
	s := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "hive-daemon",
		Version: "1.0.0",
	}, nil)

	activity := NewActivityTracker()
	syncRuntime := newSyncRuntime(syncStore, syncer, cfg)
	registerTools(s, store, syncRuntime, activity, prompts)

	syncStatus := "sin sync"
	if syncer != nil {
		syncStatus = "sync activo"
	}
	logger.Log.Printf("hive-daemon MCP server ready (10 tools, %s)", syncStatus)
	return s
}

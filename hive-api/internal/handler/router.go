package handler

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

// AuthService define las operaciones de autenticación que necesitan los handlers.
// Definimos las interfaces aquí (en handler) siguiendo el principio Go:
// "define la interfaz donde se usa, no donde se implementa".
type AuthService interface {
	Login(ctx context.Context, email, password string) (string, error)
	ValidateToken(tokenString string) (*model.Claims, error)
}

// MemoryService define las operaciones sobre memorias individuales.
type MemoryService interface {
	Create(ctx context.Context, mem *model.Memory) (*model.Memory, error)
	GetByID(ctx context.Context, id string) (*model.Memory, error)
	List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, int64, error)
	Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error)
}

// SyncService define las operaciones de sincronización.
type SyncService interface {
	Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error)
	Pull(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error)
}

// AdminService define las operaciones de administración.
type AdminService interface {
	ListUsers(ctx context.Context) ([]*model.User, error)
	SetLevel(ctx context.Context, username string, newLevel model.UserLevel) error
	Deactivate(ctx context.Context, username string) error
}

// RouterDeps agrupa las dependencias del router.
// Pasar un struct en lugar de N parámetros hace que el constructor sea legible
// y fácil de extender sin romper código existente.
type RouterDeps struct {
	AuthSvc   AuthService
	MemorySvc MemoryService
	SyncSvc   SyncService
	AdminSvc  AdminService
}

// NewRouter construye y configura el router Gin con todas las rutas y middlewares.
//
// Estructura de rutas:
//
//	GET  /health                             — sin auth
//	POST /auth/login                         — sin auth
//	GET  /auth/me                            — RequireAuth
//	GET  /memories                           — RequireAuth
//	POST /memories                           — RequireAuth
//	GET  /memories/search                    — RequireAuth (ANTES de /:id)
//	GET  /memories/:id                       — RequireAuth
//	POST /sync                               — RequireAuth
//	GET  /admin/users                        — RequireAuth + RequireAdmin
//	POST /admin/users/:username/level        — RequireAuth + RequireAdmin
//	POST /admin/users/:username/deactivate   — RequireAuth + RequireAdmin
func NewRouter(deps RouterDeps) *gin.Engine {
	r := gin.New()

	// Middlewares globales: recovery primero (captura panics en todos los handlers)
	r.Use(middleware.Recovery())

	// Instanciamos los handlers con sus dependencias
	authH := NewAuthHandler(deps.AuthSvc)
	memH := NewMemoryHandler(deps.MemorySvc)
	syncH := NewSyncHandler(deps.SyncSvc)
	adminH := NewAdminHandler(deps.AdminSvc)

	// Rutas públicas (sin autenticación)
	r.GET("/health", HealthHandler)
	r.POST("/auth/login", authH.Login)

	// Rutas autenticadas — agrupamos con el middleware RequireAuth
	// gin.RouterGroup aplica el middleware a todas las rutas del grupo.
	auth := r.Group("/", middleware.RequireAuth(deps.AuthSvc))
	{
		auth.GET("/auth/me", authH.Me)

		// CRÍTICO: /memories/search DEBE registrarse ANTES de /memories/:id
		// Si /:id se registra primero, "search" matchea como id="search"
		auth.GET("/memories/search", memH.Search)
		auth.GET("/memories", memH.List)
		auth.POST("/memories", memH.Create)
		auth.GET("/memories/:id", memH.GetByID)

		auth.POST("/sync", syncH.Sync)
	}

	// Rutas de admin — RequireAuth + RequireAdmin
	admin := r.Group("/admin", middleware.RequireAuth(deps.AuthSvc), middleware.RequireAdmin())
	{
		admin.GET("/users", adminH.ListUsers)
		admin.POST("/users/:username/level", adminH.SetLevel)
		admin.POST("/users/:username/deactivate", adminH.Deactivate)
	}

	return r
}

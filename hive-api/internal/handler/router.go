package handler

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

// AuthService define las operaciones de autenticación que necesitan los handlers.
// Definimos las interfaces aquí (en handler) siguiendo el principio Go:
// "define la interfaz donde se usa, no donde se implementa".
type AuthService interface {
	Login(ctx context.Context, email, password string) (string, error)
	ValidateToken(tokenString string) (*model.Claims, error)
	GetCurrentUser(ctx context.Context, userID string) (*model.User, error)
}

// DBPinger permite verificar la conectividad con la base de datos.
// Lo usamos en GET /health para detectar si PostgreSQL está caído.
// pgxpool.Pool implementa esta interfaz implícitamente (tiene Ping).
type DBPinger interface {
	Ping(ctx context.Context) error
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
	PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) (*model.PullResult, error)
}

type SyncAttemptService interface {
	Ingest(ctx context.Context, req model.SyncAttemptIngestRequest) (model.SyncAttemptIngestResponse, error)
	Summary(ctx context.Context, query model.SyncAttemptSummaryQuery) (model.SyncAttemptSummaryResponse, error)
}

type ProjectService interface {
	List(ctx context.Context) (model.ProjectListResponse, error)
}

// AdminService define las operaciones de administración.
type AdminService interface {
	ListUsers(ctx context.Context) ([]*model.User, error)
	SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error
	GrantAdmin(ctx context.Context, actor model.AdminActor, username string) error
	Deactivate(ctx context.Context, actor model.AdminActor, username string) error
	GetStats(ctx context.Context) (*model.AdminStatsResponse, error)
	ListAuditLogs(ctx context.Context, filter model.AuditFilter) (model.AuditListResponse, error)
}

// OverviewService provides aggregated dashboard overview metrics.
type OverviewService interface {
	GetStats(ctx context.Context) (*model.OverviewStatsResponse, error)
	GetGrowth(ctx context.Context) (*model.OverviewGrowthResponse, error)
}

// RouterDeps agrupa las dependencias del router.
// Pasar un struct en lugar de N parámetros hace que el constructor sea legible
// y fácil de extender sin romper código existente.
type RouterDeps struct {
	AuthSvc            AuthService
	MemorySvc          MemoryService
	SyncSvc            SyncService
	SyncAttemptSvc     SyncAttemptService
	ProjectSvc         ProjectService
	AdminSvc           AdminService
	OverviewSvc        OverviewService
	DB                 DBPinger // puede ser nil en tests unitarios
	AllowedOrigins     []string // orígenes permitidos para CORS (e.g. ["https://hive.hivemem.dev"])
	DashboardAssetsDir string   // directorio con assets compilados para servir /dashboard
}

// NewRouter construye y configura el router Gin con todas las rutas y middlewares.
//
// Estructura de rutas:
//
//	GET  /health                                      — sin auth
//	POST /auth/login                                  — sin auth
//	GET  /auth/me                                     — RequireAuth
//	GET  /memories                                    — RequireAuth
//	POST /memories                                    — RequireAuth
//	GET  /memories/search                             — RequireAuth (ANTES de /:id)
//	GET  /memories/:id                                — RequireAuth
//	POST /sync                                        — RequireAuth
//	POST /sync-attempts                               — RequireAuth
//	GET  /admin/users                                 — RequireAuth + RequireAdmin
//	POST /admin/users/:username/level                 — RequireAuth + RequireAdmin
//	POST /admin/users/:username/grant-admin           — RequireAuth + RequireAdmin
//	POST /admin/users/:username/deactivate            — RequireAuth + RequireAdmin
//	GET  /admin/stats                                 — RequireAuth + RequireAdmin
//	GET  /admin/audit-logs                            — RequireAuth + RequireAdmin
//	GET  /admin/sync-attempts/summary                 — RequireAuth + RequireAdmin
func NewRouter(deps RouterDeps) *gin.Engine {
	r := gin.New()

	// Middlewares globales: recovery primero (captura panics en todos los handlers)
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(deps.AllowedOrigins))

	// Instanciamos los handlers con sus dependencias
	authH := NewAuthHandler(deps.AuthSvc)
	memH := NewMemoryHandler(deps.MemorySvc)
	syncH := NewSyncHandler(deps.SyncSvc)
	syncAttemptH := NewSyncAttemptHandler(deps.SyncAttemptSvc, deps.AuthSvc)
	projectH := NewProjectHandler(deps.ProjectSvc)
	adminH := NewAdminHandler(deps.AdminSvc)
	overviewH := NewOverviewHandler(deps.OverviewSvc)
	healthH := NewHealthHandler(deps.DB)

	// Rutas públicas (sin autenticación)
	r.GET("/health", healthH.Check)
	r.POST("/auth/login", authH.Login)

	// Rutas autenticadas — agrupamos con el middleware RequireAuth
	// gin.RouterGroup aplica el middleware a todas las rutas del grupo.
	auth := r.Group("/", middleware.RequireAuth(deps.AuthSvc))
	{
		auth.GET("/auth/me", authH.Me)
		auth.GET("/projects", projectH.List)

		// CRÍTICO: /memories/search DEBE registrarse ANTES de /memories/:id
		// Si /:id se registra primero, "search" matchea como id="search"
		auth.GET("/memories/search", memH.Search)
		auth.GET("/memories", memH.List)
		auth.POST("/memories", memH.Create)
		auth.GET("/memories/:id", memH.GetByID)

		auth.POST("/sync", syncH.Sync)
		auth.POST("/sync-attempts", syncAttemptH.Ingest)
	}

	// Rutas de admin — RequireAuth + RequireAdmin
	admin := r.Group("/admin", middleware.RequireAuth(deps.AuthSvc), middleware.RequireAdmin())
	{
		admin.GET("/audit-logs", adminH.ListAuditLogs)
		admin.GET("/sync-attempts/summary", syncAttemptH.Summary)
		admin.GET("/users", adminH.ListUsers)
		admin.GET("/stats", adminH.GetStats)
		admin.POST("/users/:username/level", adminH.SetLevel)
		admin.POST("/users/:username/grant-admin", adminH.GrantAdmin)
		admin.POST("/users/:username/deactivate", adminH.Deactivate)
		admin.GET("/overview/stats", overviewH.GetStats)
		admin.GET("/overview/growth", overviewH.GetGrowth)
	}

	registerDashboardRoutes(r, deps.DashboardAssetsDir)
	r.NoRoute(jsonNotFound)

	return r
}

func registerDashboardRoutes(r *gin.Engine, dashboardAssetsDir string) {
	dashboardAssetsDir = strings.TrimSpace(dashboardAssetsDir)
	if dashboardAssetsDir == "" {
		return
	}

	serve := dashboardHandler(dashboardAssetsDir)
	r.GET("/dashboard", serve)
	r.GET("/dashboard/*path", serve)
}

func dashboardHandler(dashboardAssetsDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isDashboardAssetTraversal(c.Request.URL.Path) || isDashboardAssetTraversal(c.Request.URL.EscapedPath()) {
			jsonNotFound(c)
			return
		}

		relPath := strings.TrimPrefix(c.Param("path"), "/")
		cleanPath := filepath.Clean(relPath)
		if cleanPath == "." {
			serveDashboardIndex(c, dashboardAssetsDir)
			return
		}

		if cleanPath == "assets" || strings.HasPrefix(cleanPath, "assets/") {
			serveDashboardAsset(c, dashboardAssetsDir, cleanPath)
			return
		}

		if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
			jsonNotFound(c)
			return
		}

		serveDashboardIndex(c, dashboardAssetsDir)
	}
}

func serveDashboardIndex(c *gin.Context, dashboardAssetsDir string) {
	indexPath := filepath.Join(dashboardAssetsDir, "index.html")
	if !regularFileExists(indexPath) {
		jsonNotFound(c)
		return
	}

	c.File(indexPath)
}

func serveDashboardAsset(c *gin.Context, dashboardAssetsDir, cleanPath string) {
	assetPath := filepath.Join(dashboardAssetsDir, cleanPath)
	if !regularFileExists(assetPath) {
		jsonNotFound(c)
		return
	}

	c.File(assetPath)
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}

func isDashboardAssetTraversal(rawPath string) bool {
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		path = rawPath
	}
	if !strings.HasPrefix(path, "/dashboard/assets/") {
		return false
	}

	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func jsonNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}

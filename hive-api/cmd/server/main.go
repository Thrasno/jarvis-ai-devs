// Package main es el punto de entrada del servidor hive-api.
//
// main() hace exactamente tres cosas:
//  1. Cargar configuración (variables de entorno)
//  2. Conectar a PostgreSQL y ejecutar migraciones
//  3. Construir el router y arrancar el servidor con graceful shutdown
//
// Todo lo demás (handlers, servicios, repositorios) vive en internal/.
// main.go es el "pegamento" que conecta las piezas — no tiene lógica propia.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/handler"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildAppDeps agrupa las dependencias inyectables para buildApp.
// Separamos la construcción del router de la conexión a BD para poder
// testear el router sin necesitar PostgreSQL real (db puede ser nil en tests).
type buildAppDeps struct {
	authSvc                  handler.AuthService
	memorySvc                handler.MemoryService
	syncSvc                  handler.SyncService
	syncAttemptSvc           handler.SyncAttemptService
	projectSvc               handler.ProjectService
	projectGovernanceSvc     handler.ProjectGovernanceService
	adminSvc                 handler.AdminService
	overviewSvc              handler.OverviewService
	activitySvc              handler.ActivityService
	accountSvc               handler.AccountService
	db                       handler.DBPinger // nil en tests unitarios → health skip DB check
	allowedOrigins           []string
	dashboardAssetsDir       string
	projectBlockAdminEnabled bool
}

type serviceFactories struct {
	newUserRepo                 func(*pgxpool.Pool) repository.UserRepository
	newMemoryRepo               func(*pgxpool.Pool) repository.MemoryRepository
	newPromptRepo               func(*pgxpool.Pool) repository.PromptRepository
	newSessionRepo              func(*pgxpool.Pool) repository.SessionRepository
	newAuditRepo                func(*pgxpool.Pool) repository.AuditRepository
	newSyncAttemptRepo          func(*pgxpool.Pool) repository.SyncAttemptRepository
	newProjectRepo              func(*pgxpool.Pool) repository.ProjectRepository
	newProjectBlockRepo         func(*pgxpool.Pool) repository.ProjectBlockRepository
	newTxManager                func(*pgxpool.Pool) repository.TxManager
	newAuthService              func(repository.UserRepository, string) handler.AuthService
	newMemoryService            func(repository.MemoryRepository, repository.SessionRepository, repository.ProjectBlockRepository, repository.TxManager) handler.MemoryService
	newSyncService              func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository, repository.ProjectBlockRepository, repository.TxManager) handler.SyncService
	newSyncAttemptService       func(repository.SyncAttemptRepository) handler.SyncAttemptService
	newProjectService           func(repository.ProjectRepository, repository.SyncAttemptRepository) handler.ProjectService
	newProjectGovernanceService func(repository.ProjectBlockRepository, repository.AuditRepository, repository.TxManager) handler.ProjectGovernanceService
	newAdminService             func(repository.UserRepository, repository.MemoryRepository, repository.AuditRepository, repository.TxManager, repository.SyncAttemptRepository) handler.AdminService
	newOverviewService          func(repository.MemoryRepository, repository.SyncAttemptRepository, repository.AuditRepository) handler.OverviewService
	newActivityService          func(repository.MemoryRepository) handler.ActivityService
	newAccountService           func(repository.UserRepository, repository.AuditRepository, repository.TxManager) handler.AccountService
}

func defaultServiceFactories() serviceFactories {
	return serviceFactories{
		newUserRepo:         repository.NewPostgresUserRepository,
		newMemoryRepo:       repository.NewPostgresMemoryRepository,
		newPromptRepo:       repository.NewPostgresPromptRepository,
		newSessionRepo:      repository.NewPostgresSessionRepository,
		newAuditRepo:        repository.NewPostgresAuditRepository,
		newSyncAttemptRepo:  repository.NewPostgresSyncAttemptRepository,
		newProjectRepo:      repository.NewPostgresProjectRepository,
		newProjectBlockRepo: repository.NewPostgresProjectBlockRepository,
		newTxManager:        repository.NewPostgresTxManager,
		newAuthService: func(userRepo repository.UserRepository, jwtSecret string) handler.AuthService {
			return service.NewAuthService(userRepo, jwtSecret)
		},
		newMemoryService: func(memRepo repository.MemoryRepository, sessionRepo repository.SessionRepository, blockRepo repository.ProjectBlockRepository, tx repository.TxManager) handler.MemoryService {
			return service.NewMemoryService(memRepo, sessionRepo, blockRepo, tx)
		},
		newSyncService: func(memRepo repository.MemoryRepository, promptRepo repository.PromptRepository, sessionRepo repository.SessionRepository, auditRepo repository.AuditRepository, blockRepo repository.ProjectBlockRepository, tx repository.TxManager) handler.SyncService {
			return service.NewSyncService(memRepo, promptRepo, sessionRepo, auditRepo, blockRepo, tx)
		},
		newSyncAttemptService: func(syncAttemptRepo repository.SyncAttemptRepository) handler.SyncAttemptService {
			return service.NewSyncAttemptService(syncAttemptRepo)
		},
		newProjectService: func(projectRepo repository.ProjectRepository, syncAttemptRepo repository.SyncAttemptRepository) handler.ProjectService {
			return service.NewProjectService(projectRepo, syncAttemptRepo)
		},
		newProjectGovernanceService: func(blockRepo repository.ProjectBlockRepository, auditRepo repository.AuditRepository, tx repository.TxManager) handler.ProjectGovernanceService {
			return service.NewProjectGovernanceService(blockRepo, auditRepo, tx)
		},
		newAdminService: func(userRepo repository.UserRepository, memRepo repository.MemoryRepository, auditRepo repository.AuditRepository, tx repository.TxManager, syncRepo repository.SyncAttemptRepository) handler.AdminService {
			return service.NewAdminService(userRepo, memRepo, auditRepo, tx, syncRepo)
		},
		newOverviewService: func(memRepo repository.MemoryRepository, syncRepo repository.SyncAttemptRepository, auditRepo repository.AuditRepository) handler.OverviewService {
			return service.NewOverviewService(memRepo, syncRepo, auditRepo)
		},
		newActivityService: func(memRepo repository.MemoryRepository) handler.ActivityService {
			return service.NewActivityService(memRepo)
		},
		newAccountService: func(userRepo repository.UserRepository, auditRepo repository.AuditRepository, tx repository.TxManager) handler.AccountService {
			return service.NewAccountService(userRepo, auditRepo, tx)
		},
	}
}

// buildApp construye el router Gin con todas las dependencias inyectadas.
// Es la función que los tests usan directamente — no necesita BD real.
func buildApp(deps buildAppDeps) *gin.Engine {
	return handler.NewRouter(handler.RouterDeps{
		AuthSvc:                  deps.authSvc,
		MemorySvc:                deps.memorySvc,
		SyncSvc:                  deps.syncSvc,
		SyncAttemptSvc:           deps.syncAttemptSvc,
		ProjectSvc:               deps.projectSvc,
		ProjectGovernanceSvc:     deps.projectGovernanceSvc,
		AdminSvc:                 deps.adminSvc,
		OverviewSvc:              deps.overviewSvc,
		ActivitySvc:              deps.activitySvc,
		AccountSvc:               deps.accountSvc,
		DB:                       deps.db,
		AllowedOrigins:           deps.allowedOrigins,
		DashboardAssetsDir:       deps.dashboardAssetsDir,
		ProjectBlockAdminEnabled: deps.projectBlockAdminEnabled,
	})
}

// wireServices conecta todos los servicios con el pool de PostgreSQL.
// Este es el único lugar donde creamos las implementaciones concretas.
// Todo el resto del código solo conoce interfaces.
func wireServices(pool *pgxpool.Pool, cfg *config.Config) buildAppDeps {
	return wireServicesWithFactories(pool, cfg, defaultServiceFactories())
}

func wireServicesWithFactories(pool *pgxpool.Pool, cfg *config.Config, factories serviceFactories) buildAppDeps {
	// Repositorios — implementaciones concretas de PostgreSQL
	// (interfaces definidas en repository/)
	userRepo := factories.newUserRepo(pool)
	memRepo := factories.newMemoryRepo(pool)
	promptRepo := factories.newPromptRepo(pool)
	sessionRepo := factories.newSessionRepo(pool)
	auditRepo := factories.newAuditRepo(pool)
	syncAttemptRepo := factories.newSyncAttemptRepo(pool)
	projectRepo := factories.newProjectRepo(pool)
	projectBlockRepo := factories.newProjectBlockRepo(pool)
	txManager := factories.newTxManager(pool)

	// Servicios — lógica de negocio, inyectamos los repositorios
	authSvc := factories.newAuthService(userRepo, cfg.JWTSecret)
	memorySvc := factories.newMemoryService(memRepo, sessionRepo, projectBlockRepo, txManager)
	syncSvc := factories.newSyncService(memRepo, promptRepo, sessionRepo, auditRepo, projectBlockRepo, txManager)
	syncAttemptSvc := factories.newSyncAttemptService(syncAttemptRepo)
	projectSvc := factories.newProjectService(projectRepo, syncAttemptRepo)
	projectGovernanceSvc := factories.newProjectGovernanceService(projectBlockRepo, auditRepo, txManager)
	adminSvc := factories.newAdminService(userRepo, memRepo, auditRepo, txManager, syncAttemptRepo)
	overviewSvc := factories.newOverviewService(memRepo, syncAttemptRepo, auditRepo)
	activitySvc := factories.newActivityService(memRepo)
	accountSvc := factories.newAccountService(userRepo, auditRepo, txManager)

	return buildAppDeps{
		authSvc:                  authSvc,
		memorySvc:                memorySvc,
		syncSvc:                  syncSvc,
		syncAttemptSvc:           syncAttemptSvc,
		projectSvc:               projectSvc,
		projectGovernanceSvc:     projectGovernanceSvc,
		adminSvc:                 adminSvc,
		overviewSvc:              overviewSvc,
		activitySvc:              activitySvc,
		accountSvc:               accountSvc,
		db:                       pool, // pgxpool.Pool implementa DBPinger (tiene Ping(ctx) error)
		allowedOrigins:           cfg.AllowedOrigins,
		dashboardAssetsDir:       cfg.DashboardAssetsDir,
		projectBlockAdminEnabled: cfg != nil && cfg.ProjectBlockAdminEnabled,
	}
}

// startupMigrationSQL is the boot order, owned by the migrations package so the
// tests that prove a migration's effect run the same slice the server does.
func startupMigrationSQL() []string {
	return migrations.Ordered()
}

func runStartupMigrations(pool *pgxpool.Pool) error {
	for _, sql := range startupMigrationSQL() {
		if err := repository.RunMigrations(pool, sql); err != nil {
			return err
		}
	}
	return repository.BackfillProjectIdentityRegistry(context.Background(), pool)
}

func main() {
	// --- Paso 1: Configuración ---
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuración inválida: %v", err)
	}

	gin.SetMode(cfg.GinMode)

	// --- Paso 2: Base de datos ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("no se pudo conectar a PostgreSQL: %v", err)
	}
	defer pool.Close()

	if err := runStartupMigrations(pool); err != nil {
		log.Fatalf("migraciones fallaron: %v", err)
	}

	log.Println("✓ PostgreSQL conectado y migraciones ejecutadas")

	// --- Paso 3: Servidor ---
	deps := wireServices(pool, cfg)
	router := buildApp(deps)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown: esperamos señal SIGINT/SIGTERM antes de cerrar.
	// Esto permite que las requests en curso terminen antes de apagar.
	// Es crítico en producción para no interrumpir syncs en progreso.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("hive-api escuchando en :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("error en servidor: %v", err)
		}
	}()

	// Bloqueamos hasta recibir señal de apagado
	<-quit
	log.Println("apagando servidor...")

	// Damos 5 segundos para que las requests en curso terminen
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown forzado: %v", err)
	}

	log.Println("servidor apagado limpiamente")
}

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
	authSvc            handler.AuthService
	memorySvc          handler.MemoryService
	syncSvc            handler.SyncService
	syncAttemptSvc     handler.SyncAttemptService
	adminSvc           handler.AdminService
	overviewSvc        handler.OverviewService
	db                 handler.DBPinger // nil en tests unitarios → health skip DB check
	allowedOrigins     []string
	dashboardAssetsDir string
}

type serviceFactories struct {
	newUserRepo           func(*pgxpool.Pool) repository.UserRepository
	newMemoryRepo         func(*pgxpool.Pool) repository.MemoryRepository
	newPromptRepo         func(*pgxpool.Pool) repository.PromptRepository
	newSessionRepo        func(*pgxpool.Pool) repository.SessionRepository
	newAuditRepo          func(*pgxpool.Pool) repository.AuditRepository
	newSyncAttemptRepo    func(*pgxpool.Pool) repository.SyncAttemptRepository
	newTxManager          func(*pgxpool.Pool) repository.TxManager
	newAuthService        func(repository.UserRepository, string) handler.AuthService
	newMemoryService      func(repository.MemoryRepository, repository.SessionRepository) handler.MemoryService
	newSyncService        func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository) handler.SyncService
	newSyncAttemptService func(repository.SyncAttemptRepository) handler.SyncAttemptService
	newAdminService       func(repository.UserRepository, repository.MemoryRepository, repository.AuditRepository, repository.TxManager) handler.AdminService
	newOverviewService    func(repository.MemoryRepository, repository.SyncAttemptRepository, repository.AuditRepository) handler.OverviewService
}

func defaultServiceFactories() serviceFactories {
	return serviceFactories{
		newUserRepo:        repository.NewPostgresUserRepository,
		newMemoryRepo:      repository.NewPostgresMemoryRepository,
		newPromptRepo:      repository.NewPostgresPromptRepository,
		newSessionRepo:     repository.NewPostgresSessionRepository,
		newAuditRepo:       repository.NewPostgresAuditRepository,
		newSyncAttemptRepo: repository.NewPostgresSyncAttemptRepository,
		newTxManager:       repository.NewPostgresTxManager,
		newAuthService: func(userRepo repository.UserRepository, jwtSecret string) handler.AuthService {
			return service.NewAuthService(userRepo, jwtSecret)
		},
		newMemoryService: func(memRepo repository.MemoryRepository, sessionRepo repository.SessionRepository) handler.MemoryService {
			return service.NewMemoryService(memRepo, sessionRepo)
		},
		newSyncService: func(memRepo repository.MemoryRepository, promptRepo repository.PromptRepository, sessionRepo repository.SessionRepository, auditRepo repository.AuditRepository) handler.SyncService {
			return service.NewSyncService(memRepo, promptRepo, sessionRepo, auditRepo)
		},
		newSyncAttemptService: func(syncAttemptRepo repository.SyncAttemptRepository) handler.SyncAttemptService {
			return service.NewSyncAttemptService(syncAttemptRepo)
		},
		newAdminService: func(userRepo repository.UserRepository, memRepo repository.MemoryRepository, auditRepo repository.AuditRepository, tx repository.TxManager) handler.AdminService {
			return service.NewAdminService(userRepo, memRepo, auditRepo, tx)
		},
		newOverviewService: func(memRepo repository.MemoryRepository, syncRepo repository.SyncAttemptRepository, auditRepo repository.AuditRepository) handler.OverviewService {
			return service.NewOverviewService(memRepo, syncRepo, auditRepo)
		},
	}
}

// buildApp construye el router Gin con todas las dependencias inyectadas.
// Es la función que los tests usan directamente — no necesita BD real.
func buildApp(deps buildAppDeps) *gin.Engine {
	return handler.NewRouter(handler.RouterDeps{
		AuthSvc:            deps.authSvc,
		MemorySvc:          deps.memorySvc,
		SyncSvc:            deps.syncSvc,
		SyncAttemptSvc:     deps.syncAttemptSvc,
		AdminSvc:           deps.adminSvc,
		OverviewSvc:        deps.overviewSvc,
		DB:                 deps.db,
		AllowedOrigins:     deps.allowedOrigins,
		DashboardAssetsDir: deps.dashboardAssetsDir,
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
	txManager := factories.newTxManager(pool)

	// Servicios — lógica de negocio, inyectamos los repositorios
	authSvc := factories.newAuthService(userRepo, cfg.JWTSecret)
	memorySvc := factories.newMemoryService(memRepo, sessionRepo)
	syncSvc := factories.newSyncService(memRepo, promptRepo, sessionRepo, auditRepo)
	syncAttemptSvc := factories.newSyncAttemptService(syncAttemptRepo)
	adminSvc := factories.newAdminService(userRepo, memRepo, auditRepo, txManager)
	overviewSvc := factories.newOverviewService(memRepo, syncAttemptRepo, auditRepo)

	return buildAppDeps{
		authSvc:            authSvc,
		memorySvc:          memorySvc,
		syncSvc:            syncSvc,
		syncAttemptSvc:     syncAttemptSvc,
		adminSvc:           adminSvc,
		overviewSvc:        overviewSvc,
		db:                 pool, // pgxpool.Pool implementa DBPinger (tiene Ping(ctx) error)
		allowedOrigins:     cfg.AllowedOrigins,
		dashboardAssetsDir: cfg.DashboardAssetsDir,
	}
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

	if err := repository.RunMigrations(pool, migrations.InitialSQL); err != nil {
		log.Fatalf("migración 001 falló: %v", err)
	}

	if err := repository.RunMigrations(pool, migrations.UserPromptsSQL); err != nil {
		log.Fatalf("migración 002 falló: %v", err)
	}

	if err := repository.RunMigrations(pool, migrations.SessionsSQL); err != nil {
		log.Fatalf("migración 003 falló: %v", err)
	}

	if err := repository.RunMigrations(pool, migrations.AuditLogsSQL); err != nil {
		log.Fatalf("migración 004 falló: %v", err)
	}

	if err := repository.RunMigrations(pool, migrations.MemoryMutationsSQL); err != nil {
		log.Fatalf("migración 005 falló: %v", err)
	}

	if err := repository.RunMigrations(pool, migrations.DropTopicKeyUniqueConstraintSQL); err != nil {
		log.Fatalf("migración 006 falló: %v", err)
	}

	if err := repository.RunMigrations(pool, migrations.SyncAttemptLogsSQL); err != nil {
		log.Fatalf("migración 007 falló: %v", err)
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

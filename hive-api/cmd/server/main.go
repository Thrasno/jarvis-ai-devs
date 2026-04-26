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

	"github.com/Thrasno/jarvis-dev/hive-api/internal/config"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/handler"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/Thrasno/jarvis-dev/hive-api/migrations"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildAppDeps agrupa las dependencias inyectables para buildApp.
// Separamos la construcción del router de la conexión a BD para poder
// testear el router sin necesitar PostgreSQL real (db puede ser nil en tests).
type buildAppDeps struct {
	authSvc        handler.AuthService
	memorySvc      handler.MemoryService
	syncSvc        handler.SyncService
	adminSvc       handler.AdminService
	db             handler.DBPinger // nil en tests unitarios → health skip DB check
	allowedOrigins []string
}

// buildApp construye el router Gin con todas las dependencias inyectadas.
// Es la función que los tests usan directamente — no necesita BD real.
func buildApp(deps buildAppDeps) *gin.Engine {
	return handler.NewRouter(handler.RouterDeps{
		AuthSvc:        deps.authSvc,
		MemorySvc:      deps.memorySvc,
		SyncSvc:        deps.syncSvc,
		AdminSvc:       deps.adminSvc,
		DB:             deps.db,
		AllowedOrigins: deps.allowedOrigins,
	})
}

// wireServices conecta todos los servicios con el pool de PostgreSQL.
// Este es el único lugar donde creamos las implementaciones concretas.
// Todo el resto del código solo conoce interfaces.
func wireServices(pool *pgxpool.Pool, cfg *config.Config) buildAppDeps {
	// Repositorios — implementaciones concretas de PostgreSQL
	// (interfaces definidas en repository/)
	userRepo := repository.NewPostgresUserRepository(pool)
	memRepo := repository.NewPostgresMemoryRepository(pool)

	// Servicios — lógica de negocio, inyectamos los repositorios
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret)
	memorySvc := service.NewMemoryService(memRepo)
	syncSvc := service.NewSyncService(memRepo)
	adminSvc := service.NewAdminService(userRepo, memRepo)

	return buildAppDeps{
		authSvc:        authSvc,
		memorySvc:      memorySvc,
		syncSvc:        syncSvc,
		adminSvc:       adminSvc,
		db:             pool, // pgxpool.Pool implementa DBPinger (tiene Ping(ctx) error)
		allowedOrigins: cfg.AllowedOrigins,
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

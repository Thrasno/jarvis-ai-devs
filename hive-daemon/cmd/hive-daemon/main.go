package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPath := dbFilePath()

	store, err := db.Open(dbPath)
	if err != nil {
		logger.Log.Fatalf("open database: %v", err)
	}
	defer func() { _ = store.Close() }()

	logger.Log.Printf("database: %s", dbPath)

	recoveryTTL, err := project.ParseRecoveryTokenTTL(os.Getenv("HIVE_RECOVERY_TOKEN_TTL"))
	if err != nil {
		logger.Log.Fatalf("invalid HIVE_RECOVERY_TOKEN_TTL: %v", err)
	}
	project.SetDefaultRecoveryTokenTTL(recoveryTTL)

	// Sync es opcional — solo se activa si están las variables de entorno.
	// Sin ellas, hive-daemon funciona en modo local puro (igual que antes).
	var syncer hivemcp.SyncRunner
	cfg, err := hivesync.Load()
	if err != nil {
		logger.Log.Fatalf("sync config error: %v", err)
	}
	if cfg != nil {
		syncer = hivesync.New(cfg, store)
		logger.Log.Printf("sync habilitado → %s", cfg.APIURL)
	} else {
		logger.Log.Printf("sync desactivado (define HIVE_API_URL/HIVE_API_EMAIL/HIVE_API_PASSWORD o crea ~/.jarvis/sync.json)")
	}

	httpDone := make(chan struct{})
	go func() {
		defer close(httpDone)
		backupStore := governance.NewSQLiteBackupStore(dbPath, "", store.RawDB())
		srv := httpapi.NewServerWithProjectStoreAndGovernance(httpAddr(), store, store, governance.NewServiceWithBackup(store, backupStore))
		if err := srv.Start(rootCtx); err != nil {
			logger.Log.Printf("http server stopped: %v (mcp continues)", err)
		}
	}()

	closed, err := runStartup(store)
	if err != nil {
		logger.Log.Fatalf("startup: %v", err)
	}
	if closed > 0 {
		logger.Log.Printf("auto-closed %d stale session(s)", closed)
	}

	server := hivemcp.NewServer(store, store, syncer, cfg, store)

	runErr := server.Run(rootCtx, &sdkmcp.StdioTransport{})
	stop()

	// Always wait for HTTP goroutine before closing DB or exiting.
	<-httpDone

	if runErr != nil {
		logger.Log.Fatalf("server stopped: %v", runErr)
	}
}

// httpAddr returns the address for the HTTP server, preferring HIVE_HTTP_PORT env var
// (default 7438).
func httpAddr() string {
	port := os.Getenv("HIVE_HTTP_PORT")
	if strings.TrimSpace(port) == "" {
		return "127.0.0.1:7438"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		logger.Log.Fatalf("invalid HIVE_HTTP_PORT %q: must be a number between 1 and 65535", port)
	}
	return "127.0.0.1:" + port
}

// runStartup runs pre-server initialization hooks and returns the number of
// stale sessions that were auto-closed. Extracted for testability.
func runStartup(store *db.DB) (int64, error) {
	return store.AutoCloseStale(24*time.Hour, time.Now)
}

// dbFilePath returns the SQLite path, preferring HIVE_DB_PATH env var
// (used in tests) over the default ~/.jarvis/memory.db.
func dbFilePath() string {
	if p := os.Getenv("HIVE_DB_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		logger.Log.Fatalf("cannot determine home directory: %v", err)
	}
	dbDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		logger.Log.Fatalf("cannot create DB directory %q: %v", dbDir, err)
	}
	return filepath.Join(dbDir, "memory.db")
}

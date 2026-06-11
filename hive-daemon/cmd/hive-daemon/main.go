package main

import (
	"context"
	"fmt"
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

	// Sync is optional. Invalid or malformed config is recorded as a persistent
	// warning and startup continues in local-only mode.
	var syncer hivemcp.SyncRunner
	cfg := applyStartupSyncConfig(store, hivesync.LoadWithStatus)
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
		govSvc := governance.NewServiceWithBackup(store, backupStore)
		configSvc := httpapi.NewSyncServiceAdapter(hivesync.NewService())
		healthSvc := httpapi.NewHealthServiceAdapter(hivesync.NewHealthService(store, nil))
		srv := httpapi.NewServerWithAll(httpAddr(), store, store, govSvc, configSvc, healthSvc)
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

// applyStartupSyncConfig loads sync configuration. On error it records a
// persistent warning and returns nil so startup continues in local-only mode.
// In-memory warnings from LoadWithStatus (e.g. partial env fallback) are also
// persisted. The caller is responsible for creating a SyncRunner when cfg is non-nil.
func applyStartupSyncConfig(store *db.DB, load func() (*hivesync.Config, hivesync.SyncConfigStatus, error)) *hivesync.Config {
	cfg, status, err := load()
	if err != nil {
		logger.Log.Printf("sync config invalid, starting in local-only mode: %v", err)
		if _, warnErr := store.SaveHiveWarning(db.HiveWarningInput{
			Severity: "warning",
			Source:   "startup/sync-config",
			Message:  fmt.Sprintf("sync config invalid, sync disabled: %v", err),
		}); warnErr != nil {
			logger.Log.Printf("could not persist sync config warning: %v", warnErr)
		}
		return nil
	}
	for _, w := range status.Warnings {
		if _, warnErr := store.SaveHiveWarning(db.HiveWarningInput{
			Severity: "warning",
			Source:   "startup/sync-config",
			Message:  w,
		}); warnErr != nil {
			logger.Log.Printf("could not persist sync config warning: %v", warnErr)
		}
	}
	return cfg
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

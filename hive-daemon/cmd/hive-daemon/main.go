package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	hivemcp "github.com/Thrasno/jarvis-dev/hive-daemon/internal/mcp"
	hivesync "github.com/Thrasno/jarvis-dev/hive-daemon/internal/sync"
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

	go func() {
		srv := httpapi.NewServer(httpAddr(), store)
		if err := srv.Start(rootCtx); err != nil {
			logger.Log.Printf("http server stopped: %v (mcp continues)", err)
		}
	}()

	server := hivemcp.NewServer(store, store, syncer, cfg, store)

	if err := server.Run(rootCtx, &sdkmcp.StdioTransport{}); err != nil {
		logger.Log.Fatalf("server stopped: %v", err)
	}
}

// httpAddr returns the address for the HTTP server, preferring HIVE_HTTP_PORT env var
// (default 7438).
func httpAddr() string {
	port := os.Getenv("HIVE_HTTP_PORT")
	if strings.TrimSpace(port) == "" {
		port = "7438"
	}
	return "0.0.0.0:" + port
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

package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	hivemcp "github.com/Thrasno/jarvis-dev/hive-daemon/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	dbPath := dbFilePath()

	store, err := db.Open(dbPath)
	if err != nil {
		logger.Log.Fatalf("open database: %v", err)
	}
	defer func() { _ = store.Close() }()

	logger.Log.Printf("database: %s", dbPath)

	server := hivemcp.NewServer(store)

	if err := server.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil {
		logger.Log.Fatalf("server stopped: %v", err)
	}
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

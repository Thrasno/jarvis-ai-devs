package logger

import (
	"log"
	"os"
)

// Log is the package-level stderr-only logger for hive-daemon.
// All output goes to stderr to keep stdout clean for MCP JSON-RPC.
var Log = log.New(os.Stderr, "[hive] ", log.LstdFlags)

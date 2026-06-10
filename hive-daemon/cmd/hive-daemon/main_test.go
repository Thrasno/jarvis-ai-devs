package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// binaryPath holds the path to the compiled hive-daemon binary used in tests.
var binaryPath string

// TestMain builds the hive-daemon binary once before all tests in this package.
func TestMain(m *testing.M) {
	f, err := os.CreateTemp("", "hive-daemon-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	binaryPath = f.Name()
	_ = f.Close()

	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build hive-daemon: %v\n%s\n", err, out)
		_ = os.Remove(binaryPath)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.Remove(binaryPath)
	os.Exit(code)
}

// spawnDaemon starts a hive-daemon subprocess with a fresh temp DB
// and connects to it using the MCP SDK's CommandTransport.
func spawnDaemon(t *testing.T) *sdkmcp.ClientSession {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "memory.db")

	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "HIVE_DB_PATH="+dbPath)

	transport := &sdkmcp.CommandTransport{Command: cmd}

	ctx := context.Background()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("failed to connect to hive-daemon: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// ─── 6.0 applyStartupSyncConfig ────────────────────────────────────────────

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestApplyStartupSyncConfig(t *testing.T) {
	tests := []struct {
		name          string
		load          func() (*hivesync.Config, hivesync.SyncConfigStatus, error)
		wantCfgNil    bool
		wantWarnings  int
		checkWarnings func(t *testing.T, warnings []db.HiveWarning)
	}{
		{
			name: "no_config_no_warning",
			load: func() (*hivesync.Config, hivesync.SyncConfigStatus, error) {
				return nil, hivesync.SyncConfigStatus{Source: hivesync.ConfigSourceNone}, nil
			},
			wantCfgNil:   true,
			wantWarnings: 0,
		},
		{
			name: "load_error_persists_warning",
			load: func() (*hivesync.Config, hivesync.SyncConfigStatus, error) {
				return nil, hivesync.SyncConfigStatus{}, fmt.Errorf("malformed sync.json: unexpected EOF")
			},
			wantCfgNil:   true,
			wantWarnings: 1,
			checkWarnings: func(t *testing.T, warnings []db.HiveWarning) {
				t.Helper()
				w := warnings[0]
				if w.Source != "startup/sync-config" {
					t.Errorf("source = %q, want %q", w.Source, "startup/sync-config")
				}
				if w.Severity != "warning" {
					t.Errorf("severity = %q, want %q", w.Severity, "warning")
				}
				if !strings.Contains(w.Message, "malformed sync.json") {
					t.Errorf("message %q should mention the original error", w.Message)
				}
			},
		},
		{
			name: "status_warnings_persists_all",
			load: func() (*hivesync.Config, hivesync.SyncConfigStatus, error) {
				cfg := &hivesync.Config{APIURL: "https://example.com"}
				status := hivesync.SyncConfigStatus{
					Configured: true,
					Source:     hivesync.ConfigSourceFile,
					Warnings:   []string{"incomplete env ignored: file config used", "auto_sync disabled"},
				}
				return cfg, status, nil
			},
			wantCfgNil:   false,
			wantWarnings: 2,
			checkWarnings: func(t *testing.T, warnings []db.HiveWarning) {
				t.Helper()
				for _, w := range warnings {
					if w.Source != "startup/sync-config" {
						t.Errorf("source = %q, want %q", w.Source, "startup/sync-config")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			cfg := applyStartupSyncConfig(store, tt.load)

			if tt.wantCfgNil && cfg != nil {
				t.Errorf("expected nil cfg, got %+v", cfg)
			}
			if !tt.wantCfgNil && cfg == nil {
				t.Fatal("expected non-nil cfg")
			}

			warnings, err := store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: "active"})
			if err != nil {
				t.Fatalf("list warnings: %v", err)
			}
			if len(warnings) != tt.wantWarnings {
				t.Fatalf("expected %d warning(s), got %d", tt.wantWarnings, len(warnings))
			}
			if tt.checkWarnings != nil {
				tt.checkWarnings(t, warnings)
			}
		})
	}
}

// ─── 6.1 Startup ───────────────────────────────────────────────────────────

func TestDaemon_Starts_AndRegisters10Tools(t *testing.T) {
	session := spawnDaemon(t)
	ctx := context.Background()

	var toolNames []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error: %v", err)
		}
		toolNames = append(toolNames, tool.Name)
	}

	if len(toolNames) != 10 {
		t.Errorf("expected 10 tools, got %d: %v", len(toolNames), toolNames)
	}
}

// ─── 6.2 Stdout Purity (DIOS Mitigation #3) ────────────────────────────────

// TestStdoutPurity sends a raw MCP initialize request to the daemon
// and verifies EVERY byte on stdout is valid JSON-RPC. If any log line
// appears on stdout, json.Unmarshal will fail and the test fails.
func TestStdoutPurity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "HIVE_DB_PATH="+dbPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard // logs go to stderr — we only watch stdout

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})

	// Send MCP initialize request
	initMsg := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"purity-test","version":"1"}}}` + "\n"
	if _, err := io.WriteString(stdin, initMsg); err != nil {
		t.Fatalf("failed to write to stdin: %v", err)
	}

	// Read the initialize response with timeout
	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lineCh <- scanner.Text()
		}
		close(lineCh)
	}()

	var line string
	select {
	case l, ok := <-lineCh:
		if !ok {
			t.Fatal("stdout closed before receiving initialize response")
		}
		line = l
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for daemon initialize response")
	}

	// EVERY character on stdout must be valid JSON-RPC
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Errorf("stdout is NOT valid JSON-RPC — stdout pollution detected!\nGot: %q\nError: %v", line, err)
	}
	if msg["jsonrpc"] != "2.0" {
		t.Errorf("response missing jsonrpc:2.0 field, got: %q", line)
	}
	if strings.Contains(line, "[hive]") {
		t.Errorf("log prefix '[hive]' found on stdout — stdout pollution!\nLine: %q", line)
	}
}

// ─── 6.3 End-to-End Integration ────────────────────────────────────────────

func TestE2E_SaveAndSearch(t *testing.T) {
	session := spawnDaemon(t)
	ctx := context.Background()
	startRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "mem_session_start",
		Arguments: map[string]any{
			"id":        "e2e-save-search-session",
			"project":   "e2e-test",
			"directory": t.TempDir(),
			"dev_id":    "test-dev",
			"client":    "test",
		},
	})
	if err != nil || startRes.IsError {
		t.Fatalf("mem_session_start failed: err=%v isError=%v", err, startRes.IsError)
	}

	// Save a memory
	saveRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "mem_save",
		Arguments: map[string]any{
			"title":   "SQLite Architecture",
			"content": "We use SQLite with FTS5 for full-text search",
			"type":    "architecture",
			"project": "e2e-test",
		},
	})
	if err != nil {
		t.Fatalf("mem_save error: %v", err)
	}
	if saveRes.IsError {
		t.Fatalf("mem_save failed: %s", saveRes.Content[0].(*sdkmcp.TextContent).Text)
	}

	// Search for it
	searchRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "mem_search",
		Arguments: map[string]any{
			"query":   "SQLite",
			"project": "e2e-test",
		},
	})
	if err != nil {
		t.Fatalf("mem_search error: %v", err)
	}
	if searchRes.IsError {
		t.Fatalf("mem_search failed: %s", searchRes.Content[0].(*sdkmcp.TextContent).Text)
	}

	body := searchRes.Content[0].(*sdkmcp.TextContent).Text
	// mem_search now returns markdown, not JSON
	if !strings.Contains(body, "SQLite Architecture") {
		t.Errorf("search result should contain 'SQLite Architecture', got: %s", body)
	}
	if !strings.Contains(body, "### [") {
		t.Errorf("search result should contain markdown headers, got: %s", body)
	}
}

// ─── 6.4 AutoCloseStale wiring (SC-17) ─────────────────────────────────────

// seedSessionAt inserts a session row with a specific started_at via raw SQL.
// Only used in tests within this package (package main).
func seedSessionAt(t *testing.T, sqlDB *sql.DB, id, project string, startedAt time.Time) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at)
		 VALUES (?, lower(hex(randomblob(16))), ?, '/d', 'dev', 'cli', ?)`,
		id, project, startedAt.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		t.Fatalf("seedSessionAt(%q): %v", id, err)
	}
}

// TestRunStartup_ClosesStaleSession verifies that runStartup calls AutoCloseStale,
// closing sessions older than 24h before the MCP server accepts connections (SC-17).
func TestRunStartup_ClosesStaleSession(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rawDB := store.RawDB()
	seedSessionAt(t, rawDB, "sess-stale-wiring", "proj", time.Now().Add(-48*time.Hour))

	closed, err := runStartup(store)
	if err != nil {
		t.Fatalf("runStartup: %v", err)
	}
	if closed == 0 {
		t.Error("runStartup should have auto-closed the stale session but closed=0")
	}

	sess, err := store.GetSession("sess-stale-wiring")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.EndedAt == nil {
		t.Error("stale session ended_at should be set after runStartup")
	}
}

// TestRunStartup_ManualSaveSessionExempt verifies manual-save sessions are NOT closed.
func TestRunStartup_ManualSaveSessionExempt(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rawDB := store.RawDB()
	seedSessionAt(t, rawDB, "manual-save-proj", "proj", time.Now().Add(-7*24*time.Hour))

	_, err = runStartup(store)
	if err != nil {
		t.Fatalf("runStartup: %v", err)
	}

	sess, err := store.GetSession("manual-save-proj")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.EndedAt != nil {
		t.Error("manual-save session must NOT be auto-closed by runStartup")
	}
}

// TestE2E_TopicKeyAlwaysInserts verifies the new topic_key semantics (Issue #119):
// saving twice with the same topic_key creates two distinct rows. The second save
// must return a different id, and both rows should appear in search results.
func TestE2E_TopicKeyAlwaysInserts(t *testing.T) {
	session := spawnDaemon(t)
	ctx := context.Background()
	startRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "mem_session_start",
		Arguments: map[string]any{
			"id":        "e2e-topic-insert-session",
			"project":   "e2e-test",
			"directory": t.TempDir(),
			"dev_id":    "test-dev",
			"client":    "test",
		},
	})
	if err != nil || startRes.IsError {
		t.Fatalf("mem_session_start failed: err=%v isError=%v", err, startRes.IsError)
	}

	args := map[string]any{
		"title":     "Auth Design v1",
		"content":   "First version",
		"type":      "architecture",
		"project":   "e2e-test",
		"topic_key": "arch/auth",
	}

	// First save
	r1, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "mem_save", Arguments: args})
	if err != nil || r1.IsError {
		t.Fatalf("first mem_save failed: err=%v isError=%v", err, r1.IsError)
	}

	var resp1 map[string]any
	if err := json.Unmarshal([]byte(r1.Content[0].(*sdkmcp.TextContent).Text), &resp1); err != nil {
		t.Fatalf("first save response not valid JSON: %v", err)
	}
	id1 := resp1["id"]

	// Second save with same topic_key — must create a NEW distinct row (Issue #119).
	args["title"] = "Auth Design v2"
	args["content"] = "Updated version"
	r2, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "mem_save", Arguments: args})
	if err != nil || r2.IsError {
		t.Fatalf("second mem_save failed: err=%v isError=%v", err, r2.IsError)
	}

	var resp2 map[string]any
	if err := json.Unmarshal([]byte(r2.Content[0].(*sdkmcp.TextContent).Text), &resp2); err != nil {
		t.Fatalf("second save response not valid JSON: %v", err)
	}
	id2 := resp2["id"]

	if id1 == id2 {
		t.Errorf("two saves with same topic_key must create distinct rows: id1=%v id2=%v", id1, id2)
	}

	// Both rows should appear in search results.
	searchRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "mem_search",
		Arguments: map[string]any{"query": "Auth Design", "project": "e2e-test"},
	})
	if err != nil || searchRes.IsError {
		t.Fatal("mem_search failed after two saves")
	}

	searchBody := searchRes.Content[0].(*sdkmcp.TextContent).Text
	// The most recent row (v2) must appear in results.
	if !strings.Contains(searchBody, "Auth Design v2") {
		t.Errorf("search result should contain 'Auth Design v2', got: %s", searchBody)
	}
	// Both rows exist — search should return 2 results (v1 and v2).
	if !strings.Contains(searchBody, "2 results") {
		t.Errorf("expected 2 results for two saves with same topic_key, got: %s", searchBody)
	}
}

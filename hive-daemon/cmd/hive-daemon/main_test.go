package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
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

func TestDaemonLifecycleRetryExitsCleanlyAndReleasesDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	seedBlockedDaemonDB(t, dbPath)

	port := reserveLoopbackPort(t)
	cmd := startDaemonForLifecycleRetry(t, dbPath, port)

	requestLifecycleRetry(t, "http://127.0.0.1:"+port+"/governance/project-identity/retry")
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lifecycle retry exit = %v, want clean exit", err)
	}
	// A new managed daemon instance must replan the whole migration against the
	// same database, rather than replacing its live SQLite connection in-process.
	successorPort := reserveLoopbackPort(t)
	successor := startDaemonForLifecycleRetry(t, dbPath, successorPort)
	requestLifecycleRetry(t, "http://127.0.0.1:"+successorPort+"/governance/project-identity/retry")
	if err := successor.Wait(); err != nil {
		t.Fatalf("successor lifecycle retry exit = %v, want clean exit", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open database after lifecycle retry: %v", err)
	}
	defer func() { _ = store.Close() }()
	if err := store.RawDB().Ping(); err != nil {
		t.Fatalf("database remains unavailable after lifecycle retry: %v", err)
	}
}

func startDaemonForLifecycleRetry(t *testing.T, dbPath, port string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binaryPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("daemon stdin: %v", err)
	}
	cmd.Env = append(os.Environ(), "HIVE_DB_PATH="+dbPath, "HIVE_HTTP_PORT="+port)
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd
}

func TestIsCleanServerShutdown(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	for _, tt := range []struct {
		name   string
		ctx    context.Context
		runErr error
		want   bool
	}{
		{name: "retry cancellation is clean", ctx: canceled, runErr: context.Canceled, want: true},
		{name: "unexpected server error remains fatal", ctx: canceled, runErr: errors.New("stdio failed"), want: false},
		{name: "unrequested cancellation remains fatal", ctx: context.Background(), runErr: context.Canceled, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCleanServerShutdown(tt.ctx, tt.runErr); got != tt.want {
				t.Fatalf("isCleanServerShutdown(%v, %v) = %v, want %v", tt.ctx.Err(), tt.runErr, got, tt.want)
			}
		})
	}
}

func seedBlockedDaemonDB(t *testing.T, path string) {
	t.Helper()
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("seed database: %v", err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session', 'Foo', 'dev', 'test')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	for _, projectName := range []string{"Foo", "foo"} {
		if _, err := store.RawDB().Exec(`INSERT INTO mutation_cursors (consumer, project, sequence, event_id) VALUES ('daemon', ?, 7, ?)`, projectName, "event-"+projectName); err != nil {
			t.Fatalf("seed cursor: %v", err)
		}
	}
}

func reserveLoopbackPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:")
}

func requestLifecycleRetry(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Post(endpoint, "application/json", nil)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("lifecycle retry status = %d, want %d", resp.StatusCode, http.StatusAccepted)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("daemon retry endpoint did not become available")
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

func TestRunStartupMigrationExecutesOnceAndExposesReadyGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session', ' Foo.Bar ', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RawDB().Exec(`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES ('memory', ' Foo.Bar ', 'title', 'content', 's')`); err != nil {
		t.Fatal(err)
	}

	gate := runStartupMigration(context.Background(), store, path)
	if err := gate.Check(); err != nil {
		t.Fatalf("startup migration gate = %v", err)
	}
	if status := gate.Status(); status.State != "ready" || status.Reason != "" || status.Continuation != "" {
		t.Fatalf("startup migration status = %+v, want ready status", status)
	}
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	created, err := backups.List(context.Background())
	if err != nil || len(created) != 1 {
		t.Fatalf("migration backups = %v, %v; want one backup", created, err)
	}
	if gate.Status().BackupID != "" {
		t.Fatalf("ready migration status backup = %q, want no rollback reference", gate.Status().BackupID)
	}

	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatalf("repeated startup migration = %v", err)
	}
	created, err = backups.List(context.Background())
	if err != nil || len(created) != 1 {
		t.Fatalf("repeated migration backups = %v, %v; want no new backup", created, err)
	}
}

func TestRunStartupMigrationBlocksAndPersistsContinuationOnFailure(t *testing.T) {
	store := openTestDB(t)
	failure := fmt.Errorf("migration conflict")
	gate := runStartupMigrationWith(context.Background(), store, func(context.Context, db.ProjectMigrationPlan) error {
		return failure
	})
	if err := gate.Check(); err == nil || !strings.Contains(err.Error(), "migration-blocked") {
		t.Fatalf("migration gate error = %v, want migration-blocked", err)
	}
	status := gate.Status()
	if status.State != "migration-blocked" || status.Reason != failure.Error() || status.Continuation != "hive project identity status" {
		t.Fatalf("migration status = %+v, want blocked structured continuation", status)
	}
	warnings, err := store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: "active"})
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0].Message, `"continuation":"hive project identity status"`) || strings.Contains(warnings[0].Message, " then ") {
		t.Fatalf("persisted migration warning = %v, %v", warnings, err)
	}
}

// TestRunStartupMigrationExecutorFailureRetainsDatabaseAndBackup covers the seam
// where a migration already paid for its pre-mutation backup and then aborted
// inside its transaction: the rollback it offers must be the archive bound to that
// exact plan, and the database must be untouched.
//
// It was named "Ambiguity" while its fixture was two cursors sharing a sequence.
// The preflight now reports that collision before any mutation, so the fixture
// moved to a failure the executor can still reach, and the name follows it.
func TestRunStartupMigrationExecutorFailureRetainsDatabaseAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedInTransactionMigrationConflict(t, store)

	gate := runStartupMigration(context.Background(), store, path)
	if err := gate.Check(); err == nil || !strings.Contains(err.Error(), "migration-blocked") {
		t.Fatalf("executor failure gate = %v, want migration-blocked", err)
	}
	status := gate.Status()
	if status.BackupID == "" || status.Continuation != "hive project identity status" {
		t.Fatalf("executor failure status = %+v, want retained backup and continuation", status)
	}
	var sessionCount int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE project = 'foobar'`).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("session rows after blocked startup = %d, %v; want the original row unchanged", sessionCount, err)
	}
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	created, err := backups.List(context.Background())
	if err != nil || len(created) != 1 || created[0].ID != status.BackupID {
		t.Fatalf("blocked migration backup = %v, %v; want status backup", created, err)
	}
	warnings, err := store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: "active"})
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0].Message, `"backup_id":"`+status.BackupID+`"`) {
		t.Fatalf("persisted blocked status = %v, %v; want rollback backup reference", warnings, err)
	}
}

func TestRunStartupMigrationPreflightConflictLeavesDatabaseUnmutatedAndCreatesNoBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedPreflightProjectConflict(t, store)
	gate := runStartupMigration(context.Background(), store, path)
	status := gate.Status()
	if status.State != project.MigrationStateBlocked || status.BackupID != "" {
		t.Fatalf("startup status = %+v, want preflight block before backup", status)
	}
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	if created, err := backups.List(context.Background()); err != nil || len(created) != 0 {
		t.Fatalf("preflight backups = %v, %v; want none before mutation", created, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = db.Open(path)
	if err != nil {
		t.Fatalf("reopen unchanged blocked database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var variants int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM user_prompts WHERE project IN ('Foo.Bar', 'foo-bar')`).Scan(&variants); err != nil || variants != 2 {
		t.Fatalf("prompt rows after blocked restart = %d, %v; want both original variants", variants, err)
	}
}

// seedPreflightProjectConflict writes the two rows that make the PLANNER refuse
// the whole migration, i.e. a conflict the executor is never allowed to reach.
//
// It used to be built out of two divergent sync_state rows; those are merged
// automatically now, so the fixture moved to the collision that is still
// unresolvable: two prompts that share a canonical project and an empty sync_id
// (their inventory identity) while pointing at different rows. Nothing local can
// decide which of two distinct prompts the shared identity means.
func seedPreflightProjectConflict(t *testing.T, store *db.DB) {
	t.Helper()
	for _, projectName := range []string{"Foo.Bar", "foo-bar"} {
		if _, err := store.RawDB().Exec(`INSERT INTO user_prompts (sync_id, project, session_id, content) VALUES ('', ?, 'session', ?)`, projectName, projectName); err != nil {
			t.Fatal(err)
		}
	}
}

// TestProjectIdentityResolveThroughDaemonResolverMergesSyncStateWithoutBackup
// drives resolution through the real resolver closure and the real HTTP recovery
// route, because the preflight-conflict path never creates a migration backup
// and therefore can never present a backup id to authorize resolution: the plan
// fingerprint is the only guard, and it must reject a stale one without touching
// the database.
//
// The blocked plan is built from a prompt-identity collision rather than from
// divergent sync_state rows, which the migration now merges on its own. Resolution
// therefore no longer has to be what unblocks the plan; what it still does, and
// what this locks down, is fold the operator's two spellings into the target they
// named.
func TestProjectIdentityResolveThroughDaemonResolverMergesSyncStateWithoutBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedPreflightProjectConflict(t, store)
	for _, projectName := range []string{"Foo.Bar", "foo-bar"} {
		if _, err := store.RawDB().Exec(`INSERT INTO sync_state (project, last_error) VALUES (?, ?)`, projectName, projectName); err != nil {
			t.Fatal(err)
		}
	}
	gate := runStartupMigration(context.Background(), store, path)
	status := gate.Status()
	if status.State != project.MigrationStateBlocked || status.BackupID != "" || status.PlanFingerprint == "" {
		t.Fatalf("startup status = %+v, want preflight block with plan fingerprint and no backup", status)
	}
	srv := httpapi.NewServer("127.0.0.1:0", store)
	srv.SetMigrationGate(gate)
	srv.SetMigrationIdentityResolver(newMigrationIdentityResolver(store, gate))

	confirmation := project.IdentityResolutionConfirmation("Foo.Bar", "foo-bar")
	stale := postIdentityResolution(t, srv, project.IdentityResolutionRequest{
		SourceProject:   "Foo.Bar",
		TargetProject:   "foo-bar",
		PlanFingerprint: "not-the-plan-the-operator-was-shown",
		Confirmation:    confirmation,
	})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "stale") {
		t.Fatalf("stale fingerprint resolve = %d %s, want 409 stale rejection", stale.Code, stale.Body.String())
	}
	var untouched int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project IN ('Foo.Bar', 'foo-bar')`).Scan(&untouched); err != nil || untouched != 2 {
		t.Fatalf("sync state after rejected resolve = %d, %v; want both variants untouched", untouched, err)
	}

	accepted := postIdentityResolution(t, srv, project.IdentityResolutionRequest{
		SourceProject:   "Foo.Bar",
		TargetProject:   "foo-bar",
		PlanFingerprint: status.PlanFingerprint,
		Confirmation:    confirmation,
	})
	if accepted.Code != http.StatusOK {
		t.Fatalf("resolve through daemon resolver = %d %s, want 200", accepted.Code, accepted.Body.String())
	}
	var resolved int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = 'foo-bar'`).Scan(&resolved); err != nil || resolved != 1 {
		t.Fatalf("sync state after accepted resolve = %d, %v; want one row under the named target", resolved, err)
	}
	var remaining int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = 'Foo.Bar'`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("source spelling after accepted resolve = %d, %v; want it folded away", remaining, err)
	}
	if created, err := governance.NewSQLiteBackupStore(path, "", store.RawDB()).List(context.Background()); err != nil || len(created) != 0 {
		t.Fatalf("backups after resolution = %v, %v; want none for a never-mutating preflight block", created, err)
	}
}

// TestRunStartupMigrationPreflightBlockNeverReportsUnrelatedBackup proves the
// reported backup identifies a backup this migration created, so a rollback can
// never restore an unrelated older database.
func TestRunStartupMigrationPreflightBlockNeverReportsUnrelatedBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	unrelated, err := backups.Create(context.Background())
	if err != nil {
		t.Fatalf("create unrelated backup: %v", err)
	}
	seedPreflightProjectConflict(t, store)
	status := runStartupMigration(context.Background(), store, path).Status()
	if status.State != project.MigrationStateBlocked {
		t.Fatalf("startup status = %+v, want migration-blocked", status)
	}
	if status.BackupID != "" {
		t.Fatalf("reported backup = %q, want empty; unrelated backup %q must never authorize a rollback", status.BackupID, unrelated.ID)
	}
}

func postIdentityResolution(t *testing.T, srv *httpapi.Server, req project.IdentityResolutionRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/governance/project-identity/resolve", bytes.NewReader(body)))
	return rr
}

func TestRunStartupMigrationFreshDatabaseMigratesAndRestartsWithoutRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatalf("fresh startup migration = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = db.Open(path)
	if err != nil {
		t.Fatalf("reopen fresh migrated database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := runStartupMigration(context.Background(), store, path).Check(); err != nil {
		t.Fatalf("fresh database restart migration = %v", err)
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

func TestPendingRestoreThatMayReplayStopsStartup(t *testing.T) {
	replayable := fmt.Errorf("%w: record completed pending restore", governance.ErrPendingRestoreReplayable)
	if err := pendingRestoreStartupError(true, replayable, filepath.Join(t.TempDir(), "memory.db")); !errors.Is(err, governance.ErrPendingRestoreReplayable) {
		t.Fatalf("startup error = %v, want the daemon to refuse to serve", err)
	}
}

func TestPendingRestoreFailureThatLeftLiveDatabaseIntactKeepsServing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	if err := pendingRestoreStartupError(false, errors.New("backup archive is missing"), dbPath); err != nil {
		t.Fatalf("startup error = %v, want startup to continue on an unapplied restore", err)
	}
	if err := pendingRestoreStartupError(true, nil, dbPath); err != nil {
		t.Fatalf("startup error = %v, want startup to continue after a cleared restore", err)
	}
}

// TestPendingRestoreStartupFailureNamesTheFileThatUnblocksTheDaemon covers the
// only exit from this stop: a persistent write failure keeps the request on
// disk, so every following start replays the same restore and fails the same
// way. The daemon must not leave the operator guessing which file to delete.
func TestPendingRestoreStartupFailureNamesTheFileThatUnblocksTheDaemon(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	replayable := fmt.Errorf("%w: record completed pending restore: no space left on device", governance.ErrPendingRestoreReplayable)

	err := pendingRestoreStartupError(true, replayable, dbPath)
	if err == nil {
		t.Fatal("startup error = nil, want the daemon to refuse to serve")
	}
	message := err.Error()
	if !strings.Contains(message, governance.PendingRestorePath(dbPath)) {
		t.Fatalf("startup error = %q, want the pending restore path named", message)
	}
	if !strings.Contains(message, "delete") {
		t.Fatalf("startup error = %q, want the recovery step spelled out", message)
	}
	if !strings.Contains(message, "no space left on device") {
		t.Fatalf("startup error = %q, want the underlying failure kept", message)
	}
}

// TestPendingRestoreStartupFailureDoesNotPromisePermanentReplay covers the
// sub-case where only the completion marker's durability flush failed: the
// rename already put that marker on disk, so the next start short-circuits and
// serves normally. Promising a replay on every start from now on sends the
// operator hunting for a stop that will not happen again.
func TestPendingRestoreStartupFailureDoesNotPromisePermanentReplay(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	replayable := fmt.Errorf("%w: record completed pending restore: sync pending restore dir: input/output error", governance.ErrPendingRestoreReplayable)

	err := pendingRestoreStartupError(true, replayable, dbPath)
	if err == nil {
		t.Fatal("startup error = nil, want the daemon to refuse to serve")
	}
	message := err.Error()
	if strings.Contains(message, "every start from now on") {
		t.Fatalf("startup error = %q, want no claim that every following start replays", message)
	}
	if !strings.Contains(message, "can replay") {
		t.Fatalf("startup error = %q, want the replay reported as possible", message)
	}
}

// TestPendingRestoreStartupFailureNamesAnAbsolutePath keeps the instruction
// usable from any working directory: HIVE_DB_PATH may be relative, and the
// operator reads this line from a log, not from the daemon's shell.
func TestPendingRestoreStartupFailureNamesAnAbsolutePath(t *testing.T) {
	err := pendingRestoreStartupError(true, governance.ErrPendingRestoreReplayable, filepath.Join("relative", "memory.db"))
	if err == nil {
		t.Fatal("startup error = nil, want the daemon to refuse to serve")
	}
	absolute, absErr := filepath.Abs(governance.PendingRestorePath(filepath.Join("relative", "memory.db")))
	if absErr != nil {
		t.Fatal(absErr)
	}
	if !strings.Contains(err.Error(), absolute) {
		t.Fatalf("startup error = %q, want the absolute path %q", err.Error(), absolute)
	}
}

// TestRunStartupMigrationRetriesContentionInsteadOfBlockingTheSession proves a
// concurrent daemon that lost the race is not gated off for its whole session,
// and that the race leaves no error warning behind.
func TestRunStartupMigrationRetriesContentionInsteadOfBlockingTheSession(t *testing.T) {
	store := migratableStore(t)
	attempts := 0
	gate := runStartupMigrationWith(context.Background(), store, func(ctx context.Context, plan db.ProjectMigrationPlan) error {
		attempts++
		if attempts == 1 {
			return db.ErrProjectMigrationPlanStale
		}
		return nil
	})
	if err := gate.Check(); err != nil {
		t.Fatalf("gate after contention retry = %v, want ready", err)
	}
	if attempts != 2 {
		t.Fatalf("execute attempts = %d, want one retry after contention", attempts)
	}
	warnings, err := store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: "active"})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("warnings after contention retry = %v, %v; want none persisted", warnings, err)
	}
}

// TestRunStartupMigrationBlocksWhenContentionDoesNotClear keeps a genuinely
// unresolvable failure a block instead of retrying forever.
func TestRunStartupMigrationBlocksWhenContentionDoesNotClear(t *testing.T) {
	store := migratableStore(t)
	attempts := 0
	gate := runStartupMigrationWith(context.Background(), store, func(ctx context.Context, plan db.ProjectMigrationPlan) error {
		attempts++
		return db.ErrProjectMigrationPlanStale
	})
	if err := gate.Check(); err == nil {
		t.Fatal("gate after repeated contention = ready, want blocked")
	}
	if attempts != 2 {
		t.Fatalf("execute attempts = %d, want exactly one retry", attempts)
	}
}

// TestRunStartupMigrationBlockReportsTheReusedBackupOnEveryRestart covers the
// daemon's real shape: a client spawns a fresh process per session, so a blocked
// migration is re-attempted on every start and reuses the archive it already
// took. The rollback the operator is offered must survive that reuse.
func TestRunStartupMigrationBlockReportsTheReusedBackupOnEveryRestart(t *testing.T) {
	path, store := blockedMigrationStore(t)

	first := runStartupMigration(context.Background(), store, path).Status()
	if first.State != project.MigrationStateBlocked || first.BackupID == "" {
		t.Fatalf("first blocked status = %+v, want a reported rollback backup", first)
	}
	second := runStartupMigration(context.Background(), store, path).Status()
	if second.BackupID != first.BackupID {
		t.Fatalf("restart backup = %q, want the reused archive %q; a validated rollback must not disappear", second.BackupID, first.BackupID)
	}
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	stored, err := backups.List(context.Background())
	if err != nil || len(stored) != 1 {
		t.Fatalf("archives after two blocked starts = %v, %v; want the single reused copy", stored, err)
	}
}

// TestRunStartupMigrationContentionRetryReusesTheBackupItAlreadyTook crosses the
// two seams the existing suites keep apart: the contention retry re-plans, and
// the backup store decides reuse from the plan fingerprint. Re-planning against
// an unchanged database yields the same plan, so the retry must cost no second
// copy of memory.db and must still report the archive it can roll back to.
func TestRunStartupMigrationContentionRetryReusesTheBackupItAlreadyTook(t *testing.T) {
	path, store := blockedMigrationStore(t)
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	attempts := 0

	status := runStartupMigrationWithBackup(context.Background(), store, func(ctx context.Context, plan db.ProjectMigrationPlan) error {
		attempts++
		if attempts == 1 {
			// A peer daemon won the write lock only after this attempt had
			// already paid for its pre-mutation backup.
			if _, err := backups.EnsureTemporaryMigrationBackup(ctx, plan.Fingerprint); err != nil {
				t.Fatalf("first attempt backup: %v", err)
			}
			return db.ErrProjectMigrationPlanStale
		}
		return governance.ExecuteProjectMigrationWithBackup(ctx, store, plan, backups)
	}, backups).Status()

	if attempts != 2 {
		t.Fatalf("execute attempts = %d, want one contention retry", attempts)
	}
	stored, err := backups.List(context.Background())
	if err != nil || len(stored) != 1 {
		t.Fatalf("archives after a contention retry = %v, %v; want one copy of memory.db", stored, err)
	}
	if status.BackupID != stored[0].ID {
		t.Fatalf("reported backup = %q, want the retained archive %q", status.BackupID, stored[0].ID)
	}
}

// TestRunStartupMigrationContentionRetryReportsTheBackupForTheReplannedPlan
// covers the other half of the same cross: when a concurrent writer changes the
// database, the retry plans different work and needs its own pre-mutation
// archive. The earlier one no longer matches the live database, so reporting it
// would offer a rollback onto a state that never existed.
func TestRunStartupMigrationContentionRetryReportsTheBackupForTheReplannedPlan(t *testing.T) {
	path, store := blockedMigrationStore(t)
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	attempts := 0
	var supersededPlan string

	status := runStartupMigrationWithBackup(context.Background(), store, func(ctx context.Context, plan db.ProjectMigrationPlan) error {
		attempts++
		if attempts == 1 {
			supersededPlan = plan.Fingerprint
			if _, err := backups.EnsureTemporaryMigrationBackup(ctx, plan.Fingerprint); err != nil {
				t.Fatalf("first attempt backup: %v", err)
			}
			if _, err := store.RawDB().Exec(`INSERT INTO sync_state (project, last_error) VALUES ('Other.Project', 'x')`); err != nil {
				t.Fatalf("concurrent write: %v", err)
			}
			return db.ErrProjectMigrationPlanStale
		}
		if plan.Fingerprint == supersededPlan {
			t.Fatalf("retry plan = %q, want a replanned migration", plan.Fingerprint)
		}
		return governance.ExecuteProjectMigrationWithBackup(ctx, store, plan, backups)
	}, backups).Status()

	if status.State != project.MigrationStateBlocked || status.BackupID == "" {
		t.Fatalf("status after replanned retry = %+v, want a blocked plan with its own rollback", status)
	}
	superseded, found, err := backups.MigrationBackupForPlan(context.Background(), supersededPlan)
	if err != nil || !found {
		t.Fatalf("superseded archive = %+v, %v; want the abandoned plan's copy retained", superseded, err)
	}
	if status.BackupID == superseded.ID {
		t.Fatalf("reported backup = %q, want the replanned plan's archive, not the superseded one", status.BackupID)
	}
	current, found, err := backups.MigrationBackupForPlan(context.Background(), status.PlanFingerprint)
	if err != nil || !found || current.ID != status.BackupID {
		t.Fatalf("reported backup = %q, want the archive bound to plan %q", status.BackupID, status.PlanFingerprint)
	}
}

// blockedMigrationStore opens a database whose migration plan is executable,
// takes its pre-mutation backup, and then fails inside its transaction.
func blockedMigrationStore(t *testing.T) (string, *db.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedInTransactionMigrationConflict(t, store)
	return path, store
}

// seedInTransactionMigrationConflict writes state whose plan the read-only
// preflight approves and whose mutation the executor still aborts, so the tests
// that need "backup already paid for, then blocked" keep covering that seam.
//
// It used to be built out of two mutation cursors sharing a sequence with
// different event ids. That collision is now reported by the read-only preflight —
// the cursor inventory identity no longer embeds the event id it can disagree on —
// so it can never reach the executor any more.
//
// A missing FTS trigger can: the plan is executable, the executor takes its
// backup, and the schema-ownership rebuild then refuses to rebuild content tables
// whose triggers it cannot recreate faithfully.
func seedInTransactionMigrationConflict(t *testing.T, store *db.DB) {
	t.Helper()
	if _, err := store.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session', 'foobar', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RawDB().Exec(`DROP TRIGGER memories_ad`); err != nil {
		t.Fatal(err)
	}
}

func migratableStore(t *testing.T) *db.DB {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.RawDB().Exec(`INSERT INTO sync_state (project, last_error) VALUES (' Foo.Bar ', 'x')`); err != nil {
		t.Fatal(err)
	}
	return store
}

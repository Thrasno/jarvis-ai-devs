package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// ─── 6.1 Startup ───────────────────────────────────────────────────────────

func TestDaemon_Starts_AndRegisters5Tools(t *testing.T) {
	session := spawnDaemon(t)
	ctx := context.Background()

	var toolNames []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error: %v", err)
		}
		toolNames = append(toolNames, tool.Name)
	}

	if len(toolNames) != 6 {
		t.Errorf("expected 6 tools, got %d: %v", len(toolNames), toolNames)
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

func TestE2E_TopicKeyUpsert(t *testing.T) {
	session := spawnDaemon(t)
	ctx := context.Background()

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

	// Second save with same topic_key — should upsert
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

	if id1 != id2 {
		t.Errorf("topic_key upsert should return same id: id1=%v id2=%v", id1, id2)
	}

	// Verify only 1 result in search
	searchRes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "mem_search",
		Arguments: map[string]any{"query": "Auth Design", "project": "e2e-test"},
	})
	if err != nil || searchRes.IsError {
		t.Fatal("mem_search failed after upsert")
	}

	// mem_search now returns markdown, not JSON
	searchBody := searchRes.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(searchBody, "Auth Design v2") {
		t.Errorf("search result should contain 'Auth Design v2' after upsert, got: %s", searchBody)
	}
	// Verify upsert worked: only 1 result should appear (1 result footer)
	if strings.Contains(searchBody, "2 results") {
		t.Errorf("upsert should leave only 1 result, but got 2 in response: %s", searchBody)
	}
}

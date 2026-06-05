package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClaudePowerShellPromptHook_PostsPromptPayload(t *testing.T) {
	powershell := requirePowerShell(t)
	scriptPath := claudePowerShellHookScriptPath(t)
	ensureFirstPromptDone(t, scriptPath)

	received := make(chan string, 1)
	server, port := startPromptCaptureServer(t, received)
	defer server.Shutdown(context.Background())

	cmd := powerShellHookCommand(t, powershell, scriptPath)
	cmd.Env = append(cmd.Environ(), "HIVE_HTTP_PORT="+port)
	cmd.Stdin = strings.NewReader(`{"prompt":"capture this Windows prompt"}`)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("PowerShell hook failed: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "{}" {
		t.Fatalf("stdout = %q, want {}", stdout.String())
	}

	select {
	case body := <-received:
		var payload map[string]string
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("POST body is not JSON: %v; body=%q", err, body)
		}
		if payload["content"] != "capture this Windows prompt" {
			t.Fatalf("POST content = %q, want %q", payload["content"], "capture this Windows prompt")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("PowerShell hook did not POST prompt payload to local endpoint")
	}
}

func TestClaudePowerShellPromptHook_RequestFailurePrintsEmptyJSONAndExitsZero(t *testing.T) {
	powershell := requirePowerShell(t)
	scriptPath := claudePowerShellHookScriptPath(t)
	ensureFirstPromptDone(t, scriptPath)
	port := unusedLocalPort(t)

	cmd := powerShellHookCommand(t, powershell, scriptPath)
	cmd.Env = append(cmd.Environ(), "HIVE_HTTP_PORT="+port)
	cmd.Stdin = strings.NewReader(`{"prompt":"endpoint is down"}`)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("PowerShell hook should exit 0 when endpoint is unavailable, got: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "{}" {
		t.Fatalf("stdout = %q, want {}", stdout.String())
	}
}

func TestClaudePowerShellPromptHook_PostRunsInBackground(t *testing.T) {
	script, err := os.ReadFile(claudePowerShellHookScriptPath(t))
	if err != nil {
		t.Fatalf("read PowerShell hook: %v", err)
	}

	text := string(script)
	if !strings.Contains(text, "[Diagnostics.Process]::Start") {
		t.Fatal("PowerShell hook must dispatch prompt POST in a detached background process")
	}
	if !strings.Contains(text, "$processInfo.UseShellExecute = $true") {
		t.Fatal("PowerShell hook worker must not inherit captured stdio handles")
	}
	if !strings.Contains(text, "Invoke-RestMethod -Uri `$TargetUri") {
		t.Fatal("background process should receive the target URI instead of posting from the foreground hook")
	}
	if strings.Contains(text, "Invoke-RestMethod -Uri $uri") {
		t.Fatal("PowerShell hook must not synchronously POST from the foreground hook")
	}
	if !strings.Contains(text, "-TimeoutSec 1") {
		t.Fatal("background prompt POST should retain a short timeout")
	}
}

func TestClaudePowerShellPromptHook_ResolvesFallbackPowerShellDynamically(t *testing.T) {
	script, err := os.ReadFile(claudePowerShellHookScriptPath(t))
	if err != nil {
		t.Fatalf("read PowerShell hook: %v", err)
	}

	text := string(script)
	legacyExecutable := "powershell" + ".exe"
	if strings.Contains(strings.ToLower(text), legacyExecutable) {
		t.Fatalf("PowerShell hook must not hardcode %s", legacyExecutable)
	}
	if !strings.Contains(text, "function Resolve-PowerShellExecutable") {
		t.Fatal("PowerShell hook should centralize executable resolution")
	}
	if !strings.Contains(text, "(Get-Process -Id $PID).Path") {
		t.Fatal("PowerShell hook should prefer the current PowerShell executable")
	}
	if !strings.Contains(text, "foreach ($candidate in @('pwsh', 'powershell'))") {
		t.Fatal("PowerShell hook should resolve pwsh/powershell dynamically")
	}
	if !strings.Contains(text, "Get-Command -Name $candidate") {
		t.Fatal("PowerShell hook should use Get-Command for fallback executable lookup")
	}
	if !strings.Contains(text, "Start-Process -FilePath $powerShellPath") {
		t.Fatal("PowerShell hook fallback should start the dynamically resolved executable")
	}
}

func TestClaudePowerShellPromptHook_DoesNotBlockWhenWorkerHTTPStalls(t *testing.T) {
	powershell := requirePowerShell(t)
	scriptPath := claudePowerShellHookScriptPath(t)
	ensureFirstPromptDone(t, scriptPath)
	_, port := startStallingPromptServer(t, 3*time.Second)

	cmd := powerShellHookCommand(t, powershell, scriptPath)
	cmd.Env = append(cmd.Environ(), "HIVE_HTTP_PORT="+port)
	cmd.Stdin = strings.NewReader(`{"prompt":"do not block on stalled worker"}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	completed := make(chan error, 1)
	go func() { completed <- cmd.Run() }()

	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("PowerShell hook should exit 0 when worker stalls, got: %v; stderr=%q", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		t.Fatalf("PowerShell hook did not complete while worker HTTP request was stalled; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "{}" {
		t.Fatalf("stdout = %q, want {}", stdout.String())
	}
}

func TestClaudePowerShellPromptHook_FirstPromptInjectsSystemMessage(t *testing.T) {
	powershell := requirePowerShell(t)
	scriptPath := claudePowerShellHookScriptPath(t)

	stateFile := claudePowerShellSessionStateFile(t, "powershell-first-prompt-test")
	// Ensure the state file does not exist before the test.
	_ = os.Remove(stateFile)
	t.Cleanup(func() {
		if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
			t.Logf("first-prompt test cleanup: %v", err)
		}
	})

	port := unusedLocalPort(t)
	cmd := powerShellHookCommand(t, powershell, scriptPath)
	cmd.Env = append(cmd.Environ(), "HIVE_HTTP_PORT="+port, "HIVE_CLAUDE_SESSION_ID=powershell-first-prompt-test")
	cmd.Stdin = strings.NewReader(`{"prompt":"first user prompt"}`)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("PowerShell hook failed on first prompt: %v", err)
	}

	out := strings.TrimSpace(stdout.String())

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v; stdout=%q", err, out)
	}

	if _, ok := result["systemMessage"]; !ok {
		t.Fatalf("stdout missing 'systemMessage' key on first prompt; got %q", out)
	}

	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("session-scoped first prompt marker was not created after first prompt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(scriptPath), ".first-prompt-done")); !os.IsNotExist(err) {
		t.Fatalf("script-local .first-prompt-done marker should not exist, stat error: %v", err)
	}
}

func TestClaudePowerShellHooks_NoMetadataFallbackUsesParentProcessMarker(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("parent-process fallback uses Win32_Process and is only stable on Windows")
	}

	powershell := requirePowerShell(t)
	sessionStartPath := claudePowerShellHookScriptPath(t, "session-start.ps1")
	userPromptPath := claudePowerShellHookScriptPath(t, "user-prompt-submit.ps1")
	sessionStopPath := claudePowerShellHookScriptPath(t, "session-stop.ps1")

	stateFile := claudePowerShellSessionStateFile(t, "ppid-"+strconv.Itoa(os.Getpid()))
	_ = os.Remove(stateFile)
	t.Cleanup(func() {
		if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
			t.Logf("parent-process fallback cleanup: %v", err)
		}
	})

	startCmd := powerShellHookCommand(t, powershell, sessionStartPath)
	startCmd.Stdin = strings.NewReader(`{}`)
	if output, err := startCmd.CombinedOutput(); err != nil {
		t.Fatalf("SessionStart hook failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("SessionStart did not create parent-process fallback marker %q: %v", stateFile, err)
	}

	promptCmd := powerShellHookCommand(t, powershell, userPromptPath)
	promptCmd.Env = append(promptCmd.Environ(), "HIVE_HTTP_PORT="+unusedLocalPort(t))
	promptCmd.Stdin = strings.NewReader(`{"prompt":"same fallback session"}`)
	var promptStdout bytes.Buffer
	promptCmd.Stdout = &promptStdout
	if err := promptCmd.Run(); err != nil {
		t.Fatalf("UserPromptSubmit hook failed: %v", err)
	}
	if strings.TrimSpace(promptStdout.String()) != "{}" {
		t.Fatalf("UserPromptSubmit did not reuse parent-process fallback marker; stdout=%q", promptStdout.String())
	}

	stopCmd := powerShellHookCommand(t, powershell, sessionStopPath)
	stopCmd.Stdin = strings.NewReader(`{}`)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		t.Fatalf("Stop hook failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("Stop did not remove parent-process fallback marker, stat error: %v", err)
	}
}

func TestClaudePowerShellHooks_NoMetadataFallbackIsNotCurrentPIDOnly(t *testing.T) {
	for _, script := range []string{"session-start.ps1", "session-stop.ps1", "user-prompt-submit.ps1"} {
		t.Run(script, func(t *testing.T) {
			content, err := os.ReadFile(claudePowerShellHookScriptPath(t, script))
			if err != nil {
				t.Fatalf("read hook: %v", err)
			}
			text := string(content)
			if !strings.Contains(text, "ParentProcessId") {
				t.Fatalf("%s should prefer the parent process for no-metadata fallback", script)
			}
			resolveSessionID := text
			if idx := strings.Index(text, "function Resolve-HiveFallbackSessionId"); idx >= 0 {
				resolveSessionID = text[:idx]
			}
			if strings.Contains(resolveSessionID, "return \"ppid-$PID\"") {
				t.Fatalf("%s should not use current PID as the immediate no-metadata fallback", script)
			}
		})
	}
}

func requirePowerShell(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{"pwsh", "powershell"} {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path
		}
	}
	t.Skip("PowerShell runtime unavailable; skipping gated .ps1 hook runtime test")
	return ""
}

func claudePowerShellHookScriptPath(t *testing.T, scripts ...string) string {
	t.Helper()
	script := "user-prompt-submit.ps1"
	if len(scripts) > 0 {
		script = scripts[0]
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "embed", "hooks", "claude", script))
}

func powerShellHookCommand(t *testing.T, powershell, scriptPath string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
}

func startPromptCaptureServer(t *testing.T, received chan<- string) (*http.Server, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on local prompt capture endpoint: %v", err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if r.URL.Path != "/prompts" {
				t.Errorf("path = %s, want /prompts", r.URL.Path)
			}
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(r.Body); err != nil {
				t.Errorf("read request body: %v", err)
			}
			received <- buf.String()
			w.WriteHeader(http.StatusAccepted)
		}),
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("prompt capture server: %v", err)
		}
	}()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address %q: %v", listener.Addr().String(), err)
	}
	return server, port
}

func startStallingPromptServer(t *testing.T, stall time.Duration) (*http.Server, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on local stalling prompt endpoint: %v", err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(stall)
			w.WriteHeader(http.StatusAccepted)
		}),
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("stalling prompt server: %v", err)
		}
	}()
	t.Cleanup(func() { _ = server.Close() })

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address %q: %v", listener.Addr().String(), err)
	}
	return server, port
}

func unusedLocalPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve unused local port: %v", err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatalf("close unused local port listener: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("split unused local port: %v", err)
	}
	return port
}

// ensureFirstPromptDone creates the session-scoped state file so runtime tests
// that exercise POST / non-blocking behavior are not affected by the first-prompt
// injection path. The file is removed by t.Cleanup so it does not linger after
// the test run.
func ensureFirstPromptDone(t *testing.T, scriptPath string) {
	t.Helper()
	sessionID := "powershell-prompt-hook-test"
	t.Setenv("HIVE_CLAUDE_SESSION_ID", sessionID)
	stateFile := claudePowerShellSessionStateFile(t, sessionID)
	if err := os.WriteFile(stateFile, []byte("test"), 0644); err != nil {
		t.Fatalf("ensureFirstPromptDone: write state file: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
			t.Logf("ensureFirstPromptDone cleanup: %v", err)
		}
	})
}

func claudePowerShellSessionStateFile(t *testing.T, sessionID string) string {
	t.Helper()
	stateRoot := filepath.Join(os.TempDir(), "jarvis-hive", "claude-hooks")
	if err := os.MkdirAll(stateRoot, 0755); err != nil {
		t.Fatalf("create state root: %v", err)
	}
	safeSessionID := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, sessionID)
	if len(safeSessionID) > 160 {
		safeSessionID = safeSessionID[:160]
	}
	if strings.TrimSpace(safeSessionID) == "" {
		safeSessionID = "unknown"
	}
	return filepath.Join(stateRoot, "first-prompt-"+safeSessionID+".done")
}

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClaudePowerShellPromptHook_PostsPromptPayload(t *testing.T) {
	powershell := requirePowerShell(t)
	scriptPath := claudePowerShellHookScriptPath(t)

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
	case <-time.After(3 * time.Second):
		t.Fatal("PowerShell hook did not POST prompt payload to local endpoint")
	}
}

func TestClaudePowerShellPromptHook_RequestFailurePrintsEmptyJSONAndExitsZero(t *testing.T) {
	powershell := requirePowerShell(t)
	scriptPath := claudePowerShellHookScriptPath(t)
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

func claudePowerShellHookScriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "embed", "hooks", "claude", "user-prompt-submit.ps1"))
}

func powerShellHookCommand(t *testing.T, powershell, scriptPath string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

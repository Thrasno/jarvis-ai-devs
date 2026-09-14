package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenCodeHiveTemplate_DeclaresBoundedLifecycleContract(t *testing.T) {
	source := readOpenCodeHiveTemplate(t)
	for _, required := range []string{
		`"session.created"`,
		`"session.deleted"`,
		"SESSION_START: `${HIVE_URL}/sessions`,",
		`/sessions/${encodeURIComponent(id)}/end`,
		`dev_id`,
		`client: HIVE_CLIENT`,
		`createdFlights`,
		`finally`,
		`AbortSignal.timeout(1000)`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("OpenCode Hive template missing lifecycle contract %q", required)
		}
	}
	for _, forbidden := range []string{"ppid-", "process.ppid", "Core", "autostart", "idempotency"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("OpenCode Hive template contains deferred lifecycle behavior %q", forbidden)
		}
	}
}

func TestOpenCodeHiveTemplate_CoalescesCreatedAndKeepsPromptIndependent(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Node to execute the source-of-truth OpenCode template")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is unavailable: source execution test skipped")
	}

	startRequests := make(chan hiveTemplateRequest, 2)
	promptRequests := make(chan hiveTemplateRequest, 1)
	firstStartCanceled := make(chan time.Time, 1)
	releaseFirstStart := make(chan struct{})
	var starts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/project-identity/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/sessions":
			startRequests <- readHiveTemplateRequest(r)
			if starts.Add(1) == 1 {
				select {
				case <-r.Context().Done():
					firstStartCanceled <- time.Now()
				case <-releaseFirstStart:
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/prompts":
			promptRequests <- readHiveTemplateRequest(r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		close(releaseFirstStart)
		server.Close()
	}()

	port := hiveTemplatePort(t, server.URL)
	runner := filepath.Join(t.TempDir(), "run-hive-template.mjs")
	if err := os.WriteFile(runner, []byte(`
import { pathToFileURL } from "node:url";
const { Hive } = await import(pathToFileURL(process.argv[2]).href);
const plugin = await Hive();
const originalFetch = globalThis.fetch;
globalThis.fetch = () => { throw new Error("synchronous setup failure"); };
plugin["event"]({ event: { type: "session.created", properties: { id: "synchronous-fetch-error" } } });
globalThis.fetch = originalFetch;
const evidence = { id: "session-42", project: "jarvis-dev", directory: "/workspace/jarvis-dev" };
const created = { event: { type: "session.created", properties: evidence } };
plugin["event"](created);
plugin["event"](created);
await plugin["chat.message"](evidence, { parts: [
  { type: "text", text: "  capture  " },
  { type: "tool", text: "ignored" },
  { type: "text", text: "  this prompt  " },
] });
await new Promise((resolve) => setTimeout(resolve, 1100));
plugin["event"](created);
await new Promise((resolve) => setTimeout(resolve, 100));
`), 0600); err != nil {
		t.Fatalf("write Node runner: %v", err)
	}

	cmd := exec.Command("node", "--experimental-strip-types", runner, filepath.Join("..", "..", "embed", "hooks", "opencode", "hive.ts"))
	cmd.Env = append(os.Environ(), "HIVE_HTTP_PORT="+port, "HIVE_DEV_ID=developer-7", "HIVE_OPENCODE_SESSION_ID=", "OPENCODE_SESSION_ID=", "SESSION_ID=", "HIVE_PROJECT=", "JARVIS_PROJECT=", "HIVE_PROJECT_DIRECTORY=", "JARVIS_WORKSPACE_DIRECTORY=")
	finished := make(chan error, 1)
	go func() { finished <- cmd.Run() }()

	firstStart := waitHiveTemplateRequest(t, startRequests, "first session start")
	prompt := waitHiveTemplateRequest(t, promptRequests, "prompt while the start request is held")
	assertNoHiveTemplateRequest(t, startRequests, 200*time.Millisecond, "an immediate duplicate session start")
	firstCanceled := waitHiveTemplateTime(t, firstStartCanceled, "first start timeout cancellation")
	secondStart := waitHiveTemplateRequest(t, startRequests, "retry after start timeout")
	if secondStart.observedAt.Before(firstCanceled) {
		t.Fatalf("second start observed at %s before first timeout cleanup at %s", secondStart.observedAt, firstCanceled)
	}
	if err := <-finished; err != nil {
		t.Fatalf("execute OpenCode Hive template: %v", err)
	}

	if starts.Load() != 2 {
		t.Fatalf("session start count = %d, want exactly 2", starts.Load())
	}
	for _, request := range []hiveTemplateRequest{firstStart, secondStart} {
		if request.method != http.MethodPost || request.contentType != "application/json" {
			t.Fatalf("session start request = %s content-type %q, want POST application/json", request.method, request.contentType)
		}
		assertHiveTemplateJSON(t, request, map[string]string{
			"id": "session-42", "project": "jarvis-dev", "directory": "/workspace/jarvis-dev", "dev_id": "developer-7", "client": "opencode",
		})
	}
	if prompt.method != http.MethodPost || prompt.contentType != "application/json" {
		t.Fatalf("prompt request = %s content-type %q, want POST application/json", prompt.method, prompt.contentType)
	}
	assertHiveTemplateJSON(t, prompt, map[string]string{
		"content": "capture  \n  this prompt", "session_id": "session-42", "project": "jarvis-dev", "directory": "/workspace/jarvis-dev", "client": "opencode",
	})
}

func TestOpenCodeHiveTemplate_EndsDeletedSessionFromGenericEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Node to execute the source-of-truth OpenCode template")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is unavailable: source execution test skipped")
	}

	createdRequests := make(chan hiveTemplateRequest, 1)
	endRequests := make(chan hiveTemplateRequest, 2)
	firstEndCanceled := make(chan time.Time, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/project-identity/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/sessions":
			createdRequests <- readHiveTemplateRequest(r)
			<-r.Context().Done()
		case "/sessions/session /with?reserved%chars/end":
			endRequests <- readHiveTemplateRequest(r)
			select {
			case <-r.Context().Done():
				firstEndCanceled <- time.Now()
			case <-time.After(3 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	port := hiveTemplatePort(t, server.URL)
	runner := filepath.Join(t.TempDir(), "run-hive-deletion-template.mjs")
	if err := os.WriteFile(runner, []byte(`
import { pathToFileURL } from "node:url";
const { Hive } = await import(pathToFileURL(process.argv[2]).href);
const plugin = await Hive();
const created = { event: { type: "session.created", properties: { id: "created-pending" } } };
const deleted = { event: { type: "session.deleted", properties: { info: {
  id: "session /with?reserved%chars", project: "jarvis-dev", directory: "/workspace/jarvis-dev", summary: "finished"
} } } };
plugin["event"](created);
plugin["event"](deleted);
await new Promise((resolve) => setTimeout(resolve, 1100));
plugin["event"](deleted);
const originalFetch = globalThis.fetch;
globalThis.fetch = () => { throw new Error("synchronous end setup failure"); };
plugin["event"]({ event: { type: "session.deleted", properties: { info: { id: "unavailable-end" } } } });
globalThis.fetch = originalFetch;
plugin["event"]({ event: { type: "session.deleted", properties: { info: {} } } });
await new Promise((resolve) => setTimeout(resolve, 100));
`), 0600); err != nil {
		t.Fatalf("write Node deletion runner: %v", err)
	}

	cmd := exec.Command("node", "--experimental-strip-types", runner, filepath.Join("..", "..", "embed", "hooks", "opencode", "hive.ts"))
	cmd.Env = append(os.Environ(), "HIVE_HTTP_PORT="+port, "HIVE_OPENCODE_SESSION_ID=", "OPENCODE_SESSION_ID=", "SESSION_ID=")
	finished := make(chan error, 1)
	go func() { finished <- cmd.Run() }()

	_ = waitHiveTemplateRequest(t, createdRequests, "pending session creation")
	firstEnd := waitHiveTemplateRequest(t, endRequests, "first deleted-session end")
	firstCanceled := waitHiveTemplateTime(t, firstEndCanceled, "first end timeout cancellation")
	secondEnd := waitHiveTemplateRequest(t, endRequests, "repeat deleted-session end")
	if secondEnd.observedAt.Before(firstCanceled) {
		t.Fatalf("repeat end observed at %s before first timeout cleanup at %s", secondEnd.observedAt, firstCanceled)
	}
	if err := <-finished; err != nil {
		t.Fatalf("execute OpenCode Hive deletion template: %v", err)
	}
	assertNoHiveTemplateRequest(t, endRequests, 200*time.Millisecond, "session end without an event ID")

	for _, request := range []hiveTemplateRequest{firstEnd, secondEnd} {
		if request.method != http.MethodPost || request.contentType != "application/json" {
			t.Fatalf("session end request = %s content-type %q, want POST application/json", request.method, request.contentType)
		}
		if request.requestURI != "/sessions/session%20%2Fwith%3Freserved%25chars/end" {
			t.Fatalf("session end request URI = %q, want encoded ID", request.requestURI)
		}
		assertHiveTemplateJSON(t, request, map[string]string{
			"project": "jarvis-dev", "directory": "/workspace/jarvis-dev", "summary": "finished", "client": "opencode",
		})
	}
}

func readOpenCodeHiveTemplate(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "embed", "hooks", "opencode", "hive.ts"))
	if err != nil {
		t.Fatalf("read source-of-truth OpenCode Hive template: %v", err)
	}
	return string(content)
}

func hiveTemplatePort(t *testing.T, serverURL string) string {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return parsed.Port()
}

type hiveTemplateRequest struct {
	method      string
	contentType string
	path        string
	requestURI  string
	body        []byte
	err         error
	observedAt  time.Time
}

func readHiveTemplateRequest(request *http.Request) hiveTemplateRequest {
	body, err := io.ReadAll(request.Body)
	return hiveTemplateRequest{
		method:      request.Method,
		contentType: request.Header.Get("Content-Type"),
		path:        request.URL.Path,
		requestURI:  request.RequestURI,
		body:        body,
		err:         err,
		observedAt:  time.Now(),
	}
}

const hiveTemplateRequestWait = 10 * time.Second

func waitHiveTemplateRequest(t *testing.T, requests <-chan hiveTemplateRequest, description string) hiveTemplateRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(hiveTemplateRequestWait):
		t.Fatalf("timed out waiting for %s", description)
		return hiveTemplateRequest{}
	}
}

func assertNoHiveTemplateRequest(t *testing.T, requests <-chan hiveTemplateRequest, duration time.Duration, description string) {
	t.Helper()
	select {
	case request := <-requests:
		t.Fatalf("received %s at %s", description, request.observedAt)
	case <-time.After(duration):
	}
}

func waitHiveTemplateTime(t *testing.T, times <-chan time.Time, description string) time.Time {
	t.Helper()
	select {
	case observedAt := <-times:
		return observedAt
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		return time.Time{}
	}
}

func assertHiveTemplateJSON(t *testing.T, request hiveTemplateRequest, want map[string]string) {
	t.Helper()
	if request.err != nil {
		t.Fatalf("read %s request: %v", request.path, request.err)
	}
	var got map[string]string
	if err := json.Unmarshal(request.body, &got); err != nil {
		t.Fatalf("decode %s request: %v", request.path, err)
	}
	if len(got) != len(want) {
		t.Fatalf("%s body = %#v, want exactly %#v", request.path, got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s body[%q] = %q, want %q; body=%#v", request.path, key, got[key], value, got)
		}
	}
}

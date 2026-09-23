package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenCodeHiveTemplate_ReportsEarlyChildExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOpenCodeHiveTemplate_ChildExitFixture$")
	cmd.Env = append(os.Environ(), "HIVE_TEST_CHILD_EXIT=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	finished := make(chan error, 1)
	go func() { finished <- cmd.Run() }()
	_, err := awaitHiveTemplateRequest(make(chan hiveTemplateRequest), finished, cancel, &stderr, "initial session start", hiveTemplateRequestWait)
	if err == nil || !strings.Contains(err.Error(), "exit status 7") || !strings.Contains(err.Error(), "fixture failed before request") {
		t.Fatalf("early exit diagnostic = %v, want exit status and stderr", err)
	}
}

func TestOpenCodeHiveTemplate_AcceptsCompletedChild(t *testing.T) {
	finished := make(chan error, 1)
	finished <- nil
	var stderr bytes.Buffer
	_, err := awaitHiveTemplateRequest(nil, finished, func() {}, &stderr, "child completion", time.Second)
	if err != nil {
		t.Fatalf("completed child = %v, want success", err)
	}
}

func TestOpenCodeHiveTemplate_BoundsFinalChildCompletion(t *testing.T) {
	finished := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stderr bytes.Buffer
	go func() {
		<-ctx.Done()
		finished <- fmt.Errorf("canceled pending child")
	}()
	_, err := awaitHiveTemplateRequest(nil, finished, cancel, &stderr, "child completion", 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for child completion") {
		t.Fatalf("final completion = %v, want bounded cancellation", err)
	}
}

func TestOpenCodeHiveTemplate_TimeoutBeforeChildStarts(t *testing.T) {
	finished := make(chan error, 1)
	var stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := awaitHiveTemplateRequest(make(chan hiveTemplateRequest), finished, cancel, &stderr, "startup request", 0)
	if ctx.Err() != context.Canceled {
		t.Fatalf("startup context = %v, want cancellation", ctx.Err())
	}
	if err == nil || !strings.Contains(err.Error(), "startup request") {
		t.Fatalf("startup timeout diagnostic = %v, want bounded error", err)
	}
}

func TestOpenCodeHiveTemplate_TerminatesChildOnMissingRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOpenCodeHiveTemplate_ChildExitFixture$")
	cmd.Env = append(os.Environ(), "HIVE_TEST_CHILD_WAIT=1")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	cmd.Stdout = writer
	ready := make(chan error, 1)
	go func() {
		var marker [1]byte
		_, err := io.ReadFull(reader, marker[:])
		ready <- err
	}()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	finished := make(chan error, 1)
	go func() { finished <- cmd.Run() }()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("fixture startup: %v", err)
		}
	case <-time.After(hiveTemplateRequestWait):
		cancel()
		t.Fatal("timed out waiting for fixture startup")
	}
	_, err = awaitHiveTemplateRequest(make(chan hiveTemplateRequest), finished, cancel, &stderr, "missing request", 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for missing request") || !strings.Contains(err.Error(), "canceled after deadline") {
		t.Fatalf("missing request diagnostic = %v, want bounded timeout and child termination", err)
	}
}

func TestOpenCodeHiveTemplate_TimeoutReportsStageAndCancellation(t *testing.T) {
	finished := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		finished <- fmt.Errorf("exit status 1")
	}()
	var stderr bytes.Buffer
	_, err := awaitHiveTemplateRequest(make(chan hiveTemplateRequest), finished, cancel, &stderr, "startup request", 0, func() string {
		return "plugin ready; governance requests=1; session requests=0"
	})
	if err == nil || !strings.Contains(err.Error(), "canceled after deadline") || !strings.Contains(err.Error(), "plugin ready; governance requests=1; session requests=0") || strings.Contains(err.Error(), "child terminated:") {
		t.Fatalf("timeout attribution and stage = %v", err)
	}
}

func TestOpenCodeHiveTemplate_EarlyExitReportsStageWithoutCancellation(t *testing.T) {
	finished := make(chan error, 1)
	finished <- fmt.Errorf("exit status 7")
	var stderr bytes.Buffer
	canceled := false
	_, err := awaitHiveTemplateRequest(make(chan hiveTemplateRequest), finished, func() { canceled = true }, &stderr, "startup request", time.Hour, func() string {
		return "import started; governance requests=0; session requests=0"
	})
	if canceled || err == nil || !strings.Contains(err.Error(), "child exited before startup request") || !strings.Contains(err.Error(), "import started; governance requests=0; session requests=0") || strings.Contains(err.Error(), "canceled after deadline") {
		t.Fatalf("early exit attribution and stage = %v; canceled=%t", err, canceled)
	}
}

func TestOpenCodeHiveTemplate_ChildExitFixture(t *testing.T) {
	if os.Getenv("HIVE_TEST_CHILD_WAIT") == "1" {
		_, _ = os.Stdout.WriteString("R")
		time.Sleep(5 * time.Second)
		return
	}
	if os.Getenv("HIVE_TEST_CHILD_EXIT") != "1" {
		return
	}
	_, _ = os.Stderr.WriteString("fixture failed before request\n")
	os.Exit(7)
}

func TestOpenCodeHiveTemplate_DeclaresBoundedLifecycleContract(t *testing.T) {
	source := readOpenCodeHiveTemplate(t)
	for _, required := range []string{
		`"session.created"`,
		`"session.deleted"`,
		"SESSION_START: `${HIVE_URL}/sessions`,",
		`/sessions/${encodeURIComponent(notification.id)}/end`,
		`properties["info"]`,
		`client: HIVE_CLIENT`,
		`createdFlights`,
		`finally`,
		`AbortSignal.timeout(1000)`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("OpenCode Hive template missing lifecycle contract %q", required)
		}
	}
	for _, forbidden := range []string{"Core", "autostart", "idempotency"} {
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
	promptRequests := make(chan hiveTemplateRequest, 3)
	primaryStartCanceled := make(chan time.Time, 1)
	releasePrimaryStart := make(chan struct{})
	var primaryStartHeld atomic.Bool
	var starts atomic.Int32
	var governance atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/project-identity/status":
			governance.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/sessions":
			request := readHiveTemplateRequest(r)
			startRequests <- request
			starts.Add(1)
			if isPrimaryOpenCodeStart(request) && primaryStartHeld.CompareAndSwap(false, true) {
				select {
				case <-r.Context().Done():
					primaryStartCanceled <- time.Now()
				case <-releasePrimaryStart:
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
		close(releasePrimaryStart)
		server.Close()
	}()

	port := hiveTemplatePort(t, server.URL)
	runner := filepath.Join(t.TempDir(), "run-hive-template.mjs")
	if err := os.WriteFile(runner, []byte(`
import { pathToFileURL } from "node:url";
console.log("HIVE_STAGE:import started");
const { Hive } = await import(pathToFileURL(process.argv[2]).href);
console.log("HIVE_STAGE:import complete");
const plugin = await Hive();
console.log("HIVE_STAGE:plugin ready");
process.env.PWD = "";
const originalFetch = globalThis.fetch;
const unhandled = [];
const onUnhandled = (reason) => unhandled.push(reason);
process.on("unhandledRejection", onUnhandled);
let synchronousCreatedFetches = 0;
globalThis.fetch = (url) => {
  if (String(url).endsWith("/sessions")) synchronousCreatedFetches += 1;
  throw new Error("synchronous setup failure");
};
const synchronousCreated = { event: { type: "session.created", properties: { info: {
  id: "synchronous-fetch-error", project: "jarvis-dev", directory: "/workspace/jarvis-dev"
} } } };
if (plugin["event"](synchronousCreated) !== undefined) throw new Error("event callback awaited synchronous lifecycle failure");
queueMicrotask(() => { globalThis.fetch = originalFetch; });
await Promise.resolve();
if (synchronousCreatedFetches !== 1) throw new Error("synchronous lifecycle fetches = " + synchronousCreatedFetches);
if (unhandled.length !== 0) throw new Error("synchronous lifecycle failure was unhandled");
const evidence = { id: "session-42", project: "jarvis-dev", directory: "/workspace/jarvis-dev" };
const created = { event: { type: "session.created", id: "envelope-id", properties: { id: "unrelated-id", info: evidence } } };
if (plugin["event"](created) !== undefined) throw new Error("event callback awaited lifecycle delivery");
console.log("HIVE_STAGE:created event dispatched");
plugin["event"](created);
plugin["event"]({ event: { type: "session.created", properties: { info: {
  id: "session-42", project: "other-project", directory: "/workspace/other-project"
} } } });
process.env.HIVE_OPENCODE_SESSION_ID = "environment-session";
plugin["event"]({ event: { type: "session.created", properties: { info: {
  project: "environment-project", directory: "/environment-directory"
} } } });
process.env.HIVE_OPENCODE_SESSION_ID = "";
process.env.HIVE_PROJECT = "prompt-project";
process.env.HIVE_PROJECT_DIRECTORY = "/prompt-directory";
process.env.HIVE_OPENCODE_SESSION_ID = "prompt-session";
await plugin["chat.message"](evidence, { parts: [
  { type: "text", text: "  capture  " },
  { type: "tool", text: "ignored" },
  { type: "text", text: "  this prompt  " },
] });
process.env.HIVE_PROJECT = "";
process.env.HIVE_PROJECT_DIRECTORY = "";
process.env.HIVE_OPENCODE_SESSION_ID = "";
await plugin["chat.message"]({}, { parts: [{ type: "text", text: "fallback prompt" }] });
await plugin["chat.message"]({}, { parts: [{ type: "text", text: 42 }] });
await plugin["chat.message"]({}, { parts: {} });
const originalCwd = process.cwd;
process.cwd = () => "";
plugin["event"]({ event: { type: "session.created", properties: { info: { id: "without-evidence" } } } });
process.cwd = originalCwd;
await new Promise((resolve) => setTimeout(resolve, 1100));
plugin["event"](created);
await new Promise((resolve) => setTimeout(resolve, 100));
process.off("unhandledRejection", onUnhandled);
if (unhandled.length !== 0) throw new Error("lifecycle callback produced an unhandled rejection");
`), 0600); err != nil {
		t.Fatalf("write Node runner: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--experimental-strip-types", runner, filepath.Join("..", "..", "embed", "hooks", "opencode", "hive.ts"))
	cmd.Env = append(os.Environ(), "HIVE_HTTP_PORT="+port, "HIVE_OPENCODE_SESSION_ID=", "OPENCODE_SESSION_ID=", "SESSION_ID=", "HIVE_PROJECT=", "JARVIS_PROJECT=", "HIVE_PROJECT_DIRECTORY=", "JARVIS_WORKSPACE_DIRECTORY=", "PWD=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stages hiveTemplateStages
	cmd.Stdout = &stages
	progress := func() string {
		return fmt.Sprintf("%s; governance requests=%d; session requests=%d", stages.latest(), governance.Load(), starts.Load())
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Run() }()

	var primaryStart, distinctStart, environmentStart hiveTemplateRequest
	var primaryStartSeen, distinctStartSeen, environmentStartSeen bool
	for range 3 {
		request := waitHiveTemplateChildRequest(t, startRequests, finished, cancel, &stderr, "initial session start", progress)
		switch hiveTemplateStartEvidence(t, request) {
		case "session-42/jarvis-dev":
			if primaryStartSeen {
				t.Fatalf("received duplicate primary session start")
			}
			primaryStart, primaryStartSeen = request, true
		case "session-42/other-project":
			if distinctStartSeen {
				t.Fatalf("received duplicate distinct-evidence session start")
			}
			distinctStart, distinctStartSeen = request, true
		case "environment-session/environment-project":
			if environmentStartSeen {
				t.Fatalf("received duplicate environment-fallback session start")
			}
			environmentStart, environmentStartSeen = request, true
		default:
			t.Fatalf("received unexpected initial session start: %s", hiveTemplateStartEvidence(t, request))
		}
	}
	if !primaryStartSeen || !distinctStartSeen || !environmentStartSeen {
		t.Fatalf("initial session starts missing evidence: primary=%t distinct=%t environment=%t", primaryStartSeen, distinctStartSeen, environmentStartSeen)
	}
	prompt := waitHiveTemplateChildRequest(t, promptRequests, finished, cancel, &stderr, "prompt while the primary start request is held", progress)
	fallbackPrompt := waitHiveTemplateChildRequest(t, promptRequests, finished, cancel, &stderr, "prompt PID fallback", progress)
	numericPrompt := waitHiveTemplateChildRequest(t, promptRequests, finished, cancel, &stderr, "numeric text prompt", progress)
	assertNoHiveTemplateRequest(t, promptRequests, 200*time.Millisecond, "a malformed non-array prompt request")
	assertNoHiveTemplateRequest(t, startRequests, 200*time.Millisecond, "an immediate duplicate or no-evidence session start")
	primaryCanceled := waitHiveTemplateTime(t, primaryStartCanceled, "primary start timeout cancellation")
	secondStart := waitHiveTemplateChildRequest(t, startRequests, finished, cancel, &stderr, "retry after primary start timeout", progress)
	if secondStart.observedAt.Before(primaryCanceled) {
		t.Fatalf("retry start observed at %s before primary timeout cleanup at %s", secondStart.observedAt, primaryCanceled)
	}
	if _, err := awaitHiveTemplateRequest(nil, finished, cancel, &stderr, "OpenCode Hive template completion", hiveTemplateRequestWait, progress); err != nil {
		t.Fatal(err)
	}

	if starts.Load() != 4 {
		t.Fatalf("session start count = %d, want exactly 4", starts.Load())
	}
	for _, request := range []hiveTemplateRequest{primaryStart, secondStart} {
		if request.method != http.MethodPost || request.contentType != "application/json" {
			t.Fatalf("session start request = %s content-type %q, want POST application/json", request.method, request.contentType)
		}
		assertHiveTemplateJSON(t, request, map[string]string{
			"id": "session-42", "project": "jarvis-dev", "directory": "/workspace/jarvis-dev", "client": "opencode",
		})
	}
	assertHiveTemplateJSON(t, distinctStart, map[string]string{
		"id": "session-42", "project": "other-project", "directory": "/workspace/other-project", "client": "opencode",
	})
	assertHiveTemplateJSON(t, environmentStart, map[string]string{
		"id": "environment-session", "project": "environment-project", "directory": "/environment-directory", "client": "opencode",
	})
	if prompt.method != http.MethodPost || prompt.contentType != "application/json" {
		t.Fatalf("prompt request = %s content-type %q, want POST application/json", prompt.method, prompt.contentType)
	}
	assertHiveTemplateJSON(t, prompt, map[string]string{
		"content": "capture  \n  this prompt", "session_id": "prompt-session", "project": "prompt-project", "directory": "/prompt-directory", "client": "opencode",
	})
	var fallbackPayload map[string]string
	if err := json.Unmarshal(fallbackPrompt.body, &fallbackPayload); err != nil {
		t.Fatalf("decode fallback prompt: %v", err)
	}
	if fallbackPayload["content"] != "fallback prompt" || fallbackPayload["client"] != "opencode" || fallbackPayload["directory"] == "" || !strings.HasPrefix(fallbackPayload["session_id"], "ppid-") {
		t.Fatalf("prompt PID/cwd fallback changed: %#v", fallbackPayload)
	}
	if _, found := fallbackPayload["project"]; found {
		t.Fatalf("prompt PID/cwd fallback invented a project: %#v", fallbackPayload)
	}
	assertHiveTemplateJSON(t, numericPrompt, map[string]string{
		"content": "42", "session_id": fallbackPayload["session_id"], "directory": fallbackPayload["directory"], "client": "opencode",
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
process.env.PWD = "";
const created = { event: { type: "session.created", properties: { info: {
  id: "created-pending", project: "jarvis-dev", directory: "/workspace/jarvis-dev"
} } } };
const deleted = { event: { type: "session.deleted", properties: { info: {
  id: "session /with?reserved%chars", project: "jarvis-dev", directory: "/workspace/jarvis-dev"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--experimental-strip-types", runner, filepath.Join("..", "..", "embed", "hooks", "opencode", "hive.ts"))
	cmd.Env = append(os.Environ(), "HIVE_HTTP_PORT="+port, "HIVE_OPENCODE_SESSION_ID=", "OPENCODE_SESSION_ID=", "SESSION_ID=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	finished := make(chan error, 1)
	go func() { finished <- cmd.Run() }()

	_ = waitHiveTemplateChildRequest(t, createdRequests, finished, cancel, &stderr, "pending session creation")
	firstEnd := waitHiveTemplateChildRequest(t, endRequests, finished, cancel, &stderr, "first deleted-session end")
	firstCanceled := waitHiveTemplateTime(t, firstEndCanceled, "first end timeout cancellation")
	secondEnd := waitHiveTemplateChildRequest(t, endRequests, finished, cancel, &stderr, "repeat deleted-session end")
	if secondEnd.observedAt.Before(firstCanceled) {
		t.Fatalf("repeat end observed at %s before first timeout cleanup at %s", secondEnd.observedAt, firstCanceled)
	}
	if _, err := awaitHiveTemplateRequest(nil, finished, cancel, &stderr, "OpenCode Hive deletion template completion", hiveTemplateRequestWait); err != nil {
		t.Fatal(err)
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
			"project": "jarvis-dev", "directory": "/workspace/jarvis-dev", "client": "opencode",
		})
	}
}

func TestOpenCodeHiveTemplate_FailsOpenForLifecycleNonOKResponses(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Node to execute the source-of-truth OpenCode template")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is unavailable: source execution test skipped")
	}

	statusByID := map[string]int{
		"created-400": http.StatusBadRequest,
		"created-423": http.StatusLocked,
		"created-500": http.StatusInternalServerError,
		"deleted-400": http.StatusBadRequest,
		"deleted-423": http.StatusLocked,
		"deleted-500": http.StatusInternalServerError,
	}
	createdCounts := map[string]*atomic.Int32{
		"created-400": {},
		"created-423": {},
		"created-500": {},
	}
	deletedCounts := map[string]*atomic.Int32{
		"deleted-400": {},
		"deleted-423": {},
		"deleted-500": {},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/project-identity/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/sessions":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			id := payload["id"]
			if count, ok := createdCounts[id]; ok {
				count.Add(1)
				w.WriteHeader(statusByID[id])
				return
			}
			http.NotFound(w, r)
		default:
			for id, count := range deletedCounts {
				if r.URL.Path == "/sessions/"+id+"/end" {
					count.Add(1)
					w.WriteHeader(statusByID[id])
					return
				}
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	port := hiveTemplatePort(t, server.URL)
	runner := filepath.Join(t.TempDir(), "run-hive-lifecycle-non-ok.mjs")
	if err := os.WriteFile(runner, []byte(`
import { pathToFileURL } from "node:url";
const { Hive } = await import(pathToFileURL(process.argv[2]).href);
const plugin = await Hive();
const unhandled = [];
const onUnhandled = (reason) => unhandled.push(reason);
process.on("unhandledRejection", onUnhandled);
for (const status of [400, 423, 500]) {
  const created = { event: { type: "session.created", properties: { info: {
    id: "created-" + status, project: "jarvis-dev", directory: "/workspace/jarvis-dev"
  } } } };
  const deleted = { event: { type: "session.deleted", properties: { info: {
    id: "deleted-" + status, project: "jarvis-dev", directory: "/workspace/jarvis-dev"
  } } } };
  if (plugin["event"](created) !== undefined) throw new Error("created callback awaited a non-OK response");
  if (plugin["event"](deleted) !== undefined) throw new Error("deleted callback awaited a non-OK response");
}
await new Promise((resolve) => setTimeout(resolve, 200));
process.off("unhandledRejection", onUnhandled);
if (unhandled.length !== 0) throw new Error("non-OK lifecycle response was unhandled");
`), 0600); err != nil {
		t.Fatalf("write Node non-OK lifecycle runner: %v", err)
	}

	cmd := exec.Command("node", "--experimental-strip-types", runner, filepath.Join("..", "..", "embed", "hooks", "opencode", "hive.ts"))
	cmd.Env = append(os.Environ(), "HIVE_HTTP_PORT="+port, "HIVE_OPENCODE_SESSION_ID=", "OPENCODE_SESSION_ID=", "SESSION_ID=", "HIVE_PROJECT=", "JARVIS_PROJECT=", "HIVE_PROJECT_DIRECTORY=", "JARVIS_WORKSPACE_DIRECTORY=")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("execute OpenCode Hive non-OK lifecycle template: %v\n%s", err, output)
	}

	for id, count := range createdCounts {
		if got := count.Load(); got != 1 {
			t.Errorf("created lifecycle requests for %s = %d, want 1", id, got)
		}
	}
	for id, count := range deletedCounts {
		if got := count.Load(); got != 1 {
			t.Errorf("deleted lifecycle requests for %s = %d, want 1", id, got)
		}
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

func isPrimaryOpenCodeStart(request hiveTemplateRequest) bool {
	var payload struct {
		ID      string `json:"id"`
		Project string `json:"project"`
	}
	return request.err == nil && json.Unmarshal(request.body, &payload) == nil && payload.ID == "session-42" && payload.Project == "jarvis-dev"
}

func hiveTemplateStartEvidence(t *testing.T, request hiveTemplateRequest) string {
	t.Helper()
	if request.err != nil {
		t.Fatalf("read %s request: %v", request.path, request.err)
	}
	var payload struct {
		ID      string `json:"id"`
		Project string `json:"project"`
	}
	if err := json.Unmarshal(request.body, &payload); err != nil {
		t.Fatalf("decode %s request: %v", request.path, err)
	}
	return payload.ID + "/" + payload.Project
}

const hiveTemplateRequestWait = 10 * time.Second

func waitHiveTemplateChildRequest(t *testing.T, requests <-chan hiveTemplateRequest, finished <-chan error, cancel context.CancelFunc, stderr *bytes.Buffer, description string, progress ...func() string) hiveTemplateRequest {
	t.Helper()
	request, err := awaitHiveTemplateRequest(requests, finished, cancel, stderr, description, hiveTemplateRequestWait, progress...)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func awaitHiveTemplateRequest(requests <-chan hiveTemplateRequest, finished <-chan error, cancel context.CancelFunc, stderr *bytes.Buffer, description string, duration time.Duration, progress ...func() string) (hiveTemplateRequest, error) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case request := <-requests:
		return request, nil
	case err := <-finished:
		if requests == nil && err == nil {
			return hiveTemplateRequest{}, nil
		}
		return hiveTemplateRequest{}, fmt.Errorf("child exited before %s: %v; stderr: %s; stage: %s", description, err, stderr.String(), hiveTemplateProgress(progress))
	case <-timer.C:
		stage := hiveTemplateProgress(progress)
		cancel() // Safe before Start; CommandContext kills a started child.
		select {
		case err := <-finished:
			return hiveTemplateRequest{}, fmt.Errorf("timed out waiting for %s (canceled after deadline: %v; stderr: %s; stage: %s)", description, err, stderr.String(), stage)
		case <-time.After(time.Second):
			// Run may still be starting or waiting on OS process/pipe cleanup. Do not read stderr until it returns.
			return hiveTemplateRequest{}, fmt.Errorf("timed out waiting for %s (child cancellation pending; stage: %s)", description, stage)
		}
	}
}

func hiveTemplateProgress(progress []func() string) string {
	if len(progress) == 0 {
		return "unknown"
	}
	return progress[0]()
}

type hiveTemplateStages struct {
	mu      sync.Mutex
	pending string
	stage   string
}

func (s *hiveTemplateStages) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending += string(p)
	for {
		line, rest, found := strings.Cut(s.pending, "\n")
		if !found {
			break
		}
		s.pending = rest
		if stage, ok := strings.CutPrefix(strings.TrimSpace(line), "HIVE_STAGE:"); ok {
			s.stage = stage
		}
	}
	return len(p), nil
}

func (s *hiveTemplateStages) latest() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage == "" {
		return "runner not started"
	}
	return s.stage
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

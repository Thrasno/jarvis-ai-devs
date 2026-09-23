package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func liveDaemonBinary(t *testing.T) string {
	t.Helper()
	if supersedeLiveDaemonBin == "" {
		t.Skip("sibling hive-daemon module absent or live binary unavailable")
	}
	return supersedeLiveDaemonBin
}

type liveDaemon struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan []byte
	base  string
	done  chan error
}

func startLiveDaemon(t *testing.T, binary, database, home string) *liveDaemon {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "HOME="+home, "HIVE_DB_PATH="+database, fmt.Sprintf("HIVE_HTTP_PORT=%d", port), "HIVE_API_URL=", "HIVE_API_EMAIL=", "HIVE_API_PASSWORD=")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	d := &liveDaemon{cmd: cmd, stdin: stdin, base: fmt.Sprintf("http://127.0.0.1:%d", port), lines: make(chan []byte, 16), done: make(chan error, 1)}
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			d.lines <- append([]byte(nil), scanner.Bytes()...)
		}
		close(d.lines)
	}()
	go func() { d.done <- cmd.Wait() }()
	t.Cleanup(func() { d.stop() })
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(d.base + "/governance/project-identity/status")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				return d
			}
		}
		select {
		case err := <-d.done:
			t.Fatalf("daemon exited before readiness: %v: %s", err, stderr.String())
		default:
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("daemon HTTP not ready: %s", stderr.String())
	return nil
}

func (d *liveDaemon) stop() {
	_ = d.stdin.Close()
	if d.cmd.ProcessState == nil {
		_ = d.cmd.Process.Kill()
		select {
		case <-d.done:
		case <-time.After(3 * time.Second):
		}
	}
}

func (d *liveDaemon) rpc(t *testing.T, id int, method string, params any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.stdin.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case line, ok := <-d.lines:
			if !ok {
				t.Fatal("MCP stdout closed")
			}
			var reply struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(line, &reply); err != nil {
				t.Fatalf("MCP invalid JSON: %v", err)
			}
			if reply.ID != id {
				continue
			}
			if len(reply.Error) > 0 {
				t.Fatalf("MCP %s: %s", method, reply.Error)
			}
			return reply.Result
		case <-deadline.C:
			t.Fatalf("MCP %s timed out", method)
		}
	}
}

func liveHTTP(t *testing.T, method, url string, payload any, result any) int {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode >= 300 {
		t.Fatalf("%s %s: %d %s", method, url, response.StatusCode, raw)
	}
	if result != nil {
		if err := json.Unmarshal(raw, result); err != nil {
			t.Fatal(err)
		}
	}
	return response.StatusCode
}

func liveOpenSpecReceipt(t *testing.T, root, requestID string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".apply-progress-receipts", requestID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil || !applyprogress.ValidDigest(receipt.Payload) {
		t.Fatalf("invalid OpenSpec committed receipt for %s: %v", requestID, err)
	}
	return data
}

type liveReceiptAck struct {
	Generation uint64 `json:"generation"`
	Revision   uint64 `json:"revision"`
	Digest     string `json:"digest"`
	Payload    string `json:"payload"`
}

type liveReceiptState struct {
	Payload     string          `json:"payload"`
	OpenSpec    string          `json:"openspec"`
	Hive        string          `json:"hive"`
	OpenSpecAck *liveReceiptAck `json:"openspec_ack,omitempty"`
	HiveAck     *liveReceiptAck `json:"hive_ack,omitempty"`
}

func assertHybridReceipt(t *testing.T, data []byte, open, hive string, generation, revision uint64, digest string) {
	t.Helper()
	var state liveReceiptState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.OpenSpec != open || state.Hive != hive || state.OpenSpecAck == nil || state.OpenSpecAck.Generation != generation || state.OpenSpecAck.Revision != revision || state.OpenSpecAck.Digest != digest || state.OpenSpecAck.Payload != state.Payload {
		t.Fatalf("unexpected durable hybrid receipt: %s", data)
	}
	if hive == "committed" {
		if state.HiveAck == nil || *state.HiveAck != *state.OpenSpecAck {
			t.Fatalf("Hive ack differs from OpenSpec: %s", data)
		}
	} else if state.HiveAck != nil {
		t.Fatalf("uncommitted Hive has ack: %s", data)
	}
}

func liveHybridReceipt(t *testing.T, root, requestID string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".apply-progress-hybrid-receipts", requestID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt liveReceiptState
	if err := json.Unmarshal(data, &receipt); err != nil || !applyprogress.ValidDigest(receipt.Payload) {
		t.Fatalf("invalid hybrid receipt: %v %s", err, data)
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal(canonical, data) {
		t.Fatalf("noncanonical hybrid receipt: %v", err)
	}
	return data
}

func TestSupersedeRouteHybridLiveDaemonPartialPublicationRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("live daemon integration requires process build")
	}
	binary := liveDaemonBinary(t)
	workspace, root, original, batch := preflightOpenSpec(t, sddruntime.StoreModeHybrid)
	home := t.TempDir()
	database := filepath.Join(t.TempDir(), "hive.db")
	daemon := startLiveDaemon(t, binary, database, home)
	_ = daemon.rpc(t, 1, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "supersession-test", "version": "1"}})
	notification, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if _, err := daemon.stdin.Write(append(notification, '\n')); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(t.TempDir(), "jarvis-dev")
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "config"), []byte("[remote \"origin\"]\n url = https://github.com/test/jarvis-dev.git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	liveHTTP(t, http.MethodPost, daemon.base+"/sessions", map[string]any{"id": "live-seed", "project": "jarvis-dev", "directory": projectDir, "client": "test"}, nil)
	for i, tasks := range []string{preflightOldTasks, preflightNewTasks} {
		raw := daemon.rpc(t, 2+i, "tools/call", map[string]any{"name": "mem_save", "arguments": map[string]any{"title": "sdd/issue-653/tasks", "content": tasks, "type": "discovery", "project": "jarvis-dev", "directory": projectDir, "topic_key": "sdd/issue-653/tasks", "capture_prompt": false}})
		var outcome struct {
			IsError bool `json:"isError"`
		}
		if err := json.Unmarshal(raw, &outcome); err != nil || outcome.IsError {
			t.Fatalf("mem_save task %d: %v %s", i, err, raw)
		}
		if i == 0 {
			liveHTTP(t, http.MethodPost, daemon.base+"/sdd/changes/issue-653/store-binding/adopt", map[string]any{"project": "jarvis-dev", "mode": "hybrid", "provenance": "cli"}, nil)
			var seeded hiveclient.ApplyProgressResult
			liveHTTP(t, http.MethodPost, daemon.base+"/sdd/changes/issue-653/apply-progress/advance", hiveclient.ApplyProgressAdvanceRequest{Project: "jarvis-dev", Change: "issue-653", RequestID: "live-initial", Snapshot: original, Batches: []applyprogress.Batch{batch}}, &seeded)
			if seeded.State.Digest != original.Digest {
				t.Fatalf("seeded Hive digest %s differs from OpenSpec %s", seeded.State.Digest, original.Digest)
			}
		}
	}
	var remote hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/issue-653/apply-progress?project=jarvis-dev", nil, &remote)
	if remote.State.Digest != original.Digest {
		t.Fatalf("remote prior digest %s differs from %s", remote.State.Digest, original.Digest)
	}
	var killed bool
	var firstSeal, retrySeal []byte
	var publishReceipts []hiveclient.ApplyProgressReceipt
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sealPost := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/apply-progress/advance")
		publishPost := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/apply-progress/publish-successor")
		var raw []byte
		if sealPost || publishPost {
			var err error
			raw, err = io.ReadAll(io.LimitReader(r.Body, 2<<20))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		if sealPost && !killed {
			firstSeal = append([]byte(nil), raw...)
			killed = true
			daemon.stop()
			http.Error(w, "daemon stopped before Hive seal", http.StatusServiceUnavailable)
			return
		}
		if sealPost {
			retrySeal = append([]byte(nil), raw...)
		}
		outbound := r.Clone(r.Context())
		outbound.URL.Scheme = "http"
		outbound.URL.Host = strings.TrimPrefix(daemon.base, "http://")
		outbound.Host = outbound.URL.Host
		outbound.RequestURI = ""
		response, err := (&http.Client{Timeout: 3 * time.Second}).Do(outbound)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer response.Body.Close()
		if publishPost && response.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			var result hiveclient.ApplyProgressResult
			if err := json.Unmarshal(body, &result); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			publishReceipts = append(publishReceipts, result.Receipt)
			response.Body = io.NopCloser(bytes.NewReader(body))
		}
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	defer proxy.Close()
	t.Setenv("HIVE_DAEMON_URL", proxy.URL)
	args := []string{"--root", root, "--change", "issue-653", "--successor", "next", "--project", "jarvis-dev", "--actor", "maintainer", "--reason", "revised scope"}
	cmd := newSddSupersedeCommand()
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader("next\n"))
	cmd.SetOut(io.Discard)
	if err := cmd.Execute(); err == nil || !killed {
		t.Fatalf("expected interruption after OpenSpec seal: %v killed=%v", err, killed)
	}
	local, err := (sddprogress.OpenSpec{Root: root}).InspectPublication()
	if err != nil || local == nil || local.Status != applyprogress.StatusSuperseded || local.SealIntent == nil || local.SealIntent.OperationID == "" {
		t.Fatalf("local seal and frozen intent: %v %+v", err, local)
	}
	operationID := local.SealIntent.OperationID
	openSealReceipt := liveOpenSpecReceipt(t, root, operationID)
	partialReceipt := liveHybridReceipt(t, root, operationID)
	assertHybridReceipt(t, partialReceipt, "committed", "failed", local.Generation, local.Revision, local.Digest)
	if len(firstSeal) == 0 {
		t.Fatal("missing intercepted raw seal POST")
	}
	daemon = startLiveDaemon(t, binary, database, home)
	var pending hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/issue-653/apply-progress?project=jarvis-dev", nil, &pending)
	if pending.State.Digest != original.Digest {
		t.Fatalf("Hive advanced before interruption: %s", pending.State.Digest)
	}
	client, err := hiveclient.New(daemon.base)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if receipt, found, err := client.GetApplyProgressReceipt(ctx, "jarvis-dev", "issue-653", operationID); err != nil || found {
		t.Fatalf("Hive seal receipt exists before retry: %+v found=%t err=%v", receipt, found, err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "openspec", "changes", "next")); !os.IsNotExist(err) {
		t.Fatalf("successor created before retry: %v", err)
	}
	var occupancy struct {
		Occupied bool `json:"occupied"`
	}
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/next/successor-occupancy?project=jarvis-dev", nil, &occupancy)
	if occupancy.Occupied {
		t.Fatal("Hive successor reserved before retry")
	}
	retry := newSddSupersedeCommand()
	retry.SetArgs(args)
	retry.SetIn(strings.NewReader(""))
	retry.SetOut(io.Discard)
	if err := retry.Execute(); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if !bytes.Equal(firstSeal, retrySeal) {
		t.Fatalf("Hive seal POST changed across restart: first=%s retry=%s", firstSeal, retrySeal)
	}
	committedReceipt := liveHybridReceipt(t, root, operationID)
	assertHybridReceipt(t, committedReceipt, "committed", "committed", local.Generation, local.Revision, local.Digest)
	if len(publishReceipts) != 1 || !applyprogress.ValidDigest(publishReceipts[0].PayloadSHA256) || publishReceipts[0].RequestID == "" {
		t.Fatalf("missing Hive genesis POST receipt: %+v", publishReceipts)
	}
	target := filepath.Join(workspace, "openspec", "changes", "next")
	successor, err := (sddprogress.OpenSpec{Root: target}).InspectPublication()
	if err != nil || successor == nil || applyprogress.ValidateSuccessorGenesisPair(*local, *successor) != nil {
		t.Fatalf("successor: %v %+v", err, successor)
	}
	var hiveSeal, hiveGenesis hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/issue-653/apply-progress?project=jarvis-dev", nil, &hiveSeal)
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/next/apply-progress?project=jarvis-dev", nil, &hiveGenesis)
	if hiveSeal.State.Digest != local.Digest || hiveGenesis.State.Digest != successor.Digest || hiveSeal.State.Snapshot.Digest != local.Digest || hiveGenesis.State.Snapshot.Digest != successor.Digest {
		t.Fatalf("stored seals/genesis diverged: OpenSpec %s/%s Hive %s/%s", local.Digest, successor.Digest, hiveSeal.State.Digest, hiveGenesis.State.Digest)
	}
	if hiveSeal.State.Snapshot.SealIntent == nil || *local.SealIntent != *hiveSeal.State.Snapshot.SealIntent || local.SealIntent.OperationID != operationID {
		t.Fatal("frozen successor intent changed across stores")
	}
	_, sealBytes, err := applyprogress.SealSnapshot(*local)
	if err != nil {
		t.Fatal(err)
	}
	sealDigest := applyprogress.AdvanceReceiptDigest(local.Project, local.Change, operationID, original.Generation, original.Revision, original.Digest, "", sealBytes, [][]byte{})
	hiveReceipt, found, err := client.GetApplyProgressReceipt(ctx, "jarvis-dev", "issue-653", operationID)
	if err != nil || !found || hiveReceipt.RequestID != operationID || hiveReceipt.PayloadSHA256 != sealDigest {
		t.Fatalf("Hive seal committed receipt: %+v found=%t err=%v expected=%s", hiveReceipt, found, err, sealDigest)
	}
	if !bytes.Equal(openSealReceipt, liveOpenSpecReceipt(t, root, operationID)) {
		t.Fatal("OpenSpec seal receipt changed on retry")
	}
	bindings := make(map[string]hiveclient.SDDStoreBinding)
	for _, change := range []string{"issue-653", "next"} {
		bound, found, err := client.GetSDDStoreBinding(ctx, "jarvis-dev", change)
		if err != nil || !found || bound.Mode != hiveclient.SDDStoreModeHybrid {
			t.Fatalf("Hive %s binding: %+v found=%t err=%v", change, bound, found, err)
		}
		bindings[change] = bound
	}
	if len(successor.Coverage) != 0 || len(successor.Batches) != 0 || len(hiveGenesis.State.Snapshot.Coverage) != 0 || len(hiveGenesis.State.Snapshot.Batches) != 0 {
		t.Fatal("successor inherited predecessor credit or evidence")
	}
	genesisOpenReceipt := liveOpenSpecReceipt(t, target, operationID)
	for _, path := range []string{root, target} {
		binding, err := sddbinding.ReadOpenSpec(path)
		if err != nil || binding == nil || binding.Mode() != sddruntime.StoreModeHybrid {
			t.Fatalf("OpenSpec %s binding: %v %v", path, binding, err)
		}
	}
	second := newSddSupersedeCommand()
	second.SetArgs(args)
	second.SetIn(strings.NewReader(""))
	second.SetOut(io.Discard)
	if err := second.Execute(); err != nil {
		t.Fatalf("idempotent second retry: %v", err)
	}
	if !bytes.Equal(committedReceipt, liveHybridReceipt(t, root, operationID)) {
		t.Fatal("second retry changed durable hybrid receipt")
	}
	// The CLI may short-circuit a fully committed replay; exercise the daemon's
	// idempotent POST directly through the same proxy to observe its receipt.
	var genesisReplay hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodPost, proxy.URL+"/sdd/changes/issue-653/apply-progress/publish-successor", map[string]string{"project": "jarvis-dev"}, &genesisReplay)
	if len(publishReceipts) != 2 || publishReceipts[0] != publishReceipts[1] || genesisReplay.Receipt != publishReceipts[0] || genesisReplay.State.Digest != successor.Digest {
		t.Fatalf("Hive genesis POST replay receipt changed: %+v result=%+v", publishReceipts, genesisReplay.Receipt)
	}
	var replay hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/next/apply-progress?project=jarvis-dev", nil, &replay)
	if replay.State.Digest != successor.Digest || replay.State.Snapshot.Digest != successor.Digest {
		t.Fatalf("second retry changed genesis: %s", replay.State.Digest)
	}
	var replaySeal hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodGet, daemon.base+"/sdd/changes/issue-653/apply-progress?project=jarvis-dev", nil, &replaySeal)
	if replaySeal.State.Digest != local.Digest || replaySeal.State.Snapshot.SealIntent == nil || *replaySeal.State.Snapshot.SealIntent != *local.SealIntent {
		t.Fatal("second retry changed signed predecessor")
	}
	replayedOpenSeal, err := (sddprogress.OpenSpec{Root: root}).InspectPublication()
	if err != nil || replayedOpenSeal == nil || replayedOpenSeal.Digest != local.Digest {
		t.Fatalf("second retry changed OpenSpec seal: %v", err)
	}
	replayedOpenGenesis, err := (sddprogress.OpenSpec{Root: target}).InspectPublication()
	if err != nil || replayedOpenGenesis == nil || replayedOpenGenesis.Digest != successor.Digest {
		t.Fatalf("second retry changed OpenSpec genesis: %v", err)
	}
	if !bytes.Equal(openSealReceipt, liveOpenSpecReceipt(t, root, operationID)) || !bytes.Equal(genesisOpenReceipt, liveOpenSpecReceipt(t, target, operationID)) {
		t.Fatal("second retry changed OpenSpec receipt bytes")
	}
	receipt, found, err := client.GetApplyProgressReceipt(ctx, "jarvis-dev", "issue-653", operationID)
	if err != nil || !found || receipt != hiveReceipt {
		t.Fatalf("second retry changed Hive seal receipt: %+v found=%t err=%v", receipt, found, err)
	}
	// Reopen the same SQLite file once more after genesis and compare all durable authority.
	daemon.stop()
	daemon = startLiveDaemon(t, binary, database, home)
	client, err = hiveclient.New(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	var persistedSeal, persistedGenesis hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodGet, proxy.URL+"/sdd/changes/issue-653/apply-progress?project=jarvis-dev", nil, &persistedSeal)
	liveHTTP(t, http.MethodGet, proxy.URL+"/sdd/changes/next/apply-progress?project=jarvis-dev", nil, &persistedGenesis)
	if persistedSeal.State.Digest != hiveSeal.State.Digest || persistedGenesis.State.Digest != hiveGenesis.State.Digest || persistedSeal.State.Snapshot.Status != applyprogress.StatusSuperseded || persistedGenesis.State.Snapshot.Digest != successor.Digest {
		t.Fatal("SQLite restart changed committed heads")
	}
	for _, change := range []string{"issue-653", "next"} {
		bound, found, err := client.GetSDDStoreBinding(ctx, "jarvis-dev", change)
		if err != nil || !found || bound != bindings[change] {
			t.Fatalf("restart changed %s binding: %+v %t %v", change, bound, found, err)
		}
	}
	persistedReceipt, found, err := client.GetApplyProgressReceipt(ctx, "jarvis-dev", "issue-653", operationID)
	if err != nil || !found || persistedReceipt != hiveReceipt {
		t.Fatalf("restart changed predecessor receipt: %+v %t %v", persistedReceipt, found, err)
	}
	if !bytes.Equal(committedReceipt, liveHybridReceipt(t, root, operationID)) || !bytes.Equal(openSealReceipt, liveOpenSpecReceipt(t, root, operationID)) || !bytes.Equal(genesisOpenReceipt, liveOpenSpecReceipt(t, target, operationID)) {
		t.Fatal("restart changed local durable receipts")
	}
	// A persisted genesis receipt must replay after restart; without it the
	// occupied successor target cannot accept another publication.
	var restartedReplay hiveclient.ApplyProgressResult
	liveHTTP(t, http.MethodPost, proxy.URL+"/sdd/changes/issue-653/apply-progress/publish-successor", map[string]string{"project": "jarvis-dev"}, &restartedReplay)
	if len(publishReceipts) != 3 || restartedReplay.Outcome != "committed" || restartedReplay.Receipt != publishReceipts[0] || publishReceipts[2] != publishReceipts[0] || restartedReplay.State.Digest != persistedGenesis.State.Digest || restartedReplay.State.Generation != persistedGenesis.State.Generation || restartedReplay.State.Revision != persistedGenesis.State.Revision || !reflect.DeepEqual(restartedReplay.State.Snapshot, persistedGenesis.State.Snapshot) {
		t.Fatalf("restart changed genesis publication replay: receipts=%+v replay=%+v persisted=%+v", publishReceipts, restartedReplay, persistedGenesis)
	}
}

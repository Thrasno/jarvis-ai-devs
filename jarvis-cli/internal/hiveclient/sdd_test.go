package hiveclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestFetchSDDArtifactsContract(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/sdd/changes/change%25_/artifacts" {
			t.Errorf("escaped path = %q", got)
		}
		if got := r.URL.Query().Get("project"); got != "project name" {
			t.Errorf("project = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "" {
			t.Errorf("limit = %q", got)
		}
		_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"explore","content":"# Explore","created_at":"2026-08-01T10:00:00Z"},{"artifact":"tasks","content":"- [ ] 1.1 task","created_at":"2026-08-01T10:00:01Z"}]}`))
	})

	artifacts, err := client.FetchSDDArtifacts(context.Background(), "project name", "change%_")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	if artifacts[0].Artifact != "explore" || artifacts[0].Content != "# Explore" {
		t.Fatalf("artifact = %#v", artifacts[0])
	}
	if artifacts[1].Artifact != "tasks" || artifacts[1].Content != "- [ ] 1.1 task" {
		t.Fatalf("tasks artifact = %#v", artifacts[1])
	}
}

func TestListSDDChangesContract(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/sdd/changes" {
			t.Errorf("path = %q", got)
		}
		if got := r.URL.Query().Get("project"); got != "project" {
			t.Errorf("project = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "3" {
			t.Errorf("limit = %q", got)
		}
		if got := r.URL.Query().Get("cursor"); got != "opaque+/=" {
			t.Errorf("cursor = %q", got)
		}
		_, _ = w.Write([]byte(`{"changes":["alpha","bravo"],"next_cursor":"next"}`))
	})

	page, err := client.ListSDDChanges(context.Background(), "project", hiveclient.SDDPageRequest{Limit: 3, Cursor: "opaque+/="})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 2 || page.Changes[0] != "alpha" || page.Changes[1] != "bravo" {
		t.Fatalf("changes = %#v", page.Changes)
	}
	if page.NextCursor != "next" {
		t.Fatalf("next cursor = %q", page.NextCursor)
	}
}

func TestSDDClientPreservesTypedAPIError(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"cursor is invalid"}`))
	})
	_, err := client.ListSDDChanges(context.Background(), "project", hiveclient.SDDPageRequest{Limit: 2, Cursor: "bad"})
	var apiErr *hiveclient.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %#v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "cursor is invalid" {
		t.Fatalf("APIError = %#v", apiErr)
	}
}

func TestApplyProgressReceiptIdentityClientUsesBoundReadOnlyEndpoint(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/sdd/changes/change%25_/apply-progress/receipts/request%25_" {
			t.Fatalf("request = %s %s", r.Method, r.URL.EscapedPath())
		}
		if got := r.URL.Query().Get("project"); got != "project name" {
			t.Fatalf("project = %q", got)
		}
		_, _ = w.Write([]byte(`{"receipt":{"request_id":"request%_","payload_sha256":"payload-identity"}}`))
	})

	receipt, found, err := client.GetApplyProgressReceipt(context.Background(), "project name", "change%_", "request%_")
	if err != nil || !found || receipt.RequestID != "request%_" || receipt.PayloadSHA256 != "payload-identity" {
		t.Fatalf("receipt = %#v, found=%t, err=%v", receipt, found, err)
	}
}

func TestApplyProgressReceiptIdentityClientTreatsMissingReceiptAsUnused(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
	})

	_, found, err := client.GetApplyProgressReceipt(context.Background(), "project", "change", "missing")
	if err != nil || found {
		t.Fatalf("found=%t, err=%v", found, err)
	}
}

func TestApplyProgressClientPreservesConflictRecovery(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sdd/changes/change/apply-progress/advance" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"outcome":"conflict","code":"stale","state":{"generation":2,"revision":2,"digest":"abc"},"recovery":"read current state and retry"}`))
	})
	result, err := client.AdvanceApplyProgress(context.Background(), hiveclient.ApplyProgressAdvanceRequest{Project: "project", Change: "change", RequestID: "request"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "conflict" || result.State.Generation != 2 || result.Recovery == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplyProgressClientSendsLegacySourceProvenance(t *testing.T) {
	const source = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if got := request["legacy_source_sha256"]; got != source {
			t.Fatalf("legacy source provenance = %#v, want %q", got, source)
		}
		_, _ = w.Write([]byte(`{"outcome":"committed","code":"ok"}`))
	})
	_, err := client.AdvanceApplyProgress(context.Background(), hiveclient.ApplyProgressAdvanceRequest{Project: "project", Change: "change", RequestID: "legacy-source", LegacySourceSHA256: source})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplyProgressClientPreservesCapacityDiagnostics(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte(`{"outcome":"invalid","code":"capacity","capacity":{"document":"evidence batch","runes":40017,"limit":40000},"recovery":"split evidence at a complete entry boundary"}`))
	})
	result, err := client.AdvanceApplyProgress(context.Background(), hiveclient.ApplyProgressAdvanceRequest{Project: "project", Change: "change", RequestID: "capacity"})
	var applyErr *hiveclient.ApplyProgressError
	if !errors.As(err, &applyErr) {
		t.Fatalf("error = %v, want ApplyProgressError", err)
	}
	if result.Code != "capacity" || result.Capacity == nil || result.Capacity.Document != "evidence batch" || result.Capacity.Runes != 40017 || result.Capacity.Limit != applyprogress.MaxDocumentRunes {
		t.Fatalf("capacity result = %#v", result)
	}
	if applyErr.Result.Capacity == nil || *applyErr.Result.Capacity != *result.Capacity {
		t.Fatalf("error capacity = %#v, want %#v", applyErr.Result.Capacity, result.Capacity)
	}
}

func TestSharedApplyProgressFixturesAreCommittedBoundedWireResponses(t *testing.T) {
	for _, fixture := range [][]byte{
		sharedApplyProgressGetFixture(t),
		sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000001"),
		sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000002"),
	} {
		var envelope struct {
			Outcome string `json:"outcome"`
			Code    string `json:"code"`
		}
		if err := json.Unmarshal(fixture, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Outcome != "committed" || envelope.Code != "ok" {
			t.Fatalf("fixture = %s", fixture)
		}
	}
}

func TestGetApplyProgressConsumesBoundedSharedFixture(t *testing.T) {
	fixture := sharedApplyProgressGetFixture(t)
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sdd/changes/change/apply-progress" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("project"); got != "project" {
			t.Fatalf("project = %q", got)
		}
		_, _ = w.Write(fixture)
	})

	result, err := client.GetApplyProgress(context.Background(), "project", "change")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "committed" || result.Code != "ok" || result.State.Generation != 1 || result.State.Revision != 1 || result.State.Snapshot.Change != "change" || result.State.Snapshot.Status != applyprogress.StatusPartial {
		t.Fatalf("result = %#v", result)
	}
	if len(result.State.Batches) != 0 {
		t.Fatalf("guarded GET aggregated evidence = %#v", result.State.Batches)
	}
}

func TestGetApplyProgressEvidenceUsesReferencedBatchEndpoint(t *testing.T) {
	batch := sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000001")
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/sdd/changes/change%25_/apply-evidence/apb-00000000000000000000000000000001" || r.URL.Query().Get("expected_head_digest") != strings.Repeat("a", 64) {
			t.Fatalf("request = %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery)
		}
		if got := r.URL.Query().Get("project"); got != "project name" {
			t.Fatalf("project = %q", got)
		}
		_, _ = w.Write(batch)
	})

	got, err := client.GetApplyProgressEvidence(context.Background(), "project name", "change%_", "apb-00000000000000000000000000000001", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if got.BatchID != "apb-00000000000000000000000000000001" || len(got.Entries) != 1 {
		t.Fatalf("batch = %#v", got)
	}
}

func sharedApplyProgressEvidenceFixture(t *testing.T, batchID string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture source path")
	}
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", "testdata", "sdd-progress", "apply-evidence-"+batchID+".json")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestApplyProgressClientMakesLegacyNonJSONResponseActionable(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})
	_, err := client.GetApplyProgress(context.Background(), "project", "change")
	var applyErr *hiveclient.ApplyProgressError
	if !errors.As(err, &applyErr) {
		t.Fatalf("error = %v, want ApplyProgressError", err)
	}
	if applyErr.StatusCode != http.StatusNotFound || applyErr.Result.Code != "compatibility" || applyErr.Result.Recovery == "" {
		t.Fatalf("error = %#v", applyErr)
	}
}

func TestApplyProgressClientPreservesErrorEnvelopes(t *testing.T) {
	for _, tt := range []struct {
		name, method, code string
		status             int
	}{
		{name: "get validation", method: http.MethodGet, status: http.StatusUnprocessableEntity, code: "validation"},
		{name: "post validation", method: http.MethodPost, status: http.StatusUnprocessableEntity, code: "validation"},
		{name: "post unavailable", method: http.MethodPost, status: http.StatusServiceUnavailable, code: "unavailable"},
		{name: "request ID conflict", method: http.MethodPost, status: http.StatusConflict, code: "request_id_conflict"},
		{name: "batch collision", method: http.MethodPost, status: http.StatusConflict, code: "batch_collision"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method {
					t.Fatalf("method = %s, want %s", r.Method, tt.method)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"outcome":"invalid","code":"` + tt.code + `","state":{"generation":2,"revision":3,"digest":"current"},"receipt":{"request_id":"request-1","payload_sha256":"payload"},"recovery":"retry the same request ID"}`))
			})
			var result hiveclient.ApplyProgressResult
			var err error
			if tt.method == http.MethodGet {
				result, err = client.GetApplyProgress(context.Background(), "project", "change")
			} else {
				result, err = client.AdvanceApplyProgress(context.Background(), hiveclient.ApplyProgressAdvanceRequest{Project: "project", Change: "change", RequestID: "request-1"})
			}
			var applyErr *hiveclient.ApplyProgressError
			if !errors.As(err, &applyErr) || applyErr.StatusCode != tt.status {
				t.Fatalf("error = %#v", err)
			}
			if result.Code != tt.code || result.State.Generation != 2 || result.Receipt.RequestID != "request-1" || result.Recovery != "retry the same request ID" {
				t.Fatalf("result = %#v", result)
			}
			if applyErr.Result.Code != result.Code || applyErr.Result.State.Generation != result.State.Generation || applyErr.Result.State.Revision != result.State.Revision || applyErr.Result.State.Digest != result.State.Digest || applyErr.Result.Receipt != result.Receipt || applyErr.Result.Recovery != result.Recovery {
				t.Fatalf("error result = %#v, want %#v", applyErr.Result, result)
			}
		})
	}
}

// sharedApplyProgressGetFixture resolves from this source file, not the test process
// working directory, because Go runs packages from different module subdirectories.
func sharedApplyProgressGetFixture(t *testing.T) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture source path")
	}
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", "testdata", "sdd-progress", "apply-progress-get.json")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newSDDClient(t *testing.T, handler http.HandlerFunc) *hiveclient.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

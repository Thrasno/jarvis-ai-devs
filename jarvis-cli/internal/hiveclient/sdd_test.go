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

func TestGetApplyProgressSuccessorOccupancy(t *testing.T) {
	for _, tc := range []struct {
		name, response, category string
		occupied                 bool
	}{
		{"free", `{"occupied":false,"category":""}`, "", false},
		{"occupied memory", `{"occupied":true,"category":"memory"}`, "memory", true},
		{"occupied head", `{"occupied":true,"category":"head"}`, "head", true},
		{"occupied receipt", `{"occupied":true,"category":"receipt"}`, "receipt", true},
		{"occupied binding", `{"occupied":true,"category":"binding"}`, "binding", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.EscapedPath() != "/sdd/changes/change%25_/successor-occupancy" || r.URL.RawQuery != "project=project+name" || r.ContentLength > 0 || r.Header.Get("Content-Type") != "" {
					t.Errorf("unexpected request: %s %s?%s, length=%d", r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.ContentLength)
				}
				_, _ = w.Write([]byte(tc.response))
			})
			result, err := client.GetApplyProgressSuccessorOccupancy(context.Background(), "project name", "change%_")
			if err != nil || result.Occupied != tc.occupied || result.Category != tc.category {
				t.Fatalf("result=%#v, err=%v", result, err)
			}
		})
	}
}

func TestGetApplyProgressSuccessorOccupancyRejectsInvalidAdvisory(t *testing.T) {
	for _, tc := range []struct{ name, response string }{
		{"empty", `{}`},
		{"missing occupied", `{"category":""}`},
		{"missing category", `{"occupied":false}`},
		{"null occupied", `{"occupied":null,"category":""}`},
		{"null category", `{"occupied":false,"category":null}`},
		{"invalid category", `{"occupied":true,"category":"private server detail"}`},
		{"occupied without category", `{"occupied":true,"category":""}`},
		{"free with category", `{"occupied":false,"category":"memory"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.response))
			})
			result, err := client.GetApplyProgressSuccessorOccupancy(context.Background(), "project", "change")
			if err == nil || result != (hiveclient.ApplyProgressSuccessorOccupancy{}) {
				t.Fatalf("result=%#v, err=%v", result, err)
			}
			if strings.Contains(err.Error(), "private server detail") {
				t.Fatalf("error leaks server detail: %v", err)
			}
		})
	}
}

func TestGetApplyProgressSuccessorOccupancyErrors(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"coarse conflict"}`))
	})
	_, err := client.GetApplyProgressSuccessorOccupancy(context.Background(), "project", "change")
	var apiErr *hiveclient.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict || apiErr.Message != "coarse conflict" {
		t.Fatalf("error = %#v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.GetApplyProgressSuccessorOccupancy(ctx, "project", "change")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("transport error = %v", err)
	}
}

func TestPublishApplyProgressSuccessor(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    int
		response  string
		wantError bool
	}{
		{"genesis", http.StatusOK, `{"outcome":"committed","code":"ok","state":{"generation":1,"revision":1,"digest":"genesis"},"receipt":{"request_id":"successor","generation":1,"revision":1}}`, false},
		{"replay", http.StatusOK, `{"outcome":"replayed","code":"ok","state":{"generation":1,"revision":1,"digest":"genesis"},"receipt":{"request_id":"successor","generation":1,"revision":1}}`, false},
		{"blocked", http.StatusLocked, `{"outcome":"blocked","code":"predecessor_unsealed","recovery":"seal predecessor"}`, true},
		{"invalid", http.StatusUnprocessableEntity, `{"outcome":"invalid","code":"validation","recovery":"fix project"}`, true},
		{"server error", http.StatusInternalServerError, `{"outcome":"unavailable","code":"internal","recovery":"retry"}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.EscapedPath() != "/sdd/changes/change%25_/apply-progress/publish-successor" || r.URL.RawQuery != "" {
					t.Fatalf("request = %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body) != 1 || body["project"] != "project name" || r.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("body = %#v, content type = %q", body, r.Header.Get("Content-Type"))
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.response))
			})
			result, err := client.PublishApplyProgressSuccessor(context.Background(), "project name", "change%_")
			if tt.wantError {
				var applyErr *hiveclient.ApplyProgressError
				if !errors.As(err, &applyErr) || applyErr.StatusCode != tt.status || applyErr.Result.Code != result.Code || applyErr.Result.Recovery != result.Recovery {
					t.Fatalf("result = %#v, error = %#v", result, err)
				}
				if result.Recovery == "" || result.Code == "" {
					t.Fatalf("missing recovery envelope: %#v", result)
				}
			} else if err != nil || result.State.Generation != 1 || result.State.Revision != 1 || result.Receipt.RequestID != "successor" || result.Outcome == "" {
				t.Fatalf("result = %#v, error = %v", result, err)
			}
		})
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

func TestGetSDDStoreBindingContractAndAbsence(t *testing.T) {
	const project = "project%_"
	const change = "change%_"
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/sdd/changes/change%25_/store-binding" {
			t.Fatalf("request = %s %s", r.Method, r.URL.EscapedPath())
		}
		if got := r.URL.Query().Get("project"); got != project {
			t.Fatalf("project query = %q", got)
		}
		_, _ = w.Write([]byte(sddStoreBindingResponse(hiveclient.CanonicalProjectKey(project), change, "1", "hive", "cli", "2026-08-01T10:00:00Z", nil)))
	})

	binding, found, err := client.GetSDDStoreBinding(context.Background(), project, change)
	if err != nil || !found {
		t.Fatalf("binding=%#v found=%t err=%v", binding, found, err)
	}
	if binding.Project != hiveclient.CanonicalProjectKey(project) || binding.Change != change || binding.Mode != hiveclient.SDDStoreModeHive || binding.Provenance != "cli" || binding.CreatedAt.IsZero() {
		t.Fatalf("binding = %#v", binding)
	}

	missing := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"not_found"}`))
	})
	_, found, err = missing.GetSDDStoreBinding(context.Background(), "project", "change")
	if err != nil || found {
		t.Fatalf("found=%t err=%v", found, err)
	}
}

func TestAdoptSDDStoreBindingCreatedAndReplay(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		created bool
	}{
		{name: "created", status: http.StatusCreated, created: true},
		{name: "replay", status: http.StatusOK, created: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.EscapedPath() != "/sdd/changes/change%25_/store-binding/adopt" {
					t.Fatalf("request = %s %s", r.Method, r.URL.EscapedPath())
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if len(request) != 3 || request["project"] != "project%_" || request["mode"] != "hive" || request["provenance"] != "cli" {
					t.Fatalf("request = %#v", request)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(sddStoreBindingResponse(hiveclient.CanonicalProjectKey("project%_"), "change%_", "1", "hive", "cli", "2026-08-01T10:00:00Z", &tt.created)))
			})

			binding, created, err := client.AdoptSDDStoreBinding(context.Background(), "project%_", "change%_", hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreModeHive, Provenance: "cli"})
			if err != nil || created != tt.created || binding.Mode != hiveclient.SDDStoreModeHive {
				t.Fatalf("binding=%#v created=%t err=%v", binding, created, err)
			}
		})
	}
}

func TestAdoptSDDStoreBindingReturnsTypedConflict(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"binding_conflict","existing":{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"first","created_at":"2026-08-01T10:00:00Z"},"requested":{"project":"project","change":"change","schema_version":"1","mode":"hybrid","provenance":"second"}}`))
	})

	_, _, err := client.AdoptSDDStoreBinding(context.Background(), "project", "change", hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreModeHybrid, Provenance: "second"})
	var conflict *hiveclient.SDDStoreBindingConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %#v, want typed conflict", err)
	}
	if !errors.Is(err, hiveclient.ErrSDDStoreBindingConflict) {
		t.Fatalf("error = %#v, want conflict sentinel", err)
	}
	if conflict.Existing.Provenance != "first" || conflict.Existing.CreatedAt.IsZero() || conflict.Requested.Provenance != "second" || !conflict.Requested.CreatedAt.IsZero() {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestAdoptSDDStoreBindingRejectsConflictThatDoesNotMatchRequest(t *testing.T) {
	validRequested := `{"project":"project","change":"change","schema_version":"1","mode":"hybrid","provenance":"second"}`
	for _, tt := range []struct {
		name      string
		existing  string
		requested string
	}{
		{name: "future requested schema", existing: `{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"first","created_at":"2026-08-01T10:00:00Z"}`, requested: `{"project":"project","change":"change","schema_version":"2","mode":"hybrid","provenance":"second"}`},
		{name: "requested mode mismatch", existing: `{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"first","created_at":"2026-08-01T10:00:00Z"}`, requested: `{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"second"}`},
		{name: "requested provenance mismatch", existing: `{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"first","created_at":"2026-08-01T10:00:00Z"}`, requested: `{"project":"project","change":"change","schema_version":"1","mode":"hybrid","provenance":"other"}`},
		{name: "existing equals requested", existing: `{"project":"project","change":"change","schema_version":"1","mode":"hybrid","provenance":"second","created_at":"2026-08-01T10:00:00Z"}`, requested: validRequested},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"code":"binding_conflict","existing":` + tt.existing + `,"requested":` + tt.requested + `}`))
			})

			_, _, err := client.AdoptSDDStoreBinding(context.Background(), "project", "change", hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreModeHybrid, Provenance: "second"})
			var protocolErr *hiveclient.SDDStoreBindingProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %#v, want protocol error", err)
			}
			if errors.Is(err, hiveclient.ErrSDDStoreBindingConflict) {
				t.Fatalf("error = %#v, must not authorize conflict", err)
			}
		})
	}
}

func TestSDDStoreBindingRejectsInvalidAuthorityResponses(t *testing.T) {
	valid := sddStoreBindingResponse("project", "change", "1", "hive", "cli", "2026-08-01T10:00:00Z", nil)
	oversized := `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"` + strings.Repeat("x", 64<<10) + `","created_at":"2026-08-01T10:00:00Z"}}`
	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "missing binding field", body: `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","created_at":"2026-08-01T10:00:00Z"}}`},
		{name: "unknown field", body: `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"cli","created_at":"2026-08-01T10:00:00Z","extra":true}}`},
		{name: "duplicate field", body: `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","mode":"hybrid","provenance":"cli","created_at":"2026-08-01T10:00:00Z"}}`},
		{name: "case alias", body: `{"Binding":{}}`},
		{name: "malformed", body: `{"binding":`},
		{name: "trailing JSON", body: valid + ` {}`},
		{name: "oversized", body: oversized},
		{name: "project mismatch", body: sddStoreBindingResponse("other", "change", "1", "hive", "cli", "2026-08-01T10:00:00Z", nil)},
		{name: "change mismatch", body: sddStoreBindingResponse("project", "other", "1", "hive", "cli", "2026-08-01T10:00:00Z", nil)},
		{name: "invalid mode", body: sddStoreBindingResponse("project", "change", "1", "none", "cli", "2026-08-01T10:00:00Z", nil)},
		{name: "untrimmed provenance", body: sddStoreBindingResponse("project", "change", "1", "hive", " cli ", "2026-08-01T10:00:00Z", nil)},
		{name: "non UTC timestamp", body: sddStoreBindingResponse("project", "change", "1", "hive", "cli", "2026-08-01T10:00:00+01:00", nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tt.body)) })
			_, _, err := client.GetSDDStoreBinding(context.Background(), "project", "change")
			if err == nil {
				t.Fatal("expected fail-closed protocol error")
			}
		})
	}
}

func TestSDDStoreBindingFutureSchemaAndErrorStatusContracts(t *testing.T) {
	future := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sddStoreBindingResponse("project", "change", "2", "hybrid", "cli", "2026-08-01T10:00:00Z", nil)))
	})
	binding, found, err := future.GetSDDStoreBinding(context.Background(), "project", "change")
	if err != nil || !found || binding.SchemaVersion != "2" || binding.Mode != hiveclient.SDDStoreModeHybrid {
		t.Fatalf("binding=%#v found=%t err=%v", binding, found, err)
	}

	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "unknown 404 is not missing", status: http.StatusNotFound, body: `{"code":"unavailable"}`},
		{name: "validation is an API error", status: http.StatusUnprocessableEntity, body: `{"code":"validation"}`},
		{name: "unavailable is an API error", status: http.StatusServiceUnavailable, body: `{"code":"unavailable"}`},
		{name: "malformed error envelope", status: http.StatusServiceUnavailable, body: `{"error":"unavailable"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, found, err := client.GetSDDStoreBinding(context.Background(), "project", "change")
			if err == nil || found {
				t.Fatalf("found=%t err=%v", found, err)
			}
		})
	}
}

func TestAdoptSDDStoreBindingRejectsSuccessStatusAndValueMismatches(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		created    *bool
		schema     string
		mode       string
		provenance string
	}{
		{name: "created status needs created true", status: http.StatusCreated, created: boolPointer(false), schema: "1", mode: "hive", provenance: "cli"},
		{name: "replay status needs created false", status: http.StatusOK, created: boolPointer(true), schema: "1", mode: "hive", provenance: "cli"},
		{name: "post schema must be one", status: http.StatusOK, created: boolPointer(false), schema: "2", mode: "hive", provenance: "cli"},
		{name: "post mode must match", status: http.StatusOK, created: boolPointer(false), schema: "1", mode: "hybrid", provenance: "cli"},
		{name: "post provenance must match", status: http.StatusOK, created: boolPointer(false), schema: "1", mode: "hive", provenance: "other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(sddStoreBindingResponse("project", "change", tt.schema, tt.mode, tt.provenance, "2026-08-01T10:00:00Z", tt.created)))
			})
			_, _, err := client.AdoptSDDStoreBinding(context.Background(), "project", "change", hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreModeHive, Provenance: "cli"})
			if err == nil {
				t.Fatal("expected fail-closed protocol error")
			}
		})
	}
}

func sddStoreBindingResponse(project, change, schema, mode, provenance, createdAt string, created *bool) string {
	binding := map[string]string{
		"project":        project,
		"change":         change,
		"schema_version": schema,
		"mode":           mode,
		"provenance":     provenance,
		"created_at":     createdAt,
	}
	response := map[string]any{"binding": binding}
	if created != nil {
		response["created"] = *created
	}
	body, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func boolPointer(value bool) *bool { return &value }

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

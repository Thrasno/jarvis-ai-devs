package hiveclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
		_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"explore","content":"# Explore","created_at":"2026-08-01T10:00:00Z"}]}`))
	})

	artifacts, err := client.FetchSDDArtifacts(context.Background(), "project name", "change%_")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	if artifacts[0].Artifact != "explore" || artifacts[0].Content != "# Explore" {
		t.Fatalf("artifact = %#v", artifacts[0])
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

func TestApplyProgressClientPreservesErrorEnvelopes(t *testing.T) {
	for _, tt := range []struct {
		name, method, code string
		status             int
	}{
		{name: "get capacity", method: http.MethodGet, status: http.StatusRequestEntityTooLarge, code: "capacity"},
		{name: "post validation", method: http.MethodPost, status: http.StatusUnprocessableEntity, code: "capacity"},
		{name: "post unavailable", method: http.MethodPost, status: http.StatusServiceUnavailable, code: "capacity"},
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

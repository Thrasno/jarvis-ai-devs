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

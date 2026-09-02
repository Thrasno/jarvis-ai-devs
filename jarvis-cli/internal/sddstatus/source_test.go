package sddstatus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestHiveSourceFetchArtifactsUsesDedicatedCompleteRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/governance/memories" {
			t.Fatal("HiveSource must not use generic governance memories")
		}
		if r.URL.EscapedPath() != "/sdd/changes/epic-06/artifacts" {
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
		if got := r.URL.Query().Get("project"); got != "jarvis-dev" {
			t.Fatalf("project query = %q", got)
		}
		_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"explore","content":"# Exploration","created_at":"2026-06-22T10:00:00Z"},{"artifact":"proposal","content":"# Proposal","created_at":"2026-06-22T10:05:00Z"}]}`))
	}))
	t.Cleanup(server.Close)
	source := newHiveSource(t, server.URL)

	artifacts, contents, err := source.FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if artifacts[sddstatus.ArtifactExplore] != sddstatus.ArtifactDone || artifacts[sddstatus.ArtifactProposal] != sddstatus.ArtifactDone {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	if contents[sddstatus.ArtifactProposal] != "# Proposal" {
		t.Fatalf("contents = %#v", contents)
	}
	status := sddstatus.ComputeStatus("epic-06", "hive", sddstatus.Input{Artifacts: artifacts, Contents: contents})
	if status.NextRecommended == sddstatus.PhaseExplore {
		t.Fatalf("valid exploration produced spurious %q recommendation", status.NextRecommended)
	}
}

func TestHiveSourceListChangesConsumesAllKeysetPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/governance/memories" {
			t.Fatal("HiveSource must not use generic governance memories")
		}
		if r.URL.Path != "/sdd/changes" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Fatalf("limit = %q", got)
		}
		requests++
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = w.Write([]byte(`{"changes":["alpha","bravo"],"next_cursor":"opaque-next"}`))
		case "opaque-next":
			_, _ = w.Write([]byte(`{"changes":["charlie"]}`))
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	t.Cleanup(server.Close)
	source := newHiveSource(t, server.URL)

	changes, err := source.ListChanges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0] != "alpha" || changes[1] != "bravo" || changes[2] != "charlie" {
		t.Fatalf("changes = %#v", changes)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func newHiveSource(t *testing.T, baseURL string) *sddstatus.HiveSource {
	t.Helper()
	client, err := hiveclient.New(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	return sddstatus.NewHiveSource(client, "jarvis-dev")
}

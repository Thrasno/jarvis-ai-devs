package sddstatus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestHiveSourceFetchArtifactsUsesTopicKey(t *testing.T) {
	source := newHiveSourceForMemories(t, `{
		"memories": [
			{
				"id": 132,
				"project": "jarvis-dev",
				"topic_key": "sdd/epic-06-projects-repository-api/explore",
				"title": "Explore: epic-06-projects-repository-api",
				"content": "# Exploration",
				"created_at": "2026-06-22T10:00:00Z"
			},
			{
				"id": 137,
				"project": "jarvis-dev",
				"topic_key": "sdd/epic-06-projects-repository-api/proposal",
				"title": "Proposal: epic-06-projects-repository-api",
				"content": "# Proposal",
				"created_at": "2026-06-22T10:05:00Z"
			}
		]
	}`)

	artifacts, contents, err := source.FetchArtifacts(context.Background(), "epic-06-projects-repository-api")
	if err != nil {
		t.Fatalf("FetchArtifacts returned error: %v", err)
	}

	if got := artifacts[sddstatus.ArtifactExplore]; got != sddstatus.ArtifactDone {
		t.Fatalf("explore artifact = %q, want done", got)
	}
	if got := artifacts[sddstatus.ArtifactProposal]; got != sddstatus.ArtifactDone {
		t.Fatalf("proposal artifact = %q, want done", got)
	}
	if got := contents[sddstatus.ArtifactProposal]; got != "# Proposal" {
		t.Fatalf("proposal content = %q, want # Proposal", got)
	}
}

func TestHiveSourceListChangesUsesTopicKey(t *testing.T) {
	source := newHiveSourceForMemories(t, `{
		"memories": [
			{
				"id": 137,
				"project": "jarvis-dev",
				"topic_key": "sdd/epic-06-projects-repository-api/proposal",
				"title": "Proposal: epic-06-projects-repository-api",
				"content": "# Proposal",
				"created_at": "2026-06-22T10:05:00Z"
			}
		]
	}`)

	changes, err := source.ListChanges(context.Background())
	if err != nil {
		t.Fatalf("ListChanges returned error: %v", err)
	}

	if len(changes) != 1 || changes[0] != "epic-06-projects-repository-api" {
		t.Fatalf("changes = %#v, want [epic-06-projects-repository-api]", changes)
	}
}

func newHiveSourceForMemories(t *testing.T, body string) *sddstatus.HiveSource {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/governance/memories" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("project"); got != "jarvis-dev" {
			t.Fatalf("project query = %q, want jarvis-dev", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatalf("New hive client: %v", err)
	}
	return sddstatus.NewHiveSource(client, "jarvis-dev")
}

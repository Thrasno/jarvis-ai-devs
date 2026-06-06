package hiveclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListsProjectsFromGovernanceEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/projects" {
			t.Fatalf("request = %s %s, want GET /governance/projects", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[{"name":"alpha","directory":"/repo/alpha","active_memory_count":3,"deleted_memory_count":1,"session_count":2,"prompt_count":4,"last_activity_at":"2026-06-06T20:00:00Z"}]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	projects, err := client.Projects(context.Background())
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "alpha" || projects[0].ActiveMemoryCount != 3 {
		t.Fatalf("projects = %+v, want alpha with 3 active memories", projects)
	}
}

func TestNewUsesBoundedHTTPTimeout(t *testing.T) {
	client, err := New("http://127.0.0.1:7438")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.http == nil {
		t.Fatal("New returned nil HTTP client")
	}
	if client.http.Timeout != defaultTimeout {
		t.Fatalf("HTTP client timeout = %v, want %v", client.http.Timeout, defaultTimeout)
	}
}

func TestClientListsMemoriesWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/governance/memories" {
			t.Fatalf("path = %q, want /governance/memories", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("project") != "alpha" || query.Get("include_deleted") != "true" || query.Get("limit") != "2" {
			t.Fatalf("query = %q, want project/include_deleted/limit filters", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memories":[{"id":7,"sync_id":"sync-7","project":"alpha","category":"decision","title":"Governance boundary","created_by":"agent","created_at":"2026-06-06T20:00:00Z","deleted":true}]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	memories, err := client.Memories(context.Background(), MemoryFilter{Project: "alpha", IncludeDeleted: true, Limit: 2})
	if err != nil {
		t.Fatalf("Memories: %v", err)
	}
	if len(memories) != 1 || memories[0].ID != 7 || !memories[0].Deleted {
		t.Fatalf("memories = %+v, want deleted memory id 7", memories)
	}
}

func TestClientWarningsReportNotAvailableOnMissingEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Warnings(context.Background())
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("Warnings error = %v, want ErrNotAvailable", err)
	}
}

func TestClientReturnsDaemonErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"backup store is not configured"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Backups(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable || apiErr.Message != "backup store is not configured" {
		t.Fatalf("Backups error = %#v, want APIError 503 with daemon message", err)
	}
}

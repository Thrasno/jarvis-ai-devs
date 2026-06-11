package hiveclient

import (
	"context"
	"encoding/json"
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

func TestClientExecutesGuardWithExactConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/guards/execute" {
			t.Fatalf("request = %s %s, want POST /governance/guards/execute", r.Method, r.URL.Path)
		}
		var req GuardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.BackupID != "backup-1" || req.Confirmation != " DELETE memory 7 " || req.TargetID != 7 || req.Reason != "cleanup reason" {
			t.Fatalf("guard request = %+v, want exact backup and confirmation forwarded", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"delete","target_type":"memory","target_id":7,"backup_id":"backup-1","mutated":true}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.ExecuteGuard(context.Background(), GuardRequest{Operation: "delete", TargetType: "memory", TargetID: 7, BackupID: "backup-1", Confirmation: " DELETE memory 7 ", Reason: "cleanup reason"})
	if err != nil {
		t.Fatalf("ExecuteGuard: %v", err)
	}
	if !result.Mutated || result.Operation != "delete" || result.TargetID != 7 {
		t.Fatalf("guard result = %+v, want delete mutation result", result)
	}
}

func TestClientArchivesProjectWithExactConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/projects/alpha/archive" {
			t.Fatalf("request = %s %s, want POST /governance/projects/alpha/archive", r.Method, r.URL.Path)
		}
		var req ProjectArchiveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Project != "alpha" || req.BackupID != "backup-1" || req.Confirmation != " ARCHIVE project alpha " || req.ActorID != "tester" || req.Reason != "cleanup" {
			t.Fatalf("archive request = %+v, want exact project archive payload", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"archive","target_type":"project","project":"alpha","backup_id":"backup-1","mutated":true,"cloud_handoff_note":"Local project archive completed. No cloud project mutation was performed."}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.ArchiveProject(context.Background(), ProjectArchiveRequest{Project: "alpha", BackupID: "backup-1", Confirmation: " ARCHIVE project alpha ", ActorID: "tester", Reason: "cleanup"})
	if err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	if !result.Mutated || result.Project != "alpha" || result.CloudHandoffNote == "" {
		t.Fatalf("archive result = %+v, want local archive result with cloud handoff note", result)
	}
}

func TestClientArchivesProjectEscapesProjectNameInPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.RequestURI != "/governance/projects/team%2Falpha%20project/archive" {
			t.Fatalf("request = %s %s, want POST /governance/projects/team%%2Falpha%%20project/archive", r.Method, r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"archive","target_type":"project","project":"team/alpha project","backup_id":"backup-1","mutated":true}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.ArchiveProject(context.Background(), ProjectArchiveRequest{Project: "team/alpha project", BackupID: "backup-1", Confirmation: "ARCHIVE project team/alpha project"})
	if err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
}

func TestClientMergesProjectWithExactConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/projects/alpha/merge/beta" {
			t.Fatalf("request = %s %s, want POST /governance/projects/alpha/merge/beta", r.Method, r.URL.Path)
		}
		var req ProjectMergeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.SourceProject != "alpha" || req.TargetProject != "beta" || req.BackupID != "backup-1" || req.Confirmation != " MERGE project alpha INTO beta " || req.ActorID != "tester" || req.Reason != "dedupe" {
			t.Fatalf("merge request = %+v, want exact project merge payload", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"merge","target_type":"project","source_project":"alpha","target_project":"beta","backup_id":"backup-1","mutated":true,"cloud_handoff_note":"Local project merge metadata recorded. No cloud project mutation was performed."}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.MergeProject(context.Background(), ProjectMergeRequest{SourceProject: "alpha", TargetProject: "beta", BackupID: "backup-1", Confirmation: " MERGE project alpha INTO beta ", ActorID: "tester", Reason: "dedupe"})
	if err != nil {
		t.Fatalf("MergeProject: %v", err)
	}
	if !result.Mutated || result.SourceProject != "alpha" || result.TargetProject != "beta" || result.CloudHandoffNote == "" {
		t.Fatalf("merge result = %+v, want local merge result with cloud handoff note", result)
	}
}

func TestClientMergesProjectEscapesSourceAndTargetNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.RequestURI != "/governance/projects/team%2Fsource/merge/literal%252Ftarget" {
			t.Fatalf("request = %s %s, want escaped source and target merge path", r.Method, r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"merge","target_type":"project","source_project":"team/source","target_project":"literal%2Ftarget","backup_id":"backup-1","mutated":true}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.MergeProject(context.Background(), ProjectMergeRequest{SourceProject: "team/source", TargetProject: "literal%2Ftarget", BackupID: "backup-1", Confirmation: "MERGE project team/source INTO literal%2Ftarget"})
	if err != nil {
		t.Fatalf("MergeProject: %v", err)
	}
}

func TestClientExecuteGuardReturnsDaemonGuardError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"confirmation mismatch"}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.ExecuteGuard(context.Background(), GuardRequest{Operation: "delete", TargetType: "memory", TargetID: 7, BackupID: "backup-1", Confirmation: " DELETE memory 7 "})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "confirmation mismatch" {
		t.Fatalf("ExecuteGuard error = %#v, want APIError 400 with confirmation mismatch", err)
	}
}

// Task 4.1 — MergeProjects batch client method

func TestClientMergesProjectsBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/projects/merge" {
			t.Fatalf("request = %s %s, want POST /governance/projects/merge", r.Method, r.URL.Path)
		}
		var req ProjectMergeBatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Sources) != 2 || req.Target != "beta" || req.BackupID != "backup-1" || req.Confirmation != "MERGE projects INTO beta" {
			t.Fatalf("batch merge request = %+v, want sources=[alpha,gamma] target=beta", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"merge","target":"beta","backup_id":"backup-1","results":[{"source":"alpha","target":"beta","mutated":true},{"source":"gamma","target":"beta","mutated":true}],"has_sync_evidence":false}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.MergeProjects(context.Background(), ProjectMergeBatchRequest{
		Sources:      []string{"alpha", "gamma"},
		Target:       "beta",
		BackupID:     "backup-1",
		Confirmation: "MERGE projects INTO beta",
	})
	if err != nil {
		t.Fatalf("MergeProjects: %v", err)
	}
	if result.Target != "beta" || result.BackupID != "backup-1" {
		t.Fatalf("batch result = %+v, want target=beta backup=backup-1", result)
	}
	if len(result.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(result.Results))
	}
	if !result.Results[0].Mutated || result.Results[0].Source != "alpha" {
		t.Fatalf("result[0] = %+v, want alpha mutated", result.Results[0])
	}
	if !result.Results[1].Mutated || result.Results[1].Source != "gamma" {
		t.Fatalf("result[1] = %+v, want gamma mutated", result.Results[1])
	}
}

func TestClientMergeProjectsBatchAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"sources and target are required"}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.MergeProjects(context.Background(), ProjectMergeBatchRequest{Sources: []string{}, Target: "beta", BackupID: "b-1", Confirmation: "MERGE projects INTO beta"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("MergeProjects error = %#v, want APIError 400", err)
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
		_, _ = w.Write([]byte(`{"memories":[{"id":7,"sync_id":"sync-7","project":"alpha","category":"decision","title":"Governance boundary","created_by":"agent","created_at":"2026-06-06T20:00:00Z","deleted":true,"deleted_at":"2026-06-06T21:00:00Z","deleted_by":"tester","delete_reason":"manual cleanup"}]}`))
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
	if len(memories) != 1 || memories[0].ID != 7 || !memories[0].Deleted || memories[0].DeletedAt == nil || memories[0].DeletedBy != "tester" || memories[0].DeleteReason != "manual cleanup" {
		t.Fatalf("memories = %+v, want deleted memory id 7", memories)
	}
}

func TestClientWarningsReturnsDaemonErrorOnMissingEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Warnings(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("Warnings error = %#v, want APIError 404", err)
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

// Task 4.1 — MemoryByID + struct field additions

func TestClient_MemoryByID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/memories/42" {
			t.Fatalf("request = %s %s, want GET /governance/memories/42", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memory":{"id":42,"sync_id":"s-1","project":"alpha","category":"manual","title":"the title","content":"the content","created_by":"tester","created_at":"2026-06-08T10:00:00Z","deleted":false}}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	memory, err := client.MemoryByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("MemoryByID: %v", err)
	}
	if memory.ID != 42 {
		t.Fatalf("ID = %d, want 42", memory.ID)
	}
	if memory.Title != "the title" {
		t.Fatalf("Title = %q, want 'the title'", memory.Title)
	}
	if memory.Content != "the content" {
		t.Fatalf("Content = %q, want 'the content'", memory.Content)
	}
}

func TestClient_MemoryByID_NotFound_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"memory not found"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.MemoryByID(context.Background(), 999)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("MemoryByID error = %#v, want APIError 404", err)
	}
}

func TestClient_ProjectList_UnsyncedCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[{"name":"alpha","directory":"/repo/alpha","active_memory_count":3,"deleted_memory_count":1,"session_count":2,"prompt_count":4,"last_activity_at":"2026-06-06T20:00:00Z","unsynced_count":5}]}`))
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
	if len(projects) != 1 || projects[0].UnsyncedCount != 5 {
		t.Fatalf("projects[0].UnsyncedCount = %d, want 5; projects=%+v", projects[0].UnsyncedCount, projects)
	}
}

// TestDeleteProject_BuildsCorrectRequest verifies that DeleteProject posts to
// .../governance/projects/{name}/delete with the serialized ProjectDeleteRequest body.
// T2.1 — GetConfigStatus tests

func TestGetConfigStatus_DecodesDTOFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/config/status" {
			t.Fatalf("request = %s %s, want GET /governance/config/status", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"configured":true,"source":"file","api_url":"https://hive.example.com","email":"user@example.com","password_set":true,"password_masked":"********","auto_sync":true,"env_active":false,"restart_hint":"","warnings":null}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	status, err := client.GetConfigStatus(context.Background())
	if err != nil {
		t.Fatalf("GetConfigStatus: %v", err)
	}
	if !status.Configured {
		t.Fatalf("Configured = false, want true")
	}
	if status.APIURL != "https://hive.example.com" {
		t.Fatalf("APIURL = %q, want https://hive.example.com", status.APIURL)
	}
	if status.Email != "user@example.com" {
		t.Fatalf("Email = %q, want user@example.com", status.Email)
	}
	if status.PasswordMasked != "********" {
		t.Fatalf("PasswordMasked = %q, want ********", status.PasswordMasked)
	}
	if !status.AutoSync {
		t.Fatalf("AutoSync = false, want true")
	}
	if status.EnvActive {
		t.Fatalf("EnvActive = true, want false")
	}
}

func TestGetConfigStatus_ReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"daemon not ready"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.GetConfigStatus(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable || apiErr.Message != "daemon not ready" {
		t.Fatalf("GetConfigStatus error = %#v, want APIError 503 with daemon not ready", err)
	}
}

// T2.1 — UpdateConfig tests

func TestUpdateConfig_SendsRequestBodyAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/config" {
			t.Fatalf("request = %s %s, want POST /governance/config", r.Method, r.URL.Path)
		}
		var req ConfigUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.APIURL != "https://hive.example.com" || req.Email != "user@example.com" || req.Password != "newpassword" || !req.AutoSync {
			t.Fatalf("update request = %+v, want exact fields forwarded", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"configured":true,"source":"file","api_url":"https://hive.example.com","email":"user@example.com","password_set":true,"password_masked":"********","auto_sync":true,"env_active":false,"restart_hint":"Saved. Restart hive-daemon for the new configuration to take effect.","warnings":null,"restart_required":true}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := client.UpdateConfig(context.Background(), ConfigUpdateRequest{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "newpassword",
		AutoSync: true,
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if !resp.RestartRequired {
		t.Fatalf("RestartRequired = false, want true")
	}
	if resp.RestartHint == "" {
		t.Fatalf("RestartHint is empty, want non-empty restart hint")
	}
	if resp.EnvActive {
		t.Fatalf("EnvActive = true, want false")
	}
	if resp.Status.APIURL != "https://hive.example.com" {
		t.Fatalf("Status.APIURL = %q, want https://hive.example.com", resp.Status.APIURL)
	}
}

func TestUpdateConfig_ReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"api_url is required and must include a scheme and host"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.UpdateConfig(context.Background(), ConfigUpdateRequest{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("UpdateConfig error = %#v, want APIError 400", err)
	}
}

// T2.1 — TestConnection tests

func TestTestConnection_OKTrueReturnsNilError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/config/test" {
			t.Fatalf("request = %s %s, want POST /governance/config/test", r.Method, r.URL.Path)
		}
		var req ConfigTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.APIURL != "https://hive.example.com" || req.Email != "user@example.com" || req.Password != "testpass" {
			t.Fatalf("test request = %+v, want exact fields forwarded", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message":"Connection succeeded"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.TestConnection(context.Background(), ConfigTestRequest{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "testpass",
	})
	if err != nil {
		t.Fatalf("TestConnection: unexpected Go error %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true")
	}
	if result.Message != "Connection succeeded" {
		t.Fatalf("result.Message = %q, want Connection succeeded", result.Message)
	}
}

func TestTestConnection_OKFalseReturnsNilErrorWithResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"message":"Connection failed: 401 Unauthorized"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.TestConnection(context.Background(), ConfigTestRequest{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "wrongpass",
	})
	if err != nil {
		t.Fatalf("TestConnection: unexpected Go error for ok:false response — %v", err)
	}
	if result.OK {
		t.Fatalf("result.OK = true, want false for connection failure")
	}
	if result.Message == "" {
		t.Fatalf("result.Message is empty, want non-empty failure message")
	}
}

func TestTestConnection_TransportErrorReturnsGoError(t *testing.T) {
	// Use a server that's immediately closed — transport will fail.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close() // close immediately to force transport error

	client, err := New(serverURL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.TestConnection(context.Background(), ConfigTestRequest{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "pass",
	})
	if err == nil {
		t.Fatal("TestConnection: expected Go error for transport failure, got nil")
	}
}

func TestDeleteProject_BuildsCorrectRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/projects/alpha/delete" {
			t.Fatalf("request = %s %s, want POST /governance/projects/alpha/delete", r.Method, r.URL.Path)
		}
		var req ProjectDeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Project != "alpha" || req.BackupID != "backup-1" || req.Confirmation != "PURGE project alpha" || req.ActorID != "tester" || req.Reason != "cleanup" {
			t.Fatalf("delete request = %+v, want exact project delete payload", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"operation":"purge","target_type":"project","project":"alpha","backup_id":"backup-1","rows_deleted":5,"mutated":true,"cloud_handoff_note":"Project purged locally. Cloud data not removed — no tombstone sync protocol exists yet."}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.DeleteProject(context.Background(), ProjectDeleteRequest{
		Project:      "alpha",
		BackupID:     "backup-1",
		Confirmation: "PURGE project alpha",
		ActorID:      "tester",
		Reason:       "cleanup",
	})
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if !result.Mutated || result.Project != "alpha" || result.RowsDeleted != 5 || result.CloudHandoffNote == "" {
		t.Fatalf("delete result = %+v, want mutated alpha with rows_deleted=5 and cloud handoff note", result)
	}
}

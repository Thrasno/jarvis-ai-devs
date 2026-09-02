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

func TestClientReadsMigrationIdentityStatusAndRequestsRollback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/governance/project-identity/status":
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", r.Method)
			}
			_, _ = w.Write([]byte(`{"state":"migration-blocked","reason":"duplicate canonical project","continuation":"hive project identity status","backup_id":"migration-backup-1"}`))
		case "/governance/restores":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode rollback: %v", err)
			}
			if request["backup_id"] != "migration-backup-1" || request["confirmation"] != "RESTORE migration-backup-1" {
				t.Fatalf("rollback = %#v, want exact explicit selection", request)
			}
			_, _ = w.Write([]byte(`{"restore":{"backup_id":"migration-backup-1","status":"restart-requested","requires_daemon_restart":true,"message":"restore scheduled"}}`))
		case "/governance/project-identity/retry":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			_, _ = w.Write([]byte(`{"state":"restart-requested"}`))
		case "/governance/project-identity/resolve":
			var request IdentityResolutionRequest
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&request) != nil || request.SourceProject != "Foo-Bar" || request.TargetProject != "foo-bar" {
				t.Fatalf("identity resolve request = %s %+v", r.Method, request)
			}
			_, _ = w.Write([]byte(`{"state":"resolution-recorded"}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	status, err := client.MigrationIdentityStatus(context.Background())
	if err != nil || status.BackupID != "migration-backup-1" || status.Continuation != "hive project identity status" {
		t.Fatalf("status = %+v, err = %v", status, err)
	}
	restore, err := client.RestoreMigrationBackup(context.Background(), "migration-backup-1", "RESTORE migration-backup-1")
	if err != nil {
		t.Fatalf("RestoreMigrationBackup: %v", err)
	}
	// The daemon's outcome must survive decoding: the CLI decides between
	// "the daemon scheduled the restart" and "you must stop the daemon
	// yourself" from exactly this status.
	if restore.Status != RestoreStatusRestartRequested || !restore.RequiresDaemonRestart || restore.Message != "restore scheduled" {
		t.Fatalf("restore = %+v, want the daemon's own outcome", restore)
	}
	if err := client.ResolveMigrationIdentity(context.Background(), IdentityResolutionRequest{SourceProject: "Foo-Bar", TargetProject: "foo-bar", BackupID: "migration-backup-1", Confirmation: "RESOLVE project identity Foo-Bar INTO foo-bar"}); err != nil {
		t.Fatalf("ResolveMigrationIdentity: %v", err)
	}
	if err := client.RequestMigrationRetry(context.Background()); err != nil {
		t.Fatalf("RequestMigrationRetry: %v", err)
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

func TestClientLoadsSafetyCapabilitiesAndCreatesFreshBackup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/governance/capabilities":
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", r.Method)
			}
			_, _ = w.Write([]byte(`{"capabilities":{"delete_restore":true,"expected_identity":true,"request_receipts":true,"mutation_sync_v2":true}}`))
		case "/governance/backups":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"backup":{"id":"fresh-backup","created_at":"2026-07-15T10:00:00Z"}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	capabilities, err := client.Capabilities(context.Background())
	if err != nil || !capabilities.SupportsGuardedDeleteRestore() {
		t.Fatalf("capabilities = %+v, err = %v", capabilities, err)
	}
	backup, err := client.CreateBackup(context.Background())
	if err != nil || backup.ID != "fresh-backup" {
		t.Fatalf("backup = %+v, err = %v", backup, err)
	}
}

func TestClientReadsMutationReceiptWithTargetIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/mutations/request-1" || r.URL.Query().Get("target_id") != "7" || r.URL.Query().Get("project") != "alpha" || r.URL.Query().Get("sync_id") != "sync-7" {
			t.Fatalf("request = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"receipt":{"request_id":"request-1","operation":"delete","target_id":7,"project":"alpha","entity_sync_id":"sync-7","event_id":"event-1","local_status":"committed","shared_status":"pending"}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := client.MutationReceipt(context.Background(), "request-1", 7, "alpha", "sync-7")
	if err != nil || receipt.LocalStatus != "committed" || receipt.SharedStatus != "pending" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
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

func TestClientListsDeletedOnlyMemories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("deleted_only") != "true" || r.URL.Query().Get("include_deleted") != "" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memories":[{"id":9,"sync_id":"sync-9","project":"alpha","title":"Deleted","deleted":true}]}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	memories, err := client.Memories(context.Background(), MemoryFilter{Project: "alpha", DeletedOnly: true})
	if err != nil || len(memories) != 1 || !memories[0].Deleted {
		t.Fatalf("memories=%+v err=%v", memories, err)
	}
}

func TestClientSerializesStructuredTopicFilters(t *testing.T) {
	tests := []struct {
		name       string
		filter     MemoryFilter
		wantKey    string
		wantPrefix string
	}{
		{name: "omitted", filter: MemoryFilter{Project: "alpha"}},
		{name: "exact", filter: MemoryFilter{Project: "alpha", TopicKey: clientStringPointer("sdd/spec %_")}, wantKey: "sdd/spec %_"},
		{name: "prefix", filter: MemoryFilter{Project: "alpha", TopicPrefix: clientStringPointer("設計/認証 %_")}, wantPrefix: "設計/認証 %_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				if query.Get("topic_key") != tt.wantKey || query.Get("topic_prefix") != tt.wantPrefix {
					t.Fatalf("decoded query = %q, want topic_key=%q topic_prefix=%q", r.URL.RawQuery, tt.wantKey, tt.wantPrefix)
				}
				if tt.wantKey == "" && tt.wantPrefix == "" && (query.Has("topic_key") || query.Has("topic_prefix")) {
					t.Fatalf("omitted topic filters serialized in %q", r.URL.RawQuery)
				}
				_, _ = w.Write([]byte(`{"memories":[]}`))
			}))
			defer server.Close()
			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := client.Memories(context.Background(), tt.filter); err != nil {
				t.Fatalf("Memories: %v", err)
			}
		})
	}
}

func clientStringPointer(value string) *string { return &value }

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

func TestClientDeletedMemoryByIDUsesDeletedOnlyBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/governance/memories" || r.URL.Query().Get("project") != "alpha" || r.URL.Query().Get("id") != "42" || r.URL.Query().Get("deleted_only") != "true" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"memories":[{"id":42,"sync_id":"s-1","project":"alpha","deleted":true}]}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	memory, err := client.DeletedMemoryByID(context.Background(), 42, "alpha")

	if err != nil || memory.ID != 42 || !memory.Deleted {
		t.Fatalf("memory=%+v err=%v", memory, err)
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
	if !status.PasswordSet {
		t.Fatalf("PasswordSet = false, want true")
	}
	if status.Source != "file" {
		t.Fatalf("Source = %q, want file", status.Source)
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

func TestTestConnection_ReturnsAPIErrorOnNon2xx(t *testing.T) {
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

	_, err = client.TestConnection(context.Background(), ConfigTestRequest{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "api_url is required and must include a scheme and host" {
		t.Fatalf("TestConnection error = %#v, want APIError 400 with api_url message", err)
	}
}

func TestUpdateConfig_ForwardsMaskedSentinelVerbatim(t *testing.T) {
	var receivedPassword string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ConfigUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		receivedPassword = req.Password
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"configured":true,"source":"file","api_url":"https://hive.example.com","email":"user@example.com","password_set":true,"password_masked":"********","auto_sync":true,"env_active":false}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.UpdateConfig(context.Background(), ConfigUpdateRequest{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "********",
		AutoSync: true,
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if receivedPassword != "********" {
		t.Fatalf("server received password = %q, want ********", receivedPassword)
	}
}

// T16 — Phase 3 — hiveclient.Timeline

func TestTimeline_SuccessfulFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/projects/atlas/timeline" {
			t.Fatalf("request = %s %s, want GET /governance/projects/atlas/timeline", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memories":[{"id":1,"sync_id":"s-1","project":"atlas","category":"decision","title":"Use Go","created_by":"agent","created_at":"2026-06-01T10:00:00Z","deleted":false},{"id":2,"sync_id":"s-2","project":"atlas","category":"architecture","title":"Hexagonal layout","created_by":"agent","created_at":"2026-06-02T10:00:00Z","deleted":false}]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Timeline(context.Background(), "atlas")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(result.Memories) != 2 {
		t.Fatalf("len(result.Memories) = %d, want 2", len(result.Memories))
	}
	if result.Memories[0].Category != "decision" {
		t.Fatalf("result.Memories[0].Category = %q, want decision", result.Memories[0].Category)
	}
	if result.Memories[1].Category != "architecture" {
		t.Fatalf("result.Memories[1].Category = %q, want architecture", result.Memories[1].Category)
	}
	if result.Truncated {
		t.Fatal("result.Truncated = true, want false for normal result")
	}
}

func TestTimeline_ProjectNotFound_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"project not found"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Timeline(context.Background(), "ghost")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("Timeline error = %#v, want APIError 404", err)
	}
	if result.Memories != nil {
		t.Fatalf("result.Memories = %v, want nil on error", result.Memories)
	}
}

func TestTimeline_EscapesProjectNameInPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/governance/projects/team%2Fatlas/timeline" {
			t.Fatalf("RequestURI = %q, want /governance/projects/team%%2Fatlas/timeline", r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memories":[]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Timeline(context.Background(), "team/atlas")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
}

func TestTimeline_TruncatedFlag_True(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memories":[{"id":1,"sync_id":"s-1","project":"atlas","category":"decision","title":"A decision","created_by":"agent","created_at":"2026-06-01T10:00:00Z","deleted":false}],"truncated":true}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Timeline(context.Background(), "atlas")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(result.Memories) != 1 {
		t.Fatalf("len(result.Memories) = %d, want 1", len(result.Memories))
	}
	if !result.Truncated {
		t.Fatal("result.Truncated = false, want true when server returns truncated:true")
	}
}

func TestTimeline_TruncatedFlag_False(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"memories":[{"id":2,"sync_id":"s-2","project":"atlas","category":"architecture","title":"Layout","created_by":"agent","created_at":"2026-06-02T10:00:00Z","deleted":false}]}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Timeline(context.Background(), "atlas")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if result.Truncated {
		t.Fatal("result.Truncated = true, want false when server omits truncated field")
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

func TestStartEngramImportPreviewSendsSourceAndDecodesJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/imports/engram/preview" {
			t.Fatalf("request = %s %s, want POST /governance/imports/engram/preview", r.Method, r.URL.Path)
		}
		var req EngramImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Source != "C:/tmp/engram.db" || req.PreviewID != "" {
			t.Fatalf("request = %+v, want source-only preview", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job":{"id":"job-preview","kind":"preview","phase":"queued","message":"queued","percent":0,"done":false}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	job, err := client.StartEngramImportPreview(context.Background(), EngramImportRequest{Source: "C:/tmp/engram.db"})
	if err != nil {
		t.Fatalf("StartEngramImportPreview: %v", err)
	}
	if job.ID != "job-preview" || job.Kind != EngramImportJobKindPreview || job.Phase != EngramImportPhaseQueued {
		t.Fatalf("job = %+v, want queued preview job", job)
	}
}

func TestGetEngramImportJobDecodesProgressAndReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/imports/engram/jobs/job-preview" {
			t.Fatalf("request = %s %s, want GET import job", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job":{"id":"job-preview","kind":"preview","phase":"completed","message":"preview completed","processed":3,"total":3,"percent":100,"done":true,"report":{"preview_id":"job-preview","source_path":"C:/tmp/engram.db","source_fingerprint":"sha256:abc","projected":{"sessions":1,"prompts":1,"observations":1},"projected_by_project":[{"project":"proj-a","projected":{"sessions":1,"prompts":1,"observations":1}}],"imported":{"imported":0,"reused":0,"ambiguous":2},"ambiguous_duplicates":[{"source_id":"21","project":"proj-a","title":"Existing duplicate","reason":"multiple active Hive memories match project and title"}],"skipped_relations":2,"invalid_rows":[{"table":"observations","source_id":"22","reason":"session_id references missing or skipped session"}]}}}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	job, err := client.GetEngramImportJob(context.Background(), "job-preview")
	if err != nil {
		t.Fatalf("GetEngramImportJob: %v", err)
	}
	if !job.Done || job.Percent != 100 || job.Report == nil || job.Report.Projected.Observations != 1 || job.Report.SkippedRelations != 2 {
		t.Fatalf("job = %+v, want completed preview report", job)
	}
	if len(job.Report.InvalidRows) != 1 || job.Report.InvalidRows[0].Table != "observations" || job.Report.InvalidRows[0].SourceID != "22" || job.Report.InvalidRows[0].Reason == "" {
		t.Fatalf("invalid rows = %+v, want daemon report invalid_rows decoded", job.Report.InvalidRows)
	}
	if len(job.Report.ProjectedByProject) != 1 || job.Report.ProjectedByProject[0].Project != "proj-a" || job.Report.ProjectedByProject[0].Projected.Observations != 1 {
		t.Fatalf("projected_by_project = %+v, want proj-a per-entity counts", job.Report.ProjectedByProject)
	}
	if job.Report.Imported.Ambiguous != 2 {
		t.Fatalf("ambiguous count = %d, want 2", job.Report.Imported.Ambiguous)
	}
	if len(job.Report.AmbiguousDuplicates) != 1 || job.Report.AmbiguousDuplicates[0].SourceID != "21" || job.Report.AmbiguousDuplicates[0].Project != "proj-a" || job.Report.AmbiguousDuplicates[0].Title != "Existing duplicate" || job.Report.AmbiguousDuplicates[0].Reason == "" {
		t.Fatalf("ambiguous duplicates = %+v, want actionable duplicate details", job.Report.AmbiguousDuplicates)
	}
}

func TestStartEngramImportExecuteReturnsPreviewAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/imports/engram/execute" {
			t.Fatalf("request = %s %s, want POST /governance/imports/engram/execute", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"fresh engram import preview is required"}`))
	}))
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.StartEngramImportExecute(context.Background(), EngramImportRequest{Source: "C:/tmp/engram.db", PreviewID: "missing"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "fresh engram import preview is required" {
		t.Fatalf("StartEngramImportExecute error = %#v, want APIError 400 fresh preview", err)
	}
}

// T2.1 — GetSyncSummary tests

func TestGetSyncSummary_DecodesDTOFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/health/summary" {
			t.Fatalf("request = %s %s, want GET /governance/health/summary", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"reachable": true,
			"auth_ok": true,
			"auto_sync": true,
			"last_success_at": "2026-06-11T10:00:00Z",
			"last_failure_at": "2026-06-11T09:00:00Z",
			"last_error": "",
			"unsynced_memories": 3,
			"unsynced_prompts": 1,
			"unsynced_sessions": 2,
			"backoff_until": "0001-01-01T00:00:00Z",
			"consecutive_failures": 0
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	summary, err := client.GetSyncSummary(context.Background())
	if err != nil {
		t.Fatalf("GetSyncSummary: %v", err)
	}
	if !summary.Reachable {
		t.Fatalf("Reachable = false, want true")
	}
	if !summary.AuthOK {
		t.Fatalf("AuthOK = false, want true")
	}
	if !summary.AutoSync {
		t.Fatalf("AutoSync = false, want true")
	}
	if summary.UnsyncedMemories != 3 {
		t.Fatalf("UnsyncedMemories = %d, want 3", summary.UnsyncedMemories)
	}
	if summary.UnsyncedPrompts != 1 {
		t.Fatalf("UnsyncedPrompts = %d, want 1", summary.UnsyncedPrompts)
	}
	if summary.UnsyncedSessions != 2 {
		t.Fatalf("UnsyncedSessions = %d, want 2", summary.UnsyncedSessions)
	}
	if summary.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures = %d, want 0", summary.ConsecutiveFailures)
	}
	if summary.LastError != "" {
		t.Fatalf("LastError = %q, want empty", summary.LastError)
	}
	if summary.LastSuccessAt.IsZero() {
		t.Fatal("LastSuccessAt is zero, want non-zero")
	}
}

func TestGetSyncSummary_ReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error reading sync state"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.GetSyncSummary(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GetSyncSummary error = %#v, want APIError 500", err)
	}
}

func TestGetSyncSummary_ReturnsErrNotAvailableOn404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.GetSyncSummary(context.Background())
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("GetSyncSummary error = %#v, want ErrNotAvailable", err)
	}
}

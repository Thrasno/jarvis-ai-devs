package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_Login tests the login method with httptest server.
func TestClient_Login(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantToken      string
		wantStatusCode int
	}{
		{
			name: "successful login returns token",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/auth/login", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Return success response
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"token":      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
					"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			wantErr:   false,
			wantToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
		},
		{
			name: "login failure with 401",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, err := w.Write([]byte("invalid credentials"))
				require.NoError(t, err)
			},
			wantErr: true,
		},
		{
			name: "login failure with 500",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("server error"))
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			client := newClient(cfg)

			token, expiresAt, err := client.login(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
				assert.False(t, expiresAt.IsZero(), "expiresAt should not be zero")
			}
		})
	}
}

// TestClient_Sync tests the sync method with httptest server.
func TestClient_Sync(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		toSend        []*models.Memory
		lastSync      *time.Time
		wantErr       bool
		wantPushed    int
		wantPulled    int
	}{
		{
			name: "successful sync with observations",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/sync", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-token")

				// Return success response
				w.WriteHeader(http.StatusOK)
				resp := syncResponse{
					Pushed: 2,
					Pulled: []apiMemory{
						{
							SyncID:        "remote-sync-1",
							Project:       "test-project",
							Category:      "architecture",
							Title:         "Remote Memory 1",
							Content:       "Content from server",
							Tags:          []string{"remote"},
							FilesAffected: []string{"file.go"},
							CreatedBy:     "server-user",
							CreatedAt:     time.Now().UTC(),
							UpdatedAt:     time.Now().UTC(),
						},
					},
					Conflicts: 0,
				}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			toSend: []*models.Memory{
				createTestSyncMemory("local-sync-1"),
				createTestSyncMemory("local-sync-2"),
			},
			lastSync:   nil,
			wantErr:    false,
			wantPushed: 2,
			wantPulled: 1,
		},
		{
			name: "successful sync with no new observations",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				resp := syncResponse{
					Pushed:    0,
					Pulled:    []apiMemory{},
					Conflicts: 0,
				}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			toSend:     []*models.Memory{},
			lastSync:   nil,
			wantErr:    false,
			wantPushed: 0,
			wantPulled: 0,
		},
		{
			name: "sync with lastSync timestamp",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				var req syncRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				require.NoError(t, err)
				assert.NotNil(t, req.LastSync, "lastSync should be sent")

				w.WriteHeader(http.StatusOK)
				resp := syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			toSend: []*models.Memory{},
			lastSync: func() *time.Time {
				t := time.Now().Add(-1 * time.Hour)
				return &t
			}(),
			wantErr:    false,
			wantPushed: 0,
			wantPulled: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			client := newClient(cfg)

			resp, err := client.sync(context.Background(), "test-token", "test-project", []*models.Session{}, tt.toSend, []*models.Prompt{}, tt.lastSync, nil, nil)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPushed, resp.Pushed)
				assert.Len(t, resp.Pulled, tt.wantPulled)

				// Verify observations deserialize correctly
				if tt.wantPulled > 0 {
					pulled := resp.Pulled[0]
					assert.NotEmpty(t, pulled.SyncID)
					assert.NotEmpty(t, pulled.Title)
					assert.NotEmpty(t, pulled.Content)
					assert.False(t, pulled.CreatedAt.IsZero())
					assert.False(t, pulled.UpdatedAt.IsZero())
				}
			}
		})
	}
}

func TestClient_SyncRequest_OmitsLegacyMetadataFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		memories, ok := body["memories"].([]any)
		require.True(t, ok, "memories must be encoded as a JSON array")
		require.Len(t, memories, 1)

		memory, ok := memories[0].(map[string]any)
		require.True(t, ok, "memory payload must be encoded as a JSON object")
		assert.NotContains(t, memory, "confidence")
		assert.NotContains(t, memory, "impact_score")

		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	c := newClient(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"})
	_, err := c.sync(context.Background(), "test-token", "test-project", []*models.Session{}, []*models.Memory{createTestSyncMemory("local-sync-metadata")}, []*models.Prompt{}, nil, nil, nil)
	require.NoError(t, err)
}

// TestClient_Sync_AuthFailure tests that 401 errors are properly propagated.
func TestClient_Sync_AuthFailure(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "401 unauthorized returns auth error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, err := w.Write([]byte("token expired"))
				require.NoError(t, err)
			},
			wantErr:        true,
			wantErrContain: "401",
		},
		{
			name: "403 forbidden returns auth error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, err := w.Write([]byte("insufficient permissions"))
				require.NoError(t, err)
			},
			wantErr:        true,
			wantErrContain: "403",
		},
		{
			name: "500 server error returns error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("internal server error"))
				require.NoError(t, err)
			},
			wantErr:        true,
			wantErrContain: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			client := newClient(cfg)

			_, err := client.sync(context.Background(), "invalid-token", "test-project", []*models.Session{}, []*models.Memory{}, []*models.Prompt{}, nil, nil, nil)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContain, "error should contain status code")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestClient_Sync_WithPrompts tests that prompts are serialized into the request payload.
func TestClient_Sync_WithPrompts(t *testing.T) {
	tests := []struct {
		name              string
		prompts           []*models.Prompt
		wantPromptsPushed int
		serverAssert      func(t *testing.T, req syncRequest)
	}{
		{
			name: "3 prompts are sent in payload",
			prompts: []*models.Prompt{
				createTestPrompt("sync-1", "proj", "first prompt"),
				createTestPrompt("sync-2", "proj", "second prompt"),
				createTestPrompt("sync-3", "proj", "third prompt"),
			},
			wantPromptsPushed: 3,
			serverAssert: func(t *testing.T, req syncRequest) {
				t.Helper()
				if len(req.Prompts) != 3 {
					t.Errorf("expected 3 prompts in request, got %d", len(req.Prompts))
				}
				if req.Prompts[0].SyncID != "sync-1" {
					t.Errorf("expected SyncID %q, got %q", "sync-1", req.Prompts[0].SyncID)
				}
				if req.Prompts[0].Content != "first prompt" {
					t.Errorf("expected Content %q, got %q", "first prompt", req.Prompts[0].Content)
				}
				if req.Prompts[1].SyncID != "sync-2" {
					t.Errorf("expected SyncID %q, got %q", "sync-2", req.Prompts[1].SyncID)
				}
			},
		},
		{
			name:              "0 prompts — field omitted from payload",
			prompts:           []*models.Prompt{},
			wantPromptsPushed: 0,
			serverAssert: func(t *testing.T, req syncRequest) {
				t.Helper()
				if len(req.Prompts) != 0 {
					t.Errorf("expected 0 prompts in request, got %d", len(req.Prompts))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/sync" {
					var req syncRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					require.NoError(t, err)
					tt.serverAssert(t, req)

					w.WriteHeader(http.StatusOK)
					resp := syncResponse{
						Pushed:        0,
						Pulled:        []apiMemory{},
						Conflicts:     0,
						PromptsPushed: tt.wantPromptsPushed,
					}
					require.NoError(t, json.NewEncoder(w).Encode(resp))
				}
			}))
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			c := newClient(cfg)

			resp, err := c.sync(context.Background(), "test-token", "test-project", []*models.Session{}, []*models.Memory{}, tt.prompts, nil, nil, nil)
			require.NoError(t, err)
			if resp.PromptsPushed != tt.wantPromptsPushed {
				t.Errorf("expected PromptsPushed=%d, got %d", tt.wantPromptsPushed, resp.PromptsPushed)
			}
		})
	}
}

func TestClient_Sync_MutationProtocolV2PayloadAndResponse(t *testing.T) {
	now := time.Date(2026, 5, 11, 14, 0, 0, 0, time.UTC)
	cursor := &db.MutationCursor{Sequence: 7, EventID: "evt-7"}
	pending := []db.MutationEnvelope{{
		EventID:      "evt-local-8",
		EntityType:   "memory",
		EntitySyncID: "mem-local-1",
		Project:      "test-project",
		Op:           db.MutationOpDelete,
		Sequence:     8,
		OccurredAt:   now,
		Tombstone: &db.MutationTombstonePayload{
			DeletedAt: now,
			DeletedBy: "tester",
			Reason:    "cleanup",
		},
	}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sync", r.URL.Path)
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, mutationProtocolVersion, req.ProtocolVersion)
		require.NotNil(t, req.MutationCursor)
		assert.Equal(t, int64(7), req.MutationCursor.Sequence)
		assert.Equal(t, "evt-7", req.MutationCursor.EventID)
		require.Len(t, req.Mutations, 1)
		assert.Equal(t, "evt-local-8", req.Mutations[0].EventID)
		assert.Equal(t, db.MutationOpDelete, req.Mutations[0].Op)
		require.NotNil(t, req.Mutations[0].Tombstone)
		assert.Equal(t, "cleanup", req.Mutations[0].Tombstone.Reason)

		resp := map[string]any{
			"pushed":             1,
			"pulled":             []any{},
			"conflicts":          0,
			"compatibility_mode": "mutation-sync-v2",
			"next_mutation_cursor": map[string]any{
				"sequence": float64(9),
				"event_id": "evt-remote-9",
			},
			"pulled_mutations": []map[string]any{{
				"event_id":       "evt-remote-9",
				"entity_type":    "memory",
				"entity_sync_id": "mem-remote-1",
				"project":        "test-project",
				"op":             "restore",
				"sequence":       9,
				"occurred_at":    now.Format(time.RFC3339),
			}},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	c := newClient(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"})
	resp, err := c.sync(context.Background(), "test-token", "test-project", []*models.Session{}, nil, nil, nil, pending, cursor)
	require.NoError(t, err)
	assert.Equal(t, "mutation-sync-v2", resp.CompatibilityMode)
	require.NotNil(t, resp.NextMutationCursor)
	assert.Equal(t, int64(9), resp.NextMutationCursor.Sequence)
	require.Len(t, resp.PulledMutations, 1)
	assert.Equal(t, "evt-remote-9", resp.PulledMutations[0].EventID)
}

func TestClient_Sync_LegacyResponseLeavesMutationProtocolFieldsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	c := newClient(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"})
	resp, err := c.sync(context.Background(), "test-token", "test-project", []*models.Session{}, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, resp.CompatibilityMode)
	assert.Nil(t, resp.NextMutationCursor)
	assert.Empty(t, resp.PulledMutations)
}

// createTestPrompt creates a test prompt for sync operations.
func createTestPrompt(syncID, project, content string) *models.Prompt {
	return &models.Prompt{
		SyncID:    syncID,
		Project:   project,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
}

// createTestSyncMemory creates a test memory for sync operations.
func createTestSyncMemory(syncID string) *models.Memory {
	return &models.Memory{
		SyncID:        syncID,
		Project:       "test-project",
		Category:      "test",
		Title:         "Test Memory",
		Content:       "Test content for " + syncID,
		Tags:          []string{"test"},
		FilesAffected: []string{},
		CreatedBy:     "test-user",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSyncStore implements the SyncStore interface for testing.
type mockSyncStore struct {
	unsynced             []*models.Memory
	lastSync             time.Time
	jwt                  string
	markedSynced         []string
	savedFromRemote      []*models.Memory
	unsyncedPrompts      []*models.Prompt
	unsyncedPromptsErr   error
	markedPromptSynced   []string
	markPromptSyncedErr  error
}

func (m *mockSyncStore) GetUnsynced(project string) ([]*models.Memory, error) {
	return m.unsynced, nil
}

func (m *mockSyncStore) MarkSynced(syncID string, at time.Time) error {
	m.markedSynced = append(m.markedSynced, syncID)
	return nil
}

func (m *mockSyncStore) SaveFromRemote(mem *models.Memory) error {
	m.savedFromRemote = append(m.savedFromRemote, mem)
	return nil
}

func (m *mockSyncStore) GetLastSync(project string) (time.Time, error) {
	return m.lastSync, nil
}

func (m *mockSyncStore) SetLastSync(project string, at time.Time) error {
	m.lastSync = at
	return nil
}

func (m *mockSyncStore) GetJWT() string {
	return m.jwt
}

func (m *mockSyncStore) SetJWT(token string, expiresAt time.Time) error {
	m.jwt = token
	return nil
}

func (m *mockSyncStore) GetUnsyncedPrompts(ctx context.Context, project string) ([]*models.Prompt, error) {
	return m.unsyncedPrompts, m.unsyncedPromptsErr
}

func (m *mockSyncStore) MarkPromptSynced(ctx context.Context, syncID string, at time.Time) error {
	m.markedPromptSynced = append(m.markedPromptSynced, syncID)
	return m.markPromptSyncedErr
}

// TestSyncer_Run tests the complete sync cycle.
func TestSyncer_Run(t *testing.T) {
	tests := []struct {
		name                  string
		setupStore            func() *mockSyncStore
		serverHandlers        []http.HandlerFunc
		wantErr               bool
		wantPushed            int
		wantPulled            int
		wantPromptsPushed     int
		wantMarkedSynced      int
		wantSavedFromRemote   int
		wantMarkedPromptCount int
	}{
		{
			name: "successful sync with valid JWT",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{
					jwt: "valid-cached-token",
					unsynced: []*models.Memory{
						createTestSyncMemory("local-1"),
						createTestSyncMemory("local-2"),
					},
				}
			},
			serverHandlers: []http.HandlerFunc{
				// Only sync endpoint (no login needed)
				func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sync" {
						w.WriteHeader(http.StatusOK)
						resp := syncResponse{
							Pushed: 2,
							Pulled: []apiMemory{
								{
									SyncID:        "remote-1",
									Project:       "test-project",
									Category:      "architecture",
									Title:         "Remote Memory",
									Content:       "Content from server",
									Tags:          []string{},
									FilesAffected: []string{},
									CreatedBy:     "server",
									CreatedAt:     time.Now().UTC(),
									UpdatedAt:     time.Now().UTC(),
									Confidence:    0.8,
									ImpactScore:   5.0,
								},
							},
							Conflicts: 0,
						}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					}
				},
			},
			wantErr:               false,
			wantPushed:            2,
			wantPulled:            1,
			wantPromptsPushed:     0,
			wantMarkedSynced:      2,
			wantSavedFromRemote:   1,
			wantMarkedPromptCount: 0,
		},
		{
			name: "sync with no JWT triggers login then sync",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{
					jwt: "", // No cached JWT
					unsynced: []*models.Memory{
						createTestSyncMemory("local-1"),
					},
				}
			},
			serverHandlers: []http.HandlerFunc{
				func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/auth/login":
						w.WriteHeader(http.StatusOK)
						resp := map[string]interface{}{
							"token":      "fresh-token",
							"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
						}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					case "/sync":
						// Verify token is present
						assert.Contains(t, r.Header.Get("Authorization"), "Bearer fresh-token")
						w.WriteHeader(http.StatusOK)
						resp := syncResponse{
							Pushed:    1,
							Pulled:    []apiMemory{},
							Conflicts: 0,
						}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					}
				},
			},
			wantErr:               false,
			wantPushed:            1,
			wantPulled:            0,
			wantPromptsPushed:     0,
			wantMarkedSynced:      1,
			wantSavedFromRemote:   0,
			wantMarkedPromptCount: 0,
		},
		{
			name: "sync with empty unsynced list",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{
					jwt:      "valid-token",
					unsynced: []*models.Memory{}, // No local changes
				}
			},
			serverHandlers: []http.HandlerFunc{
				func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sync" {
						w.WriteHeader(http.StatusOK)
						resp := syncResponse{
							Pushed:    0,
							Pulled:    []apiMemory{},
							Conflicts: 0,
						}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					}
				},
			},
			wantErr:               false,
			wantPushed:            0,
			wantPulled:            0,
			wantPromptsPushed:     0,
			wantMarkedSynced:      0,
			wantSavedFromRemote:   0,
			wantMarkedPromptCount: 0,
		},
		{
			name: "sync with lastSync timestamp",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{
					jwt:      "valid-token",
					unsynced: []*models.Memory{},
					lastSync: time.Now().Add(-1 * time.Hour),
				}
			},
			serverHandlers: []http.HandlerFunc{
				func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sync" {
						var req syncRequest
						err := json.NewDecoder(r.Body).Decode(&req)
						require.NoError(t, err)
						assert.NotNil(t, req.LastSync, "should send lastSync timestamp")

						w.WriteHeader(http.StatusOK)
						resp := syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					}
				},
			},
			wantErr:               false,
			wantPushed:            0,
			wantPulled:            0,
			wantPromptsPushed:     0,
			wantMarkedSynced:      0,
			wantSavedFromRemote:   0,
			wantMarkedPromptCount: 0,
		},
		// S1: 3 unsynced prompts → PromptsPushed=3, MarkPromptSynced called 3 times
		{
			name: "S1: 3 unsynced prompts are pushed and marked synced",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{
					jwt:      "valid-token",
					unsynced: []*models.Memory{},
					unsyncedPrompts: []*models.Prompt{
						createTestPrompt("p-sync-1", "test-project", "first prompt"),
						createTestPrompt("p-sync-2", "test-project", "second prompt"),
						createTestPrompt("p-sync-3", "test-project", "third prompt"),
					},
				}
			},
			serverHandlers: []http.HandlerFunc{
				func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sync" {
						w.WriteHeader(http.StatusOK)
						resp := syncResponse{
							Pushed:        0,
							Pulled:        []apiMemory{},
							Conflicts:     0,
							PromptsPushed: 3,
						}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					}
				},
			},
			wantErr:               false,
			wantPushed:            0,
			wantPulled:            0,
			wantPromptsPushed:     3,
			wantMarkedSynced:      0,
			wantSavedFromRemote:   0,
			wantMarkedPromptCount: 3,
		},
		// S2: 0 unsynced prompts → PromptsPushed=0, no MarkPromptSynced calls
		{
			name: "S2: 0 unsynced prompts → PromptsPushed=0 no mark calls",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{
					jwt:             "valid-token",
					unsynced:        []*models.Memory{},
					unsyncedPrompts: []*models.Prompt{},
				}
			},
			serverHandlers: []http.HandlerFunc{
				func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/sync" {
						w.WriteHeader(http.StatusOK)
						resp := syncResponse{
							Pushed:        0,
							Pulled:        []apiMemory{},
							Conflicts:     0,
							PromptsPushed: 0,
						}
						require.NoError(t, json.NewEncoder(w).Encode(resp))
					}
				},
			},
			wantErr:               false,
			wantPushed:            0,
			wantPulled:            0,
			wantPromptsPushed:     0,
			wantMarkedSynced:      0,
			wantSavedFromRemote:   0,
			wantMarkedPromptCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tt.setupStore()

			// Create test server
			mux := http.NewServeMux()
			for _, handler := range tt.serverHandlers {
				mux.HandleFunc("/", handler)
			}
			server := httptest.NewServer(mux)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}

			syncer := New(cfg, store)

			result, err := syncer.Sync(context.Background(), "test-project")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPushed, result.Pushed)
				assert.Equal(t, tt.wantPulled, result.Pulled)
				assert.Equal(t, tt.wantPromptsPushed, result.PromptsPushed, "wrong PromptsPushed count")
				assert.Equal(t, "test-project", result.Project)

				// Verify store interactions
				assert.Len(t, store.markedSynced, tt.wantMarkedSynced, "wrong number of memories marked as synced")
				assert.Len(t, store.savedFromRemote, tt.wantSavedFromRemote, "wrong number of remote memories saved")
				assert.Len(t, store.markedPromptSynced, tt.wantMarkedPromptCount, "wrong number of prompts marked as synced")
			}
		})
	}
}

// TestSyncer_Run_AuthFailureRetry tests 401 handling with token refresh and retry.
func TestSyncer_Run_AuthFailureRetry(t *testing.T) {
	store := &mockSyncStore{
		jwt: "expired-token",
		unsynced: []*models.Memory{
			createTestSyncMemory("local-1"),
		},
	}

	syncAttempts := 0
	loginAttempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login":
			loginAttempts++
			w.WriteHeader(http.StatusOK)
			resp := map[string]interface{}{
				"token":      "refreshed-token",
				"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			}
			require.NoError(t, json.NewEncoder(w).Encode(resp))
		case "/sync":
			syncAttempts++
			// First attempt fails with 401, second succeeds
			if syncAttempts == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, err := w.Write([]byte("token expired"))
				require.NoError(t, err)
			} else {
				// Verify we got the refreshed token
				assert.Contains(t, r.Header.Get("Authorization"), "Bearer refreshed-token")
				w.WriteHeader(http.StatusOK)
				resp := syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			}
		}
	}))
	defer server.Close()

	cfg := &Config{
		APIURL:   server.URL,
		Email:    "test@example.com",
		Password: "password123",
	}

	syncer := New(cfg, store)

	// Note: Current implementation doesn't auto-retry on 401
	// This test documents the EXPECTED behavior (401 returns error)
	// If retry logic is added later, update this test
	_, err := syncer.Sync(context.Background(), "test-project")

	// Current behavior: 401 causes error (no auto-retry in syncer.Sync)
	assert.Error(t, err, "sync should fail with 401 (no auto-retry in current implementation)")
	assert.Contains(t, err.Error(), "401", "error should mention 401")

	// If we implement retry logic, the test should become:
	// assert.NoError(t, err)
	// assert.Equal(t, 2, syncAttempts, "should retry after 401")
	// assert.Equal(t, 1, loginAttempts, "should refresh token")
}

// TestSyncer_Run_PersistentError tests that persistent errors are returned.
func TestSyncer_Run_PersistentError(t *testing.T) {
	store := &mockSyncStore{
		jwt: "valid-token",
		unsynced: []*models.Memory{
			createTestSyncMemory("local-1"),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("database error"))
			require.NoError(t, err)
		}
	}))
	defer server.Close()

	cfg := &Config{
		APIURL:   server.URL,
		Email:    "test@example.com",
		Password: "password123",
	}

	syncer := New(cfg, store)

	_, err := syncer.Sync(context.Background(), "test-project")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500", "error should mention status code")
}

// TestSyncer_MarkPromptSyncedError_NonFatal tests that a MarkPromptSynced error
// does not abort the Sync operation (S4 scenario).
func TestSyncer_MarkPromptSyncedError_NonFatal(t *testing.T) {
	store := &mockSyncStore{
		jwt:      "valid-token",
		unsynced: []*models.Memory{},
		unsyncedPrompts: []*models.Prompt{
			createTestPrompt("p-sync-1", "test-project", "prompt one"),
			createTestPrompt("p-sync-2", "test-project", "prompt two"),
		},
		markPromptSyncedErr: fmt.Errorf("db write failure"),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			w.WriteHeader(http.StatusOK)
			resp := syncResponse{
				Pushed:        0,
				Pulled:        []apiMemory{},
				Conflicts:     0,
				PromptsPushed: 2,
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

	syncer := New(cfg, store)

	result, err := syncer.Sync(context.Background(), "test-project")

	// Sync must succeed even if MarkPromptSynced fails
	assert.NoError(t, err, "Sync should succeed even when MarkPromptSynced errors")
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.PromptsPushed, "PromptsPushed should reflect server response")
}

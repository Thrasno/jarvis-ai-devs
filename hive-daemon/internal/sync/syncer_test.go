package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSyncStore implements the SyncStore interface for testing.
type mockSyncStore struct {
	mu                  sync.Mutex
	unsynced            []*models.Memory
	lastSync            time.Time
	jwt                 string
	markedSynced        []string
	savedFromRemote     []*models.Memory
	unsyncedPrompts     []*models.Prompt
	unsyncedPromptsErr  error
	markedPromptSynced  []string
	markPromptSyncedErr error
	healthByProject     map[string]db.SyncHealth
	recordAttemptCalls  []string
	recordSuccessCalls  []string
	recordFailureCalls  []string
}

func (m *mockSyncStore) GetUnsynced(project string) ([]*models.Memory, error) {
	return m.unsynced, nil
}

func (m *mockSyncStore) MarkSynced(syncID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markedSynced = append(m.markedSynced, syncID)
	return nil
}

func (m *mockSyncStore) SaveFromRemote(mem *models.Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedFromRemote = append(m.savedFromRemote, mem)
	return nil
}

func (m *mockSyncStore) GetLastSync(project string) (time.Time, error) {
	return m.lastSync, nil
}

func (m *mockSyncStore) SetLastSync(project string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSync = at
	return nil
}

func (m *mockSyncStore) GetSyncHealth(project string) (db.SyncHealth, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.healthByProject == nil {
		return db.SyncHealth{Project: project}, nil
	}
	health, ok := m.healthByProject[project]
	if !ok {
		return db.SyncHealth{Project: project}, nil
	}
	return health, nil
}

func (m *mockSyncStore) RecordSyncAttempt(project string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordAttemptCalls = append(m.recordAttemptCalls, project)
	health := m.getHealthLocked(project)
	health.LastAttemptAt = at
	m.healthByProject[project] = health
	return nil
}

func (m *mockSyncStore) RecordSyncSuccess(project string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordSuccessCalls = append(m.recordSuccessCalls, project)
	health := m.getHealthLocked(project)
	health.LastAttemptAt = at
	health.LastSuccessAt = at
	health.LastFailureAt = time.Time{}
	health.ConsecutiveFailures = 0
	health.BackoffUntil = time.Time{}
	health.LastError = ""
	m.healthByProject[project] = health
	m.lastSync = at
	return nil
}

func (m *mockSyncStore) RecordSyncFailure(project string, at time.Time, consecutiveFailures int, backoffUntil time.Time, syncErr error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordFailureCalls = append(m.recordFailureCalls, project)
	health := m.getHealthLocked(project)
	health.LastAttemptAt = at
	health.LastFailureAt = at
	health.ConsecutiveFailures = consecutiveFailures
	health.BackoffUntil = backoffUntil
	if syncErr != nil {
		health.LastError = sanitizeRecordedSyncError(syncErr)
	} else {
		health.LastError = ""
	}
	m.healthByProject[project] = health
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markedPromptSynced = append(m.markedPromptSynced, syncID)
	return m.markPromptSyncedErr
}

func (m *mockSyncStore) getHealthLocked(project string) db.SyncHealth {
	if m.healthByProject == nil {
		m.healthByProject = make(map[string]db.SyncHealth)
	}
	health, ok := m.healthByProject[project]
	if !ok {
		health = db.SyncHealth{Project: project}
	}
	return health
}

func sanitizeRecordedSyncError(err error) string {
	if err == nil {
		return ""
	}

	message := err.Error()
	for _, prefix := range []string{"login failed (", "sync failed ("} {
		if len(message) >= len(prefix) && message[:len(prefix)] == prefix {
			head, _, found := strings.Cut(strings.TrimSpace(message), ":")
			if found {
				return strings.TrimSpace(head)
			}
		}
	}

	return strings.TrimSpace(message)
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

func TestSyncer_Sync_HealthLifecycle(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name                     string
		health                   db.SyncHealth
		jitter                   time.Duration
		serverStatus             int
		serverBody               string
		wantErr                  string
		wantFailures             int
		wantBackoff              time.Duration
		wantLastError            string
		wantLastErrorNotContains string
		wantLastSuccessAt        time.Time
		wantLastFailureAt        time.Time
		wantRecordSuccessCalls   int
		wantRecordFailureCalls   int
	}{
		{
			name: "success resets persisted failure state",
			health: db.SyncHealth{
				Project:             "test-project",
				ConsecutiveFailures: 3,
				BackoffUntil:        baseNow.Add(-time.Minute),
				LastFailureAt:       baseNow.Add(-2 * time.Minute),
				LastError:           "old failure",
			},
			serverStatus:           http.StatusOK,
			serverBody:             `{"pushed":0,"pulled":[],"conflicts":0}`,
			wantFailures:           0,
			wantBackoff:            0,
			wantLastSuccessAt:      baseNow,
			wantRecordSuccessCalls: 1,
		},
		{
			name:                     "repeated failures grow with jitter and sanitize health",
			health:                   db.SyncHealth{Project: "test-project", ConsecutiveFailures: 1},
			jitter:                   10 * time.Second,
			serverStatus:             http.StatusInternalServerError,
			serverBody:               "upstream exploded\nsecond line",
			wantErr:                  "sync con servidor",
			wantFailures:             2,
			wantBackoff:              70 * time.Second,
			wantLastError:            "sync failed (500)",
			wantLastErrorNotContains: "upstream exploded",
			wantLastFailureAt:        baseNow,
			wantRecordFailureCalls:   1,
		},
		{
			name:                     "backoff delay never exceeds cap even with jitter",
			health:                   db.SyncHealth{Project: "test-project", ConsecutiveFailures: 10},
			jitter:                   3 * time.Minute,
			serverStatus:             http.StatusInternalServerError,
			serverBody:               "still failing",
			wantErr:                  "sync con servidor",
			wantFailures:             11,
			wantBackoff:              15 * time.Minute,
			wantLastError:            "sync failed (500)",
			wantLastErrorNotContains: "still failing",
			wantLastFailureAt:        baseNow,
			wantRecordFailureCalls:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockSyncStore{
				jwt:             "valid-token",
				unsynced:        []*models.Memory{},
				healthByProject: map[string]db.SyncHealth{"test-project": tt.health},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/sync" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				w.WriteHeader(tt.serverStatus)
				_, err := w.Write([]byte(tt.serverBody))
				require.NoError(t, err)
			}))
			defer server.Close()

			syncer := newTestSyncer(&Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}, store, syncDeps{
				now:    func() time.Time { return baseNow },
				jitter: func(max time.Duration) time.Duration { return tt.jitter },
			})

			result, err := syncer.Sync(context.Background(), "test-project")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}

			health, healthErr := store.GetSyncHealth("test-project")
			require.NoError(t, healthErr)
			assert.Equal(t, tt.wantFailures, health.ConsecutiveFailures)
			assert.Equal(t, tt.wantLastSuccessAt, health.LastSuccessAt)
			assert.Equal(t, tt.wantLastFailureAt, health.LastFailureAt)
			if tt.wantBackoff == 0 {
				assert.True(t, health.BackoffUntil.IsZero())
			} else {
				assert.Equal(t, baseNow.Add(tt.wantBackoff), health.BackoffUntil)
			}
			assert.Len(t, store.recordSuccessCalls, tt.wantRecordSuccessCalls)
			assert.Len(t, store.recordFailureCalls, tt.wantRecordFailureCalls)
			if tt.wantLastError == "" {
				assert.Empty(t, health.LastError)
			} else {
				assert.Equal(t, tt.wantLastError, health.LastError)
				if tt.wantLastErrorNotContains != "" {
					assert.NotContains(t, health.LastError, tt.wantLastErrorNotContains)
				}
			}
		})
	}
}

func TestSyncer_Sync_RespectsPersistedBackoffAfterRestart(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt:      "valid-token",
		unsynced: []*models.Memory{},
		healthByProject: map[string]db.SyncHealth{
			"test-project": {
				Project:             "test-project",
				ConsecutiveFailures: 2,
				BackoffUntil:        baseNow.Add(5 * time.Minute),
			},
		},
	}

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		syncCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{
		APIURL:   server.URL,
		Email:    "test@example.com",
		Password: "password123",
	}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrSyncBackoff)

	var backoffErr *BackoffError
	require.ErrorAs(t, err, &backoffErr)
	assert.Equal(t, baseNow.Add(5*time.Minute), backoffErr.RetryAt)
	assert.Zero(t, syncCalls.Load())
	assert.Empty(t, store.recordAttemptCalls)
	assert.Empty(t, store.recordSuccessCalls)
	assert.Empty(t, store.recordFailureCalls)
}

func TestSyncer_Sync_InFlightIsolation(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		firstProject  string
		secondProject string
		wantSecondErr error
		wantSyncCalls int32
	}{
		{
			name:          "same project rejects second in-flight call",
			firstProject:  "alpha",
			secondProject: "alpha",
			wantSecondErr: ErrSyncInFlight,
			wantSyncCalls: 1,
		},
		{
			name:          "different project stays isolated",
			firstProject:  "alpha",
			secondProject: "beta",
			wantSyncCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockSyncStore{jwt: "valid-token", unsynced: []*models.Memory{}}

			started := make(chan struct{}, 2)
			release := make(chan struct{})
			var syncCalls atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				syncCalls.Add(1)
				started <- struct{}{}
				<-release
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
				require.NoError(t, err)
			}))
			defer server.Close()

			syncer := newTestSyncer(&Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}, store, syncDeps{
				now:    func() time.Time { return baseNow },
				jitter: func(max time.Duration) time.Duration { return 0 },
			})

			firstDone := make(chan error, 1)
			go func() {
				_, err := syncer.Sync(context.Background(), tt.firstProject)
				firstDone <- err
			}()

			<-started
			if tt.wantSecondErr != nil {
				_, secondErr := syncer.Sync(context.Background(), tt.secondProject)
				require.Error(t, secondErr)
				assert.ErrorIs(t, secondErr, tt.wantSecondErr)
				close(release)
			} else {
				secondDone := make(chan error, 1)
				go func() {
					_, err := syncer.Sync(context.Background(), tt.secondProject)
					secondDone <- err
				}()
				<-started
				close(release)
				require.NoError(t, <-secondDone)
			}

			require.NoError(t, <-firstDone)
			assert.Equal(t, tt.wantSyncCalls, syncCalls.Load())
		})
	}
}

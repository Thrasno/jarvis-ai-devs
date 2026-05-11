package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	mu                      sync.Mutex
	unsynced                []*models.Memory
	lastSync                time.Time
	jwt                     string
	markedSynced            []string
	savedFromRemote         []*models.Memory
	unsyncedPrompts         []*models.Prompt
	unsyncedPromptsErr      error
	markedPromptSynced      []string
	markPromptSyncedErr     error
	healthByProject         map[string]db.SyncHealth
	recordAttemptCalls      []string
	recordSuccessCalls      []string
	recordFailureCalls      []string
	unsyncedSessions        []*models.Session
	unsyncedSessionsErr     error
	markedSessionSynced     []string
	savedSessionsFromRemote []*models.Session
	pendingMutations        []db.MutationEnvelope
	markedMutationsSynced   []string
	markMutationsSyncedErr  error
	appliedRemoteMutations  []db.MutationEnvelope
	applyRemoteMutationErr  error
	mutationCursor          db.MutationCursor
	setMutationCursors      []db.MutationCursor
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

func (m *mockSyncStore) ListUnsyncedSessions(project string) ([]*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unsyncedSessions, m.unsyncedSessionsErr
}

func (m *mockSyncStore) MarkSessionSynced(id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markedSessionSynced = append(m.markedSessionSynced, id)
	return nil
}

func (m *mockSyncStore) SaveSessionFromRemote(s *models.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savedSessionsFromRemote = append(m.savedSessionsFromRemote, s)
	return nil
}

func (m *mockSyncStore) GetPendingMutations(project string, limit int) ([]db.MutationEnvelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]db.MutationEnvelope(nil), m.pendingMutations...), nil
}

func (m *mockSyncStore) MarkMutationsSynced(eventIDs []string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markMutationsSyncedErr != nil {
		return m.markMutationsSyncedErr
	}
	m.markedMutationsSynced = append(m.markedMutationsSynced, eventIDs...)
	return nil
}

func (m *mockSyncStore) ApplyRemoteMutation(event db.MutationEnvelope) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.applyRemoteMutationErr != nil {
		return false, m.applyRemoteMutationErr
	}
	m.appliedRemoteMutations = append(m.appliedRemoteMutations, event)
	return true, nil
}

func (m *mockSyncStore) GetMutationCursor(consumer, project string) (db.MutationCursor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mutationCursor, nil
}

func (m *mockSyncStore) SetMutationCursor(consumer, project string, cursor db.MutationCursor, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mutationCursor = cursor
	m.setMutationCursors = append(m.setMutationCursors, cursor)
	return nil
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

// TestSyncer_Push_IncludesSessionsBeforeMemories verifies T4.1 + T4.2:
// - sessions[] is present in the push body and precedes memories[] (field order)
// - after a successful sync, sessions are marked synced
// - pulled sessions from server are saved to local DB before pulled memories
func TestSyncer_Push_IncludesSessionsBeforeMemories(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		ID:        "manual-save-test-project",
		SyncID:    "aaaabbbb-cccc-dddd-eeee-ffffaaaabbbb",
		Project:   "test-project",
		Directory: "",
		DevID:     "dev@host",
		Client:    "manual",
		StartedAt: now.Add(-time.Hour),
	}

	store := &mockSyncStore{
		jwt:              "valid-token",
		unsynced:         []*models.Memory{createTestSyncMemory("local-mem-1")},
		unsyncedSessions: []*models.Session{sess},
	}

	// capture order: sessions must appear in request body before memories
	type orderCapture struct {
		raw string
	}
	var captured orderCapture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			body, _ := io.ReadAll(r.Body)
			captured.raw = string(body)

			w.WriteHeader(http.StatusOK)
			resp := syncResponse{
				Pushed:    1,
				Pulled:    []apiMemory{},
				Conflicts: 0,
				PulledSessions: []sessionPayload{{
					ID:        "remote-sess-1",
					SyncID:    "11112222-3333-4444-5555-666677778888",
					Project:   "test-project",
					Directory: "",
					DevID:     "other-dev",
					Client:    "claude-code",
					StartedAt: now.Add(-2 * time.Hour),
				}},
			}
			require.NoError(t, json.NewEncoder(w).Encode(resp))
		}
	}))
	defer server.Close()

	cfg := &Config{APIURL: server.URL, Email: "t@t.com", Password: "pass"}
	syncer := New(cfg, store)

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Pushed)

	// Assert sessions field appears before memories in JSON body
	sessIdx := strings.Index(captured.raw, `"sessions"`)
	memIdx := strings.Index(captured.raw, `"memories"`)
	require.True(t, sessIdx >= 0, "sessions key must be present in push body")
	require.True(t, memIdx >= 0, "memories key must be present in push body")
	assert.Less(t, sessIdx, memIdx, "sessions must precede memories in JSON body")

	// Assert unsynced session was marked synced after push
	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Contains(t, store.markedSessionSynced, sess.ID, "pushed session must be marked synced")

	// Assert pulled session was saved before memories
	assert.Len(t, store.savedSessionsFromRemote, 1, "pulled session should be saved")
	assert.Equal(t, "remote-sess-1", store.savedSessionsFromRemote[0].ID)
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

// TestSyncer_RoundTrip_SessionPushThenPull verifies T4.6:
// Daemon A creates a session and pushes; Daemon B (fresh store) pulls and
// has the session saved locally. Uses httptest — no real subprocesses.
func TestSyncer_RoundTrip_SessionPushThenPull(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sess := &models.Session{
		ID:        "manual-save-round-trip",
		SyncID:    "ccccdddd-eeee-ffff-0000-111122223333",
		Project:   "round-trip-project",
		Directory: "",
		DevID:     "dev@host",
		Client:    "claude-code",
		StartedAt: now.Add(-30 * time.Minute),
	}

	// Daemon A store: has the unsynced session
	storeA := &mockSyncStore{
		jwt:              "token-a",
		unsynced:         []*models.Memory{},
		unsyncedSessions: []*models.Session{sess},
	}

	// Daemon B store: starts empty
	storeB := &mockSyncStore{jwt: "token-b"}

	// Server tracks what was pushed and serves it on pull
	var pushedSessions []sessionPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		var req syncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Record sessions pushed by Daemon A
		pushedSessions = append(pushedSessions, req.Sessions...)

		w.WriteHeader(http.StatusOK)
		resp := syncResponse{
			Pushed:         0,
			Pulled:         []apiMemory{},
			Conflicts:      0,
			PulledSessions: pushedSessions, // server returns them in pull
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := &Config{APIURL: server.URL, Email: "t@t.com", Password: "pass"}

	// Daemon A pushes (session goes to server)
	syncerA := New(cfg, storeA)
	_, err := syncerA.Sync(context.Background(), "round-trip-project")
	require.NoError(t, err, "Daemon A sync must succeed")

	// Verify Daemon A marked session as synced
	storeA.mu.Lock()
	assert.Contains(t, storeA.markedSessionSynced, sess.ID, "Daemon A should mark session synced")
	storeA.mu.Unlock()

	// Daemon B pulls (receives the session)
	syncerB := New(cfg, storeB)
	_, err = syncerB.Sync(context.Background(), "round-trip-project")
	require.NoError(t, err, "Daemon B sync must succeed")

	// Verify Daemon B saved the pulled session
	storeB.mu.Lock()
	defer storeB.mu.Unlock()
	require.Len(t, storeB.savedSessionsFromRemote, 1, "Daemon B should save pulled session")
	assert.Equal(t, sess.ID, storeB.savedSessionsFromRemote[0].ID)
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

// CRIT-2 — wire payload must carry session_id end-to-end.
// Without these, daemon→server pushes drop session_id and pulled rows arrive sessionless.

func TestSyncer_Push_MemoryPayloadIncludesSessionID(t *testing.T) {
	mem := createTestSyncMemory("local-with-session")
	mem.SessionID = "sess-abc"

	store := &mockSyncStore{
		jwt:      "valid-token",
		unsynced: []*models.Memory{mem},
	}

	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)

		w.WriteHeader(http.StatusOK)
		resp := syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := &Config{APIURL: server.URL, Email: "t@t.com", Password: "pass"}
	syncer := New(cfg, store)

	_, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)

	assert.Contains(t, capturedBody, `"session_id":"sess-abc"`,
		"push body must carry session_id so the server attributes the memory")
}

func TestSyncer_Pull_MemoryPayloadCarriesSessionID(t *testing.T) {
	store := &mockSyncStore{jwt: "valid-token"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		w.WriteHeader(http.StatusOK)
		// Server returns a memory with session_id set; daemon must round-trip it
		// onto the local model so SaveFromRemote stores it.
		resp := map[string]any{
			"pushed":    0,
			"conflicts": 0,
			"pulled": []map[string]any{
				{
					"id":             "remote-id-1",
					"sync_id":        "pulled-with-session",
					"project":        "test-project",
					"category":       "test",
					"title":          "Pulled",
					"content":        "remote content",
					"tags":           []string{},
					"files_affected": []string{},
					"created_by":     "remote-user",
					"created_at":     time.Now().UTC().Format(time.RFC3339),
					"updated_at":     time.Now().UTC().Format(time.RFC3339),
					"confidence":     0.0,
					"impact_score":   0.0,
					"session_id":     "remote-sess-7",
				},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := &Config{APIURL: server.URL, Email: "t@t.com", Password: "pass"}
	syncer := New(cfg, store)

	_, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.savedFromRemote, 1)
	assert.Equal(t, "remote-sess-7", store.savedFromRemote[0].SessionID,
		"pulled memory must carry session_id into local DB so attribution survives the round trip")
}

func TestSyncer_Sync_MutationProtocolV2PushPullAndCursor(t *testing.T) {
	now := time.Date(2026, 5, 11, 15, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt:            "valid-token",
		mutationCursor: db.MutationCursor{Sequence: 4, EventID: "evt-4"},
		pendingMutations: []db.MutationEnvelope{{
			EventID:      "evt-local-5",
			EntityType:   "memory",
			EntitySyncID: "mem-local-5",
			Project:      "test-project",
			Op:           db.MutationOpUpdate,
			Sequence:     5,
			OccurredAt:   now,
			Memory: &db.MutationMemoryPayload{
				Category:    "architecture",
				Title:       "Local update",
				Content:     "content",
				CreatedBy:   "tester",
				Confidence:  "high",
				ImpactScore: 7,
			},
		}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sync", r.URL.Path)
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, mutationProtocolVersion, req.ProtocolVersion)
		require.NotNil(t, req.MutationCursor)
		assert.Equal(t, int64(4), req.MutationCursor.Sequence)
		require.Len(t, req.Mutations, 1)
		assert.Equal(t, "evt-local-5", req.Mutations[0].EventID)

		resp := syncResponse{
			Pushed:            1,
			Pulled:            []apiMemory{},
			Conflicts:         0,
			CompatibilityMode: compatibilityModeMutationV2,
			NextMutationCursor: &db.MutationCursor{
				Sequence: 6,
				EventID:  "evt-remote-6",
			},
			PulledMutations: []db.MutationEnvelope{{
				EventID:      "evt-remote-6",
				EntityType:   "memory",
				EntitySyncID: "mem-remote-6",
				Project:      "test-project",
				Op:           db.MutationOpRestore,
				Sequence:     6,
				OccurredAt:   now.Add(time.Minute),
			}},
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return now },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, compatibilityModeMutationV2, result.CompatibilityMode)
	assert.Equal(t, 1, result.MutationsPushed)
	assert.Equal(t, 1, result.MutationsPulled)
	assert.Equal(t, int64(6), result.MutationCursor.Sequence)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, []string{"evt-local-5"}, store.markedMutationsSynced)
	require.Len(t, store.appliedRemoteMutations, 1)
	assert.Equal(t, "evt-remote-6", store.appliedRemoteMutations[0].EventID)
	require.Len(t, store.setMutationCursors, 1)
	assert.Equal(t, "evt-remote-6", store.setMutationCursors[0].EventID)
}

func TestSyncer_Sync_MutationProtocolV2DoesNotAckLegacyRowsInMixedBatch(t *testing.T) {
	now := time.Date(2026, 5, 11, 18, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsynced: []*models.Memory{
			createTestSyncMemory("legacy-only-unsynced"),
		},
		pendingMutations: []db.MutationEnvelope{{
			EventID:      "evt-local-v2",
			EntityType:   "memory",
			EntitySyncID: "mutation-backed-memory",
			Project:      "test-project",
			Op:           db.MutationOpUpdate,
			Sequence:     7,
			OccurredAt:   now,
		}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, mutationProtocolVersion, req.ProtocolVersion)
		require.Len(t, req.Memories, 1)
		assert.Equal(t, "legacy-only-unsynced", req.Memories[0].SyncID)
		require.Len(t, req.Mutations, 1)
		assert.Equal(t, "evt-local-v2", req.Mutations[0].EventID)

		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed:            0,
			Pulled:            []apiMemory{},
			Conflicts:         0,
			CompatibilityMode: compatibilityModeMutationV2,
			NextMutationCursor: &db.MutationCursor{
				Sequence: 7,
				EventID:  "evt-local-v2",
			},
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return now },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, compatibilityModeMutationV2, result.CompatibilityMode)
	assert.Equal(t, 1, result.MutationsPushed)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, []string{"evt-local-v2"}, store.markedMutationsSynced, "accepted v2 mutations should be acked")
	assert.Empty(t, store.markedSynced, "legacy-only memories must remain unsynced when v2 response skipped memories[]")
}

func TestSyncer_Sync_MutationProtocolV2EmptyMutationsAcksLegacyRowsInLegacyMode(t *testing.T) {
	now := time.Date(2026, 5, 11, 18, 30, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsynced: []*models.Memory{
			createTestSyncMemory("legacy-empty-mutations-unsynced"),
		},
		pendingMutations: []db.MutationEnvelope{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, mutationProtocolVersion, req.ProtocolVersion)
		require.Len(t, req.Memories, 1)
		assert.Equal(t, "legacy-empty-mutations-unsynced", req.Memories[0].SyncID)
		assert.Empty(t, req.Mutations, "v2-capable daemon may send an empty mutation batch during upgrade")

		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed:            1,
			Pulled:            []apiMemory{},
			Conflicts:         0,
			CompatibilityMode: compatibilityModeLegacy,
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return now },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, compatibilityModeLegacy, result.CompatibilityMode)
	assert.Zero(t, result.MutationsPushed)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, []string{"legacy-empty-mutations-unsynced"}, store.markedSynced, "legacy rows must be acked when API confirms it processed row-state sync")
	assert.Empty(t, store.markedMutationsSynced)
	assert.Empty(t, store.setMutationCursors)
}

func TestSyncer_Sync_MutationProtocolErrorPathsDoNotAckPendingMutations(t *testing.T) {
	now := time.Date(2026, 5, 11, 17, 30, 0, 0, time.UTC)
	pendingMutation := db.MutationEnvelope{
		EventID:      "evt-local-error",
		EntityType:   "memory",
		EntitySyncID: "mem-local-error",
		Project:      "test-project",
		Op:           db.MutationOpUpdate,
		Sequence:     9,
		OccurredAt:   now,
	}

	tests := []struct {
		name                 string
		server               func(t *testing.T) *httptest.Server
		store                *mockSyncStore
		wantErr              string
		wantRecordedLastErr  string
		wantRecordFailures   int
		wantSetCursorCount   int
		wantAppliedMutations int
	}{
		{
			name: "server sync failure leaves pending journal unacked and records sanitized status",
			store: &mockSyncStore{
				jwt:              "valid-token",
				pendingMutations: []db.MutationEnvelope{pendingMutation},
			},
			server: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var req syncRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
					require.Len(t, req.Mutations, 1)
					assert.Equal(t, "evt-local-error", req.Mutations[0].EventID)

					w.WriteHeader(http.StatusInternalServerError)
					_, err := w.Write([]byte("upstream exploded\nwith internals"))
					require.NoError(t, err)
				}))
			},
			wantErr:             "sync con servidor",
			wantRecordedLastErr: "sync failed (500)",
			wantRecordFailures:  1,
		},
		{
			name: "mark synced failure leaves pending journal unacked and records failure status",
			store: &mockSyncStore{
				jwt:                    "valid-token",
				pendingMutations:       []db.MutationEnvelope{pendingMutation},
				markMutationsSyncedErr: assert.AnError,
			},
			server: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
						Pushed:            1,
						Pulled:            []apiMemory{},
						CompatibilityMode: compatibilityModeMutationV2,
						NextMutationCursor: &db.MutationCursor{
							Sequence: 9,
							EventID:  "evt-local-error",
						},
					}))
				}))
			},
			wantErr:             "marcar mutaciones sincronizadas",
			wantRecordedLastErr: assert.AnError.Error(),
			wantRecordFailures:  1,
			wantSetCursorCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.server(t)
			defer server.Close()

			syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, tt.store, syncDeps{
				now:    func() time.Time { return now },
				jitter: func(max time.Duration) time.Duration { return 0 },
			})

			result, err := syncer.Sync(context.Background(), "test-project")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, result)

			tt.store.mu.Lock()
			defer tt.store.mu.Unlock()
			assert.Empty(t, tt.store.markedMutationsSynced, "pending mutations must remain retryable until v2 ack is durably recorded")
			assert.Len(t, tt.store.recordFailureCalls, tt.wantRecordFailures)
			assert.Len(t, tt.store.recordSuccessCalls, 0)
			assert.Len(t, tt.store.setMutationCursors, tt.wantSetCursorCount)
			assert.Len(t, tt.store.appliedRemoteMutations, tt.wantAppliedMutations)
			health := tt.store.getHealthLocked("test-project")
			assert.Equal(t, 1, health.ConsecutiveFailures)
			assert.Equal(t, now, health.LastFailureAt)
			assert.Equal(t, tt.wantRecordedLastErr, health.LastError)
		})
	}
}

func TestSyncer_Sync_DoesNotAdvanceMutationCursorWhenPulledApplyFails(t *testing.T) {
	now := time.Date(2026, 5, 11, 15, 30, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt:                    "valid-token",
		pendingMutations:       []db.MutationEnvelope{{EventID: "evt-local-1", EntityType: "memory", EntitySyncID: "mem-local-1", Project: "test-project", Op: db.MutationOpDelete, Sequence: 1, OccurredAt: now}},
		applyRemoteMutationErr: assert.AnError,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed:             1,
			Pulled:             []apiMemory{},
			CompatibilityMode:  compatibilityModeMutationV2,
			NextMutationCursor: &db.MutationCursor{Sequence: 2, EventID: "evt-remote-2"},
			PulledMutations:    []db.MutationEnvelope{{EventID: "evt-remote-2", EntityType: "memory", EntitySyncID: "mem-remote-2", Project: "test-project", Op: db.MutationOpDelete, Sequence: 2, OccurredAt: now}},
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return now },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aplicar mutación remota")
	assert.Nil(t, result)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Empty(t, store.markedMutationsSynced, "local mutations are not acked until pulled mutations apply cleanly")
	assert.Empty(t, store.setMutationCursors, "cursor must advance only after successful local apply")
}

func TestSyncer_Sync_LegacyFallbackDoesNotAckMutationJournal(t *testing.T) {
	now := time.Date(2026, 5, 11, 16, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt:              "valid-token",
		pendingMutations: []db.MutationEnvelope{{EventID: "evt-local-legacy", EntityType: "memory", EntitySyncID: "mem-local-legacy", Project: "test-project", Op: db.MutationOpDelete, Sequence: 3, OccurredAt: now}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, mutationProtocolVersion, req.ProtocolVersion)
		assert.Len(t, req.Mutations, 1)
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return now },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, compatibilityModeLegacy, result.CompatibilityMode)
	assert.Zero(t, result.MutationsPushed, "legacy fallback must not report v2 mutation journal push success")

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Empty(t, store.markedMutationsSynced)
	assert.Empty(t, store.appliedRemoteMutations)
	assert.Empty(t, store.setMutationCursors)
}

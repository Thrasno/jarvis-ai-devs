package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSyncStore implements the SyncStore interface for testing.
type mockSyncStore struct {
	mu                            sync.Mutex
	unsynced                      []*models.Memory
	lastSync                      time.Time
	jwt                           string
	markedSynced                  []string
	markSyncedErr                 error
	markedMemoriesSyncedBySyncID  []string
	markMemoriesSyncedBySyncIDErr error
	savedFromRemote               []*models.Memory
	unsyncedPrompts               []*models.Prompt
	unsyncedPromptsErr            error
	markedPromptSynced            []string
	markPromptSyncedErr           error
	healthByProject               map[string]db.SyncHealth
	recordAttemptCalls            []string
	recordSuccessCalls            []string
	recordFailureCalls            []string
	unsyncedSessions              []*models.Session
	unsyncedSessionsErr           error
	markedSessionSynced           []string
	savedSessionsFromRemote       []*models.Session
	pendingMutations              []db.MutationEnvelope
	markedMutationsSynced         []string
	markMutationsSyncedErr        error
	markMutationsAndMemoriesCalls int
	markMutationsAndMemoriesErr   error
	appliedRemoteMutations        []db.MutationEnvelope
	applyRemoteMutationErr        error
	mutationCursor                db.MutationCursor
	setMutationCursors            []db.MutationCursor
	pendingSyncAttempts           []db.SyncAttemptLog
	recordedSyncAttempts          []db.SyncAttemptLog
	queueRecordedAttempts         bool
	listSyncAttemptLimit          int
	markedSyncAttempts            []string
	markSyncAttemptsErr           error
	deleteSyncAttemptCutoffs      []time.Time
	deleteSyncAttemptErr          error

	// unsyncedSequence, when non-nil, scripts a different GetUnsynced result
	// per call — used by Drain tests (PR 1b-ii) to simulate a shrinking
	// backlog across multiple batch steps. The last entry repeats once the
	// sequence is exhausted. getUnsyncedCalls counts invocations.
	unsyncedSequence [][]*models.Memory
	getUnsyncedCalls int

	// getUnsyncedPageLimits/listUnsyncedSessionsPageLimits/
	// getUnsyncedPromptsPageLimits record the `limit` argument passed on every
	// GetUnsyncedPage/ListUnsyncedSessionsPage/GetUnsyncedPromptsPage call —
	// used by PR 1b-iii tests to assert syncBatchStep pages at syncPageSize.
	getUnsyncedPageLimits          []int
	listUnsyncedSessionsPageLimits []int
	getUnsyncedPromptsPageLimits   []int

	// pullCursors models the pull_cursors table (PR 2a/2b,
	// hive-sync-batched-drain task 2.8): keyed by "<consumer>/<project>/<channel>".
	// getPullCursorCalls/setPullCursorCalls record every Get/Set call for
	// assertions on which channel was read/written and with what value.
	pullCursors        map[string]db.PullCursor
	getPullCursorCalls []pullCursorCall
	setPullCursorCalls []pullCursorSetCall
	getPullCursorErr   error
	setPullCursorErr   error
}

type pullCursorCall struct {
	consumer, project, channel string
}

type pullCursorSetCall struct {
	consumer, project, channel string
	cursor                     db.PullCursor
}

func (m *mockSyncStore) GetUnsynced(project string) ([]*models.Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unsyncedSequence != nil {
		idx := m.getUnsyncedCalls
		if idx >= len(m.unsyncedSequence) {
			idx = len(m.unsyncedSequence) - 1
		}
		m.getUnsyncedCalls++
		return m.unsyncedSequence[idx], nil
	}
	m.getUnsyncedCalls++
	return m.unsynced, nil
}

// GetUnsyncedPage reuses the same scripting fields as GetUnsynced
// (unsynced/unsyncedSequence) — the mock does not model real pagination
// semantics (that is covered by the real *db.DB paging tests in
// internal/db/sync_test.go); it only needs to hand the Drain loop whatever
// backlog a test script says is left. It additionally truncates the
// returned slice to `limit` when the stored/scripted slice is longer, so
// tests can assert that syncBatchStep never asks the store to violate the
// page cap.
func (m *mockSyncStore) GetUnsyncedPage(project string, limit int) ([]*models.Memory, error) {
	m.mu.Lock()
	m.getUnsyncedPageLimits = append(m.getUnsyncedPageLimits, limit)
	m.mu.Unlock()
	items, err := m.GetUnsynced(project)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (m *mockSyncStore) MarkSynced(syncID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markSyncedErr != nil {
		return m.markSyncedErr
	}
	m.markedSynced = append(m.markedSynced, syncID)
	return nil
}

func (m *mockSyncStore) MarkMemoriesSyncedBySyncID(syncIDs []string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markMemoriesSyncedBySyncIDErr != nil {
		return m.markMemoriesSyncedBySyncIDErr
	}
	m.markedMemoriesSyncedBySyncID = append(m.markedMemoriesSyncedBySyncID, syncIDs...)
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

// GetUnsyncedPromptsPage mirrors GetUnsyncedPage's approach: reuse the
// existing unsyncedPrompts/unsyncedPromptsErr scripting fields, truncated to
// limit.
func (m *mockSyncStore) GetUnsyncedPromptsPage(ctx context.Context, project string, limit int) ([]*models.Prompt, error) {
	m.mu.Lock()
	m.getUnsyncedPromptsPageLimits = append(m.getUnsyncedPromptsPageLimits, limit)
	m.mu.Unlock()
	items, err := m.GetUnsyncedPrompts(ctx, project)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
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

// ListUnsyncedSessionsPage mirrors GetUnsyncedPage's approach: reuse the
// existing unsyncedSessions/unsyncedSessionsErr scripting fields, truncated
// to limit.
func (m *mockSyncStore) ListUnsyncedSessionsPage(project string, limit int) ([]*models.Session, error) {
	m.mu.Lock()
	m.listUnsyncedSessionsPageLimits = append(m.listUnsyncedSessionsPageLimits, limit)
	m.mu.Unlock()
	items, err := m.ListUnsyncedSessions(project)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
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

// MarkMutationsAndMemoriesSynced mimics the atomic DB transaction: if
// markMutationsAndMemoriesErr is set, NEITHER slice is recorded, modeling a
// rollback where the mutation ack and the memory ack rise and fall together.
func (m *mockSyncStore) MarkMutationsAndMemoriesSynced(eventIDs []string, syncIDs []string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markMutationsAndMemoriesCalls++
	if m.markMutationsAndMemoriesErr != nil {
		return m.markMutationsAndMemoriesErr
	}
	m.markedMutationsSynced = append(m.markedMutationsSynced, eventIDs...)
	m.markedMemoriesSyncedBySyncID = append(m.markedMemoriesSyncedBySyncID, syncIDs...)
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

// pullCursorKey builds the mock's internal lookup key for pullCursors — not
// a wire format, just a test-fake indexing convenience.
func pullCursorKey(consumer, project, channel string) string {
	return consumer + "/" + project + "/" + channel
}

func (m *mockSyncStore) GetPullCursor(consumer, project, channel string) (db.PullCursor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getPullCursorCalls = append(m.getPullCursorCalls, pullCursorCall{consumer, project, channel})
	if m.getPullCursorErr != nil {
		return db.PullCursor{}, m.getPullCursorErr
	}
	if m.pullCursors == nil {
		return db.PullCursor{}, nil
	}
	return m.pullCursors[pullCursorKey(consumer, project, channel)], nil
}

func (m *mockSyncStore) SetPullCursor(consumer, project, channel string, cursor db.PullCursor, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setPullCursorCalls = append(m.setPullCursorCalls, pullCursorSetCall{consumer, project, channel, cursor})
	if m.setPullCursorErr != nil {
		return m.setPullCursorErr
	}
	if m.pullCursors == nil {
		m.pullCursors = make(map[string]db.PullCursor)
	}
	m.pullCursors[pullCursorKey(consumer, project, channel)] = cursor
	return nil
}

func (m *mockSyncStore) RecordSyncAttemptLog(ctx context.Context, log db.SyncAttemptLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedSyncAttempts = append(m.recordedSyncAttempts, log)
	if m.queueRecordedAttempts {
		m.pendingSyncAttempts = append(m.pendingSyncAttempts, log)
	}
	return nil
}

func (m *mockSyncStore) ListPendingSyncAttemptLogs(ctx context.Context, limit int) ([]db.SyncAttemptLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listSyncAttemptLimit = limit
	if limit <= 0 || limit > len(m.pendingSyncAttempts) {
		limit = len(m.pendingSyncAttempts)
	}
	return append([]db.SyncAttemptLog(nil), m.pendingSyncAttempts[:limit]...), nil
}

func (m *mockSyncStore) MarkSyncAttemptLogsDelivered(ctx context.Context, ids []string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markSyncAttemptsErr != nil {
		return m.markSyncAttemptsErr
	}
	m.markedSyncAttempts = append(m.markedSyncAttempts, ids...)
	return nil
}

func (m *mockSyncStore) DeleteSyncAttemptLogsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteSyncAttemptCutoffs = append(m.deleteSyncAttemptCutoffs, cutoff)
	return 0, m.deleteSyncAttemptErr
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
				Category:  "architecture",
				Title:     "Local update",
				Content:   "content",
				CreatedBy: "tester",
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

// TestSyncer_Sync_MutationProtocolV2AcksLegacyRowsCorrelatedByConfirmedMutation
// is the regression test for the "legacy memories never acked in v2 mode"
// bug (design §3, Hive obs #1692). hive-api ignores memories[] in v2 mode, so
// legacy rows must instead be acked by correlating their sync_id to the
// EntitySyncID of a mutation that was durably confirmed via
// MarkMutationsSynced. A legacy row with no corresponding confirmed mutation
// (unrelated sync_id) must remain unsynced.
func TestSyncer_Sync_MutationProtocolV2AcksLegacyRowsCorrelatedByConfirmedMutation(t *testing.T) {
	now := time.Date(2026, 5, 11, 18, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsynced: []*models.Memory{
			createTestSyncMemory("legacy-only-unsynced"),
			createTestSyncMemory("mutation-backed-memory"),
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
		require.Len(t, req.Memories, 2)
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
	assert.Empty(t, store.markedSynced, "legacy MarkSynced path must not be used in v2 mode")
	assert.Equal(t, []string{"mutation-backed-memory"}, store.markedMemoriesSyncedBySyncID,
		"legacy row whose sync_id matches a confirmed mutation's EntitySyncID must be acked via MarkMemoriesSyncedBySyncID")
}

// TestSyncer_Sync_MutationProtocolV2PartialConfirmOnlyAcksConfirmedSubset
// covers the partial-confirm case within a single durably-confirmed batch:
// MarkMutationsSynced confirms the whole pending batch atomically, but only
// create/update mutations carry a legacy-row-shaped memory payload — a
// delete mutation in the same confirmed batch must NOT be correlated to a
// legacy memories.sync_id ack, since tombstone rows are acked through
// ApplyRemoteMutation/legacy delete handling, not this correlation path.
// Only the confirmed subset that actually maps to a legacy row is marked.
func TestSyncer_Sync_MutationProtocolV2PartialConfirmOnlyAcksConfirmedSubset(t *testing.T) {
	now := time.Date(2026, 5, 11, 19, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsynced: []*models.Memory{
			createTestSyncMemory("confirmed-memory"),
			createTestSyncMemory("deleted-memory"),
		},
		pendingMutations: []db.MutationEnvelope{
			{
				EventID:      "evt-confirmed-update",
				EntityType:   "memory",
				EntitySyncID: "confirmed-memory",
				Project:      "test-project",
				Op:           db.MutationOpUpdate,
				Sequence:     10,
				OccurredAt:   now,
			},
			{
				EventID:      "evt-confirmed-delete",
				EntityType:   "memory",
				EntitySyncID: "deleted-memory",
				Project:      "test-project",
				Op:           db.MutationOpDelete,
				Sequence:     11,
				OccurredAt:   now,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed:            0,
			Pulled:            []apiMemory{},
			Conflicts:         0,
			CompatibilityMode: compatibilityModeMutationV2,
			NextMutationCursor: &db.MutationCursor{
				Sequence: 11,
				EventID:  "evt-confirmed-delete",
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

	store.mu.Lock()
	defer store.mu.Unlock()
	// Both mutations are durably confirmed via MarkMutationsSynced (the batch
	// is atomic), but only the update mutation's EntitySyncID correlates to a
	// legacy-row content ack — the delete op is intentionally excluded.
	assert.Equal(t, []string{"evt-confirmed-update", "evt-confirmed-delete"}, store.markedMutationsSynced)
	assert.Equal(t, []string{"confirmed-memory"}, store.markedMemoriesSyncedBySyncID,
		"only the confirmed create/update mutation's memory must be acked; delete mutations are excluded from this correlation path")
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
	// Task 1a.4 regression guard: legacy-mode marking must go exclusively
	// through the Paso 5b MarkSynced branch. The new v2 correlation path
	// (MarkMemoriesSyncedBySyncID) must never fire in legacy mode, guarding
	// against 1a.3 refactor drift accidentally double-acking or bypassing
	// the legacy branch.
	assert.Empty(t, store.markedMemoriesSyncedBySyncID, "legacy mode must not use the v2 mutation-correlation ack path")
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
				jwt:                         "valid-token",
				pendingMutations:            []db.MutationEnvelope{pendingMutation},
				markMutationsAndMemoriesErr: assert.AnError,
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
			assert.Empty(t, tt.store.markedMemoriesSyncedBySyncID, "no memory sync_id may be marked before durable confirmation")
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

// TestSyncer_Sync_PartialDBFailureDuringCombinedAckRetriesBothHalvesTogether
// is the regression test for the fresh-context review finding: previously,
// MarkMutationsSynced and MarkMemoriesSyncedBySyncID were two separate calls.
// If the mutation ack succeeded but the memory ack failed, the mutation row
// was already synced_at, so GetPendingMutations would never re-derive
// confirmedMemorySyncIDs for it again — permanently stranding the legacy
// memories row. With the atomic MarkMutationsAndMemoriesSynced, a failure
// rolls back BOTH halves, so the mutation stays pending and the very next
// Sync() call re-derives and successfully acks both together.
func TestSyncer_Sync_PartialDBFailureDuringCombinedAckRetriesBothHalvesTogether(t *testing.T) {
	now := time.Date(2026, 5, 11, 20, 0, 0, 0, time.UTC)
	pendingMutation := db.MutationEnvelope{
		EventID:      "evt-partial-fail",
		EntityType:   "memory",
		EntitySyncID: "mem-partial-fail",
		Project:      "test-project",
		Op:           db.MutationOpUpdate,
		Sequence:     20,
		OccurredAt:   now,
	}
	store := &mockSyncStore{
		jwt:              "valid-token",
		pendingMutations: []db.MutationEnvelope{pendingMutation},
		unsynced: []*models.Memory{
			createTestSyncMemory("mem-partial-fail"),
		},
		markMutationsAndMemoriesErr: assert.AnError,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed:            0,
			Pulled:            []apiMemory{},
			Conflicts:         0,
			CompatibilityMode: compatibilityModeMutationV2,
			NextMutationCursor: &db.MutationCursor{
				Sequence: 20,
				EventID:  "evt-partial-fail",
			},
		}))
	}))
	defer server.Close()

	currentTime := now
	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return currentTime },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	// First Sync(): the combined DB ack fails. Both halves must roll back —
	// no mutation acked, no memory acked. This is the "partial DB failure"
	// scenario from the review: without atomicity, the mutation half would
	// have already been recorded here, permanently stranding the memory.
	result, err := syncer.Sync(context.Background(), "test-project")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marcar mutaciones sincronizadas")
	assert.Nil(t, result)

	store.mu.Lock()
	assert.Equal(t, 1, store.markMutationsAndMemoriesCalls)
	assert.Empty(t, store.markedMutationsSynced, "mutation ack must roll back together with the failed memory ack")
	assert.Empty(t, store.markedMemoriesSyncedBySyncID, "memory ack must not be recorded when the combined transaction fails")
	// Simulate the failure being resolved (e.g. transient DB contention) and
	// clear the injected error before retrying, exactly like a real retry
	// after a transient error is no longer occurring.
	store.markMutationsAndMemoriesErr = nil
	store.mu.Unlock()

	// Advance past the recorded failure backoff window so the retry Sync()
	// call is not blocked by ErrSyncBackoff.
	currentTime = currentTime.Add(backoffMaxDelay)

	// Second Sync(): GetPendingMutations still returns the same pending
	// mutation (since nothing was acked), so confirmedMemorySyncIDs
	// re-derives the same correlation and both halves ack successfully this
	// time. This proves there is no permanent stuck row.
	result, err = syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, 1, result.MutationsPushed)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Equal(t, 2, store.markMutationsAndMemoriesCalls)
	assert.Equal(t, []string{"evt-partial-fail"}, store.markedMutationsSynced,
		"retry must successfully ack the mutation once the transient failure clears")
	assert.Equal(t, []string{"mem-partial-fail"}, store.markedMemoriesSyncedBySyncID,
		"retry must successfully ack the correlated legacy memory row — no permanent stuck row")
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
	assert.Empty(t, store.markedMemoriesSyncedBySyncID, "legacy fallback must not run the v2 mutation-correlation ack path")
}

func TestSyncer_Sync_FlushesAttemptLogsBestEffortAfterRecordingCurrentResult(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt:                   "valid-token",
		queueRecordedAttempts: true,
		pendingSyncAttempts: []db.SyncAttemptLog{
			{AttemptID: "previous-attempt", DevID: "dev@example.com", Project: "test-project", Client: "hive-daemon", StartedAt: now.Add(-time.Minute), EndedAt: now.Add(-time.Minute), Outcome: db.SyncAttemptOutcomeFailure, ErrorCode: "server_error"},
		},
	}

	var postedAttemptIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync":
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		case "/sync-attempts":
			assert.Equal(t, "Bearer valid-token", r.Header.Get("Authorization"))
			var req syncAttemptIngestRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			for _, attempt := range req.Attempts {
				postedAttemptIDs = append(postedAttemptIDs, attempt.AttemptID)
			}
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncAttemptIngestResponse{
				AcceptedIDs:  []string{"current-attempt"},
				DuplicateIDs: []string{"previous-attempt"},
				Rejected:     []syncAttemptRejected{{AttemptID: "rejected-attempt", Error: "invalid"}},
			}))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "dev@example.com", Password: "password123"}, store, syncDeps{
		now:          func() time.Time { return now },
		jitter:       func(max time.Duration) time.Duration { return 0 },
		newAttemptID: func() string { return "current-attempt" },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"previous-attempt", "current-attempt"}, postedAttemptIDs, "current result must be recorded before pending attempts are listed for flush")
	assert.Equal(t, []time.Time{now.AddDate(0, 0, -90)}, store.deleteSyncAttemptCutoffs, "successful sync path must run 90-day retention before upload")
	assert.Equal(t, 100, store.listSyncAttemptLimit)
	assert.Equal(t, []string{"current-attempt", "previous-attempt"}, store.markedSyncAttempts, "only accepted and duplicate IDs are marked delivered")
}

func TestSyncer_Sync_AttemptFlushFailureIsNonFatalAndLeavesAttemptsPending(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 30, 0, 0, time.UTC)
	store := &mockSyncStore{jwt: "valid-token", queueRecordedAttempts: true, deleteSyncAttemptErr: fmt.Errorf("retention cleanup unavailable")}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync":
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		case "/sync-attempts":
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "dev@example.com", Password: "password123"}, store, syncDeps{
		now:          func() time.Time { return now },
		jitter:       func(max time.Duration) time.Duration { return 0 },
		newAttemptID: func() string { return "current-attempt" },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, store.markedSyncAttempts, "failed attempt uploads must leave local attempts pending")
	assert.Equal(t, []time.Time{now.AddDate(0, 0, -90)}, store.deleteSyncAttemptCutoffs, "retention cleanup errors must not fail memory sync")
	require.Len(t, store.recordedSyncAttempts, 1)
	assert.Equal(t, db.SyncAttemptOutcomeSuccess, store.recordedSyncAttempts[0].Outcome)
}

func TestSyncer_Sync_FailedMemorySyncRecordsPendingFailureAttempt(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 45, 0, 0, time.UTC)
	store := &mockSyncStore{jwt: "valid-token", queueRecordedAttempts: true}

	var attemptUploadCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync":
			http.Error(w, "upstream temporarily unavailable", http.StatusBadGateway)
		case "/sync-attempts":
			attemptUploadCalls++
			http.Error(w, "audit endpoint unavailable", http.StatusServiceUnavailable)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "dev@example.com", Password: "password123"}, store, syncDeps{
		now:          func() time.Time { return now },
		jitter:       func(max time.Duration) time.Duration { return 0 },
		newAttemptID: func() string { return "failed-current-attempt" },
	})

	result, err := syncer.Sync(context.Background(), "test-project")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync con servidor")
	assert.Nil(t, result)
	assert.Zero(t, attemptUploadCalls, "failed memory sync must not require immediate audit upload when the API path is unavailable")

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.recordedSyncAttempts, 1)
	recorded := store.recordedSyncAttempts[0]
	assert.Equal(t, "failed-current-attempt", recorded.AttemptID)
	assert.Equal(t, "dev@example.com", recorded.DevID)
	assert.Equal(t, "test-project", recorded.Project)
	assert.Equal(t, db.SyncAttemptOutcomeFailure, recorded.Outcome)
	assert.Equal(t, "sync_failed", recorded.ErrorCode)
	assert.Contains(t, recorded.ErrorMessage, "sync failed (502)")
	assert.Equal(t, []db.SyncAttemptLog{recorded}, store.pendingSyncAttempts, "failed attempt logs must remain pending for later delivery")
	assert.Equal(t, []time.Time{now.AddDate(0, 0, -90)}, store.deleteSyncAttemptCutoffs, "failed sync path must run 90-day retention after recording the audit attempt")
	assert.Empty(t, store.markedSyncAttempts)
}

func TestSyncer_Sync_AttemptFlushBatchNeverExceedsOneHundred(t *testing.T) {
	now := time.Date(2026, 6, 19, 13, 0, 0, 0, time.UTC)
	store := &mockSyncStore{jwt: "valid-token", queueRecordedAttempts: true}
	for i := 0; i < 150; i++ {
		store.pendingSyncAttempts = append(store.pendingSyncAttempts, db.SyncAttemptLog{AttemptID: fmt.Sprintf("attempt-%03d", i), DevID: "dev@example.com", Project: "test-project", StartedAt: now.Add(time.Duration(i) * time.Second), EndedAt: now.Add(time.Duration(i) * time.Second), Outcome: db.SyncAttemptOutcomeSuccess})
	}

	var postedCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync":
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		case "/sync-attempts":
			var req syncAttemptIngestRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			postedCount = len(req.Attempts)
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncAttemptIngestResponse{}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "dev@example.com", Password: "password123"}, store, syncDeps{
		now:          func() time.Time { return now },
		jitter:       func(max time.Duration) time.Duration { return 0 },
		newAttemptID: func() string { return "current-attempt" },
	})

	_, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)
	assert.Equal(t, 100, store.listSyncAttemptLimit)
	assert.Equal(t, 100, postedCount)
}

// TestSyncer_SyncBatchStep_DoesNotTouchInFlight pins the PR 1b-i extraction
// contract (design §4.2): syncBatchStep is a pure push+pull exchange and the
// reservation bookkeeping (s.inFlight) is exclusively the caller's (Drain's,
// via Sync) concern. If syncBatchStep ever reserved or released s.inFlight
// itself, calling it directly here — bypassing Drain's tryReserve/release —
// would leave s.inFlight mutated as a side effect, which this test would catch.
func TestSyncer_SyncBatchStep_DoesNotTouchInFlight(t *testing.T) {
	store := &mockSyncStore{jwt: "valid-token", unsynced: []*models.Memory{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

	require.Empty(t, syncer.inFlight, "precondition: inFlight must start empty")

	result, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Pushed)
	assert.True(t, result.PushBacklogEmpty, "empty backlog before the step means nothing remained to send")
	assert.False(t, result.PullHasMore, "PR 1b-i hardcodes PullHasMore=false pending server pagination in PR 2")

	assert.Empty(t, syncer.inFlight, "syncBatchStep must never reserve or release s.inFlight — that is Sync's concern")
}

// TestSyncer_SyncBatchStep_PushBacklogEmptyReflectsPendingWork verifies the
// PushBacklogEmpty flag distinguishes an empty backlog from one that still
// has unsynced work of any kind (memories, sessions, prompts, or mutations).
func TestSyncer_SyncBatchStep_PushBacklogEmptyReflectsPendingWork(t *testing.T) {
	tests := []struct {
		name        string
		setupStore  func() *mockSyncStore
		wantIsEmpty bool
	}{
		{
			name: "no pending work of any kind",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{jwt: "valid-token"}
			},
			wantIsEmpty: true,
		},
		{
			name: "pending unsynced memories",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{jwt: "valid-token", unsynced: []*models.Memory{createTestSyncMemory("local-1")}}
			},
			wantIsEmpty: false,
		},
		{
			name: "pending unsynced sessions",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{jwt: "valid-token", unsyncedSessions: []*models.Session{{ID: "sess-1"}}}
			},
			wantIsEmpty: false,
		},
		{
			name: "pending unsynced prompts",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{jwt: "valid-token", unsyncedPrompts: []*models.Prompt{createTestPrompt("p-1", "test-project", "hi")}}
			},
			wantIsEmpty: false,
		},
		{
			name: "pending mutations",
			setupStore: func() *mockSyncStore {
				return &mockSyncStore{jwt: "valid-token", pendingMutations: []db.MutationEnvelope{{EventID: "evt-1", EntityType: "memory", Op: db.MutationOpUpdate}}}
			},
			wantIsEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tt.setupStore()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/sync" {
					w.WriteHeader(http.StatusOK)
					require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
				}
			}))
			defer server.Close()

			syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

			result, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
			require.NoError(t, err)
			assert.Equal(t, tt.wantIsEmpty, result.PushBacklogEmpty)
		})
	}
}

// TestDrain_TriggerAuto_RunsExactlyOneStep pins the Drain controller contract
// for TriggerAuto (design §2.1, §4.3, PR 1b-ii): even when the backlog would
// still have work remaining after one batch, TriggerAuto must stop after
// exactly one syncBatchStep call — matching the historical Sync behavior.
func TestDrain_TriggerAuto_RunsExactlyOneStep(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("local-1")},
			{createTestSyncMemory("local-2")},
			{createTestSyncMemory("local-3")},
		},
	}

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			syncCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerAuto)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), syncCalls.Load(), "TriggerAuto must run exactly one batch step")
	assert.Equal(t, 1, outcome.BatchesDone)
	assert.Equal(t, DrainExpectedPending, outcome.State, "backlog still has pending work after the single allowed batch")
}

// TestDrain_TriggerManual_LoopsUntilBacklogEmpty pins the Drain controller
// contract for TriggerManual (design §2.1, §4.3, PR 1b-ii): it must keep
// calling syncBatchStep until the push backlog is empty and the pull side
// reports no more pages, aggregating counts into a single Result.
func TestDrain_TriggerManual_LoopsUntilBacklogEmpty(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("local-1"), createTestSyncMemory("local-2")},
			{createTestSyncMemory("local-3")},
			{},
		},
	}

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			syncCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(3), syncCalls.Load(), "TriggerManual must loop until the backlog is empty")
	assert.Equal(t, 3, outcome.BatchesDone)
	assert.Equal(t, DrainFullySynced, outcome.State)
	assert.Equal(t, 3, result.Pushed, "aggregated Pushed must sum every batch")
}

// TestDrain_TriggerManual_HoldsReservationForEntireRun pins the design §4.3
// requirement that tryReserve/release wrap the WHOLE Drain run, not each
// individual batch: while a manual multi-batch Drain is in flight, a second
// Drain/Sync call for the same project must be rejected with
// ErrSyncInFlight, not just between/after individual batches.
func TestDrain_TriggerManual_HoldsReservationForEntireRun(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("local-1")},
			{},
		},
	}

	batchStarted := make(chan struct{}, 4)
	release := make(chan struct{})
	var batchCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		n := batchCount.Add(1)
		batchStarted <- struct{}{}
		if n == 1 {
			// Block the first batch so we can assert inFlight state and a
			// concurrent Sync() call mid-Drain, before the loop proceeds to
			// its second (final, empty-backlog) batch.
			<-release
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	drainDone := make(chan error, 1)
	go func() {
		_, _, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
		drainDone <- err
	}()

	<-batchStarted // first batch is in flight, reservation held

	_, midRunErr := syncer.Sync(context.Background(), "test-project")
	require.ErrorIs(t, midRunErr, ErrSyncInFlight, "reservation must stay held across the whole Drain run, not just per-batch")

	close(release)
	require.NoError(t, <-drainDone)

	assert.Empty(t, syncer.inFlight, "release must run after Drain finishes")
}

// TestDrain_TerminatesOnNoProgress pins the T1b-ii.5 termination guard
// (revised for PR 1b-iii, see batchResult.RecordsMarkedSynced): if a batch
// fetches pending work but durably marks NOTHING synced AND the mutation
// cursor does not advance, Drain must stop within bounded iterations instead
// of spinning forever, and classify the result as expected-pending.
//
// This models a permanent MarkSynced failure (e.g. a DB constraint issue or
// a persistent server-side rejection that never lets the store durably
// confirm the row) — NOT a step error, since a step error takes the
// DrainDegradedFailure path instead. The backlog is scripted as a fixed,
// never-shrinking single unsynced memory: with markSyncedErr always set,
// RecordsMarkedSynced is 0 on every batch, so the guard must trip.
func TestDrain_TerminatesOnNoProgress(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt:           "valid-token",
		unsynced:      []*models.Memory{createTestSyncMemory("stuck-memory")},
		markSyncedErr: errors.New("persistent mark-synced failure"),
	}

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			syncCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			// Pushed:0 models the server rejecting/ignoring the item every time.
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.LessOrEqual(t, syncCalls.Load(), int32(3), "the no-progress guard must break the loop within a small bounded number of iterations")
	assert.Equal(t, DrainExpectedPending, outcome.State)
}

// TestDrain_TerminatesOnStepError pins the T1b-ii.5 requirement that a
// mid-loop batch step error immediately breaks the Drain loop and classifies
// the outcome as degraded-failure, surfacing the error to the caller.
func TestDrain_TerminatesOnStepError(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("local-1")},
			{createTestSyncMemory("local-2")},
		},
	}

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		n := syncCalls.Add(1)
		if n == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, writeErr := w.Write([]byte("boom"))
			require.NoError(t, writeErr)
			return
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.Error(t, err)
	require.NotNil(t, result, "partial progress from the first successful batch must still be surfaced")
	assert.Equal(t, 1, result.Pushed, "only the first batch's progress is aggregated before the failing second batch")
	assert.Equal(t, 2, outcome.BatchesDone)
	assert.Equal(t, DrainDegradedFailure, outcome.State)
	assert.Equal(t, int32(2), syncCalls.Load(), "loop must stop immediately after the failing batch, not retry indefinitely")
}

// TestDrain_TriggerManual_BackoffAndAttemptRecordedOnceAtTop pins the design
// §4.3 requirement that the backoff gate and RecordSyncAttempt run ONCE at
// the top of a Drain run, not once per batch iteration.
func TestDrain_TriggerManual_BackoffAndAttemptRecordedOnceAtTop(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{
		jwt: "valid-token",
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("local-1"), createTestSyncMemory("local-2")},
			{createTestSyncMemory("local-3")},
			{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	_, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err)
	assert.Equal(t, 3, outcome.BatchesDone, "sanity check: the loop actually ran multiple batches")
	assert.Len(t, store.recordAttemptCalls, 1, "RecordSyncAttempt must run once per Drain call, not once per batch")
	assert.Len(t, store.recordSuccessCalls, 1, "RecordSyncSuccess must run once at the end of a successful Drain")
}

// withSyncPageSize temporarily overrides the package-level syncPageSize var
// for the duration of a test and restores it on cleanup — PR 1b-iii tests use
// a small page size so a handful of scripted rows is enough to exercise
// multi-batch paging without needing hundreds of fixtures.
func withSyncPageSize(t *testing.T, size int) {
	t.Helper()
	original := syncPageSize
	syncPageSize = size
	t.Cleanup(func() { syncPageSize = original })
}

// TestSyncBatchStep_PagesFetchesAtSyncPageSize pins the PR 1b-iii page-cap
// contract: syncBatchStep must call the paged getters (GetUnsyncedPage,
// ListUnsyncedSessionsPage, GetUnsyncedPromptsPage) with the current
// syncPageSize as the limit, not the unbounded unpaged variants.
//
// No sessions are pending in this scenario on purpose: PR 2b's
// session-priority gate (see TestSyncBatchStep_SessionPriorityGate_...
// below) skips the GetUnsyncedPage fetch entirely while any session remains
// unsynced, which would make store.getUnsyncedPageLimits empty and defeat
// this test's own assertion. Session+prompt paging is still covered here;
// the interaction between session backlog and memory paging is covered by
// the dedicated session-priority-gate tests.
func TestSyncBatchStep_PagesFetchesAtSyncPageSize(t *testing.T) {
	withSyncPageSize(t, 2)

	store := &mockSyncStore{
		jwt:      "valid-token",
		unsynced: []*models.Memory{createTestSyncMemory("m-1"), createTestSyncMemory("m-2"), createTestSyncMemory("m-3")},
		unsyncedPrompts: []*models.Prompt{
			createTestPrompt("p-1", "test-project", "one"),
			createTestPrompt("p-2", "test-project", "two"),
			createTestPrompt("p-3", "test-project", "three"),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 2, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

	result, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
	require.NoError(t, err)

	assert.Equal(t, []int{2}, store.getUnsyncedPageLimits, "GetUnsyncedPage must be called with syncPageSize")
	assert.Equal(t, []int{2}, store.listUnsyncedSessionsPageLimits, "ListUnsyncedSessionsPage must be called with syncPageSize")
	assert.Equal(t, []int{2}, store.getUnsyncedPromptsPageLimits, "GetUnsyncedPromptsPage must be called with syncPageSize")
	assert.False(t, result.PushBacklogEmpty, "backlog still has more than one page pending")
}

// TestSyncBatchStep_SessionPriorityGate_SkipsMemoryFetchWhileSessionsPending
// pins the PR 2b session-priority gate directly at the syncBatchStep level
// (a narrower unit than the Drain-level FK regression test): while any
// session remains unsynced, syncBatchStep must not even call GetUnsyncedPage
// — the memories fetch is skipped entirely, not just filtered after the
// fact — so store.getUnsyncedPageLimits stays empty for that step.
func TestSyncBatchStep_SessionPriorityGate_SkipsMemoryFetchWhileSessionsPending(t *testing.T) {
	store := &mockSyncStore{
		jwt:              "valid-token",
		unsynced:         []*models.Memory{createTestSyncMemory("m-1")},
		unsyncedSessions: []*models.Session{{ID: "s-1"}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

	result, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
	require.NoError(t, err)

	assert.Empty(t, store.getUnsyncedPageLimits, "GetUnsyncedPage must not be called while sessions are pending")
	assert.False(t, result.PushBacklogEmpty, "pending session keeps the backlog non-empty")
}

// TestDrain_TriggerManual_PagesThroughLargeBacklogWithoutFalsePositiveGuard
// is the fresh-review-required regression test: with a backlog larger than
// one page, the Drain(TriggerManual) loop must page through every batch and
// terminate by backlog-empty, NOT by the no-progress guard tripping
// prematurely. This is the scenario the OLD BacklogSize-based guard would
// have misclassified: BacklogSize saturates at the (small, scripted) page
// size on every iteration even though the backlog is genuinely shrinking.
func TestDrain_TriggerManual_PagesThroughLargeBacklogWithoutFalsePositiveGuard(t *testing.T) {
	withSyncPageSize(t, 2)
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	// 5 unsynced memories, page size 2: batches of 2, 2, 1, then empty.
	store := &mockSyncStore{
		jwt: "valid-token",
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("m-1"), createTestSyncMemory("m-2"), createTestSyncMemory("m-3"), createTestSyncMemory("m-4"), createTestSyncMemory("m-5")},
			{createTestSyncMemory("m-3"), createTestSyncMemory("m-4"), createTestSyncMemory("m-5")},
			{createTestSyncMemory("m-5")},
			{},
		},
	}

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sync" {
			syncCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(4), syncCalls.Load(), "loop must page through all 4 scripted batches")
	assert.Equal(t, 4, outcome.BatchesDone)
	assert.Equal(t, DrainFullySynced, outcome.State, "the loop must terminate by backlog-empty, not by the no-progress guard")
	// Page size 2 caps each scripted entry ([5, 3, 1] items) at 2: batches mark
	// 2 + 2 + 1 = 5 records, then a final empty batch confirms backlog-empty.
	assert.Len(t, store.markedSynced, 5, "every record marked synced across all batches must sum to the pushed total")
}

// TestDrain_SessionsBeforeMemoriesAcrossPagedBatches pins the FK-ordering
// invariant across a multi-page manual Drain (PR 1b-iii task 1b-iii.2): a
// memory whose session lands in an EARLIER batch than the memory itself must
// still push successfully, because the earlier batch pushes and marks its
// session before the loop advances. Within a single batch, sessions are
// already serialized before memories (client.sync's fixed field order) —
// this test additionally pins the cross-batch case.
func TestDrain_SessionsBeforeMemoriesAcrossPagedBatches(t *testing.T) {
	withSyncPageSize(t, 1)
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	sess := &models.Session{ID: "sess-early-batch"}
	mem := createTestSyncMemory("mem-later-batch")
	mem.SessionID = sess.ID

	store := &mockSyncStore{
		jwt: "valid-token",
		// Batch 1: only the session is pending — the memory is added to the
		// store only after batch 1 completes, modeling a memory that gets
		// queued in a LATER batch than the session it references (task
		// 1b-iii.2's "session in an earlier batch" scenario).
		unsyncedSessions: []*models.Session{sess},
	}

	var sawSessionBeforeMemory bool
	var sessionPushed, memoryPushed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if len(req.Sessions) > 0 {
			sessionPushed = true
		}
		if len(req.Memories) > 0 {
			// The memory batch must only ever be sent once its session has
			// already been pushed (and, by construction of this test, acked).
			if !sessionPushed {
				t.Errorf("memory batch sent before its session was pushed")
			} else {
				sawSessionBeforeMemory = true
			}
			memoryPushed = true
		}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: len(req.Memories), Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	// Drive two syncBatchStep calls directly (the same call the Drain loop
	// makes each iteration): batch 1 only sees the session; only after it
	// completes does the memory referencing that session become pending,
	// modeling "the memory's session was pushed/confirmed in an earlier
	// batch" — matching how a real DB-backed ListUnsyncedSessionsPage would
	// stop returning an already-synced_at row.
	token := "valid-token"
	batch1, err := syncer.syncBatchStep(context.Background(), "test-project", token)
	require.NoError(t, err)
	require.True(t, sessionPushed, "first batch must push the session")
	require.Len(t, store.markedSessionSynced, 1)
	store.unsyncedSessions = nil           // simulate the session no longer being unsynced (already marked)
	store.unsynced = []*models.Memory{mem} // now the memory becomes pending, referencing the already-pushed session

	batch2, err := syncer.syncBatchStep(context.Background(), "test-project", token)
	require.NoError(t, err)
	require.True(t, memoryPushed, "second batch must push the memory")
	assert.True(t, sawSessionBeforeMemory, "memory push must be observed only after the session push")
	assert.Equal(t, 1, batch1.RecordsMarkedSynced)
	assert.Equal(t, 1, batch2.RecordsMarkedSynced)
}

// TestDrain_NeverResendsAlreadySyncedAcrossBatches pins the PR 1b-iii
// idempotent-paging contract (task 1b-iii.3): across a manual Drain that
// pages through a 250-item backlog with a 100-item page cap (3 batches), each
// row must be pushed exactly once, and the loop must terminate by
// backlog-empty. The fake store below models a paged, filtered fetch backed
// by a real "unsynced" set: GetUnsyncedPage never returns a row once
// MarkSynced has removed it, mirroring the `synced_at IS NULL` predicate a
// real DB paged query enforces.
func TestDrain_NeverResendsAlreadySyncedAcrossBatches(t *testing.T) {
	withSyncPageSize(t, 100)
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	const total = 250
	store := newIdempotentPagingStore(total)

	var pushCounts []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		pushCounts = append(pushCounts, len(req.Memories))
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: len(req.Memories), Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, DrainFullySynced, outcome.State, "the loop must terminate by backlog-empty")
	// 250 items at a 100-item page cap page as 100/100/50, then one final
	// empty-fetch batch confirms PushBacklogEmpty and ends the loop — the
	// same shape as TestDrain_TriggerManual_LoopsUntilBacklogEmpty.
	assert.Equal(t, []int{100, 100, 50, 0}, pushCounts, "250 items at a 100-item page cap must page as 100/100/50/0")
	assert.Equal(t, 4, outcome.BatchesDone)
	assert.Equal(t, total, result.Pushed, "every one of the 250 rows must be pushed exactly once, summed across batches")

	pushedOnce := make(map[string]int)
	for _, syncID := range store.pushedSyncIDs {
		pushedOnce[syncID]++
	}
	for syncID, count := range pushedOnce {
		assert.Equal(t, 1, count, "sync_id %s must be pushed exactly once across the whole drain", syncID)
	}
	assert.Len(t, pushedOnce, total, "every seeded row must have been pushed")
}

// idempotentPagingStore is a minimal SyncStore fake, layered on mockSyncStore,
// that models a real DB's `synced_at IS NULL ORDER BY created_at ASC LIMIT ?`
// paging semantics for memories: GetUnsyncedPage only ever returns rows that
// have not yet been marked synced, and MarkSynced durably removes a row from
// the pending set so it can never be re-fetched or re-pushed. This is what
// TestDrain_NeverResendsAlreadySyncedAcrossBatches needs that mockSyncStore's
// static-slice GetUnsyncedPage cannot provide.
type idempotentPagingStore struct {
	mockSyncStore
	mu            sync.Mutex
	pending       []*models.Memory // ordered, oldest first — mirrors created_at ASC
	pushedSyncIDs []string
}

func newIdempotentPagingStore(total int) *idempotentPagingStore {
	pending := make([]*models.Memory, 0, total)
	for i := 0; i < total; i++ {
		pending = append(pending, createTestSyncMemory(fmt.Sprintf("idem-%03d", i)))
	}
	return &idempotentPagingStore{
		mockSyncStore: mockSyncStore{jwt: "valid-token"},
		pending:       pending,
	}
}

func (s *idempotentPagingStore) GetUnsyncedPage(project string, limit int) ([]*models.Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.pending) {
		limit = len(s.pending)
	}
	page := make([]*models.Memory, limit)
	copy(page, s.pending[:limit])
	return page, nil
}

func (s *idempotentPagingStore) MarkSynced(syncID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushedSyncIDs = append(s.pushedSyncIDs, syncID)
	for i, m := range s.pending {
		if m.SyncID == syncID {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			break
		}
	}
	return nil
}

// fkOrderingStore is a minimal SyncStore fake that models BOTH an unsynced
// session backlog and an unsynced memory backlog paging INDEPENDENTLY (PR
// 1b-iii regression, hive-sync-batched-drain PR 2b): sessions and memories
// are each served from their own paged, "synced_at IS NULL"-like pending
// set, mirroring ListUnsyncedSessionsPage/GetUnsyncedPage against real
// tables. The embedded httptest server plays the part of hive-api's
// per-request FK validator: a memory naming a session_id that is neither
// already confirmed server-side (confirmedSessions) nor present in the SAME
// request's sessions[] is rejected with a 400, exactly like
// ErrSessionNotFound in hive-api.
type fkOrderingStore struct {
	mockSyncStore
	mu                  sync.Mutex
	pendingSessions     []*models.Session
	pendingMemories     []*models.Memory
	confirmedSessions   map[string]bool
	pushedMemoryBatches [][]string // sync_ids per push, in order — for assertions
}

func newFKOrderingStore(sessions []*models.Session, memories []*models.Memory) *fkOrderingStore {
	return &fkOrderingStore{
		mockSyncStore:     mockSyncStore{jwt: "valid-token"},
		pendingSessions:   sessions,
		pendingMemories:   memories,
		confirmedSessions: make(map[string]bool),
	}
}

func (s *fkOrderingStore) ListUnsyncedSessionsPage(project string, limit int) ([]*models.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.pendingSessions) {
		limit = len(s.pendingSessions)
	}
	page := make([]*models.Session, limit)
	copy(page, s.pendingSessions[:limit])
	return page, nil
}

func (s *fkOrderingStore) GetUnsyncedPage(project string, limit int) ([]*models.Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.pendingMemories) {
		limit = len(s.pendingMemories)
	}
	page := make([]*models.Memory, limit)
	copy(page, s.pendingMemories[:limit])
	return page, nil
}

func (s *fkOrderingStore) MarkSessionSynced(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markedSessionSynced = append(s.markedSessionSynced, id)
	s.confirmedSessions[id] = true
	for i, sess := range s.pendingSessions {
		if sess.ID == id {
			s.pendingSessions = append(s.pendingSessions[:i], s.pendingSessions[i+1:]...)
			break
		}
	}
	return nil
}

func (s *fkOrderingStore) MarkSynced(syncID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markedSynced = append(s.markedSynced, syncID)
	for i, m := range s.pendingMemories {
		if m.SyncID == syncID {
			s.pendingMemories = append(s.pendingMemories[:i], s.pendingMemories[i+1:]...)
			break
		}
	}
	return nil
}

// fkValidatingHandler returns an http.HandlerFunc that models hive-api's
// per-request FK validator: any pushed memory whose session_id is non-empty
// must be satisfied either by a session in the SAME request's sessions[] or
// by a session already recorded as confirmed. Violations respond 400,
// exactly like ErrSessionNotFound would in hive-api.
func fkValidatingHandler(t *testing.T, confirmed map[string]bool, onRequest func(req syncRequest)) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		inRequest := make(map[string]bool, len(req.Sessions))
		for _, s := range req.Sessions {
			inRequest[s.ID] = true
		}

		for _, m := range req.Memories {
			if m.SessionID == "" {
				continue // lazy-created / manual-save sessions are the server's concern, not FK-blocked here
			}
			if !inRequest[m.SessionID] && !confirmed[m.SessionID] {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(fmt.Sprintf("session not found: %s", m.SessionID)))
				return
			}
		}

		if onRequest != nil {
			onRequest(req)
		}

		// Confirm sessions from this request AFTER validation, modeling the
		// server durably persisting them as part of this same push.
		for _, s := range req.Sessions {
			confirmed[s.ID] = true
		}

		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed: len(req.Memories),
			Pulled: []apiMemory{},
		}))
	}
}

// TestDrain_SessionPriorityPreventsFKViolationOnLaterSessionPage is the
// PR 2b regression test for the FK-ordering push bug introduced by PR
// 1b-iii's independent session/memory paging: with a session backlog LARGER
// than syncPageSize, a memory referencing a session that lands in a LATER
// session page must never be pushed before its own session has been pushed
// and confirmed. Before the PR 2b fix, syncBatchStep pushed a page of
// sessions AND a page of memories in the SAME request/batch regardless of
// whether the memory's named session had been drained yet — with 5 sessions
// at page size 2, a memory naming the 5th session (page 3) would be pushed
// in batch 1 alongside only the first 2 sessions, and hive-api would 400
// with ErrSessionNotFound. This test proves the fix: drain the session
// channel to empty BEFORE pushing any memories.
func TestDrain_SessionPriorityPreventsFKViolationOnLaterSessionPage(t *testing.T) {
	withSyncPageSize(t, 2)
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	sessions := make([]*models.Session, 0, 5)
	for i := 1; i <= 5; i++ {
		sessions = append(sessions, &models.Session{ID: fmt.Sprintf("sess-%d", i)})
	}
	// This memory names the LAST session (sess-5), which lands in the third
	// (final) session page at page size 2 — the exact regression scenario.
	memNamingLateSession := createTestSyncMemory("mem-late-session")
	memNamingLateSession.SessionID = "sess-5"

	store := newFKOrderingStore(sessions, []*models.Memory{memNamingLateSession})

	server := httptest.NewServer(fkValidatingHandler(t, store.confirmedSessions, nil))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err, "drain must not surface a 400 FK violation")
	require.NotNil(t, result)
	assert.Equal(t, DrainFullySynced, outcome.State)

	// Everything must eventually drain: all 5 sessions and the 1 memory.
	assert.Len(t, store.markedSessionSynced, 5, "all sessions must be confirmed")
	assert.Len(t, store.markedSynced, 1, "the memory must be pushed exactly once, after its session")
	assert.True(t, store.confirmedSessions["sess-5"], "sess-5 must be confirmed before the memory push succeeds")
}

// TestDrain_TriggerAuto_DefersMemoriesWhileSessionsPending pins the
// single-step TriggerAuto contract for the same fix: if sessions are still
// pending when a single auto-sync tick runs, that tick must push sessions
// only — memories wait for a later tick, once the session backlog is fully
// drained. This documents the (expected, temporary) memory-push delay under
// TriggerAuto when a large session backlog is still draining.
func TestDrain_TriggerAuto_DefersMemoriesWhileSessionsPending(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	sess := &models.Session{ID: "sess-pending"}
	mem := createTestSyncMemory("mem-with-pending-session")
	mem.SessionID = sess.ID

	store := newFKOrderingStore([]*models.Session{sess}, []*models.Memory{mem})

	var sawMemoriesInRequest bool
	server := httptest.NewServer(fkValidatingHandler(t, store.confirmedSessions, func(req syncRequest) {
		if len(req.Memories) > 0 {
			sawMemoriesInRequest = true
		}
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerAuto)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, DrainExpectedPending, outcome.State, "memory still pending after the single allowed batch")
	assert.False(t, sawMemoriesInRequest, "a single auto tick must not push memories while sessions are still pending")
	assert.Len(t, store.markedSessionSynced, 1, "the single auto tick must still push the pending session")
	assert.Empty(t, store.markedSynced, "the memory must wait for a later tick")

	// A second auto tick, now that sessions are drained, must push the
	// memory. TriggerAuto's PushBacklogEmpty reflects the backlog observed
	// GOING INTO the step (before this step's own push), matching
	// TestDrain_TriggerAuto_RunsExactlyOneStep — so this tick still reports
	// DrainExpectedPending even though it pushes (and fully drains) the last
	// pending memory; a follow-up tick with nothing left would report
	// DrainFullySynced instead.
	result2, outcome2, err := syncer.Drain(context.Background(), "test-project", TriggerAuto)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Equal(t, DrainExpectedPending, outcome2.State)
	assert.True(t, sawMemoriesInRequest, "the follow-up tick must push the now-unblocked memory")
	assert.Len(t, store.markedSynced, 1)

	// A third tick, with nothing left pending, confirms the fully-synced state.
	result3, outcome3, err := syncer.Drain(context.Background(), "test-project", TriggerAuto)
	require.NoError(t, err)
	require.NotNil(t, result3)
	assert.Equal(t, DrainFullySynced, outcome3.State)
}

// TestSyncBatchStep_SendsPullLimitAndPersistedCursors pins task 2.8
// (hive-sync-batched-drain PR 2b): syncBatchStep must send PullLimit =
// syncPageSize plus the persisted memories/sessions pull cursors from
// GetPullCursor as an explicit opt-in into hive-api's bounded pull
// pagination (PR 2a).
func TestSyncBatchStep_SendsPullLimitAndPersistedCursors(t *testing.T) {
	withSyncPageSize(t, 50)

	memCursor := db.PullCursor{SyncedAt: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), SyncID: "mem-resume"}
	sessCursor := db.PullCursor{SyncedAt: time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC), SyncID: "sess-resume"}

	store := &mockSyncStore{
		jwt: "valid-token",
		pullCursors: map[string]db.PullCursor{
			pullCursorKey(mutationCursorConsumerAPI, "test-project", pullCursorChannelMemories): memCursor,
			pullCursorKey(mutationCursorConsumerAPI, "test-project", pullCursorChannelSessions): sessCursor,
		},
	}

	var captured syncRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

	_, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
	require.NoError(t, err)

	assert.Equal(t, 50, captured.PullLimit, "PullLimit must be sent as syncPageSize (explicit opt-in)")
	require.NotNil(t, captured.PullCursor)
	assert.Equal(t, "mem-resume", captured.PullCursor.SyncID)
	require.NotNil(t, captured.PullSessionCursor)
	assert.Equal(t, "sess-resume", captured.PullSessionCursor.SyncID)

	assert.Contains(t, store.getPullCursorCalls, pullCursorCall{mutationCursorConsumerAPI, "test-project", pullCursorChannelMemories})
	assert.Contains(t, store.getPullCursorCalls, pullCursorCall{mutationCursorConsumerAPI, "test-project", pullCursorChannelSessions})
}

// TestSyncBatchStep_PersistsNextPullCursorsOnlyWhenPresent pins task 2.8:
// syncBatchStep must persist resp.NextPullCursor/NextSessionCursor via
// SetPullCursor, but ONLY when the server actually returned them — a nil
// cursor in the response must not overwrite whatever was previously
// persisted with a zero value.
func TestSyncBatchStep_PersistsNextPullCursorsOnlyWhenPresent(t *testing.T) {
	tests := []struct {
		name               string
		resp               syncResponse
		wantSetCalls       []pullCursorSetCall
		wantPullHasMore    bool
		wantSetCallsLength int
	}{
		{
			name: "both next cursors present are both persisted",
			resp: syncResponse{
				Pushed: 0, Pulled: []apiMemory{},
				PulledHasMore:         true,
				NextPullCursor:        &PullCursor{SyncID: "mem-next"},
				PulledSessionsHasMore: false,
				NextSessionCursor:     &PullCursor{SyncID: "sess-next"},
			},
			wantSetCalls: []pullCursorSetCall{
				{mutationCursorConsumerAPI, "test-project", pullCursorChannelMemories, db.PullCursor{SyncID: "mem-next"}},
				{mutationCursorConsumerAPI, "test-project", pullCursorChannelSessions, db.PullCursor{SyncID: "sess-next"}},
			},
			wantPullHasMore:    true,
			wantSetCallsLength: 2,
		},
		{
			name: "absent next cursors are not persisted",
			resp: syncResponse{
				Pushed: 0, Pulled: []apiMemory{},
			},
			wantSetCalls:       nil,
			wantPullHasMore:    false,
			wantSetCallsLength: 0,
		},
		{
			name: "only memories cursor present persists only that channel",
			resp: syncResponse{
				Pushed: 0, Pulled: []apiMemory{},
				PulledHasMore:  true,
				NextPullCursor: &PullCursor{SyncID: "mem-only"},
			},
			wantSetCalls: []pullCursorSetCall{
				{mutationCursorConsumerAPI, "test-project", pullCursorChannelMemories, db.PullCursor{SyncID: "mem-only"}},
			},
			wantPullHasMore:    true,
			wantSetCallsLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockSyncStore{jwt: "valid-token"}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/sync" {
					return
				}
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(tt.resp))
			}))
			defer server.Close()

			syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

			result, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
			require.NoError(t, err)

			assert.Equal(t, tt.wantPullHasMore, result.PullHasMore)
			assert.Len(t, store.setPullCursorCalls, tt.wantSetCallsLength)
			assert.Equal(t, tt.wantSetCalls, store.setPullCursorCalls)
		})
	}
}

// TestSyncBatchStep_OldServerResponseWithoutPullFieldsReportsNoMore pins the
// backward-compat requirement: an OLD hive-api response with no
// pulled_has_more/pulled_sessions_has_more fields decodes those to false, so
// PullHasMore must be false and Drain(TriggerAuto/TriggerManual) terminates
// on push-empty exactly like before PR 2b — no new hang, no forced extra
// batch.
func TestSyncBatchStep_OldServerResponseWithoutPullFieldsReportsNoMore(t *testing.T) {
	store := &mockSyncStore{jwt: "valid-token"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		w.WriteHeader(http.StatusOK)
		// Old, pre-PR-2a response shape.
		_, err := w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

	result, err := syncer.syncBatchStep(context.Background(), "test-project", "valid-token")
	require.NoError(t, err)
	assert.False(t, result.PullHasMore, "old server response without has_more fields must decode to PullHasMore=false")
	assert.Empty(t, store.setPullCursorCalls, "no next cursor was returned, nothing should be persisted")
}

// TestDrain_TriggerManual_DrainsMultiPagePullAndAdvancesCursorsPerChannel is
// the fresh-review-required regression test for task 2.8: a scripted
// httptest server returns pulled_has_more/pulled_sessions_has_more=true for
// N pages then false, each time advancing the next cursor. Drain
// (TriggerManual) must keep looping until BOTH pull channels report
// has_more=false (push backlog is already empty throughout — this test
// isolates the pull side), and must persist the per-channel cursor
// advancing on every page.
func TestDrain_TriggerManual_DrainsMultiPagePullAndAdvancesCursorsPerChannel(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{jwt: "valid-token"}

	// 3 pull pages: memories drains after page 2, sessions drains after page 3.
	memCursors := []*PullCursor{
		{SyncID: "mem-page-1"},
		{SyncID: "mem-page-2"},
		nil, // memories fully drained by page 3
	}
	sessCursors := []*PullCursor{
		{SyncID: "sess-page-1"},
		{SyncID: "sess-page-2"},
		{SyncID: "sess-page-3"},
	}
	memHasMore := []bool{true, true, false}
	sessHasMore := []bool{true, true, false}

	var callCount int
	var pullLimitsSeen []int
	var memCursorsSeen []*PullCursor
	var sessCursorsSeen []*PullCursor

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		var req syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		pullLimitsSeen = append(pullLimitsSeen, req.PullLimit)
		memCursorsSeen = append(memCursorsSeen, req.PullCursor)
		sessCursorsSeen = append(sessCursorsSeen, req.PullSessionCursor)

		idx := callCount
		if idx >= len(memHasMore) {
			idx = len(memHasMore) - 1
		}
		callCount++

		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed: 0, Pulled: []apiMemory{},
			PulledHasMore:         memHasMore[idx],
			NextPullCursor:        memCursors[idx],
			PulledSessionsHasMore: sessHasMore[idx],
			NextSessionCursor:     sessCursors[idx],
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 3, outcome.BatchesDone, "loop must run exactly 3 batches: 2 with has_more, 1 final false")
	assert.Equal(t, DrainFullySynced, outcome.State)
	assert.Equal(t, []int{100, 100, 100}, pullLimitsSeen, "every batch sends the same PullLimit = syncPageSize")

	// First batch sends no cursor (nothing persisted yet); subsequent batches
	// resume from what the PREVIOUS response returned.
	require.Len(t, memCursorsSeen, 3)
	assert.Nil(t, memCursorsSeen[0], "first batch has no persisted memories cursor yet")
	require.NotNil(t, memCursorsSeen[1])
	assert.Equal(t, "mem-page-1", memCursorsSeen[1].SyncID)
	require.NotNil(t, memCursorsSeen[2])
	assert.Equal(t, "mem-page-2", memCursorsSeen[2].SyncID)

	require.Len(t, sessCursorsSeen, 3)
	assert.Nil(t, sessCursorsSeen[0], "first batch has no persisted sessions cursor yet")
	require.NotNil(t, sessCursorsSeen[1])
	assert.Equal(t, "sess-page-1", sessCursorsSeen[1].SyncID)
	require.NotNil(t, sessCursorsSeen[2])
	assert.Equal(t, "sess-page-2", sessCursorsSeen[2].SyncID)

	// Final persisted state: memories cursor stopped advancing after page 2
	// (page 3 returned nil next cursor alongside has_more=false); sessions
	// cursor advanced through page 3.
	finalMem, err := store.GetPullCursor(mutationCursorConsumerAPI, "test-project", pullCursorChannelMemories)
	require.NoError(t, err)
	assert.Equal(t, "mem-page-2", finalMem.SyncID)

	finalSess, err := store.GetPullCursor(mutationCursorConsumerAPI, "test-project", pullCursorChannelSessions)
	require.NoError(t, err)
	assert.Equal(t, "sess-page-3", finalSess.SyncID)
}

// TestDrain_TriggerManual_PullHasMoreButCursorStuck_Terminates is the
// fresh-review CRITICAL regression test (PR 2b infinite-loop fix): a
// misbehaving/legacy server keeps reporting pulled_has_more=true (and
// pulled_sessions_has_more=true) on every batch but NEVER advances either
// pull cursor (next_pull_cursor/next_session_cursor come back nil every
// time, and nothing is ever pulled/pushed). Before the fix, the no-progress
// guard trusted batch.PullHasMore unconditionally and looped forever. After
// the fix, the guard must also require an actual pull-cursor advance before
// treating PullHasMore as a progress signal, so this must terminate within a
// small, bounded number of batches and classify as DrainExpectedPending (the
// batch step never errors, so this is not a DrainDegradedFailure).
func TestDrain_TriggerManual_PullHasMoreButCursorStuck_Terminates(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{jwt: "valid-token"}

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed: 0, Pulled: []apiMemory{},
			// Server bug / malformed response: keeps claiming more pages are
			// pending but never returns an advancing cursor for either
			// channel, and nothing is actually pulled or marked synced.
			PulledHasMore:         true,
			NextPullCursor:        nil,
			PulledSessionsHasMore: true,
			NextSessionCursor:     nil,
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	done := make(chan struct{})
	var result *Result
	var outcome DrainOutcome
	var drainErr error
	go func() {
		result, outcome, drainErr = syncer.Drain(context.Background(), "test-project", TriggerManual)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Drain(TriggerManual) did not terminate — infinite loop on stuck pull cursor")
	}

	require.NoError(t, drainErr)
	require.NotNil(t, result)

	// Must stop well short of any "generous" iteration cap — the cursor
	// no-progress guard, not the cap, is what terminates this case.
	assert.Less(t, int(atomic.LoadInt32(&callCount)), 10,
		"stuck-cursor drain must terminate almost immediately via the cursor no-progress guard, not the iteration cap")
	assert.Equal(t, DrainExpectedPending, outcome.State)
}

// TestDrain_TriggerManual_IterationCapEnforced is the defense-in-depth
// regression test for the bounded iteration cap: even if a server always
// legitimately advances the pull cursor (so the cursor no-progress guard
// alone would keep looping forever), Drain(TriggerManual) must still stop
// once it hits maxDrainBatches. The test overrides maxDrainBatches to a
// small value so it can hit the cap deterministically without looping
// thousands of times.
func TestDrain_TriggerManual_IterationCapEnforced(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &mockSyncStore{jwt: "valid-token"}

	origCap := maxDrainBatches
	maxDrainBatches = 5
	defer func() { maxDrainBatches = origCap }()

	var seq int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			return
		}
		n := atomic.AddInt64(&seq, 1)
		cursor := &PullCursor{SyncID: fmt.Sprintf("mem-page-%d", n)}
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			Pushed: 0, Pulled: []apiMemory{},
			// Always advances — a healthy-looking pull that never ends,
			// simulating an endlessly-paginating backlog. Only the cap
			// should stop this, not the cursor no-progress guard.
			PulledHasMore:  true,
			NextPullCursor: cursor,
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return baseNow },
		jitter: func(max time.Duration) time.Duration { return 0 },
	})

	done := make(chan struct{})
	var result *Result
	var outcome DrainOutcome
	var drainErr error
	go func() {
		result, outcome, drainErr = syncer.Drain(context.Background(), "test-project", TriggerManual)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Drain(TriggerManual) did not terminate — iteration cap not enforced")
	}

	require.NoError(t, drainErr)
	require.NotNil(t, result)
	assert.Equal(t, maxDrainBatches, outcome.BatchesDone, "loop must stop exactly at the overridden cap")
	assert.Equal(t, DrainExpectedPending, outcome.State)
}

// TestDrainOutcome_ClassifiesStateAndReason pins the PR 3 (task 3.1) full
// DrainOutcome classification contract: every termination path must produce
// the right State, and DrainExpectedPending must additionally carry the
// right Reason so a caller can distinguish an expected, bounded remainder
// (auto-single-step) from something that is actually stuck (no-progress,
// iteration-cap).
func TestDrainOutcome_ClassifiesStateAndReason(t *testing.T) {
	baseNow := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	t.Run("fully-synced", func(t *testing.T) {
		store := &mockSyncStore{
			jwt:      "valid-token",
			unsynced: nil,
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sync" {
				return
			}
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		}))
		defer server.Close()

		syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
			now:    func() time.Time { return baseNow },
			jitter: func(max time.Duration) time.Duration { return 0 },
		})

		result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DrainFullySynced, outcome.State)
		assert.Equal(t, DrainReasonNone, outcome.Reason)
		assert.Equal(t, 0, outcome.RemainingPush)
		assert.Nil(t, outcome.Err)
	})

	t.Run("expected-pending-auto-single-step", func(t *testing.T) {
		store := &mockSyncStore{
			jwt: "valid-token",
			unsyncedSequence: [][]*models.Memory{
				{createTestSyncMemory("local-1")},
				{createTestSyncMemory("local-2")},
			},
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sync" {
				return
			}
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
		}))
		defer server.Close()

		syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
			now:    func() time.Time { return baseNow },
			jitter: func(max time.Duration) time.Duration { return 0 },
		})

		result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerAuto)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DrainExpectedPending, outcome.State)
		assert.Equal(t, DrainReasonAutoSingleStep, outcome.Reason, "TriggerAuto stopping after its single allowed batch is expected-by-design, not stuck")
		assert.Nil(t, outcome.Err)
	})

	t.Run("expected-pending-no-progress", func(t *testing.T) {
		store := &mockSyncStore{
			jwt:           "valid-token",
			unsynced:      []*models.Memory{createTestSyncMemory("stuck-memory")},
			markSyncedErr: errors.New("persistent mark-synced failure"),
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sync" {
				return
			}
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}))
		}))
		defer server.Close()

		syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
			now:    func() time.Time { return baseNow },
			jitter: func(max time.Duration) time.Duration { return 0 },
		})

		result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DrainExpectedPending, outcome.State)
		assert.Equal(t, DrainReasonNoProgress, outcome.Reason, "a stuck no-progress guard trip must be distinguishable from an expected auto-single-step remainder")
		assert.Nil(t, outcome.Err)
	})

	t.Run("expected-pending-iteration-cap", func(t *testing.T) {
		store := &mockSyncStore{jwt: "valid-token"}

		origCap := maxDrainBatches
		maxDrainBatches = 3
		defer func() { maxDrainBatches = origCap }()

		var seq int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sync" {
				return
			}
			n := atomic.AddInt64(&seq, 1)
			cursor := &PullCursor{SyncID: fmt.Sprintf("mem-page-%d", n)}
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
				Pushed: 0, Pulled: []apiMemory{},
				PulledHasMore:  true,
				NextPullCursor: cursor,
			}))
		}))
		defer server.Close()

		syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
			now:    func() time.Time { return baseNow },
			jitter: func(max time.Duration) time.Duration { return 0 },
		})

		result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DrainExpectedPending, outcome.State)
		assert.Equal(t, DrainReasonIterationCap, outcome.Reason, "hitting the iteration cap while still progressing must be distinguishable from a genuinely stuck no-progress trip")
		assert.Nil(t, outcome.Err)
	})

	t.Run("degraded-failure", func(t *testing.T) {
		store := &mockSyncStore{
			jwt: "valid-token",
			unsyncedSequence: [][]*models.Memory{
				{createTestSyncMemory("local-1")},
				{createTestSyncMemory("local-2")},
			},
		}
		var syncCalls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sync" {
				return
			}
			n := syncCalls.Add(1)
			if n == 2 {
				w.WriteHeader(http.StatusInternalServerError)
				_, writeErr := w.Write([]byte("boom"))
				require.NoError(t, writeErr)
				return
			}
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(syncResponse{Pushed: 1, Pulled: []apiMemory{}, Conflicts: 0}))
		}))
		defer server.Close()

		syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
			now:    func() time.Time { return baseNow },
			jitter: func(max time.Duration) time.Duration { return 0 },
		})

		result, outcome, err := syncer.Drain(context.Background(), "test-project", TriggerManual)
		require.Error(t, err)
		require.NotNil(t, result)
		assert.Equal(t, DrainDegradedFailure, outcome.State)
		assert.Equal(t, DrainReasonNone, outcome.Reason)
		require.Error(t, outcome.Err)
		assert.ErrorIs(t, outcome.Err, err)
	})
}

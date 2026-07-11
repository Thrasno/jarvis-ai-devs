package sync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observableEvents struct {
	mu     sync.Mutex
	events []string
}

func (e *observableEvents) append(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *observableEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

func (e *observableEvents) Write(p []byte) (int, error) {
	if strings.Contains(string(p), ErrStoredCredentialsRejected.Error()) {
		e.append("stale-diagnostic")
	}
	return len(p), nil
}

func installStaleDiagnosticSink(t *testing.T, events *observableEvents) {
	t.Helper()
	original := logger.Log.Writer()
	logger.Log.SetOutput(events)
	t.Cleanup(func() { logger.Log.SetOutput(original) })
}

func TestClient_HTTPStatusErrorsAreTypedAndSecretSafe(t *testing.T) {
	secret := "response-secret-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(secret))
	}))
	defer server.Close()

	c := newClient(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"})
	calls := []struct {
		name string
		call func() error
	}{
		{"login", func() error { _, _, err := c.login(context.Background()); return err }},
		{"sync", func() error {
			_, err := c.sync(context.Background(), "jwt-secret", "project", nil, nil, nil, nil, nil, nil, pullOptions{})
			return err
		}},
		{"ack", func() error { return c.ackProjectBlock(context.Background(), "jwt-secret", db.ProjectBlockAck{}) }},
		{"attempt upload", func() error { _, err := c.syncAttempts(context.Background(), "jwt-secret", nil); return err }},
	}
	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			var statusErr *HTTPStatusError
			require.ErrorAs(t, err, &statusErr)
			assert.Equal(t, http.StatusUnauthorized, statusErr.StatusCode)
			assert.NotContains(t, err.Error(), secret)
			assert.NotContains(t, err.Error(), "jwt-secret")
		})
	}
}

func TestSyncer_InitialLogin401LatchesCredentialsAcrossConcurrentCalls(t *testing.T) {
	events := &observableEvents{}
	installStaleDiagnosticSink(t, events)
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		events.append("login-401")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	store := &mockSyncStore{events: events}
	s := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() { defer group.Done(); _, err := s.getOrRefreshToken(context.Background()); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.ErrorIs(t, err, ErrStoredCredentialsRejected)
		assert.NotContains(t, err.Error(), "password123")
	}
	assert.Equal(t, int32(1), logins.Load())
	assert.Equal(t, 1, store.clearJWTCalls, "the rejected initial login must invalidate the persisted cached session exactly once")

	_, err := s.getOrRefreshToken(context.Background())
	require.ErrorIs(t, err, ErrStoredCredentialsRejected)
	assert.Equal(t, int32(1), logins.Load())

	_, _, err = s.Drain(context.Background(), "test-project", TriggerAuto)
	require.ErrorIs(t, err, ErrStoredCredentialsRejected)
	assert.Equal(t, int32(1), logins.Load(), "a later drain after successful cleanup must not make another network request")
	assert.Equal(t, 1, store.clearJWTCalls)
	assert.Equal(t, []string{"login-401", "clear-jwt", "stale-diagnostic"}, events.snapshot())
}

func TestSyncer_InitialLogin401StopsWhenCachedSessionCleanupFails(t *testing.T) {
	events := &observableEvents{}
	installStaleDiagnosticSink(t, events)
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		events.append("login-401")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	store := &mockSyncStore{events: events, clearJWTErr: errors.New("database details must not leak")}
	s := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{})

	_, err := s.getOrRefreshToken(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "clear cached Hive API session")
	assert.NotContains(t, err.Error(), "database details must not leak")
	assert.NotContains(t, err.Error(), "password123")
	assert.NotErrorIs(t, err, ErrStoredCredentialsRejected, "cleanup failure must not commit the stale-credentials latch")
	assert.Equal(t, int32(1), logins.Load(), "cleanup failure must stop the current login attempt")
	assert.Equal(t, 1, store.clearJWTCalls)

	_, err = s.getOrRefreshToken(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrStoredCredentialsRejected)
	assert.Equal(t, int32(2), logins.Load(), "without a committed stale latch, the next explicit attempt may retry")
	assert.Equal(t, 2, store.clearJWTCalls)
	assert.Equal(t, []string{"login-401", "clear-jwt", "login-401", "clear-jwt"}, events.snapshot(), "cleanup failure must stop at the clear attempt without emitting a stale diagnostic or committing the latch")
	assert.False(t, s.credentialsStale)
	assert.False(t, s.staleDiagnosticEmitted)
}

func TestSyncer_InitialLoginServerFailureRemainsRetryable(t *testing.T) {
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, &mockSyncStore{}, syncDeps{})
	for range 2 {
		_, err := s.getOrRefreshToken(context.Background())
		var statusErr *HTTPStatusError
		require.True(t, errors.As(err, &statusErr))
		assert.Equal(t, http.StatusInternalServerError, statusErr.StatusCode)
	}
	assert.Equal(t, int32(2), logins.Load())
}

func TestStaleCredentialDiagnosticIsActionableAndSanitized(t *testing.T) {
	assert.Contains(t, ErrStoredCredentialsRejected.Error(), "Jarvis → Hive API Config")
	assert.Contains(t, ErrStoredCredentialsRejected.Error(), "restart hive-daemon")
	assert.NotContains(t, strings.ToLower(ErrStoredCredentialsRejected.Error()), "token")
}

func TestSyncer_StaleCredentialsHaveStableAttemptClassification(t *testing.T) {
	store := &mockSyncStore{}
	s := newTestSyncer(&Config{Email: "daemon@example.com"}, store, syncDeps{})
	s.recordFailureAttemptLog(context.Background(), "project", s.deps.now(), ErrStoredCredentialsRejected)
	require.Len(t, store.recordedSyncAttempts, 1)
	attempt := store.recordedSyncAttempts[0]
	assert.Equal(t, 401, attempt.HTTPStatus)
	assert.Equal(t, "auth_credentials_stale", attempt.ErrorCode)
	assert.Equal(t, `{"auth_recovery":"stopped"}`, attempt.MetadataJSON)
}

func TestSyncer_ConcurrentCachedTokenRecoveryReusesReplacement(t *testing.T) {
	events := &observableEvents{}
	installStaleDiagnosticSink(t, events)
	oldSyncs := make(chan struct{}, 2)
	releaseOldSyncs := make(chan struct{})
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login":
			logins.Add(1)
			_, _ = w.Write([]byte(`{"token":"replacement-jwt","expires_at":"2030-01-01T00:00:00Z"}`))
		case "/sync":
			if r.Header.Get("Authorization") == "Bearer rejected-jwt" {
				oldSyncs <- struct{}{}
				<-releaseOldSyncs
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer replacement-jwt" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
		}
	}))
	defer server.Close()

	store := &mockSyncStore{jwt: "rejected-jwt", events: events}
	s := newTestSyncer(&Config{APIURL: server.URL, Email: "daemon@example.com", Password: "password123"}, store, syncDeps{})
	errs := make(chan error, 2)
	for _, project := range []string{"project-a", "project-b"} {
		go func(project string) { _, _, err := s.Drain(context.Background(), project, TriggerAuto); errs <- err }(project)
	}
	<-oldSyncs
	<-oldSyncs
	close(releaseOldSyncs)
	for range 2 {
		require.NoError(t, <-errs)
	}
	assert.Equal(t, int32(1), logins.Load())
	assert.Equal(t, 1, store.clearJWTCalls)
	assert.Equal(t, "replacement-jwt", store.jwt)
	assert.False(t, s.credentialsStale)
	assert.False(t, s.reauthenticatedTokenRejected)
	assert.Equal(t, []string{"clear-jwt"}, events.snapshot())
}

func TestSyncer_CachedToken401RecoversOnceAndRecordsOneSuccess(t *testing.T) {
	var logins, syncs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login":
			logins.Add(1)
			_, _ = w.Write([]byte(`{"token":"fresh-jwt","expires_at":"2030-01-01T00:00:00Z"}`))
		case "/sync":
			if syncs.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &mockSyncStore{jwt: "cached-jwt"}
	s := newTestSyncer(&Config{APIURL: server.URL, Email: "daemon@example.com", Password: "password123"}, store, syncDeps{})
	_, _, err := s.Drain(context.Background(), "project", TriggerAuto)
	require.NoError(t, err)
	assert.Equal(t, int32(1), logins.Load())
	assert.Equal(t, int32(2), syncs.Load())
	assert.Equal(t, 1, store.clearJWTCalls)
	assert.Equal(t, "fresh-jwt", store.jwt)
	require.Len(t, store.recordedSyncAttempts, 1)
	assert.Equal(t, db.SyncAttemptOutcomeSuccess, store.recordedSyncAttempts[0].Outcome)
	assert.Equal(t, `{"auth_recovery":"token_refreshed"}`, store.recordedSyncAttempts[0].MetadataJSON)
}

func TestSyncer_CachedTokenRecoveryTerminalStates(t *testing.T) {
	tests := []struct {
		name          string
		loginStatus   int
		retryStatus   int
		clearErr      error
		wantErr       error
		wantCode      string
		wantLogins    int32
		wantSyncs     int32
		wantClear     int
		laterNetworks int32
		latched       bool
	}{
		{name: "relogin 401 latches stale credentials", loginStatus: http.StatusUnauthorized, wantErr: ErrStoredCredentialsRejected, wantCode: "auth_credentials_stale", wantLogins: 1, wantSyncs: 1, wantClear: 2, latched: true},
		{name: "fresh token 401 latches server rejection", loginStatus: http.StatusOK, retryStatus: http.StatusUnauthorized, wantErr: ErrReauthenticatedTokenRejected, wantCode: "auth_token_rejected_after_login", wantLogins: 1, wantSyncs: 2, wantClear: 2, latched: true},
		{name: "relogin 500 remains retryable", loginStatus: http.StatusInternalServerError, wantErr: ErrAuthReloginFailed, wantCode: "auth_relogin_failed", wantLogins: 1, wantSyncs: 1, wantClear: 1, laterNetworks: 1},
		{name: "clear failure stops before relogin", clearErr: errors.New("storage secret"), wantErr: ErrCachedSessionCleanupFailed, wantCode: "sync_failed", wantLogins: 0, wantSyncs: 1, wantClear: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logins, syncs atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/login":
					logins.Add(1)
					if tt.loginStatus != http.StatusOK {
						w.WriteHeader(tt.loginStatus)
						return
					}
					_, _ = w.Write([]byte(`{"token":"fresh-jwt","expires_at":"2030-01-01T00:00:00Z"}`))
				case "/sync":
					if syncs.Add(1) == 1 || tt.retryStatus == http.StatusOK {
						if syncs.Load() == 1 {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
						return
					}
					w.WriteHeader(tt.retryStatus)
				}
			}))
			defer server.Close()

			store := &mockSyncStore{jwt: "cached-jwt", clearJWTErr: tt.clearErr}
			s := newTestSyncer(&Config{APIURL: server.URL, Email: "daemon@example.com", Password: "password123"}, store, syncDeps{})
			_, _, err := s.Drain(context.Background(), "project", TriggerAuto)
			require.ErrorIs(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "password123")
			assert.NotContains(t, err.Error(), "storage secret")
			assert.Equal(t, tt.wantLogins, logins.Load())
			assert.Equal(t, tt.wantSyncs, syncs.Load())
			assert.Equal(t, tt.wantClear, store.clearJWTCalls)
			require.Len(t, store.recordedSyncAttempts, 1)
			assert.Equal(t, tt.wantCode, store.recordedSyncAttempts[0].ErrorCode)

			if tt.latched {
				_, _, err = s.Drain(context.Background(), "another-project", TriggerAuto)
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.wantLogins, logins.Load(), "latched state must prevent later login")
				assert.Equal(t, tt.wantSyncs, syncs.Load(), "latched state must prevent later sync")
			}
			if tt.laterNetworks > 0 {
				_, _, err = s.Drain(context.Background(), "another-project", TriggerAuto)
				require.Error(t, err)
				assert.Equal(t, tt.wantLogins+tt.laterNetworks, logins.Load())
			}
		})
	}
}

func TestSyncer_RecoverySetJWTFailureIsSanitizedRetryableAndRecorded(t *testing.T) {
	secret := "sensitive-storage-error"
	var logins, syncs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync":
			syncs.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		case "/auth/login":
			logins.Add(1)
			_, _ = w.Write([]byte(`{"token":"fresh-jwt","expires_at":"2030-01-01T00:00:00Z"}`))
		}
	}))
	defer server.Close()

	store := &mockSyncStore{jwt: "cached-jwt", setJWTErr: errors.New(secret)}
	s := newTestSyncer(&Config{APIURL: server.URL, Email: "daemon@example.com", Password: "password123"}, store, syncDeps{})
	_, _, err := s.Drain(context.Background(), "project", TriggerAuto)
	require.ErrorIs(t, err, ErrAuthReloginFailed)
	assert.NotContains(t, err.Error(), secret)
	assert.Equal(t, int32(1), logins.Load())
	assert.Equal(t, int32(1), syncs.Load())
	require.Len(t, store.recordedSyncAttempts, 1)
	assert.Equal(t, "auth_relogin_failed", store.recordedSyncAttempts[0].ErrorCode)
	assert.Empty(t, store.jwt, "failed recovery must not claim a fresh session")
	store.mu.Lock()
	store.healthByProject["project"] = db.SyncHealth{Project: "project"}
	store.mu.Unlock()

	_, _, err = s.Drain(context.Background(), "project", TriggerAuto)
	require.Error(t, err, "storage failure must remain retryable, not latched")
	assert.Equal(t, int32(2), logins.Load())
	assert.Equal(t, int32(1), syncs.Load(), "no token exists after failed persistence")
}

func TestSyncer_RecoveryUsedAcrossBatchesRejectsFreshTokenAndLatches(t *testing.T) {
	withSyncPageSize(t, 1)
	events := &observableEvents{}
	var logins, syncs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/login":
			logins.Add(1)
			events.append("login")
			_, _ = w.Write([]byte(`{"token":"fresh-jwt","expires_at":"2030-01-01T00:00:00Z"}`))
		case "/sync":
			n := syncs.Add(1)
			events.append("sync-" + r.Header.Get("Authorization"))
			if n == 1 || n == 3 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"pushed":1,"pulled":[],"conflicts":0}`))
		}
	}))
	defer server.Close()

	store := &mockSyncStore{
		jwt:    "cached-jwt",
		events: events,
		unsyncedSequence: [][]*models.Memory{
			{createTestSyncMemory("batch-1"), createTestSyncMemory("batch-2")},
			{createTestSyncMemory("batch-2")},
		},
	}
	s := newTestSyncer(&Config{APIURL: server.URL, Email: "daemon@example.com", Password: "password123"}, store, syncDeps{})
	result, outcome, err := s.Drain(context.Background(), "project", TriggerManual)
	require.ErrorIs(t, err, ErrReauthenticatedTokenRejected)
	require.NotNil(t, result, "the first recovered batch must remain accounted for")
	assert.Equal(t, 2, outcome.BatchesDone)
	assert.Equal(t, int32(1), logins.Load())
	assert.Equal(t, int32(3), syncs.Load())
	assert.Equal(t, 2, store.clearJWTCalls, "clear cached then rejected fresh JWT")
	require.Len(t, store.recordedSyncAttempts, 1)
	assert.Equal(t, "auth_token_rejected_after_login", store.recordedSyncAttempts[0].ErrorCode)
	assert.Equal(t, []string{"sync-Bearer cached-jwt", "clear-jwt", "login", "sync-Bearer fresh-jwt", "sync-Bearer fresh-jwt", "clear-jwt"}, events.snapshot())
	store.mu.Lock()
	store.healthByProject["project"] = db.SyncHealth{Project: "project"}
	store.mu.Unlock()

	_, _, err = s.Drain(context.Background(), "project", TriggerManual)
	require.ErrorIs(t, err, ErrReauthenticatedTokenRejected)
	assert.Equal(t, int32(1), logins.Load())
	assert.Equal(t, int32(3), syncs.Load(), "latched drain must make zero network calls")
}

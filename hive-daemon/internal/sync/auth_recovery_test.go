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

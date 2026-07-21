package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureDaemonLog redirects the package stderr logger to a buffer for the
// duration of the test and restores it afterwards.
func captureDaemonLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := logger.Log.Writer()
	logger.Log.SetOutput(&buf)
	t.Cleanup(func() { logger.Log.SetOutput(prev) })
	return &buf
}

// TestPostSessions_UnresolvableDirectory_LogsRefusalNeverRegistersDefault
// verifies that when project is empty and the directory cannot be derived
// (unresolvable path → typed error), the handler logs the refusal with the
// reason, never falls back to registering the reserved "default" name, and
// never creates a session (spec: No Fallback to "default" Registration).
func TestPostSessions_UnresolvableDirectory_LogsRefusalNeverRegistersDefault(t *testing.T) {
	logBuf := captureDaemonLog(t)

	created := false
	store := &mockSessionStore{
		createSessionFn: func(id, proj, directory, devID, client string) error {
			created = true
			return nil
		},
	}
	srv := newServerWithSessions(store)

	// A path that does not exist → hivederive returns a typed error, so the
	// derived name is refused rather than defaulting.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	reqBody := mustMarshal(t, map[string]string{
		"id":        "sess-unresolvable",
		"project":   "",
		"directory": missing,
		"client":    "hook",
	})
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Honest failure: no derivable project → reject, never register "default".
	require.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
	assert.False(t, created, "session must NOT be created when derivation fails")

	logged := logBuf.String()
	assert.Contains(t, logged, "derive:", "refusal must be logged with the derive reason")
	assert.Contains(t, logged, `refusing to register "default"`,
		"log must state that registering the reserved default name is refused")
}

package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncAttemptLogs_LocalQueueLifecycle(t *testing.T) {
	d := setupTestDB(t)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	ctx := context.Background()
	base := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)

	for _, record := range []SyncAttemptLog{
		attempt("attempt-success", "dev@example.com", SyncAttemptOutcomeSuccess, base),
		attempt("attempt-failure", "dev@example.com", SyncAttemptOutcomeFailure, base.Add(time.Minute)),
	} {
		require.NoError(t, d.RecordSyncAttemptLog(ctx, record))
	}
	pending, err := d.ListPendingSyncAttemptLogs(ctx, 100)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, SyncAttemptOutcomeSuccess, pending[0].Outcome)
	assert.Equal(t, SyncAttemptOutcomeFailure, pending[1].Outcome)
	assert.Equal(t, 500, pending[1].HTTPStatus)
	assert.Equal(t, "server_error", pending[1].ErrorCode)

	err = d.RecordSyncAttemptLog(ctx, attempt("missing-dev", "", SyncAttemptOutcomeFailure, base))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dev_id is required")
	require.NoError(t, seedInvalidAttemptWithoutDevID(d, base))
	pending, err = d.ListPendingSyncAttemptLogs(ctx, 100)
	require.NoError(t, err)
	assert.Len(t, pending, 2, "invalid local rows must not be deliverable")

	deliveredAt := base.Add(2 * time.Minute)
	require.NoError(t, d.MarkSyncAttemptLogsDelivered(ctx, []string{"attempt-success"}, deliveredAt))
	assertDeliveredAt(t, d, "attempt-success", deliveredAt)
	pending, err = d.ListPendingSyncAttemptLogs(ctx, 100)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "attempt-failure", pending[0].AttemptID)
}

func TestSyncAttemptLogs_PendingLimitAndRetention(t *testing.T) {
	d := setupTestDB(t)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	ctx := context.Background()
	base := time.Date(2026, 6, 19, 11, 0, 0, 0, time.UTC)

	for i := 0; i < 150; i++ {
		require.NoError(t, d.RecordSyncAttemptLog(ctx, attempt(fmt.Sprintf("attempt-%03d", i), "dev@example.com", SyncAttemptOutcomeSuccess, base.Add(time.Duration(i)*time.Second))))
	}
	pending, err := d.ListPendingSyncAttemptLogs(ctx, 500)
	require.NoError(t, err)
	require.Len(t, pending, 100)
	assert.Equal(t, "attempt-000", pending[0].AttemptID)
	assert.Equal(t, "attempt-099", pending[99].AttemptID)

	old := attempt("old-attempt", "dev@example.com", SyncAttemptOutcomeFailure, base.AddDate(0, 0, -91))
	kept := attempt("kept-attempt", "dev@example.com", SyncAttemptOutcomeSuccess, base.AddDate(0, 0, -89))
	require.NoError(t, d.RecordSyncAttemptLog(ctx, old))
	require.NoError(t, d.RecordSyncAttemptLog(ctx, kept))
	deleted, err := d.DeleteSyncAttemptLogsOlderThan(ctx, base.AddDate(0, 0, -90))
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	var count int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM sync_attempt_logs WHERE attempt_id = 'old-attempt'`).Scan(&count))
	assert.Zero(t, count)
}

func TestSanitizeSyncAttemptError_RedactsUnsafeContentAndTruncates(t *testing.T) {
	raw := "Authorization: Bearer secret-token\n" +
		"Cookie: session=secret\n" +
		"request body: {\"password\":\"secret\"}\n" +
		"panic: boom\n at github.com/acme/app.main(/home/andres/projects/jarvis/main.go:42)\n" +
		"failed for dev@example.com at /Users/andres/private/file.txt with token=abc123 " + strings.Repeat("x", 700)

	got := SanitizeSyncAttemptError("dev@example.com", raw)

	for _, unsafe := range []string{"secret-token", "Authorization", "Cookie", "request body", "password", "github.com/acme/app.main", "/home/andres", "/Users/andres", "dev@example.com", "token=abc123"} {
		assert.NotContains(t, got, unsafe)
	}
	assert.LessOrEqual(t, len([]rune(got)), 500)
	assert.Contains(t, got, "[redacted-path]")
}

func TestSanitizeSyncAttemptError_RedactsSensitiveHeadersAndColonSecrets(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		unsafe    []string
		wantParts []string
	}{
		{
			name: "generic sensitive headers",
			raw:  "X-API-Key: api-secret\nProxy-Authorization: Bearer proxy-secret\nSet-Cookie: session=secret\nconnection failed with status 503",
			unsafe: []string{
				"X-API-Key", "api-secret", "Proxy-Authorization", "proxy-secret", "Set-Cookie", "session=secret",
			},
			wantParts: []string{"connection failed with status 503"},
		},
		{
			name:      "colon form secrets",
			raw:       "sync failed token: token-secret api_key: key-secret password: pass-secret secret: raw-secret",
			unsafe:    []string{"token-secret", "key-secret", "pass-secret", "raw-secret"},
			wantParts: []string{"token: [redacted]", "api_key: [redacted]", "password: [redacted]", "secret: [redacted]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSyncAttemptError("dev@example.com", tt.raw)

			for _, unsafe := range tt.unsafe {
				assert.NotContains(t, got, unsafe)
			}
			for _, want := range tt.wantParts {
				assert.Contains(t, got, want)
			}
		})
	}
}

func attempt(id, devID string, outcome SyncAttemptOutcome, started time.Time) SyncAttemptLog {
	log := SyncAttemptLog{AttemptID: id, DevID: devID, Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: started, EndedAt: started.Add(time.Second), Outcome: outcome}
	if outcome == SyncAttemptOutcomeFailure {
		log.HTTPStatus = 500
		log.ErrorCode = "server_error"
		log.ErrorMessage = "sync failed safely"
	}
	return log
}

func seedInvalidAttemptWithoutDevID(d *DB, at time.Time) error {
	_, err := d.sqlDB.Exec(`INSERT INTO sync_attempt_logs (attempt_id, dev_id, project, started_at, ended_at, outcome) VALUES (?, '', ?, ?, ?, ?)`,
		"invalid-local-row", "jarvis-dev", formatSQLiteTime(at), formatSQLiteTime(at), string(SyncAttemptOutcomeFailure))
	return err
}

func assertDeliveredAt(t *testing.T, d *DB, attemptID string, want time.Time) {
	t.Helper()
	var stored string
	require.NoError(t, d.sqlDB.QueryRow(`SELECT delivered_at FROM sync_attempt_logs WHERE attempt_id = ?`, attemptID).Scan(&stored))
	got, err := parseTimeStr(stored)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

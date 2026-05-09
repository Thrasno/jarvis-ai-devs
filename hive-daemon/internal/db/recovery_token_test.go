package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	hivedb "github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/project"
)

func TestRecoveryTokens_ArePersistedAcrossReopenAndConsumedOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokens.db")
	expiresAt := time.Date(2026, 5, 9, 18, 0, 0, 0, time.UTC)

	d, err := hivedb.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	token, err := d.CreateRecoveryToken(ctx, project.TokenRequest{
		Token:            "tok-reopen",
		Reason:           string(project.CodeProjectAmbiguous),
		RequestedProject: "jarvis dev",
		Candidates: []project.Candidate{
			{Project: "jarvis-dev", Directory: "/repo/jarvis"},
			{Project: "jarvis.dev", Directory: "/repo/dot"},
		},
		ContextHash: "ctx-1",
		CreatedAt:   expiresAt.Add(-15 * time.Minute),
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateRecoveryToken: %v", err)
	}
	if token != "tok-reopen" {
		t.Fatalf("token = %q, want tok-reopen", token)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := hivedb.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	if err := reopened.ConsumeRecoveryToken(ctx, project.TokenValidation{
		Token:           token,
		SelectedProject: "jarvis-dev",
		ContextHash:     "ctx-1",
		Now:             expiresAt.Add(-time.Second),
	}); err != nil {
		t.Fatalf("ConsumeRecoveryToken first use: %v", err)
	}

	err = reopened.ConsumeRecoveryToken(ctx, project.TokenValidation{
		Token:           token,
		SelectedProject: "jarvis-dev",
		ContextHash:     "ctx-1",
		Now:             expiresAt.Add(-time.Second),
	})
	if !errors.Is(err, project.ErrRecoveryTokenConsumed) {
		t.Fatalf("second consume error = %v, want ErrRecoveryTokenConsumed", err)
	}
}

func TestRecoveryTokens_RejectExpiredUnknownWrongContextAndNotCandidate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 5, 9, 18, 0, 0, 0, time.UTC)
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	issue := func(t *testing.T, token, contextHash string, expiresAt time.Time) string {
		t.Helper()
		created, err := d.CreateRecoveryToken(ctx, project.TokenRequest{
			Token:            token,
			Reason:           string(project.CodeProjectAmbiguous),
			RequestedProject: "jarvis dev",
			Candidates:       []project.Candidate{{Project: "jarvis-dev"}},
			ContextHash:      contextHash,
			CreatedAt:        now.Add(-time.Minute),
			ExpiresAt:        expiresAt,
		})
		if err != nil {
			t.Fatalf("CreateRecoveryToken: %v", err)
		}
		return created
	}

	tests := []struct {
		name    string
		token   string
		issue   func(t *testing.T) string
		context string
		choice  string
		want    error
	}{
		{name: "unknown", token: "missing", context: "ctx", choice: "jarvis-dev", want: project.ErrRecoveryTokenInvalid},
		{name: "expired", issue: func(t *testing.T) string { return issue(t, "tok-expired", "ctx", now.Add(-time.Second)) }, context: "ctx", choice: "jarvis-dev", want: project.ErrRecoveryTokenExpired},
		{name: "wrong context", issue: func(t *testing.T) string { return issue(t, "tok-context", "ctx-original", now.Add(time.Minute)) }, context: "ctx-other", choice: "jarvis-dev", want: project.ErrRecoveryTokenWrongContext},
		{name: "not candidate", issue: func(t *testing.T) string { return issue(t, "tok-candidate", "ctx", now.Add(time.Minute)) }, context: "ctx", choice: "other-project", want: project.ErrRecoveryTokenNotCandidate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.token
			if tt.issue != nil {
				token = tt.issue(t)
			}

			err := d.ConsumeRecoveryToken(ctx, project.TokenValidation{
				Token:           token,
				SelectedProject: tt.choice,
				ContextHash:     tt.context,
				Now:             now,
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("ConsumeRecoveryToken error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRecoveryTokens_ExpiredTokenCleanupPersists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 5, 9, 18, 0, 0, 0, time.UTC)
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	_, err = d.CreateRecoveryToken(ctx, project.TokenRequest{
		Token:            "tok-expired-cleanup",
		Reason:           string(project.CodeProjectAmbiguous),
		RequestedProject: "jarvis dev",
		Candidates:       []project.Candidate{{Project: "jarvis-dev"}},
		ContextHash:      "ctx",
		CreatedAt:        now.Add(-time.Hour),
		ExpiresAt:        now.Add(-time.Second),
	})
	if err != nil {
		t.Fatalf("CreateRecoveryToken: %v", err)
	}

	err = d.ConsumeRecoveryToken(ctx, project.TokenValidation{
		Token:           "tok-expired-cleanup",
		SelectedProject: "jarvis-dev",
		ContextHash:     "ctx",
		Now:             now,
	})
	if !errors.Is(err, project.ErrRecoveryTokenExpired) {
		t.Fatalf("ConsumeRecoveryToken error = %v, want ErrRecoveryTokenExpired", err)
	}

	var count int
	if err := d.RawDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM recovery_tokens WHERE token = ?`, "tok-expired-cleanup").Scan(&count); err != nil {
		t.Fatalf("count recovery tokens: %v", err)
	}
	if count != 0 {
		t.Fatalf("expired token rows = %d, want 0 after cleanup", count)
	}
}

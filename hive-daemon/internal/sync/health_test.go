package sync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubHealthStore provides a controllable implementation of HealthStore for tests.
type stubHealthStore struct {
	rows               []db.SyncHealth
	listErr            error
	unsyncedMemories   int
	unsyncedPrompts    int
	unsyncedSessions   int
	countMemoriesErr   error
	countPromptsErr    error
	countSessionsErr   error
}

func (s *stubHealthStore) ListGovernanceSyncHealth(_ context.Context) ([]db.SyncHealth, error) {
	return s.rows, s.listErr
}

func (s *stubHealthStore) CountUnsyncedMemories(_ context.Context) (int, error) {
	return s.unsyncedMemories, s.countMemoriesErr
}

func (s *stubHealthStore) CountUnsyncedPrompts(_ context.Context) (int, error) {
	return s.unsyncedPrompts, s.countPromptsErr
}

func (s *stubHealthStore) CountUnsyncedSessions(_ context.Context) (int, error) {
	return s.unsyncedSessions, s.countSessionsErr
}

// stubConfigLoader allows tests to control LoadWithStatus output.
type stubConfigLoader struct {
	cfg      *hivesync.Config
	status   hivesync.SyncConfigStatus
	err      error
}

func (l *stubConfigLoader) Load() (*hivesync.Config, hivesync.SyncConfigStatus, error) {
	return l.cfg, l.status, l.err
}

func TestHealthService_Summary_States(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	successAt := now.Add(-1 * time.Hour)
	failureAt := now.Add(-30 * time.Minute)

	tests := []struct {
		name           string
		rows           []db.SyncHealth
		cfg            *hivesync.Config
		status         hivesync.SyncConfigStatus
		loaderErr      error
		wantReachable  bool
		wantAuthOK     bool
		wantAutoSync   bool
		wantLastError  string
		wantConsec     int
		wantMemories   int
		wantPrompts    int
		wantSessions   int
	}{
		{
			name: "healthy: configured, no errors, recent success",
			rows: []db.SyncHealth{
				{
					Project:             "proj-a",
					LastSuccessAt:       successAt,
					ConsecutiveFailures: 0,
					LastError:           "",
				},
			},
			cfg:           &hivesync.Config{APIURL: "https://api.example.com", AutoSync: true},
			status:        hivesync.SyncConfigStatus{Configured: true, AutoSync: true},
			wantReachable: true,
			wantAuthOK:    true,
			wantAutoSync:  true,
			wantLastError: "",
			wantConsec:    0,
		},
		{
			name: "degraded: consecutive failures > 0 but no auth error",
			rows: []db.SyncHealth{
				{
					Project:             "proj-b",
					LastSuccessAt:       successAt,
					LastFailureAt:       failureAt,
					ConsecutiveFailures: 3,
					LastError:           "connection refused",
				},
			},
			cfg:           &hivesync.Config{APIURL: "https://api.example.com", AutoSync: true},
			status:        hivesync.SyncConfigStatus{Configured: true, AutoSync: true},
			wantReachable: false, // non-auth network error → unreachable
			wantAuthOK:    true,
			wantAutoSync:  true,
			wantLastError: "connection refused",
			wantConsec:    3,
		},
		{
			name: "auth failed: last_error contains 401",
			rows: []db.SyncHealth{
				{
					Project:             "proj-c",
					LastSuccessAt:       successAt,
					LastFailureAt:       failureAt,
					ConsecutiveFailures: 5,
					LastError:           "sync failed (401): unauthorized",
				},
			},
			cfg:           &hivesync.Config{APIURL: "https://api.example.com", AutoSync: true},
			status:        hivesync.SyncConfigStatus{Configured: true, AutoSync: true},
			wantReachable: true, // auth errors do NOT mean unreachable
			wantAuthOK:    false,
			wantAutoSync:  true,
			wantLastError: "sync failed (401): unauthorized",
			wantConsec:    5,
		},
		{
			name: "auth failed: last_error contains unauthorized (case-insensitive)",
			rows: []db.SyncHealth{
				{
					Project:             "proj-d",
					LastFailureAt:       failureAt,
					ConsecutiveFailures: 2,
					LastError:           "UNAUTHORIZED access",
				},
			},
			cfg:           &hivesync.Config{APIURL: "https://api.example.com", AutoSync: false},
			status:        hivesync.SyncConfigStatus{Configured: true, AutoSync: false},
			wantReachable: true,
			wantAuthOK:    false,
			wantAutoSync:  false,
			wantLastError: "UNAUTHORIZED access",
			wantConsec:    2,
		},
		{
			name: "sync disabled: no config",
			rows: []db.SyncHealth{},
			cfg:           nil,
			status:        hivesync.SyncConfigStatus{Configured: false},
			wantReachable: false,
			wantAuthOK:    true,
			wantAutoSync:  false,
			wantLastError: "",
			wantConsec:    0,
		},
		{
			name: "aggregation: max consecutive failures across projects",
			rows: []db.SyncHealth{
				{
					Project:             "proj-x",
					LastFailureAt:       failureAt.Add(-10 * time.Minute),
					ConsecutiveFailures: 2,
					LastError:           "old error",
				},
				{
					Project:             "proj-y",
					LastFailureAt:       failureAt,
					ConsecutiveFailures: 7,
					LastError:           "newer error",
				},
			},
			cfg:           &hivesync.Config{APIURL: "https://api.example.com"},
			status:        hivesync.SyncConfigStatus{Configured: true},
			wantReachable: false, // non-auth network error
			wantAuthOK:    true,
			wantAutoSync:  false,
			wantLastError: "newer error", // from most-recent failure project
			wantConsec:    7,
		},
		{
			// R2-1: cfgErr must not mask a DB-derived auth error.
			// AuthOK must be derived from sync history BEFORE config loading.
			name: "auth error preserved when config load also fails",
			rows: []db.SyncHealth{
				{
					Project:             "proj-auth",
					LastFailureAt:       failureAt,
					ConsecutiveFailures: 2,
					LastError:           "401 unauthorized",
				},
			},
			cfg:           nil,
			status:        hivesync.SyncConfigStatus{Configured: false},
			loaderErr:     errors.New("config load error"),
			wantReachable: false,
			wantAuthOK:    false, // must be false — auth error from DB is not masked by cfgErr
			wantAutoSync:  false,
			wantLastError: "401 unauthorized; config unavailable: config load error",
			wantConsec:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubHealthStore{
				rows:             tt.rows,
				unsyncedMemories: tt.wantMemories,
				unsyncedPrompts:  tt.wantPrompts,
				unsyncedSessions: tt.wantSessions,
			}
			loader := &stubConfigLoader{
				cfg:    tt.cfg,
				status: tt.status,
				err:    tt.loaderErr,
			}

			svc := hivesync.NewHealthService(store, loader)
			summary, err := svc.Summary(context.Background())
			require.NoError(t, err)

			assert.Equal(t, tt.wantReachable, summary.Reachable, "Reachable")
			assert.Equal(t, tt.wantAuthOK, summary.AuthOK, "AuthOK")
			assert.Equal(t, tt.wantAutoSync, summary.AutoSync, "AutoSync")
			assert.Equal(t, tt.wantLastError, summary.LastError, "LastError")
			assert.Equal(t, tt.wantConsec, summary.ConsecutiveFailures, "ConsecutiveFailures")
			assert.Equal(t, tt.wantMemories, summary.UnsyncedMemories, "UnsyncedMemories")
			assert.Equal(t, tt.wantPrompts, summary.UnsyncedPrompts, "UnsyncedPrompts")
			assert.Equal(t, tt.wantSessions, summary.UnsyncedSessions, "UnsyncedSessions")
		})
	}
}

func TestHealthService_Summary_BackoffUntilNullSemantics(t *testing.T) {
	now := time.Now().UTC()
	futureBackoff := now.Add(5 * time.Minute)
	pastBackoff := now.Add(-5 * time.Minute)

	tests := []struct {
		name              string
		rows              []db.SyncHealth
		wantBackoffIsZero bool
	}{
		{
			name: "future backoff is preserved",
			rows: []db.SyncHealth{
				{Project: "p", BackoffUntil: futureBackoff, LastFailureAt: now.Add(-1 * time.Minute)},
			},
			wantBackoffIsZero: false,
		},
		{
			name: "past backoff is treated as zero",
			rows: []db.SyncHealth{
				{Project: "p", BackoffUntil: pastBackoff},
			},
			wantBackoffIsZero: true,
		},
		{
			name:              "no backoff rows → zero",
			rows:              []db.SyncHealth{},
			wantBackoffIsZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubHealthStore{rows: tt.rows}
			loader := &stubConfigLoader{}
			svc := hivesync.NewHealthService(store, loader)
			summary, err := svc.Summary(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.wantBackoffIsZero, summary.BackoffUntil.IsZero(), "BackoffUntil.IsZero()")
		})
	}
}

func TestHealthService_Summary_UnsyncedCounts(t *testing.T) {
	store := &stubHealthStore{
		unsyncedMemories: 5,
		unsyncedPrompts:  3,
		unsyncedSessions: 1,
	}
	loader := &stubConfigLoader{}
	svc := hivesync.NewHealthService(store, loader)
	summary, err := svc.Summary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, summary.UnsyncedMemories)
	assert.Equal(t, 3, summary.UnsyncedPrompts)
	assert.Equal(t, 1, summary.UnsyncedSessions)
}

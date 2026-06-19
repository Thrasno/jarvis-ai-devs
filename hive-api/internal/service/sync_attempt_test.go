package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSyncAttemptRepo struct {
	accepted   map[string]model.SyncAttemptLog
	duplicates map[string]bool
	summary    []model.SyncAttemptSummaryRecord
	deleteFn   func(context.Context, time.Time) (int64, error)
	cutoffs    []time.Time
}

func (r *fakeSyncAttemptRepo) UpsertBatch(ctx context.Context, attempts []model.SyncAttemptLog) (model.SyncAttemptStoreResult, error) {
	if r.accepted == nil {
		r.accepted = map[string]model.SyncAttemptLog{}
	}
	if r.duplicates == nil {
		r.duplicates = map[string]bool{}
	}
	result := model.SyncAttemptStoreResult{}
	for _, attempt := range attempts {
		key := attempt.DevID + ":" + attempt.AttemptID
		if r.duplicates[key] {
			result.DuplicateIDs = append(result.DuplicateIDs, attempt.AttemptID)
			continue
		}
		r.duplicates[key] = true
		r.accepted[key] = attempt
		result.AcceptedIDs = append(result.AcceptedIDs, attempt.AttemptID)
	}
	return result, nil
}

func (r *fakeSyncAttemptRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	r.cutoffs = append(r.cutoffs, cutoff)
	if r.deleteFn == nil {
		return 0, nil
	}
	return r.deleteFn(ctx, cutoff)
}

func (r *fakeSyncAttemptRepo) ListForSummary(ctx context.Context, filter model.SyncAttemptSummaryFilter) ([]model.SyncAttemptSummaryRecord, error) {
	return r.summary, nil
}

func TestSyncAttemptService_IngestValidationAndIdempotency(t *testing.T) {
	svc := NewSyncAttemptService(&fakeSyncAttemptRepo{})
	now := time.Now().UTC()

	t.Run("rejects missing dev_id", func(t *testing.T) {
		resp, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: []model.SyncAttemptPayload{{AttemptID: "attempt-missing-dev", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess}}})

		require.NoError(t, err)
		assert.Empty(t, resp.AcceptedIDs)
		require.Len(t, resp.Rejected, 1)
		assert.Equal(t, "attempt-missing-dev", resp.Rejected[0].AttemptID)
		assert.Contains(t, resp.Rejected[0].Error, "dev_id")
	})

	t.Run("rejects oversized batch", func(t *testing.T) {
		attempts := make([]model.SyncAttemptPayload, 101)
		for i := range attempts {
			attempts[i] = model.SyncAttemptPayload{AttemptID: "attempt", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess}
		}

		_, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: attempts})

		require.ErrorIs(t, err, ErrSyncAttemptBatchTooLarge)
	})

	t.Run("duplicate retry is successful and idempotent", func(t *testing.T) {
		repo := &fakeSyncAttemptRepo{}
		svc := NewSyncAttemptService(repo)
		payload := model.SyncAttemptPayload{AttemptID: "attempt-1", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess}

		first, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: []model.SyncAttemptPayload{payload}})
		require.NoError(t, err)
		second, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: []model.SyncAttemptPayload{payload}})

		require.NoError(t, err)
		assert.Equal(t, []string{"attempt-1"}, first.AcceptedIDs)
		assert.Empty(t, first.DuplicateIDs)
		assert.Empty(t, second.AcceptedIDs)
		assert.Equal(t, []string{"attempt-1"}, second.DuplicateIDs)
		assert.Len(t, repo.accepted, 1)
	})
}

func TestSyncAttemptService_ResponseDistinguishesAcceptedDuplicateRejected(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeSyncAttemptRepo{duplicates: map[string]bool{"dev@example.com:existing": true}}
	svc := NewSyncAttemptService(repo)

	resp, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: []model.SyncAttemptPayload{
		{AttemptID: "new", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "existing", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "invalid", DevID: "", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess},
	}})

	require.NoError(t, err)
	assert.Equal(t, []string{"new"}, resp.AcceptedIDs)
	assert.Equal(t, []string{"existing"}, resp.DuplicateIDs)
	require.Len(t, resp.Rejected, 1)
	assert.Equal(t, "invalid", resp.Rejected[0].AttemptID)
}

func TestSyncAttemptService_IngestRunsRetentionBestEffort(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeSyncAttemptRepo{deleteFn: func(ctx context.Context, cutoff time.Time) (int64, error) {
		return 0, errors.New("cleanup unavailable")
	}}
	svc := NewSyncAttemptService(repo)

	resp, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: []model.SyncAttemptPayload{
		{AttemptID: "attempt-retention", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess},
	}})

	require.NoError(t, err)
	assert.Equal(t, []string{"attempt-retention"}, resp.AcceptedIDs)
	require.Len(t, repo.cutoffs, 1, "ingestion runtime path must invoke 90-day retention")
	assert.WithinDuration(t, time.Now().UTC().AddDate(0, 0, -90), repo.cutoffs[0], 2*time.Second)
}

func TestSyncAttemptService_RetentionUsesNinetyDayCutoff(t *testing.T) {
	called := false
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	repo := &fakeSyncAttemptRepo{deleteFn: func(ctx context.Context, cutoff time.Time) (int64, error) {
		called = true
		assert.Equal(t, now.AddDate(0, 0, -90), cutoff)
		return 3, nil
	}}
	svc := NewSyncAttemptService(repo)

	deleted, err := svc.DeleteExpired(context.Background(), now)

	require.NoError(t, err)
	assert.True(t, called)
	assert.EqualValues(t, 3, deleted)
}

func TestSyncAttemptService_SanitizesServerSideErrorMessage(t *testing.T) {
	repo := &fakeSyncAttemptRepo{}
	svc := NewSyncAttemptService(repo)
	now := time.Now().UTC()
	long := strings.Repeat("x", 700)
	unsafe := "Authorization: Bearer secret-token\nCookie: session=abc\nrequest body: password=secret\npanic stack trace goroutine 1 [running]\n/home/andres/project/main.go:10 dev@example.com dev@example.com " + long

	resp, err := svc.Ingest(context.Background(), model.SyncAttemptIngestRequest{Attempts: []model.SyncAttemptPayload{{
		AttemptID:    "attempt-sanitize",
		DevID:        "dev@example.com",
		Project:      "jarvis-dev",
		Client:       "hive-daemon",
		StartedAt:    now,
		Outcome:      model.SyncAttemptOutcomeFailure,
		HTTPStatus:   intPtr(500),
		ErrorCode:    stringPtr("network_error"),
		ErrorMessage: stringPtr(unsafe),
	}}})

	require.NoError(t, err)
	assert.Equal(t, []string{"attempt-sanitize"}, resp.AcceptedIDs)
	stored := repo.accepted["dev@example.com:attempt-sanitize"]
	require.NotNil(t, stored.ErrorMessage)
	assert.NotContains(t, *stored.ErrorMessage, "Authorization")
	assert.NotContains(t, *stored.ErrorMessage, "Cookie")
	assert.NotContains(t, *stored.ErrorMessage, "request body")
	assert.NotContains(t, *stored.ErrorMessage, "goroutine")
	assert.NotContains(t, *stored.ErrorMessage, "/home/andres")
	assert.NotContains(t, *stored.ErrorMessage, "dev@example.com")
	assert.LessOrEqual(t, len([]rune(*stored.ErrorMessage)), 500)
	assert.Equal(t, 500, *stored.HTTPStatus)
	assert.Equal(t, "network_error", *stored.ErrorCode)
}

func intPtr(v int) *int { return &v }

func stringPtr(v string) *string { return &v }

func TestBuildSyncAttemptSummary_CalculatesWindowedKPIsAndGroups(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-2 * time.Hour)
	lastFailure := now.Add(-3 * time.Hour)
	records := []model.SyncAttemptSummaryRecord{
		{DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: lastSuccess, Outcome: model.SyncAttemptOutcomeSuccess},
		{DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: lastFailure, Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("network_error")},
		{DevID: "ben@example.com", Project: "other", Client: "hive-daemon", DaemonID: "daemon-b", StartedAt: now.Add(-48 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("timeout")},
		{DevID: "old@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-old", StartedAt: now.Add(-31 * 24 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("old")},
	}

	summary := BuildSyncAttemptSummary(records, model.SyncAttemptSummaryQuery{}, now)

	require.Len(t, summary.Windows, 3)
	twentyFourHours := summary.Windows[0]
	assert.Equal(t, "24h", twentyFourHours.Window)
	assert.Equal(t, 2, twentyFourHours.Total)
	assert.Equal(t, 1, twentyFourHours.Successes)
	assert.Equal(t, 1, twentyFourHours.Failures)
	assert.Equal(t, 0.5, twentyFourHours.FailureRate)
	require.NotNil(t, twentyFourHours.LastSuccessAt)
	assert.Equal(t, lastSuccess, *twentyFourHours.LastSuccessAt)
	require.NotNil(t, twentyFourHours.LastFailureAt)
	assert.Equal(t, lastFailure, *twentyFourHours.LastFailureAt)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "ada@example.com", Count: 2}}, twentyFourHours.ByDeveloper)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "jarvis-dev", Count: 2}}, twentyFourHours.ByProject)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "hive-daemon", Count: 2}}, twentyFourHours.ByClient)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "daemon-a", Count: 2}}, twentyFourHours.ByDaemon)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "failure", Count: 1}, {Key: "success", Count: 1}}, twentyFourHours.ByOutcome)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "network_error", Count: 1}}, twentyFourHours.ByErrorCode)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "network_error", Count: 1}}, twentyFourHours.TopErrors)

	sevenDays := summary.Windows[1]
	assert.Equal(t, "7d", sevenDays.Window)
	assert.Equal(t, 3, sevenDays.Total)
	assert.Equal(t, 2, sevenDays.Failures)
	assert.Equal(t, []model.SyncAttemptDimensionCount{{Key: "network_error", Count: 1}, {Key: "timeout", Count: 1}}, sevenDays.TopErrors)
}

func TestBuildSyncAttemptSummary_FiltersByWindowAndDimensions(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	records := []model.SyncAttemptSummaryRecord{
		{DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
		{DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: now.Add(-48 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("network_error")},
		{DevID: "ben@example.com", Project: "jarvis-dev", Client: "other-client", DaemonID: "daemon-b", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("timeout")},
	}

	summary := BuildSyncAttemptSummary(records, model.SyncAttemptSummaryQuery{Window: "24h", DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", Outcome: "success"}, now)

	require.Len(t, summary.Windows, 1)
	assert.Equal(t, "24h", summary.Windows[0].Window)
	assert.Equal(t, 1, summary.Windows[0].Total)
	assert.Equal(t, 1, summary.Windows[0].Successes)
	assert.Equal(t, 0, summary.Windows[0].Failures)
	assert.Equal(t, 0.0, summary.Windows[0].FailureRate)
}

func TestBuildSyncAttemptSummary_AbsenceOfAttemptsIsNotFailureAndHasNoLiveHealthFields(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	summary := BuildSyncAttemptSummary(nil, model.SyncAttemptSummaryQuery{Window: "24h"}, now)

	require.Len(t, summary.Windows, 1)
	assert.Equal(t, 0, summary.Windows[0].Total)
	assert.Equal(t, 0, summary.Windows[0].Failures)
	assert.Equal(t, 0.0, summary.Windows[0].FailureRate)
	encoded, err := json.Marshal(summary)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "healthy")
	assert.NotContains(t, string(encoded), "degraded")
	assert.NotContains(t, string(encoded), "unknown")
	assert.NotContains(t, string(encoded), "status")
}

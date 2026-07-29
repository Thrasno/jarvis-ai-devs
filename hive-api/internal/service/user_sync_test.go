package service

import (
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestUserSyncStatus(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	success := model.SyncAttemptOutcomeSuccess
	failure := model.SyncAttemptOutcomeFailure

	tests := []struct {
		name string
		row  model.UserSyncProjectionRow
		want model.UserSyncStatus
	}{
		{
			name: "latest successful attempt at inclusive boundary is recent",
			row: model.UserSyncProjectionRow{
				IsActive:             true,
				LatestEndedAt:        timePtr(now.Add(-24 * time.Hour)),
				LatestOutcome:        &success,
				LatestSuccessEndedAt: timePtr(now.Add(-24 * time.Hour)),
			},
			want: model.UserSyncStatusLast24h,
		},
		{
			name: "later failure overrides earlier success while retaining last sync",
			row: model.UserSyncProjectionRow{
				IsActive:             true,
				LatestEndedAt:        timePtr(now.Add(-time.Hour)),
				LatestOutcome:        &failure,
				LatestSuccessEndedAt: timePtr(now.Add(-2 * time.Hour)),
			},
			want: model.UserSyncStatusInactive,
		},
		{
			name: "future completion is inactive",
			row: model.UserSyncProjectionRow{
				IsActive:             true,
				LatestEndedAt:        timePtr(now.Add(time.Minute)),
				LatestOutcome:        &success,
				LatestSuccessEndedAt: timePtr(now.Add(-time.Hour)),
			},
			want: model.UserSyncStatusInactive,
		},
		{
			name: "incomplete attempt does not replace completed success",
			row: model.UserSyncProjectionRow{
				IsActive:             true,
				LatestEndedAt:        timePtr(now.Add(-time.Hour)),
				LatestOutcome:        &success,
				LatestSuccessEndedAt: timePtr(now.Add(-time.Hour)),
			},
			want: model.UserSyncStatusLast24h,
		},
		{
			name: "inactive account takes precedence over recent success",
			row: model.UserSyncProjectionRow{
				IsActive:             false,
				LatestEndedAt:        timePtr(now.Add(-time.Hour)),
				LatestOutcome:        &success,
				LatestSuccessEndedAt: timePtr(now.Add(-time.Hour)),
			},
			want: model.UserSyncStatusInactive,
		},
		{
			name: "no retained success is never",
			row:  model.UserSyncProjectionRow{IsActive: true},
			want: model.UserSyncStatusNever,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, userSyncStatus(tt.row, now))
		})
	}
}

func TestUserSyncLastSyncAtUsesLatestSuccessfulCompletion(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-2 * time.Hour)
	failure := model.SyncAttemptOutcomeFailure
	row := model.UserSyncProjectionRow{
		IsActive:             true,
		LatestEndedAt:        timePtr(now.Add(-time.Hour)),
		LatestOutcome:        &failure,
		LatestSuccessEndedAt: &lastSuccess,
	}

	assert.Equal(t, &lastSuccess, userSyncLastSyncAt(row))
}

func TestUserSyncStatus_OldRetainedSuccessRemainsInactiveWithExactLastSync(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-48 * time.Hour)
	success := model.SyncAttemptOutcomeSuccess
	row := model.UserSyncProjectionRow{
		IsActive:             true,
		LatestEndedAt:        &lastSuccess,
		LatestOutcome:        &success,
		LatestSuccessEndedAt: &lastSuccess,
	}

	assert.Equal(t, model.UserSyncStatusInactive, userSyncStatus(row, now))
	assert.Equal(t, &lastSuccess, userSyncLastSyncAt(row))
}

package service

import (
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

func userSyncStatus(row model.UserSyncProjectionRow, now time.Time) model.UserSyncStatus {
	if !row.IsActive {
		return model.UserSyncStatusInactive
	}
	if row.LatestSuccessEndedAt == nil {
		return model.UserSyncStatusNever
	}
	if row.LatestEndedAt != nil && row.LatestOutcome != nil &&
		*row.LatestOutcome == model.SyncAttemptOutcomeSuccess &&
		!row.LatestEndedAt.After(now) && !row.LatestEndedAt.Before(now.Add(-24*time.Hour)) {
		return model.UserSyncStatusLast24h
	}
	return model.UserSyncStatusInactive
}

func userSyncLastSyncAt(row model.UserSyncProjectionRow) *time.Time {
	return row.LatestSuccessEndedAt
}

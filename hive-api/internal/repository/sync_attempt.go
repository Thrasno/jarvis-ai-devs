package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

type SyncAttemptRepository interface {
	UpsertBatch(ctx context.Context, attempts []model.SyncAttemptLog) (model.SyncAttemptStoreResult, error)
	ListForSummary(ctx context.Context, filter model.SyncAttemptSummaryFilter) ([]model.SyncAttemptSummaryRecord, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)

	// SyncHealthByProject returns per-project sync health rows for the given window.
	SyncHealthByProject(ctx context.Context, window time.Duration) ([]model.ProjectSyncHealthRow, error)
	ProjectSyncHealth(ctx context.Context) (model.ProjectSyncHealthProjection, error)
	UserSyncProjection(ctx context.Context, now time.Time) (model.UserSyncProjection, error)
}

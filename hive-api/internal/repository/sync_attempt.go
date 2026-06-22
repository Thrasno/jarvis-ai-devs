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

	// DaemonHealth returns healthy and total daemon counts within the given windows.
	// healthy = daemons whose last attempt in healthyWindow was a success.
	// total = distinct daemons with any attempt in totalWindow.
	DaemonHealth(ctx context.Context, healthyWindow, totalWindow time.Duration) (healthy, total int, err error)

	// SyncHealthByProject returns per-project sync health rows for the given window.
	SyncHealthByProject(ctx context.Context, window time.Duration) ([]model.ProjectSyncHealthRow, error)
}

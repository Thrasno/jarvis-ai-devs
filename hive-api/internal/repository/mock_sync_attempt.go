package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

// MockSyncAttemptRepository is a test double for SyncAttemptRepository.
type MockSyncAttemptRepository struct {
	mock.Mock
}

var _ SyncAttemptRepository = (*MockSyncAttemptRepository)(nil)

func (m *MockSyncAttemptRepository) UpsertBatch(ctx context.Context, attempts []model.SyncAttemptLog) (model.SyncAttemptStoreResult, error) {
	args := m.Called(ctx, attempts)
	return args.Get(0).(model.SyncAttemptStoreResult), args.Error(1)
}

func (m *MockSyncAttemptRepository) ListForSummary(ctx context.Context, filter model.SyncAttemptSummaryFilter) ([]model.SyncAttemptSummaryRecord, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.SyncAttemptSummaryRecord), args.Error(1)
}

func (m *MockSyncAttemptRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	args := m.Called(ctx, cutoff)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockSyncAttemptRepository) SyncHealthByProject(ctx context.Context, window time.Duration) ([]model.ProjectSyncHealthRow, error) {
	args := m.Called(ctx, window)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.ProjectSyncHealthRow), args.Error(1)
}

func (m *MockSyncAttemptRepository) ProjectSyncHealth(ctx context.Context) (model.ProjectSyncHealthProjection, error) {
	args := m.Called(ctx)
	return args.Get(0).(model.ProjectSyncHealthProjection), args.Error(1)
}

func (m *MockSyncAttemptRepository) UserSyncProjection(ctx context.Context, now time.Time) (model.UserSyncProjection, error) {
	for _, call := range m.ExpectedCalls {
		if call.Method == "UserSyncProjection" {
			args := m.Called(ctx, now)
			return args.Get(0).(model.UserSyncProjection), args.Error(1)
		}
	}
	return model.UserSyncProjection{Rows: []model.UserSyncProjectionRow{}}, nil
}

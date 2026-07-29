package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAdminService_ListUsersAddsSyncContextWithTwoRepositoryCalls(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-24 * time.Hour)
	clockCalls := 0
	users := &repository.MockUserRepository{}
	syncs := &repository.MockSyncAttemptRepository{}
	memories := &repository.MockMemoryRepository{}
	audit := &repository.MockAuditRepository{}
	tx := repository.NewMockTxManager(users, audit)
	users.On("List", ctx).Return([]*model.User{
		{ID: "active", Username: "active", Email: "active@example.com", Level: model.LevelMember, IsActive: true},
		{ID: "inactive", Username: "inactive", Email: "inactive@example.com", Level: model.LevelViewer, IsActive: false},
	}, nil).Once()
	syncs.On("UserSyncProjection", ctx, now).Return(model.UserSyncProjection{Rows: []model.UserSyncProjectionRow{
		{PortalUserID: "active", IsActive: true, LatestEndedAt: &lastSuccess, LatestOutcome: outcomePtr(model.SyncAttemptOutcomeSuccess), LatestSuccessEndedAt: &lastSuccess},
		{PortalUserID: "inactive", IsActive: false, LatestEndedAt: &lastSuccess, LatestOutcome: outcomePtr(model.SyncAttemptOutcomeSuccess), LatestSuccessEndedAt: &lastSuccess},
	}}, nil).Once()

	svc := service.NewAdminService(users, memories, audit, tx, syncs, func() time.Time { clockCalls++; return now.Add(time.Duration(clockCalls-1) * time.Minute) })
	result, err := svc.ListUsers(ctx)

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, model.UserSyncStatusLast24h, result[0].SyncStatus)
	assert.Equal(t, &lastSuccess, result[0].LastSyncAt)
	assert.Equal(t, 1, clockCalls)
	assert.Equal(t, model.UserSyncStatusInactive, result[1].SyncStatus)
	assert.Equal(t, &lastSuccess, result[1].LastSyncAt)
	users.AssertExpectations(t)
	syncs.AssertExpectations(t)
	syncs.AssertNumberOfCalls(t, "UserSyncProjection", 1)
	users.AssertNumberOfCalls(t, "List", 1)
	memories.AssertNotCalled(t, mock.Anything)
}

func TestAdminService_ListUsersDegradesWhenSyncProjectionFails(t *testing.T) {
	ctx := context.Background()
	users, syncs := &repository.MockUserRepository{}, &repository.MockSyncAttemptRepository{}
	memories, audit := &repository.MockMemoryRepository{}, &repository.MockAuditRepository{}
	users.On("List", ctx).Return([]*model.User{{ID: "1", Username: "admin", IsActive: true}}, nil)
	syncs.On("UserSyncProjection", ctx, mock.Anything).Return(model.UserSyncProjection{}, errors.New("projection unavailable"))

	result, err := service.NewAdminService(users, memories, audit, repository.NewMockTxManager(users, audit), syncs).ListUsers(ctx)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, model.UserSyncStatusUnknown, result[0].SyncStatus)
	assert.True(t, result[0].IsActive)
}

func outcomePtr(value model.SyncAttemptOutcome) *model.SyncAttemptOutcome { return &value }

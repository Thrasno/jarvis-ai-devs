package main

// mocks_test.go — dobles de test para las interfaces de servicio.
// Mismo patrón que en el paquete handler, pero aquí en cmd/server.

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

type mockAuth struct{ mock.Mock }

func (m *mockAuth) Login(ctx context.Context, email, password string) (string, error) {
	args := m.Called(ctx, email, password)
	return args.String(0), args.Error(1)
}
func (m *mockAuth) ValidateToken(t string) (*model.Claims, error) {
	args := m.Called(t)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Claims), args.Error(1)
}
func (m *mockAuth) GetCurrentUser(ctx context.Context, userID string) (*model.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

type mockMemory struct{ mock.Mock }

func (m *mockMemory) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	args := m.Called(ctx, mem)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}
func (m *mockMemory) GetByID(ctx context.Context, id string) (*model.Memory, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}
func (m *mockMemory) List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.Memory), args.Get(1).(int64), args.Error(2)
}
func (m *mockMemory) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, int64, error) {
	args := m.Called(ctx, query, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.Memory), args.Get(1).(int64), args.Error(2)
}

type mockSync struct{ mock.Mock }

func (m *mockSync) Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	args := m.Called(ctx, req, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SyncResponse), args.Error(1)
}

func (m *mockSync) Sync(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	args := m.Called(ctx, req, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SyncResponse), args.Error(1)
}
func (m *mockSync) PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, limit int, memoriesCursor, sessionsCursor model.PullCursor) (*model.PullResult, error) {
	args := m.Called(ctx, project, since, excludeSyncIDs, limit, memoriesCursor, sessionsCursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.PullResult), args.Error(1)
}

type mockProject struct{ mock.Mock }

func (m *mockProject) List(ctx context.Context) (model.ProjectListResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(model.ProjectListResponse), args.Error(1)
}

type mockProjectGovernance struct{ mock.Mock }

func (m *mockProjectGovernance) BlockProject(ctx context.Context, actor model.AdminActor, project string, req model.ProjectBlockRequest) (model.ProjectBlockResponse, error) {
	args := m.Called(ctx, actor, project, req)
	return args.Get(0).(model.ProjectBlockResponse), args.Error(1)
}
func (m *mockProjectGovernance) Status(ctx context.Context, project string) (model.ProjectBlockStatusResponse, error) {
	args := m.Called(ctx, project)
	return args.Get(0).(model.ProjectBlockStatusResponse), args.Error(1)
}
func (m *mockProjectGovernance) Acknowledge(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error) {
	args := m.Called(ctx, ack)
	return args.Get(0).(model.ProjectBlockAck), args.Error(1)
}

type mockAdmin struct{ mock.Mock }

func (m *mockAdmin) ListUsers(ctx context.Context) ([]*model.User, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.User), args.Error(1)
}
func (m *mockAdmin) CreateUser(ctx context.Context, actor model.AdminActor, req model.CreateUserRequest) error {
	return m.Called(ctx, actor, req).Error(0)
}
func (m *mockAdmin) ResetTemporaryPassword(ctx context.Context, actor model.AdminActor, username string, req model.ResetTemporaryPasswordRequest) error {
	return m.Called(ctx, actor, username, req).Error(0)
}
func (m *mockAdmin) Activate(ctx context.Context, actor model.AdminActor, username string) error {
	return m.Called(ctx, actor, username).Error(0)
}
func (m *mockAdmin) SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error {
	return m.Called(ctx, actor, username, newLevel).Error(0)
}
func (m *mockAdmin) GrantAdmin(ctx context.Context, actor model.AdminActor, username string) error {
	return m.Called(ctx, actor, username).Error(0)
}
func (m *mockAdmin) Deactivate(ctx context.Context, actor model.AdminActor, username string) error {
	return m.Called(ctx, actor, username).Error(0)
}
func (m *mockAdmin) GetStats(ctx context.Context) (*model.AdminStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AdminStatsResponse), args.Error(1)
}
func (m *mockAdmin) ListAuditLogs(ctx context.Context, filter model.AuditFilter) (model.AuditListResponse, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(model.AuditListResponse), args.Error(1)
}

type mockOverview struct{ mock.Mock }

func (m *mockOverview) GetForLevel(ctx context.Context, level model.UserLevel) (*model.CapabilityOverviewResponse, error) {
	args := m.Called(ctx, level)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.CapabilityOverviewResponse), args.Error(1)
}

func (m *mockOverview) GetStats(ctx context.Context) (*model.OverviewStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OverviewStatsResponse), args.Error(1)
}

func (m *mockOverview) GetGrowth(ctx context.Context) (*model.OverviewGrowthResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OverviewGrowthResponse), args.Error(1)
}

type mockActivity struct{ mock.Mock }

func (m *mockActivity) List(ctx context.Context, query model.ActivityFeedQuery) (*model.ActivityFeedResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ActivityFeedResponse), args.Error(1)
}

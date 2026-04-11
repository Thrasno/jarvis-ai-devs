package main

// mocks_test.go — dobles de test para las interfaces de servicio.
// Mismo patrón que en el paquete handler, pero aquí en cmd/server.

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
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
func (m *mockMemory) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error) {
	args := m.Called(ctx, query, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Memory), args.Error(1)
}

type mockSync struct{ mock.Mock }

func (m *mockSync) Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	args := m.Called(ctx, req, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SyncResponse), args.Error(1)
}
func (m *mockSync) Pull(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error) {
	args := m.Called(ctx, project, since, excludeSyncIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Memory), args.Error(1)
}

type mockAdmin struct{ mock.Mock }

func (m *mockAdmin) ListUsers(ctx context.Context) ([]*model.User, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.User), args.Error(1)
}
func (m *mockAdmin) SetLevel(ctx context.Context, username string, newLevel model.UserLevel) error {
	return m.Called(ctx, username, newLevel).Error(0)
}
func (m *mockAdmin) GrantAdmin(ctx context.Context, username string) error {
	return m.Called(ctx, username).Error(0)
}
func (m *mockAdmin) Deactivate(ctx context.Context, username string) error {
	return m.Called(ctx, username).Error(0)
}
func (m *mockAdmin) GetStats(ctx context.Context) (*model.AdminStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AdminStatsResponse), args.Error(1)
}

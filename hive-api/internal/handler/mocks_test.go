package handler

// mocks_test.go contiene los dobles de test para los servicios que usan los handlers.
// Al estar en el paquete handler (package handler, no handler_test), los tests
// de este paquete pueden acceder a las estructuras internas del handler.
//
// Todos los mocks implementan las interfaces de service usando testify/mock.
// El patrón es idéntico al de los mocks de repositorio en el paquete repository.

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

// --- AuthService mock ---

type mockAuthSvc struct {
	mock.Mock
}

func (m *mockAuthSvc) Login(ctx context.Context, email, password string) (string, error) {
	args := m.Called(ctx, email, password)
	return args.String(0), args.Error(1)
}

func (m *mockAuthSvc) ValidateToken(tokenString string) (*model.Claims, error) {
	args := m.Called(tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Claims), args.Error(1)
}

func (m *mockAuthSvc) GetCurrentUser(ctx context.Context, userID string) (*model.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

// --- MemoryService mock ---

type mockMemorySvc struct {
	mock.Mock
}

func (m *mockMemorySvc) Create(ctx context.Context, mem *model.Memory) (*model.Memory, error) {
	args := m.Called(ctx, mem)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}

func (m *mockMemorySvc) GetByID(ctx context.Context, id string) (*model.Memory, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Memory), args.Error(1)
}

func (m *mockMemorySvc) List(ctx context.Context, filter model.MemoryFilter) ([]*model.Memory, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.Memory), args.Get(1).(int64), args.Error(2)
}

func (m *mockMemorySvc) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, error) {
	args := m.Called(ctx, query, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Memory), args.Error(1)
}

// --- SyncService mock ---

type mockSyncSvc struct {
	mock.Mock
}

func (m *mockSyncSvc) Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	args := m.Called(ctx, req, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SyncResponse), args.Error(1)
}

func (m *mockSyncSvc) Pull(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) ([]*model.Memory, error) {
	args := m.Called(ctx, project, since, excludeSyncIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Memory), args.Error(1)
}

// --- AdminService mock ---

type mockAdminSvc struct {
	mock.Mock
}

func (m *mockAdminSvc) ListUsers(ctx context.Context) ([]*model.User, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.User), args.Error(1)
}

func (m *mockAdminSvc) SetLevel(ctx context.Context, username string, newLevel model.UserLevel) error {
	args := m.Called(ctx, username, newLevel)
	return args.Error(0)
}

func (m *mockAdminSvc) GrantAdmin(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *mockAdminSvc) Deactivate(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *mockAdminSvc) GetStats(ctx context.Context) (*model.AdminStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AdminStatsResponse), args.Error(1)
}

// --- DBPinger mock ---

type mockDBPinger struct {
	mock.Mock
}

func (m *mockDBPinger) Ping(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

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

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
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

func (m *mockAuthSvc) LoginWithDevice(ctx context.Context, email, password string, device model.ProjectBlockAckSubject) (string, error) {
	args := m.Called(ctx, email, password, device)
	return args.String(0), args.Error(1)
}

func (m *mockAuthSvc) ValidateToken(_ context.Context, tokenString string) (*model.Claims, error) {
	args := m.Called(tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Claims), args.Error(1)
}

func (m *mockAuthSvc) GetCurrentUser(ctx context.Context, userID string) (*model.User, error) {
	for _, call := range m.ExpectedCalls {
		if call.Method == "GetCurrentUser" {
			args := m.Called(ctx, userID)
			if args.Get(0) == nil {
				return nil, args.Error(1)
			}
			return args.Get(0).(*model.User), args.Error(1)
		}
	}

	if userID != "" {
		return &model.User{ID: userID, Username: "adminuser", Level: model.LevelAdmin, IsActive: true}, nil
	}

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

func (m *mockMemorySvc) Search(ctx context.Context, query string, filter model.MemoryFilter) ([]*model.Memory, int64, error) {
	args := m.Called(ctx, query, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*model.Memory), args.Get(1).(int64), args.Error(2)
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

func (m *mockSyncSvc) Sync(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	for _, call := range m.ExpectedCalls {
		if call.Method == "Sync" {
			args := m.Called(ctx, req, userID)
			if args.Get(0) == nil {
				return nil, args.Error(1)
			}
			return args.Get(0).(*model.SyncResponse), args.Error(1)
		}
	}

	pushResp, err := m.Push(ctx, req, userID)
	if err != nil {
		return nil, err
	}
	var since time.Time
	if req.LastSync != nil {
		since = *req.LastSync
	}
	excludeIDs := make([]string, 0, len(req.Memories))
	for _, memory := range req.Memories {
		excludeIDs = append(excludeIDs, memory.SyncID)
	}
	var memoriesCursor, sessionsCursor model.PullCursor
	if req.PullCursor != nil {
		memoriesCursor = *req.PullCursor
	}
	if req.PullSessionCursor != nil {
		sessionsCursor = *req.PullSessionCursor
	}
	pullResult, err := m.PullAll(ctx, req.Project, since, excludeIDs, model.ClampPullLimit(req.PullLimit), memoriesCursor, sessionsCursor)
	if err != nil {
		return nil, err
	}
	pulledSessions := make([]model.SyncSessionResponse, 0, len(pullResult.Sessions))
	for _, session := range pullResult.Sessions {
		pulledSessions = append(pulledSessions, model.SyncSessionResponse{ID: session.ID, SyncID: session.SyncID, Project: session.Project, Directory: session.Directory, DevID: session.DevID, Client: session.Client, StartedAt: session.StartedAt, EndedAt: session.EndedAt, Summary: session.Summary})
	}
	pulled := pullResult.Memories
	if pulled == nil {
		pulled = []*model.Memory{}
	}
	return &model.SyncResponse{Pushed: pushResp.Pushed, Pulled: pulled, Conflicts: pushResp.Conflicts, PromptsPushed: pushResp.PromptsPushed, PulledSessions: pulledSessions, NextMutationCursor: pushResp.NextMutationCursor, PulledMutations: pushResp.PulledMutations, CompatibilityMode: pushResp.CompatibilityMode, PulledHasMore: pullResult.MemoriesHasMore, NextPullCursor: pullResult.NextPullCursor, PulledSessionsHasMore: pullResult.SessionsHasMore, NextSessionCursor: pullResult.NextSessionCursor}, nil
}

func (m *mockSyncSvc) PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, limit int, memoriesCursor, sessionsCursor model.PullCursor) (*model.PullResult, error) {
	args := m.Called(ctx, project, since, excludeSyncIDs, limit, memoriesCursor, sessionsCursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.PullResult), args.Error(1)
}

type mockSyncAttemptSvc struct {
	mock.Mock
	actor *model.SyncAttemptActor
}

type mockProjectGovernanceSvc struct {
	mock.Mock
}

func (m *mockProjectGovernanceSvc) BlockProject(ctx context.Context, actor model.AdminActor, project string, req model.ProjectBlockRequest) (model.ProjectBlockResponse, error) {
	args := m.Called(ctx, actor, project, req)
	return args.Get(0).(model.ProjectBlockResponse), args.Error(1)
}

func (m *mockProjectGovernanceSvc) Status(ctx context.Context, project string) (model.ProjectBlockStatusResponse, error) {
	args := m.Called(ctx, project)
	return args.Get(0).(model.ProjectBlockStatusResponse), args.Error(1)
}

func (m *mockProjectGovernanceSvc) Acknowledge(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error) {
	args := m.Called(ctx, ack)
	return args.Get(0).(model.ProjectBlockAck), args.Error(1)
}

func (m *mockProjectGovernanceSvc) Inbox(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error) {
	args := m.Called(ctx, subject)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.ProjectBlockCommand), args.Error(1)
}

func (m *mockProjectGovernanceSvc) QuarantineProgress(ctx context.Context, projectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error) {
	args := m.Called(ctx, projectKey, generation, after, limit)
	return args.Get(0).(model.QuarantineProgressResponse), args.Error(1)
}

func (m *mockProjectGovernanceSvc) ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error) {
	args := m.Called(ctx)
	return args.Get(0).([]model.QuarantineSummary), args.Error(1)
}

func (m *mockSyncAttemptSvc) Ingest(ctx context.Context, req model.SyncAttemptIngestRequest, actors ...model.SyncAttemptActor) (model.SyncAttemptIngestResponse, error) {
	if len(actors) > 0 {
		m.actor = &actors[0]
	}
	args := m.Called(ctx, req)
	return args.Get(0).(model.SyncAttemptIngestResponse), args.Error(1)
}

func (m *mockSyncAttemptSvc) Summary(ctx context.Context, query model.SyncAttemptSummaryQuery) (model.SyncAttemptSummaryResponse, error) {
	args := m.Called(ctx, query)
	return args.Get(0).(model.SyncAttemptSummaryResponse), args.Error(1)
}

type mockProjectSvc struct {
	mock.Mock
}

func (m *mockProjectSvc) List(ctx context.Context, health string) (model.ProjectListResponse, error) {
	args := m.Called(ctx, health)
	return args.Get(0).(model.ProjectListResponse), args.Error(1)
}

// --- AdminService mock ---

type mockAdminSvc struct {
	mock.Mock
}

func (m *mockAdminSvc) ListUsers(ctx context.Context) ([]model.AdminUserResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.AdminUserResponse), args.Error(1)
}

func (m *mockAdminSvc) CreateUser(ctx context.Context, actor model.AdminActor, req model.CreateUserRequest) error {
	args := m.Called(ctx, actor, req)
	return args.Error(0)
}

func (m *mockAdminSvc) ResetTemporaryPassword(ctx context.Context, actor model.AdminActor, username string, req model.ResetTemporaryPasswordRequest) error {
	args := m.Called(ctx, actor, username, req)
	return args.Error(0)
}

func (m *mockAdminSvc) Activate(ctx context.Context, actor model.AdminActor, username string) error {
	args := m.Called(ctx, actor, username)
	return args.Error(0)
}

func (m *mockAdminSvc) SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error {
	args := m.Called(ctx, actor, username, newLevel)
	return args.Error(0)
}

func (m *mockAdminSvc) GrantAdmin(ctx context.Context, actor model.AdminActor, username string) error {
	args := m.Called(ctx, actor, username)
	return args.Error(0)
}

func (m *mockAdminSvc) Deactivate(ctx context.Context, actor model.AdminActor, username string) error {
	args := m.Called(ctx, actor, username)
	return args.Error(0)
}

func (m *mockAdminSvc) GetStats(ctx context.Context) (*model.AdminStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AdminStatsResponse), args.Error(1)
}

func (m *mockAdminSvc) ListAuditLogs(ctx context.Context, filter model.AuditFilter) (model.AuditListResponse, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(model.AuditListResponse), args.Error(1)
}

// --- DBPinger mock ---

type mockDBPinger struct {
	mock.Mock
}

func (m *mockDBPinger) Ping(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

// --- OverviewService mock ---

type mockOverviewSvc struct {
	mock.Mock
}

func (m *mockOverviewSvc) GetForLevel(ctx context.Context, level model.UserLevel) (*model.CapabilityOverviewResponse, error) {
	args := m.Called(ctx, level)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.CapabilityOverviewResponse), args.Error(1)
}

func (m *mockOverviewSvc) GetStats(ctx context.Context) (*model.OverviewStatsResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OverviewStatsResponse), args.Error(1)
}

func (m *mockOverviewSvc) GetGrowth(ctx context.Context) (*model.OverviewGrowthResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OverviewGrowthResponse), args.Error(1)
}

type mockActivitySvc struct {
	mock.Mock
}

func (m *mockActivitySvc) List(ctx context.Context, query model.ActivityFeedQuery) (*model.ActivityFeedResponse, error) {
	args := m.Called(ctx, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ActivityFeedResponse), args.Error(1)
}

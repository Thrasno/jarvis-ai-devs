package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testAdminActor = model.AdminActor{UserID: "admin-1", Username: "adminuser"}

func newTestAdminService(t *testing.T) (service.AdminService, *repository.MockUserRepository, *repository.MockMemoryRepository, *repository.MockAuditRepository, *repository.MockTxManager) {
	t.Helper()
	mockUserRepo := &repository.MockUserRepository{}
	mockMemRepo := &repository.MockMemoryRepository{}
	mockAuditRepo := &repository.MockAuditRepository{}
	mockTx := repository.NewMockTxManager(mockUserRepo, mockAuditRepo)
	svc := service.NewAdminService(mockUserRepo, mockMemRepo, mockAuditRepo, mockTx)
	return svc, mockUserRepo, mockMemRepo, mockAuditRepo, mockTx
}

// --- Tests de SetLevel ---

// TestSetLevel_MemberToAdmin_Success verifica que ascender a admin funciona
// cuando hay menos de 3 admins.
func TestSetLevel_MemberToAdmin_Success(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-2", Username: "bob", Level: model.LevelMember}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(2, nil) // 2 admins actuales → hay cupo
	mockUserRepo.On("UpdateLevel", ctx, "user-2", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserLevelChange &&
			entry.Outcome == model.AuditOutcomeSuccess &&
			entry.ActorUserID != nil && *entry.ActorUserID == testAdminActor.UserID &&
			entry.Metadata["target_username"] == "bob" &&
			entry.Metadata["target_user_id"] == "user-2" &&
			entry.Metadata["old_level"] == string(model.LevelMember) &&
			entry.Metadata["new_level"] == string(model.LevelAdmin) &&
			entry.Metadata["actor_username"] == testAdminActor.Username &&
			entry.Metadata["email"] == nil
	})).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "bob", model.LevelAdmin)

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_AuditFailureRollsBackMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "user-2", Username: "bob", Email: "bob@example.com", Level: model.LevelMember}
	auditErr := errors.New("audit insert failed")

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-2", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)

	err := svc.SetLevel(ctx, testAdminActor, "bob", model.LevelAdmin)

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

// TestSetLevel_MaxAdminsReached verifica que el límite de 3 admins se aplica.
// Intentar ascender a un cuarto admin debe devolver ErrMaxAdminsReached.
func TestSetLevel_MaxAdminsReached(t *testing.T) {
	svc, mockUserRepo, _, _, _ := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-4", Username: "carol", Level: model.LevelMember}

	mockUserRepo.On("GetByUsername", ctx, "carol").Return(targetUser, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil) // ya hay 3 admins → límite alcanzado

	err := svc.SetLevel(ctx, testAdminActor, "carol", model.LevelAdmin)

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	// UpdateLevel NO debe llamarse — la operación fue rechazada antes
	mockUserRepo.AssertNotCalled(t, "UpdateLevel")
	mockUserRepo.AssertExpectations(t)
}

// TestSetLevel_AlreadyAdmin verifica que cambiar admin→admin no verifica el límite.
// Si el usuario ya es admin, no necesitamos contar porque no incrementamos el número.
func TestSetLevel_AlreadyAdmin_SkipsCheck(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	adminUser := &model.User{ID: "user-1", Username: "andres", Level: model.LevelAdmin}

	mockUserRepo.On("GetByUsername", ctx, "andres").Return(adminUser, nil)
	// CountAdmins NO debe llamarse — el usuario ya es admin, no hay que verificar el límite
	mockUserRepo.On("UpdateLevel", ctx, "user-1", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.Anything).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "andres", model.LevelAdmin)

	require.NoError(t, err)
	mockUserRepo.AssertNotCalled(t, "CountAdmins")
	mockUserRepo.AssertExpectations(t)
}

func TestSetLevel_ActiveAdminDowngradeAllowedWhenAnotherAdminRemains(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()

	adminUser := &model.User{ID: "user-1", Username: "andres", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "andres").Return(adminUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(2, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-1", model.LevelMember).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.Anything).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "andres", model.LevelMember)

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_ActiveAdminDowngradeRejectedWhenNoOtherAdminRemains(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	adminUser := &model.User{ID: "user-1", Username: "lastadmin", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "lastadmin").Return(adminUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)

	err := svc.SetLevel(ctx, testAdminActor, "lastadmin", model.LevelViewer)

	assert.ErrorIs(t, err, service.ErrInsufficientAdmins)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-1", model.LevelViewer)
	mockAuditRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
}

func TestSetLevel_ActiveAdminDowngradeLockFailureSkipsCountAndMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	lockErr := errors.New("lock failed")

	adminUser := &model.User{ID: "user-1", Username: "otheradmin", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "otheradmin").Return(adminUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(lockErr)

	err := svc.SetLevel(ctx, testAdminActor, "otheradmin", model.LevelMember)

	assert.ErrorIs(t, err, lockErr)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-1", model.LevelMember)
	mockAuditRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
}

func TestSetLevel_InactiveAdminDowngradeSkipsActiveAdminInvariant(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	adminUser := &model.User{ID: "user-1", Username: "inactiveadmin", Level: model.LevelAdmin, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "inactiveadmin").Return(adminUser, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-1", model.LevelMember).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.Anything).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "inactiveadmin", model.LevelMember)

	require.NoError(t, err)
	mockUserRepo.AssertNotCalled(t, "LockActiveAdminInvariant", ctx)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

// TestSetLevel_UserNotFound verifica que intentar cambiar el nivel de un usuario inexistente falla.
func TestSetLevel_UserNotFound(t *testing.T) {
	svc, mockUserRepo, _, _, _ := newTestAdminService(t)
	ctx := context.Background()

	mockUserRepo.On("GetByUsername", ctx, "noexiste").Return(nil, repository.ErrNotFound)

	err := svc.SetLevel(ctx, testAdminActor, "noexiste", model.LevelAdmin)

	assert.ErrorIs(t, err, repository.ErrNotFound)
	mockUserRepo.AssertExpectations(t)
}

// --- Tests de Deactivate ---

// TestDeactivate_Success verifica que desactivar un usuario funciona.
func TestDeactivate_Success(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-2", Username: "bob", IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil)
	mockUserRepo.On("Deactivate", ctx, "user-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserDeactivate &&
			entry.ActorUserID != nil && *entry.ActorUserID == testAdminActor.UserID &&
			entry.Metadata["target_username"] == "bob" &&
			entry.Metadata["target_user_id"] == "user-2" &&
			entry.Metadata["actor_username"] == testAdminActor.Username
	})).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "bob")

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestDeactivate_AuditFailureRollsBackMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "user-2", Username: "bob", IsActive: true}
	auditErr := errors.New("audit insert failed")

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil)
	mockUserRepo.On("Deactivate", ctx, "user-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)

	err := svc.Deactivate(ctx, testAdminActor, "bob")

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestDeactivate_ActiveAdminAllowedWhenAnotherAdminRemains(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "admin-2", Username: "otheradmin", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "otheradmin").Return(targetUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(2, nil)
	mockUserRepo.On("Deactivate", ctx, "admin-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserDeactivate &&
			entry.Metadata["target_username"] == "otheradmin" &&
			entry.Metadata["target_user_id"] == "admin-2"
	})).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "otheradmin")

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestDeactivate_ActiveAdminRejectedWhenNoOtherAdminRemains(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "admin-2", Username: "lastadmin", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "lastadmin").Return(targetUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)

	err := svc.Deactivate(ctx, testAdminActor, "lastadmin")

	assert.ErrorIs(t, err, service.ErrInsufficientAdmins)
	mockUserRepo.AssertNotCalled(t, "Deactivate", ctx, "admin-2")
	mockAuditRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
}

func TestDeactivate_ActiveAdminLockFailureSkipsCountAndMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "admin-2", Username: "otheradmin", Level: model.LevelAdmin, IsActive: true}
	lockErr := errors.New("lock failed")

	mockUserRepo.On("GetByUsername", ctx, "otheradmin").Return(targetUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(lockErr)

	err := svc.Deactivate(ctx, testAdminActor, "otheradmin")

	assert.ErrorIs(t, err, lockErr)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertNotCalled(t, "Deactivate", ctx, "admin-2")
	mockAuditRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
}

func TestDeactivate_ActiveNonAdminAllowedWithoutAdminCount(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "user-2", Username: "memberuser", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "memberuser").Return(targetUser, nil)
	mockUserRepo.On("Deactivate", ctx, "user-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "memberuser")

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertNotCalled(t, "LockActiveAdminInvariant", ctx)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

// TestDeactivate_AlreadyInactive verifica que desactivar un usuario ya inactivo es idempotente.
func TestDeactivate_AlreadyInactive(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	inactiveUser := &model.User{ID: "user-2", Username: "bob", IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(inactiveUser, nil)
	// Deactivate se llama igualmente — el repo es idempotente (UPDATE siempre funciona)
	mockUserRepo.On("Deactivate", ctx, "user-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.Anything).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "bob")

	require.NoError(t, err)
	mockUserRepo.AssertExpectations(t)
}

// --- Tests de ListUsers ---

func TestListUsers_Success(t *testing.T) {
	svc, mockUserRepo, _, _, _ := newTestAdminService(t)
	ctx := context.Background()

	users := []*model.User{
		{ID: "1", Username: "andres"},
		{ID: "2", Username: "bob"},
	}
	mockUserRepo.On("List", ctx).Return(users, nil)

	result, err := svc.ListUsers(ctx)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	mockUserRepo.AssertExpectations(t)
}

func TestListAuditLogs_NormalizesFilterAndReturnsStableResponse(t *testing.T) {
	svc, _, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	project := "jarvis-dev"
	filter := model.AuditFilter{Project: &project, Limit: 0, Offset: -10}
	expectedFilter := model.AuditFilter{Project: &project, Limit: model.DefaultAuditLimit, Offset: 0}

	mockAuditRepo.On("List", ctx, expectedFilter).Return(nil, nil)
	mockAuditRepo.On("Count", ctx, expectedFilter).Return(int64(0), nil)

	result, err := svc.ListAuditLogs(ctx, filter)

	require.NoError(t, err)
	assert.NotNil(t, result.AuditLogs)
	assert.Len(t, result.AuditLogs, 0)
	assert.Equal(t, int64(0), result.Total)
	assert.Equal(t, model.DefaultAuditLimit, result.Limit)
	assert.Equal(t, 0, result.Offset)
	mockAuditRepo.AssertExpectations(t)
}

func TestListAuditLogs_ReturnsEntriesAndTotalForAppliedFilters(t *testing.T) {
	svc, _, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	action := model.AuditActionSyncConflict
	filter := model.AuditFilter{Action: &action, Limit: 5, Offset: 10}
	entries := []*model.AuditEntry{{ID: "audit-1", Action: action, Metadata: model.AuditMetadata{}}}

	mockAuditRepo.On("List", ctx, filter).Return(entries, nil)
	mockAuditRepo.On("Count", ctx, filter).Return(int64(12), nil)

	result, err := svc.ListAuditLogs(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, entries, result.AuditLogs)
	assert.Equal(t, int64(12), result.Total)
	assert.Equal(t, 5, result.Limit)
	assert.Equal(t, 10, result.Offset)
	mockAuditRepo.AssertExpectations(t)
}

// --- Tests de GrantAdmin ---

func TestGrantAdmin_Success(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()

	member := &model.User{ID: "user-5", Username: "newadmin", Level: model.LevelMember}
	mockUserRepo.On("GetByUsername", ctx, "newadmin").Return(member, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-5", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserLevelChange &&
			entry.Metadata["target_username"] == "newadmin" &&
			entry.Metadata["target_user_id"] == "user-5" &&
			entry.Metadata["old_level"] == string(model.LevelMember) &&
			entry.Metadata["new_level"] == string(model.LevelAdmin)
	})).Return(nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "newadmin")
	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestGrantAdmin_AuditFailureRollsBackMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	member := &model.User{ID: "user-5", Username: "newadmin", Level: model.LevelMember}
	auditErr := errors.New("audit insert failed")

	mockUserRepo.On("GetByUsername", ctx, "newadmin").Return(member, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-5", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)

	err := svc.GrantAdmin(ctx, testAdminActor, "newadmin")

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestGrantAdmin_AlreadyAdmin_Idempotent(t *testing.T) {
	svc, mockUserRepo, _, _, mockTx := newTestAdminService(t)
	ctx := context.Background()

	admin := &model.User{ID: "user-1", Username: "existing", Level: model.LevelAdmin}
	mockUserRepo.On("GetByUsername", ctx, "existing").Return(admin, nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "existing")
	require.NoError(t, err)
	assert.False(t, mockTx.Committed)
	assert.False(t, mockTx.RolledBack)
	// No debe llamar ni CountAdmins ni UpdateLevel — es idempotente
	mockUserRepo.AssertNotCalled(t, "CountAdmins")
	mockUserRepo.AssertNotCalled(t, "UpdateLevel")
}

func TestGrantAdmin_MaxAdmins(t *testing.T) {
	svc, mockUserRepo, _, _, _ := newTestAdminService(t)
	ctx := context.Background()

	member := &model.User{ID: "user-6", Username: "blocked", Level: model.LevelMember}
	mockUserRepo.On("GetByUsername", ctx, "blocked").Return(member, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "blocked")
	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
}

// --- Tests de GetStats ---

func TestGetStats_Success(t *testing.T) {
	svc, mockUserRepo, mockMemRepo, _, _ := newTestAdminService(t)
	ctx := context.Background()

	users := []*model.User{
		{ID: "1", Level: model.LevelAdmin, IsActive: true},
		{ID: "2", Level: model.LevelMember, IsActive: true},
		{ID: "3", Level: model.LevelMember, IsActive: false},
	}
	byProject := []model.ProjectCount{
		{Project: "jarvis-dev", Count: 42},
	}
	mockUserRepo.On("List", ctx).Return(users, nil)
	mockMemRepo.On("Count", ctx, model.MemoryFilter{}).Return(int64(42), nil)
	mockMemRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return(byProject, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Users.Total)
	assert.Equal(t, 2, stats.Users.Active)
	assert.Equal(t, 1, stats.Users.ByLevel["admin"])
	assert.Equal(t, 2, stats.Users.ByLevel["member"])
	assert.Equal(t, int64(42), stats.Memories.Total)
	assert.NotNil(t, stats.Memories.ByProject)
	assert.Len(t, stats.Memories.ByProject, 1)
	assert.Equal(t, "jarvis-dev", stats.Memories.ByProject[0].Project)
	assert.NotNil(t, stats.Memories.ByCategory)
}

func TestGetStats_ByProjectError(t *testing.T) {
	svc, mockUserRepo, mockMemRepo, _, _ := newTestAdminService(t)
	ctx := context.Background()

	users := []*model.User{{ID: "1", Level: model.LevelAdmin, IsActive: true}}
	repoErr := errors.New("db error")
	mockUserRepo.On("List", ctx).Return(users, nil)
	mockMemRepo.On("Count", ctx, model.MemoryFilter{}).Return(int64(0), nil)
	mockMemRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return(nil, repoErr)

	_, err := svc.GetStats(ctx)
	assert.ErrorIs(t, err, repoErr)
}

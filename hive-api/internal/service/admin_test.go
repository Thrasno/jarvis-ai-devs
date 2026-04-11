package service_test

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdminService(t *testing.T) (service.AdminService, *repository.MockUserRepository, *repository.MockMemoryRepository) {
	t.Helper()
	mockUserRepo := &repository.MockUserRepository{}
	mockMemRepo := &repository.MockMemoryRepository{}
	svc := service.NewAdminService(mockUserRepo, mockMemRepo)
	return svc, mockUserRepo, mockMemRepo
}

// --- Tests de SetLevel ---

// TestSetLevel_MemberToAdmin_Success verifica que ascender a admin funciona
// cuando hay menos de 3 admins.
func TestSetLevel_MemberToAdmin_Success(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-2", Username: "bob", Level: model.LevelMember}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(2, nil) // 2 admins actuales → hay cupo
	mockUserRepo.On("UpdateLevel", ctx, "user-2", model.LevelAdmin).Return(nil)

	err := svc.SetLevel(ctx, "bob", model.LevelAdmin)

	require.NoError(t, err)
	mockUserRepo.AssertExpectations(t)
}

// TestSetLevel_MaxAdminsReached verifica que el límite de 3 admins se aplica.
// Intentar ascender a un cuarto admin debe devolver ErrMaxAdminsReached.
func TestSetLevel_MaxAdminsReached(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-4", Username: "carol", Level: model.LevelMember}

	mockUserRepo.On("GetByUsername", ctx, "carol").Return(targetUser, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil) // ya hay 3 admins → límite alcanzado

	err := svc.SetLevel(ctx, "carol", model.LevelAdmin)

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	// UpdateLevel NO debe llamarse — la operación fue rechazada antes
	mockUserRepo.AssertNotCalled(t, "UpdateLevel")
	mockUserRepo.AssertExpectations(t)
}

// TestSetLevel_AlreadyAdmin verifica que cambiar admin→admin no verifica el límite.
// Si el usuario ya es admin, no necesitamos contar porque no incrementamos el número.
func TestSetLevel_AlreadyAdmin_SkipsCheck(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	adminUser := &model.User{ID: "user-1", Username: "andres", Level: model.LevelAdmin}

	mockUserRepo.On("GetByUsername", ctx, "andres").Return(adminUser, nil)
	// CountAdmins NO debe llamarse — el usuario ya es admin, no hay que verificar el límite
	mockUserRepo.On("UpdateLevel", ctx, "user-1", model.LevelAdmin).Return(nil)

	err := svc.SetLevel(ctx, "andres", model.LevelAdmin)

	require.NoError(t, err)
	mockUserRepo.AssertNotCalled(t, "CountAdmins")
	mockUserRepo.AssertExpectations(t)
}

// TestSetLevel_Downgrade verifica que degradar admin→member no verifica el límite.
func TestSetLevel_Downgrade_SkipsCheck(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	adminUser := &model.User{ID: "user-1", Username: "andres", Level: model.LevelAdmin}

	mockUserRepo.On("GetByUsername", ctx, "andres").Return(adminUser, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-1", model.LevelMember).Return(nil)

	err := svc.SetLevel(ctx, "andres", model.LevelMember)

	require.NoError(t, err)
	mockUserRepo.AssertNotCalled(t, "CountAdmins")
	mockUserRepo.AssertExpectations(t)
}

// TestSetLevel_UserNotFound verifica que intentar cambiar el nivel de un usuario inexistente falla.
func TestSetLevel_UserNotFound(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	mockUserRepo.On("GetByUsername", ctx, "noexiste").Return(nil, repository.ErrNotFound)

	err := svc.SetLevel(ctx, "noexiste", model.LevelAdmin)

	assert.ErrorIs(t, err, repository.ErrNotFound)
	mockUserRepo.AssertExpectations(t)
}

// --- Tests de Deactivate ---

// TestDeactivate_Success verifica que desactivar un usuario funciona.
func TestDeactivate_Success(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-2", Username: "bob", IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil)
	mockUserRepo.On("Deactivate", ctx, "user-2").Return(nil)

	err := svc.Deactivate(ctx, "bob")

	require.NoError(t, err)
	mockUserRepo.AssertExpectations(t)
}

// TestDeactivate_AlreadyInactive verifica que desactivar un usuario ya inactivo es idempotente.
func TestDeactivate_AlreadyInactive(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	inactiveUser := &model.User{ID: "user-2", Username: "bob", IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(inactiveUser, nil)
	// Deactivate se llama igualmente — el repo es idempotente (UPDATE siempre funciona)
	mockUserRepo.On("Deactivate", ctx, "user-2").Return(nil)

	err := svc.Deactivate(ctx, "bob")

	require.NoError(t, err)
	mockUserRepo.AssertExpectations(t)
}

// --- Tests de ListUsers ---

func TestListUsers_Success(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
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

// --- Tests de GrantAdmin ---

func TestGrantAdmin_Success(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	member := &model.User{ID: "user-5", Username: "newadmin", Level: model.LevelMember}
	mockUserRepo.On("GetByUsername", ctx, "newadmin").Return(member, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-5", model.LevelAdmin).Return(nil)

	err := svc.GrantAdmin(ctx, "newadmin")
	require.NoError(t, err)
	mockUserRepo.AssertExpectations(t)
}

func TestGrantAdmin_AlreadyAdmin_Idempotent(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	admin := &model.User{ID: "user-1", Username: "existing", Level: model.LevelAdmin}
	mockUserRepo.On("GetByUsername", ctx, "existing").Return(admin, nil)

	err := svc.GrantAdmin(ctx, "existing")
	require.NoError(t, err)
	// No debe llamar ni CountAdmins ni UpdateLevel — es idempotente
	mockUserRepo.AssertNotCalled(t, "CountAdmins")
	mockUserRepo.AssertNotCalled(t, "UpdateLevel")
}

func TestGrantAdmin_MaxAdmins(t *testing.T) {
	svc, mockUserRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	member := &model.User{ID: "user-6", Username: "blocked", Level: model.LevelMember}
	mockUserRepo.On("GetByUsername", ctx, "blocked").Return(member, nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)

	err := svc.GrantAdmin(ctx, "blocked")
	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
}

// --- Tests de GetStats ---

func TestGetStats_Success(t *testing.T) {
	svc, mockUserRepo, mockMemRepo := newTestAdminService(t)
	ctx := context.Background()

	users := []*model.User{
		{ID: "1", Level: model.LevelAdmin, IsActive: true},
		{ID: "2", Level: model.LevelMember, IsActive: true},
		{ID: "3", Level: model.LevelMember, IsActive: false},
	}
	mockUserRepo.On("List", ctx).Return(users, nil)
	mockMemRepo.On("Count", ctx, model.MemoryFilter{}).Return(int64(42), nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Users.Total)
	assert.Equal(t, 2, stats.Users.Active)
	assert.Equal(t, 1, stats.Users.ByLevel["admin"])
	assert.Equal(t, 2, stats.Users.ByLevel["member"])
	assert.Equal(t, int64(42), stats.Memories.Total)
	assert.NotNil(t, stats.Memories.ByProject)
	assert.NotNil(t, stats.Memories.ByCategory)
}

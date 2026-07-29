package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
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

func metadataDoesNotContain(value string) func(*model.AuditEntry) bool {
	return func(entry *model.AuditEntry) bool {
		for _, metadataValue := range entry.Metadata {
			if strings.Contains(strings.ToLower(toString(metadataValue)), strings.ToLower(value)) {
				return false
			}
		}
		return true
	}
}

func failureAudit(action model.AuditAction, username string, checks ...func(*model.AuditEntry) bool) func(*model.AuditEntry) bool {
	return func(entry *model.AuditEntry) bool {
		if entry.Action != action || entry.Outcome != model.AuditOutcomeFailure || entry.Metadata["target_username"] != username {
			return false
		}
		for _, check := range checks {
			if !check(entry) {
				return false
			}
		}
		return true
	}
}

func failureReason(code string) func(*model.AuditEntry) bool {
	return func(entry *model.AuditEntry) bool {
		return entry.ReasonCode != nil && *entry.ReasonCode == code
	}
}

func bestEffortAuditContext(original context.Context) any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		return ctx != nil && ctx != original && ctx.Err() == nil
	})
}

func toString(value any) string {
	return fmt.Sprint(value)
}

func TestCreateUser_HashesTemporaryPasswordAndAuditsWithoutPlaintext(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	temporaryPassword := "temporary-secret"
	req := model.CreateUserRequest{Username: "newuser", Email: "newuser@example.com", Level: model.LevelMember, TemporaryPassword: temporaryPassword}
	created := &model.User{ID: "user-9", Username: "newuser", Email: "newuser@example.com", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("Create", ctx, mock.MatchedBy(func(user *model.User) bool {
		return user.Username == req.Username &&
			user.Email == req.Email &&
			user.Level == req.Level &&
			user.IsActive &&
			user.Password != temporaryPassword &&
			bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(temporaryPassword)) == nil
	})).Return(created, nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserCreate &&
			entry.Metadata["target_username"] == req.Username &&
			entry.Metadata["target_user_id"] == created.ID &&
			metadataDoesNotContain(temporaryPassword)(entry)
	})).Return(nil)

	err := svc.CreateUser(ctx, testAdminActor, req)

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestCreateUser_AuditFailureRollsBackMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	auditErr := errors.New("audit insert failed")
	req := model.CreateUserRequest{Username: "newuser", Email: "newuser@example.com", Level: model.LevelMember, TemporaryPassword: "temporary-secret"}
	created := &model.User{ID: "user-9", Username: "newuser", Email: "newuser@example.com", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("Create", ctx, mock.MatchedBy(func(user *model.User) bool {
		return user.Username == req.Username &&
			user.Email == req.Email &&
			user.Level == req.Level &&
			user.IsActive &&
			user.Password != req.TemporaryPassword
	})).Return(created, nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserCreate, req.Username))).Return(nil)

	err := svc.CreateUser(ctx, testAdminActor, req)

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestCreateUser_AdminRejectedWhenMaxAdminsReached(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	req := model.CreateUserRequest{Username: "newadmin", Email: "newadmin@example.com", Level: model.LevelAdmin, TemporaryPassword: "temporary-secret"}

	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserCreate &&
			entry.Outcome == model.AuditOutcomeFailure &&
			entry.Metadata["target_username"] == req.Username &&
			entry.Metadata["target_level"] == string(req.Level)
	})).Return(nil)

	err := svc.CreateUser(ctx, testAdminActor, req)

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	mockUserRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestCreateUser_FailureAuditedWithoutPlaintext(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	temporaryPassword := "temporary-secret"
	req := model.CreateUserRequest{Username: "existing", Email: "existing@example.com", Level: model.LevelMember, TemporaryPassword: temporaryPassword}

	mockUserRepo.On("Create", ctx, mock.MatchedBy(func(user *model.User) bool {
		return user.Username == req.Username && user.Email == req.Email && user.Password != temporaryPassword
	})).Return(nil, repository.ErrConflict)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserCreate &&
			entry.Outcome == model.AuditOutcomeFailure &&
			entry.Metadata["target_username"] == req.Username &&
			entry.Metadata["target_level"] == string(req.Level) &&
			metadataDoesNotContain(temporaryPassword)(entry)
	})).Return(nil)

	err := svc.CreateUser(ctx, testAdminActor, req)

	assert.ErrorIs(t, err, repository.ErrConflict)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestCreateUser_TemporaryPasswordOverBcryptByteLimitRejectedBeforeHash(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	req := model.CreateUserRequest{
		Username:          "newuser",
		Email:             "newuser@example.com",
		Level:             model.LevelMember,
		TemporaryPassword: strings.Repeat("ñ", 37),
	}

	err := svc.CreateUser(ctx, testAdminActor, req)

	assert.ErrorIs(t, err, model.ErrTemporaryPasswordTooLong)
	mockUserRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	mockAuditRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
}

func TestCreateUser_FailureAuditUsesFreshContextAndLogsInsertFailure(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	var logs bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := model.CreateUserRequest{Username: "newadmin", Email: "newadmin@example.com", Level: model.LevelAdmin, TemporaryPassword: "temporary-secret"}
	auditErr := errors.New("audit repository unavailable")

	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", bestEffortAuditContext(ctx), mock.MatchedBy(failureAudit(model.AuditActionUserCreate, req.Username,
		failureReason("max_admins_reached"),
		metadataDoesNotContain(req.TemporaryPassword),
	))).Return(auditErr)

	err := svc.CreateUser(ctx, testAdminActor, req)

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	output := logs.String()
	assert.Contains(t, output, "warn: failed to insert admin audit entry")
	assert.Contains(t, output, string(model.AuditActionUserCreate))
	assert.Contains(t, output, "max_admins_reached")
	assert.Contains(t, output, auditErr.Error())
	assert.NotContains(t, output, req.TemporaryPassword)
	assert.NotContains(t, output, req.Username)
	assert.NotContains(t, output, testAdminActor.Username)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestResetTemporaryPassword_TemporaryPasswordOverBcryptByteLimitRejectedBeforeHash(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	req := model.ResetTemporaryPasswordRequest{TemporaryPassword: strings.Repeat("ñ", 37)}

	err := svc.ResetTemporaryPassword(ctx, testAdminActor, "targetuser", req)

	assert.ErrorIs(t, err, model.ErrTemporaryPasswordTooLong)
	mockUserRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
	mockUserRepo.AssertNotCalled(t, "UpdatePassword", mock.Anything, mock.Anything, mock.Anything)
	mockAuditRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
}

func TestResetTemporaryPassword_HashesAndAuditsWithoutPlaintext(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	temporaryPassword := "reset-secret"
	target := &model.User{ID: "user-2", Username: "targetuser", Email: "target@example.com", Level: model.LevelMember, IsActive: true}
	req := model.ResetTemporaryPasswordRequest{TemporaryPassword: temporaryPassword}

	mockUserRepo.On("GetByUsername", ctx, "targetuser").Return(target, nil)
	mockUserRepo.On("UpdatePassword", ctx, "user-2", mock.MatchedBy(func(hash string) bool {
		return hash != temporaryPassword && bcrypt.CompareHashAndPassword([]byte(hash), []byte(temporaryPassword)) == nil
	})).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserPasswordReset &&
			entry.Metadata["target_username"] == "targetuser" &&
			entry.Metadata["target_user_id"] == "user-2" &&
			metadataDoesNotContain(temporaryPassword)(entry)
	})).Return(nil)

	err := svc.ResetTemporaryPassword(ctx, testAdminActor, "targetuser", req)

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestResetTemporaryPassword_AuditFailureRollsBackMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	auditErr := errors.New("audit insert failed")
	temporaryPassword := "reset-secret"
	target := &model.User{ID: "user-2", Username: "targetuser", Email: "target@example.com", Level: model.LevelMember, IsActive: true}
	req := model.ResetTemporaryPasswordRequest{TemporaryPassword: temporaryPassword}

	mockUserRepo.On("GetByUsername", ctx, "targetuser").Return(target, nil)
	mockUserRepo.On("UpdatePassword", ctx, "user-2", mock.MatchedBy(func(hash string) bool {
		return hash != temporaryPassword && bcrypt.CompareHashAndPassword([]byte(hash), []byte(temporaryPassword)) == nil
	})).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserPasswordReset, "targetuser"))).Return(nil)

	err := svc.ResetTemporaryPassword(ctx, testAdminActor, "targetuser", req)

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestResetTemporaryPassword_SelfRejectedByService(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	temporaryPassword := "reset-secret"

	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserPasswordReset, testAdminActor.Username,
		failureReason("self_admin_mutation"),
		metadataDoesNotContain(temporaryPassword),
	))).Return(nil)

	err := svc.ResetTemporaryPassword(ctx, testAdminActor, testAdminActor.Username, model.ResetTemporaryPasswordRequest{TemporaryPassword: temporaryPassword})

	assert.ErrorIs(t, err, service.ErrSelfAdminMutation)
	mockUserRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
	mockUserRepo.AssertNotCalled(t, "UpdatePassword", mock.Anything, mock.Anything, mock.Anything)
	mockAuditRepo.AssertExpectations(t)
}

func TestResetTemporaryPassword_FailureAuditedWithoutPlaintext(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	temporaryPassword := "reset-secret"
	target := &model.User{ID: "user-2", Username: "targetuser", Email: "target@example.com", Level: model.LevelMember, IsActive: true}
	repoErr := errors.New("password update failed")

	mockUserRepo.On("GetByUsername", ctx, "targetuser").Return(target, nil)
	mockUserRepo.On("UpdatePassword", ctx, "user-2", mock.MatchedBy(func(hash string) bool {
		return hash != temporaryPassword && bcrypt.CompareHashAndPassword([]byte(hash), []byte(temporaryPassword)) == nil
	})).Return(repoErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserPasswordReset &&
			entry.Outcome == model.AuditOutcomeFailure &&
			entry.Metadata["target_username"] == "targetuser" &&
			entry.Metadata["target_user_id"] == "user-2" &&
			metadataDoesNotContain(temporaryPassword)(entry)
	})).Return(nil)

	err := svc.ResetTemporaryPassword(ctx, testAdminActor, "targetuser", model.ResetTemporaryPasswordRequest{TemporaryPassword: temporaryPassword})

	assert.ErrorIs(t, err, repoErr)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestDeactivate_SelfRejectedByService(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserDeactivate, testAdminActor.Username,
		failureReason("self_admin_mutation"),
	))).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, testAdminActor.Username)

	assert.ErrorIs(t, err, service.ErrSelfAdminMutation)
	mockUserRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_SelfRejectedByService(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, testAdminActor.Username,
		failureReason("self_admin_mutation"),
		func(entry *model.AuditEntry) bool { return entry.Metadata["new_level"] == string(model.LevelMember) },
	))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, testAdminActor.Username, model.LevelMember)

	assert.ErrorIs(t, err, service.ErrSelfAdminMutation)
	mockUserRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
	mockAuditRepo.AssertExpectations(t)
}

func TestActivate_AdminRejectedWhenMaxAdminsReached(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	target := &model.User{ID: "admin-2", Username: "inactiveadmin", Level: model.LevelAdmin, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "inactiveadmin").Return(target, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserActivate &&
			entry.Outcome == model.AuditOutcomeFailure &&
			entry.Metadata["target_username"] == "inactiveadmin" &&
			entry.Metadata["target_user_id"] == "admin-2"
	})).Return(nil)

	err := svc.Activate(ctx, testAdminActor, "inactiveadmin")

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	mockUserRepo.AssertNotCalled(t, "Activate", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestActivate_SelfRejectedByService(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserActivate, testAdminActor.Username,
		failureReason("self_admin_mutation"),
	))).Return(nil)

	err := svc.Activate(ctx, testAdminActor, testAdminActor.Username)

	assert.ErrorIs(t, err, service.ErrSelfAdminMutation)
	mockUserRepo.AssertNotCalled(t, "GetByUsername", mock.Anything, mock.Anything)
	mockUserRepo.AssertNotCalled(t, "Activate", mock.Anything, mock.Anything)
	mockAuditRepo.AssertExpectations(t)
}

func TestActivate_RechecksInactiveUserUnderAdminInvariantLock(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	initial := &model.User{ID: "user-2", Username: "raceuser", Level: model.LevelMember, IsActive: false}
	rechecked := &model.User{ID: "user-2", Username: "raceuser", Level: model.LevelAdmin, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "raceuser").Return(initial, nil).Once()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil).Once()
	mockUserRepo.On("GetByUsername", ctx, "raceuser").Return(rechecked, nil).Once()
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserActivate &&
			entry.Outcome == model.AuditOutcomeFailure &&
			entry.Metadata["target_username"] == "raceuser" &&
			entry.Metadata["target_user_id"] == "user-2"
	})).Return(nil)

	err := svc.Activate(ctx, testAdminActor, "raceuser")

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	mockUserRepo.AssertNotCalled(t, "Activate", mock.Anything, mock.Anything)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestActivate_ActiveAdminAllowedWhenSeatAvailable(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	target := &model.User{ID: "admin-2", Username: "inactiveadmin", Level: model.LevelAdmin, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "inactiveadmin").Return(target, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(2, nil)
	mockUserRepo.On("Activate", ctx, "admin-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserActivate &&
			entry.Metadata["target_username"] == "inactiveadmin" &&
			entry.Metadata["target_user_id"] == "admin-2"
	})).Return(nil)

	err := svc.Activate(ctx, testAdminActor, "inactiveadmin")

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestActivate_AuditFailureRollsBackMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	auditErr := errors.New("audit insert failed")
	target := &model.User{ID: "user-2", Username: "inactiveuser", Level: model.LevelMember, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "inactiveuser").Return(target, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("Activate", ctx, "user-2").Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserActivate, "inactiveuser"))).Return(nil)

	err := svc.Activate(ctx, testAdminActor, "inactiveuser")

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

// --- Tests de SetLevel ---

// TestSetLevel_MemberToAdmin_Success verifica que ascender a admin funciona
// cuando hay menos de 3 admins.
func TestSetLevel_MemberToAdmin_Success(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-2", Username: "bob", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
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
	targetUser := &model.User{ID: "user-2", Username: "bob", Email: "bob@example.com", Level: model.LevelMember, IsActive: true}
	auditErr := errors.New("audit insert failed")

	mockUserRepo.On("GetByUsername", ctx, "bob").Return(targetUser, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-2", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "bob"))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "bob", model.LevelAdmin)

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_MaxAdminsReachedWritesFailureAudit(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "user-4", Username: "carol", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "carol").Return(targetUser, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "carol", func(entry *model.AuditEntry) bool {
		return entry.Metadata["target_user_id"] == "user-4" &&
			entry.Metadata["old_level"] == string(model.LevelMember) &&
			entry.Metadata["new_level"] == string(model.LevelAdmin) &&
			entry.ReasonCode != nil && *entry.ReasonCode == "max_admins_reached"
	}))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "carol", model.LevelAdmin)

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-4", model.LevelAdmin)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

// TestSetLevel_MaxAdminsReached verifica que el límite de 3 admins se aplica.
// Intentar ascender a un cuarto admin debe devolver ErrMaxAdminsReached.
func TestSetLevel_MaxAdminsReached(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	targetUser := &model.User{ID: "user-4", Username: "carol", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "carol").Return(targetUser, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil) // ya hay 3 admins → límite alcanzado
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "carol"))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "carol", model.LevelAdmin)

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	// UpdateLevel NO debe llamarse — la operación fue rechazada antes
	mockUserRepo.AssertNotCalled(t, "UpdateLevel")
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_ActiveMemberPromotionLockFailureSkipsCountAndMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	lockErr := errors.New("lock failed")
	targetUser := &model.User{ID: "user-4", Username: "carol", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "carol").Return(targetUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(lockErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "carol"))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "carol", model.LevelAdmin)

	assert.ErrorIs(t, err, lockErr)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-4", model.LevelAdmin)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_InactiveMemberPromotionSkipsActiveAdminSeatCheck(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "user-4", Username: "inactivecarol", Level: model.LevelMember, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "inactivecarol").Return(targetUser, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-4", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserLevelChange &&
			entry.Metadata["target_username"] == "inactivecarol" &&
			entry.Metadata["target_user_id"] == "user-4" &&
			entry.Metadata["old_level"] == string(model.LevelMember) &&
			entry.Metadata["new_level"] == string(model.LevelAdmin)
	})).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "inactivecarol", model.LevelAdmin)

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
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
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "lastadmin"))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "lastadmin", model.LevelViewer)

	assert.ErrorIs(t, err, service.ErrInsufficientAdmins)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-1", model.LevelViewer)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestSetLevel_ActiveAdminDowngradeLockFailureSkipsCountAndMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	lockErr := errors.New("lock failed")

	adminUser := &model.User{ID: "user-1", Username: "otheradmin", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "otheradmin").Return(adminUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(lockErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "otheradmin"))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "otheradmin", model.LevelMember)

	assert.ErrorIs(t, err, lockErr)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-1", model.LevelMember)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
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
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	mockUserRepo.On("GetByUsername", ctx, "noexiste").Return(nil, repository.ErrNotFound)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "noexiste"))).Return(nil)

	err := svc.SetLevel(ctx, testAdminActor, "noexiste", model.LevelAdmin)

	assert.ErrorIs(t, err, repository.ErrNotFound)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
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
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserDeactivate, "bob"))).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "bob")

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestDeactivate_InsufficientAdminsWritesFailureAudit(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "admin-2", Username: "lastadmin", Level: model.LevelAdmin, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "lastadmin").Return(targetUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserDeactivate, "lastadmin", func(entry *model.AuditEntry) bool {
		return entry.Metadata["target_user_id"] == "admin-2" && entry.ReasonCode != nil && *entry.ReasonCode == "insufficient_admins"
	}))).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "lastadmin")

	assert.ErrorIs(t, err, service.ErrInsufficientAdmins)
	mockUserRepo.AssertNotCalled(t, "Deactivate", ctx, "admin-2")
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
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserDeactivate, "lastadmin"))).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "lastadmin")

	assert.ErrorIs(t, err, service.ErrInsufficientAdmins)
	mockUserRepo.AssertNotCalled(t, "Deactivate", ctx, "admin-2")
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestDeactivate_ActiveAdminLockFailureSkipsCountAndMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	targetUser := &model.User{ID: "admin-2", Username: "otheradmin", Level: model.LevelAdmin, IsActive: true}
	lockErr := errors.New("lock failed")

	mockUserRepo.On("GetByUsername", ctx, "otheradmin").Return(targetUser, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(lockErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserDeactivate, "otheradmin"))).Return(nil)

	err := svc.Deactivate(ctx, testAdminActor, "otheradmin")

	assert.ErrorIs(t, err, lockErr)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertNotCalled(t, "Deactivate", ctx, "admin-2")
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
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

func TestListAuditLogs_SyncConflictRemainsHistoricalEventWithoutOpenConflictWording(t *testing.T) {
	svc, _, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	action := model.AuditActionSyncConflict
	filter := model.AuditFilter{Action: &action, Limit: 1}
	entries := []*model.AuditEntry{{ID: "audit-1", Action: action, Outcome: model.AuditOutcomeConflict, Metadata: model.AuditMetadata{}}}

	mockAuditRepo.On("List", ctx, filter).Return(entries, nil)
	mockAuditRepo.On("Count", ctx, filter).Return(int64(1), nil)

	result, err := svc.ListAuditLogs(ctx, filter)
	require.NoError(t, err)
	payload, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"action":"sync_conflict"`)
	assert.NotContains(t, string(payload), "open_conflicts")
	assert.NotContains(t, string(payload), "open conflict")
	mockAuditRepo.AssertExpectations(t)
}

// --- Tests de GrantAdmin ---

func TestGrantAdmin_Success(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()

	member := &model.User{ID: "user-5", Username: "newadmin", Level: model.LevelMember, IsActive: true}
	mockUserRepo.On("GetByUsername", ctx, "newadmin").Return(member, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
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
	member := &model.User{ID: "user-5", Username: "newadmin", Level: model.LevelMember, IsActive: true}
	auditErr := errors.New("audit insert failed")

	mockUserRepo.On("GetByUsername", ctx, "newadmin").Return(member, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(1, nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-5", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.AnythingOfType("*model.AuditEntry")).Return(auditErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "newadmin"))).Return(nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "newadmin")

	assert.ErrorIs(t, err, auditErr)
	assert.True(t, mockTx.RolledBack)
	assert.False(t, mockTx.Committed)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestGrantAdmin_MaxAdminsWritesFailureAudit(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	member := &model.User{ID: "user-6", Username: "blocked", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "blocked").Return(member, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "blocked", func(entry *model.AuditEntry) bool {
		return entry.Metadata["target_user_id"] == "user-6" &&
			entry.Metadata["old_level"] == string(model.LevelMember) &&
			entry.Metadata["new_level"] == string(model.LevelAdmin) &&
			entry.ReasonCode != nil && *entry.ReasonCode == "max_admins_reached"
	}))).Return(nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "blocked")

	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-6", model.LevelAdmin)
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
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()

	member := &model.User{ID: "user-6", Username: "blocked", Level: model.LevelMember, IsActive: true}
	mockUserRepo.On("GetByUsername", ctx, "blocked").Return(member, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("CountAdmins", ctx).Return(3, nil)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "blocked"))).Return(nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "blocked")
	assert.ErrorIs(t, err, service.ErrMaxAdminsReached)
	mockAuditRepo.AssertExpectations(t)
}

func TestGrantAdmin_ActiveMemberPromotionLockFailureSkipsCountAndMutation(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, _ := newTestAdminService(t)
	ctx := context.Background()
	lockErr := errors.New("lock failed")
	member := &model.User{ID: "user-6", Username: "blocked", Level: model.LevelMember, IsActive: true}

	mockUserRepo.On("GetByUsername", ctx, "blocked").Return(member, nil)
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(lockErr)
	mockAuditRepo.On("Insert", mock.Anything, mock.MatchedBy(failureAudit(model.AuditActionUserLevelChange, "blocked"))).Return(nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "blocked")

	assert.ErrorIs(t, err, lockErr)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertNotCalled(t, "UpdateLevel", ctx, "user-6", model.LevelAdmin)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
}

func TestGrantAdmin_InactiveMemberPromotionSkipsActiveAdminSeatCheck(t *testing.T) {
	svc, mockUserRepo, _, mockAuditRepo, mockTx := newTestAdminService(t)
	ctx := context.Background()
	member := &model.User{ID: "user-6", Username: "inactiveblocked", Level: model.LevelMember, IsActive: false}

	mockUserRepo.On("GetByUsername", ctx, "inactiveblocked").Return(member, nil).Twice()
	mockUserRepo.On("LockActiveAdminInvariant", ctx).Return(nil)
	mockUserRepo.On("UpdateLevel", ctx, "user-6", model.LevelAdmin).Return(nil)
	mockAuditRepo.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
		return entry.Action == model.AuditActionUserLevelChange &&
			entry.Metadata["target_username"] == "inactiveblocked" &&
			entry.Metadata["target_user_id"] == "user-6" &&
			entry.Metadata["old_level"] == string(model.LevelMember) &&
			entry.Metadata["new_level"] == string(model.LevelAdmin)
	})).Return(nil)

	err := svc.GrantAdmin(ctx, testAdminActor, "inactiveblocked")

	require.NoError(t, err)
	assert.True(t, mockTx.Committed)
	mockUserRepo.AssertNotCalled(t, "CountAdmins", ctx)
	mockUserRepo.AssertExpectations(t)
	mockAuditRepo.AssertExpectations(t)
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

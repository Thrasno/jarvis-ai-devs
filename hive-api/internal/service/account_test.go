package service_test

import (
	"context"
	"errors"
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

func newTestAccountService() (service.AccountService, *repository.MockUserRepository, *repository.MockAuditRepository, *repository.MockTxManager) {
	users := &repository.MockUserRepository{}
	audit := &repository.MockAuditRepository{}
	tx := repository.NewMockTxManager(users, audit)
	return service.NewAccountService(users, audit, tx), users, audit, tx
}

func TestAccountChangePassword(t *testing.T) {
	ctx := context.Background()
	current, next := "current-password", "next-password"
	user := &model.User{ID: "user-1", Username: "ada", Password: hashPassword(t, current), Level: model.LevelMember, IsActive: true, SecurityVersion: 4}

	t.Run("changes password, increments security version, and audits atomically", func(t *testing.T) {
		svc, users, audit, tx := newTestAccountService()
		users.On("GetByIDForUpdate", ctx, "user-1").Return(user, nil)
		users.On("UpdatePasswordAndIncrementSecurityVersion", ctx, "user-1", mock.MatchedBy(func(hash string) bool {
			cost, err := bcrypt.Cost([]byte(hash))
			return err == nil && cost == bcrypt.MinCost && bcrypt.CompareHashAndPassword([]byte(hash), []byte(next)) == nil
		})).Return(nil)
		audit.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
			return entry.Action == model.AuditActionUserPasswordChange && entry.Outcome == model.AuditOutcomeSuccess &&
				entry.ActorUserID != nil && *entry.ActorUserID == "user-1" && entry.Metadata["actor_username"] == "ada" &&
				!strings.Contains(entry.Metadata["actor_username"].(string), current) && !strings.Contains(entry.Metadata["actor_username"].(string), next)
		})).Return(nil)

		err := svc.ChangePassword(ctx, "user-1", current, next)

		require.NoError(t, err)
		assert.True(t, tx.Committed)
		users.AssertExpectations(t)
		audit.AssertExpectations(t)
	})

	t.Run("allows an active admin to change only their own password", func(t *testing.T) {
		svc, users, audit, tx := newTestAccountService()
		admin := &model.User{ID: "admin-1", Username: "grace", Password: hashPassword(t, current), Level: model.LevelAdmin, IsActive: true}
		users.On("GetByIDForUpdate", ctx, "admin-1").Return(admin, nil)
		users.On("UpdatePasswordAndIncrementSecurityVersion", ctx, "admin-1", mock.Anything).Return(nil)
		audit.On("Insert", ctx, mock.MatchedBy(func(entry *model.AuditEntry) bool {
			return entry.ActorUserID != nil && *entry.ActorUserID == "admin-1" && entry.Metadata["actor_username"] == "grace"
		})).Return(nil)

		err := svc.ChangePassword(ctx, "admin-1", current, next)

		require.NoError(t, err)
		assert.True(t, tx.Committed)
		users.AssertExpectations(t)
		audit.AssertExpectations(t)
	})

	for _, tt := range []struct {
		name    string
		user    *model.User
		current string
		want    error
	}{
		{"wrong current password", user, "other-password", service.ErrInvalidCurrentPassword},
		{"malformed stored bcrypt hash", &model.User{ID: "user-1", Password: "not-a-bcrypt-hash", IsActive: true}, current, service.ErrInvalidCurrentPassword},
		{"inactive user", &model.User{ID: "user-1", Password: user.Password}, current, service.ErrUserInactive},
		{"missing user", nil, current, repository.ErrNotFound},
	} {
		t.Run(tt.name+" does not mutate", func(t *testing.T) {
			svc, users, audit, tx := newTestAccountService()
			users.On("GetByIDForUpdate", ctx, "user-1").Return(tt.user, func() error {
				if tt.user == nil {
					return repository.ErrNotFound
				}
				return nil
			}())

			err := svc.ChangePassword(ctx, "user-1", tt.current, next)

			assert.ErrorIs(t, err, tt.want)
			assert.True(t, tx.RolledBack)
			users.AssertNotCalled(t, "UpdatePasswordAndIncrementSecurityVersion", mock.Anything, mock.Anything, mock.Anything)
			audit.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
		})
	}

	t.Run("rejects UTF-8 byte lengths outside bcrypt bounds before transaction", func(t *testing.T) {
		svc, users, audit, tx := newTestAccountService()
		err := svc.ChangePassword(ctx, "user-1", current, strings.Repeat("ñ", 37))
		assert.ErrorIs(t, err, service.ErrInvalidPasswordLength)
		assert.False(t, tx.Committed || tx.RolledBack)
		users.AssertNotCalled(t, "GetByIDForUpdate", mock.Anything, mock.Anything)
		audit.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	})

	t.Run("accepts inclusive UTF-8 byte boundaries", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			value string
		}{
			{"eight bytes", "12345678"},
			{"seventy-two bytes", strings.Repeat("a", 72)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				svc, users, audit, tx := newTestAccountService()
				users.On("GetByIDForUpdate", ctx, "user-1").Return(user, nil)
				users.On("UpdatePasswordAndIncrementSecurityVersion", ctx, "user-1", mock.Anything).Return(nil)
				audit.On("Insert", ctx, mock.Anything).Return(nil)

				err := svc.ChangePassword(ctx, "user-1", current, tt.value)

				require.NoError(t, err)
				assert.True(t, tx.Committed)
				users.AssertExpectations(t)
				audit.AssertExpectations(t)
			})
		}
	})

	t.Run("audit failure rolls back password mutation", func(t *testing.T) {
		svc, users, audit, tx := newTestAccountService()
		users.On("GetByIDForUpdate", ctx, "user-1").Return(user, nil)
		users.On("UpdatePasswordAndIncrementSecurityVersion", ctx, "user-1", mock.Anything).Return(nil)
		auditErr := errors.New("audit unavailable")
		audit.On("Insert", ctx, mock.Anything).Return(auditErr)

		err := svc.ChangePassword(ctx, "user-1", current, next)

		assert.ErrorIs(t, err, auditErr)
		assert.True(t, tx.RolledBack)
	})
}

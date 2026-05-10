package repository

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresTxManager_RollsBackUserMutationWhenAuditInsertFails(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	users := NewPostgresUserRepository(pool)
	txManager := NewPostgresTxManager(pool)

	tests := []struct {
		name       string
		user       *model.User
		mutate     func(context.Context, TxRepositories, string) error
		assertUser func(*testing.T, *model.User)
	}{
		{
			name: "level change is rolled back",
			user: &model.User{
				Username: "rollback-level",
				Email:    "rollback-level@example.com",
				Password: "hashedpass",
				Level:    model.LevelMember,
				IsActive: true,
			},
			mutate: func(ctx context.Context, repos TxRepositories, userID string) error {
				return repos.Users.UpdateLevel(ctx, userID, model.LevelAdmin)
			},
			assertUser: func(t *testing.T, got *model.User) {
				assert.Equal(t, model.LevelMember, got.Level, "failed audit insert must roll back level change")
				assert.True(t, got.IsActive)
			},
		},
		{
			name: "deactivation is rolled back",
			user: &model.User{
				Username: "rollback-deactivate",
				Email:    "rollback-deactivate@example.com",
				Password: "hashedpass",
				Level:    model.LevelMember,
				IsActive: true,
			},
			mutate: func(ctx context.Context, repos TxRepositories, userID string) error {
				return repos.Users.Deactivate(ctx, userID)
			},
			assertUser: func(t *testing.T, got *model.User) {
				assert.Equal(t, model.LevelMember, got.Level)
				assert.True(t, got.IsActive, "failed audit insert must roll back deactivation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := users.Create(ctx, tt.user)
			require.NoError(t, err)

			err = txManager.WithinTx(ctx, func(ctx context.Context, repos TxRepositories) error {
				require.NoError(t, tt.mutate(ctx, repos, created.ID))

				invalidActorID := "not-a-uuid"
				return repos.Audit.Insert(ctx, &model.AuditEntry{
					ActorUserID: &invalidActorID,
					Action:      model.AuditActionUserLevelChange,
					Outcome:     model.AuditOutcomeSuccess,
					EntryCount:  1,
					Metadata: model.AuditMetadata{
						"target_user_id": created.ID,
					},
				})
			})
			require.Error(t, err)

			got, err := users.GetByID(ctx, created.ID)
			require.NoError(t, err)
			tt.assertUser(t, got)
		})
	}
}

func TestPostgresTxManager_CommitsWhenCallbackSucceeds(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	txManager := NewPostgresTxManager(pool)

	err := txManager.WithinTx(context.Background(), func(ctx context.Context, repos TxRepositories) error {
		require.NotNil(t, repos.Users)
		require.NotNil(t, repos.Audit)
		return nil
	})

	require.NoError(t, err)
}

func TestPostgresTxManager_ReturnsBeginErrorWhenPoolIsClosed(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	txManager := NewPostgresTxManager(pool)
	pool.Close()

	err := txManager.WithinTx(context.Background(), func(ctx context.Context, repos TxRepositories) error {
		t.Fatal("callback should not run when beginning the transaction fails")
		return nil
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "begin transaction")
}

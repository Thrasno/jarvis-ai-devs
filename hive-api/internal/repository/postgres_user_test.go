package repository

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresUserRepository_Create verifica que podemos crear usuarios correctamente.
// Casos a probar:
// - Usuario válido → se crea con ID y timestamps generados
// - Usuario con username duplicado → error
func TestPostgresUserRepository_Create(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	tests := []struct {
		name    string
		user    *model.User
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid user creates successfully",
			user: &model.User{
				Username: "testuser",
				Email:    "test@example.com",
				Password: "hashedpassword",
				Level:    model.LevelMember,
				IsActive: true,
			},
			wantErr: false,
		},
		{
			name: "duplicate username returns error",
			user: &model.User{
				Username: "testuser", // mismo username que el anterior
				Email:    "other@example.com",
				Password: "hashedpass",
				Level:    model.LevelMember,
				IsActive: true,
			},
			wantErr: true,
			errMsg:  "unique",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.Create(ctx, tt.user)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			// Si no esperábamos error, verificar que se creó correctamente
			require.NoError(t, err)
			assert.NotEmpty(t, created.ID, "ID should be generated")
			assert.NotZero(t, created.CreatedAt, "CreatedAt should be set")
			assert.NotZero(t, created.UpdatedAt, "UpdatedAt should be set")
			assert.Equal(t, tt.user.Username, created.Username)
			assert.Equal(t, tt.user.Email, created.Email)
			assert.Equal(t, tt.user.Level, created.Level)
			assert.Equal(t, tt.user.IsActive, created.IsActive)
		})
	}
}

// TestPostgresUserRepository_GetByID verifica que podemos recuperar usuarios por ID.
// Casos a probar:
// - ID existente → usuario encontrado
// - ID no existente → error ErrNotFound
func TestPostgresUserRepository_GetByID(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	// Crear un usuario de prueba
	testUser := &model.User{
		Username: "getbyid_test",
		Email:    "getbyid@example.com",
		Password: "hashedpass",
		Level:    model.LevelMember,
		IsActive: true,
	}
	created, err := repo.Create(ctx, testUser)
	require.NoError(t, err)

	tests := []struct {
		name    string
		id      string
		wantErr bool
		wantMsg string
	}{
		{
			name:    "existing ID returns user",
			id:      created.ID,
			wantErr: false,
		},
		{
			name:    "non-existent ID returns ErrNotFound",
			id:      "00000000-0000-0000-0000-000000000000",
			wantErr: true,
			wantMsg: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.GetByID(ctx, tt.id)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrNotFound)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, created.ID, user.ID)
			assert.Equal(t, created.Username, user.Username)
			assert.Equal(t, created.Email, user.Email)
			assert.Equal(t, created.Level, user.Level)
		})
	}
}

// TestPostgresUserRepository_List verifica que podemos listar usuarios.
// Casos a probar:
// - Sin usuarios → lista vacía
// - Con usuarios → lista ordenada por created_at DESC
func TestPostgresUserRepository_List(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	tests := []struct {
		name      string
		setup     func(t *testing.T)
		wantCount int
	}{
		{
			name:      "empty database returns empty list",
			setup:     func(t *testing.T) {},
			wantCount: 0,
		},
		{
			name: "multiple users returned in DESC order",
			setup: func(t *testing.T) {
				// Crear 3 usuarios
				users := []*model.User{
					{Username: "user1", Email: "user1@example.com", Password: "pass1", Level: model.LevelMember, IsActive: true},
					{Username: "user2", Email: "user2@example.com", Password: "pass2", Level: model.LevelAdmin, IsActive: true},
					{Username: "user3", Email: "user3@example.com", Password: "pass3", Level: model.LevelViewer, IsActive: false},
				}
				for _, u := range users {
					_, err := repo.Create(ctx, u)
					require.NoError(t, err)
				}
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Truncar entre tests para aislamiento
			err := truncateTables(ctx, pool)
			require.NoError(t, err)

			tt.setup(t)

			users, err := repo.List(ctx)
			require.NoError(t, err)
			assert.Len(t, users, tt.wantCount)

			// Si hay usuarios, verificar orden DESC (último creado primero)
			if tt.wantCount > 1 {
				for i := 0; i < len(users)-1; i++ {
					assert.True(t, users[i].CreatedAt.After(users[i+1].CreatedAt) || users[i].CreatedAt.Equal(users[i+1].CreatedAt),
						"users should be ordered by created_at DESC")
				}
			}
		})
	}
}

// TestPostgresUserRepository_UpdateLevel verifica que podemos cambiar el nivel de un usuario.
// Casos a probar:
// - Upgrade: member → admin
// - Downgrade: admin → viewer
// - Usuario no existente → no error (UPDATE no falla si no encuentra, solo actualiza 0 rows)
func TestPostgresUserRepository_UpdateLevel(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	// Crear usuario de prueba
	testUser := &model.User{
		Username: "leveltest",
		Email:    "leveltest@example.com",
		Password: "hashedpass",
		Level:    model.LevelMember,
		IsActive: true,
	}
	created, err := repo.Create(ctx, testUser)
	require.NoError(t, err)

	tests := []struct {
		name      string
		userID    string
		newLevel  model.UserLevel
		wantErr   bool
		checkFunc func(t *testing.T)
	}{
		{
			name:     "upgrade member to admin",
			userID:   created.ID,
			newLevel: model.LevelAdmin,
			wantErr:  false,
			checkFunc: func(t *testing.T) {
				user, err := repo.GetByID(ctx, created.ID)
				require.NoError(t, err)
				assert.Equal(t, model.LevelAdmin, user.Level)
			},
		},
		{
			name:     "downgrade to viewer",
			userID:   created.ID,
			newLevel: model.LevelViewer,
			wantErr:  false,
			checkFunc: func(t *testing.T) {
				user, err := repo.GetByID(ctx, created.ID)
				require.NoError(t, err)
				assert.Equal(t, model.LevelViewer, user.Level)
			},
		},
		{
			name:     "non-existent user does not error",
			userID:   "00000000-0000-0000-0000-000000000000",
			newLevel: model.LevelAdmin,
			wantErr:  false,
			checkFunc: func(t *testing.T) {
				// No check needed — just verify no error
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.UpdateLevel(ctx, tt.userID, tt.newLevel)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.checkFunc(t)
		})
	}
}

// TestPostgresUserRepository_Deactivate verifica que podemos desactivar usuarios.
// Casos a probar:
// - Usuario activo → is_active = false
// - Usuario ya inactivo → sigue inactivo (idempotente)
func TestPostgresUserRepository_Deactivate(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	// Crear usuario activo
	testUser := &model.User{
		Username: "deactivatetest",
		Email:    "deactivate@example.com",
		Password: "hashedpass",
		Level:    model.LevelMember,
		IsActive: true,
	}
	created, err := repo.Create(ctx, testUser)
	require.NoError(t, err)

	tests := []struct {
		name    string
		userID  string
		wantErr bool
	}{
		{
			name:    "active user becomes inactive",
			userID:  created.ID,
			wantErr: false,
		},
		{
			name:    "already inactive user stays inactive (idempotent)",
			userID:  created.ID,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Deactivate(ctx, tt.userID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verificar que el usuario está inactivo
			user, err := repo.GetByID(ctx, tt.userID)
			require.NoError(t, err)
			assert.False(t, user.IsActive, "user should be inactive")
		})
	}
}

// TestPostgresUserRepository_GetByEmail verifica que podemos recuperar usuarios por email.
// Casos a probar:
// - Email existente → usuario encontrado
// - Email no existente → error ErrNotFound
func TestPostgresUserRepository_GetByEmail(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	// Crear un usuario de prueba
	testUser := &model.User{
		Username: "emailtest",
		Email:    "emailtest@example.com",
		Password: "hashedpass",
		Level:    model.LevelMember,
		IsActive: true,
	}
	created, err := repo.Create(ctx, testUser)
	require.NoError(t, err)

	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{
			name:    "existing email returns user",
			email:   created.Email,
			wantErr: false,
		},
		{
			name:    "non-existent email returns ErrNotFound",
			email:   "nonexistent@example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.GetByEmail(ctx, tt.email)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrNotFound)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, created.ID, user.ID)
			assert.Equal(t, created.Username, user.Username)
			assert.Equal(t, created.Email, user.Email)
			assert.Equal(t, created.Level, user.Level)
		})
	}
}

// TestPostgresUserRepository_GetByUsername verifica que podemos recuperar usuarios por username.
// Casos a probar:
// - Username existente → usuario encontrado
// - Username no existente → error ErrNotFound
func TestPostgresUserRepository_GetByUsername(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	// Crear un usuario de prueba
	testUser := &model.User{
		Username: "usernametest",
		Email:    "usernametest@example.com",
		Password: "hashedpass",
		Level:    model.LevelAdmin,
		IsActive: true,
	}
	created, err := repo.Create(ctx, testUser)
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{
			name:     "existing username returns user",
			username: created.Username,
			wantErr:  false,
		},
		{
			name:     "non-existent username returns ErrNotFound",
			username: "nonexistentuser",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := repo.GetByUsername(ctx, tt.username)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrNotFound)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, created.ID, user.ID)
			assert.Equal(t, created.Username, user.Username)
			assert.Equal(t, created.Email, user.Email)
			assert.Equal(t, created.Level, user.Level)
		})
	}
}

// TestPostgresUserRepository_CountAdmins verifies counting admin users.
func TestPostgresUserRepository_CountAdmins(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresUserRepository(pool)

	// Initially should have no admins
	count, err := repo.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "should start with 0 admins")

	// Create users with different levels
	admin1 := &model.User{
		Username: "admin1",
		Email:    "admin1@test.com",
		Password: "hashed1",
		Level:    model.LevelAdmin,
		IsActive: true,
	}
	admin2 := &model.User{
		Username: "admin2",
		Email:    "admin2@test.com",
		Password: "hashed2",
		Level:    model.LevelAdmin,
		IsActive: true,
	}
	member := &model.User{
		Username: "member",
		Email:    "member@test.com",
		Password: "hashed3",
		Level:    model.LevelMember,
		IsActive: true,
	}
	viewer := &model.User{
		Username: "viewer",
		Email:    "viewer@test.com",
		Password: "hashed4",
		Level:    model.LevelViewer,
		IsActive: true,
	}

	createdAdmin1, err := repo.Create(ctx, admin1)
	require.NoError(t, err)

	// After one admin
	count, err = repo.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have 1 admin")

	// Create more users
	_, err = repo.Create(ctx, admin2)
	require.NoError(t, err)
	_, err = repo.Create(ctx, member)
	require.NoError(t, err)
	_, err = repo.Create(ctx, viewer)
	require.NoError(t, err)

	// Should only count admins, not other levels
	count, err = repo.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "should have 2 admins, ignoring member and viewer")

	// Deactivate one admin - count should decrease (CountAdmins filters is_active = true)
	err = repo.Deactivate(ctx, createdAdmin1.ID)
	require.NoError(t, err)

	count, err = repo.CountAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should only count active admins")
}

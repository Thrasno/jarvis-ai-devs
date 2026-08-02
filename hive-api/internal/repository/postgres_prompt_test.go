package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startPostgresWithPrompts inicia un contenedor PostgreSQL y ejecuta AMBAS migraciones:
// la inicial (001) y la de prompts (002). Retorna el pool y una cleanup function.
func startPostgresWithPrompts(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	// Reusar el helper base que corre la migración inicial
	pool, cleanup := startPostgres(t)

	// Ejecutar la migración adicional para user_prompts usando el mismo path de producción
	err := RunMigrations(pool, migrations.UserPromptsSQL)
	require.NoError(t, err, "Failed to run user_prompts migration")

	err = RunMigrations(pool, migrations.ProjectBlocksSQL)
	require.NoError(t, err, "Failed to run project_blocks migration")
	err = RunMigrations(pool, migrations.QuarantineContractSQL)
	require.NoError(t, err, "Failed to run quarantine contract migration")

	return pool, cleanup
}

// TestPostgresPromptRepository_Upsert verifica el comportamiento de upsert de prompts.
//
// Casos:
//   - S1: nuevo prompt → se inserta, saved=true
//   - S3: mismo sync_id enviado dos veces → segunda inserción se ignora, saved=false
func TestPostgresPromptRepository_Upsert(t *testing.T) {
	pool, cleanup := startPostgresWithPrompts(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresPromptRepository(pool)

	now := time.Now()
	validSyncID := "550e8400-e29b-41d4-a716-446655440070"

	t.Run("S1: new prompt upsert returns saved=true", func(t *testing.T) {
		prompt := &model.Prompt{
			SyncID:    validSyncID,
			Project:   "project-a",
			Content:   "Respond concisely using bullet points",
			CreatedBy: "user@example.com",
			CreatedAt: now,
		}

		saved, err := repo.Upsert(ctx, prompt)

		require.NoError(t, err, "Upsert of new prompt should not error")
		assert.True(t, saved, "First upsert should return saved=true (new insert)")
	})

	t.Run("S3: idempotent re-upsert of same sync_id returns saved=false", func(t *testing.T) {
		// Same sync_id as the S1 subtest above — same row, ON CONFLICT DO NOTHING
		prompt := &model.Prompt{
			SyncID:    validSyncID,
			Project:   "project-a",
			Content:   "Respond concisely using bullet points",
			CreatedBy: "user@example.com",
			CreatedAt: now,
		}

		saved, err := repo.Upsert(ctx, prompt)

		require.NoError(t, err, "Idempotent re-upsert should not error")
		assert.False(t, saved, "Second upsert with same sync_id should return saved=false")
	})
}

// TestPostgresPromptRepository_ProjectIsolation verifica que los prompts de un proyecto
// no afectan a las operaciones de otro proyecto (S5: project isolation).
//
// La implementación solo tiene Upsert — verificamos isolación mediante la inserción
// en proyectos distintos con sync_ids distintos, confirmando que cada operación
// es independiente y que un sync_id de proyecto A no colisiona con proyecto B.
func TestPostgresPromptRepository_ProjectIsolation(t *testing.T) {
	pool, cleanup := startPostgresWithPrompts(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresPromptRepository(pool)

	now := time.Now()
	syncIDA := "550e8400-e29b-41d4-a716-446655440071"
	syncIDB := "550e8400-e29b-41d4-a716-446655440072"

	t.Run("S5: project A and B prompts are inserted independently", func(t *testing.T) {
		promptA := &model.Prompt{
			SyncID:    syncIDA,
			Project:   "project-a",
			Content:   "Project A specific instruction",
			CreatedBy: "user@example.com",
			CreatedAt: now,
		}
		promptB := &model.Prompt{
			SyncID:    syncIDB,
			Project:   "project-b",
			Content:   "Project B specific instruction",
			CreatedBy: "user@example.com",
			CreatedAt: now,
		}

		savedA, errA := repo.Upsert(ctx, promptA)
		savedB, errB := repo.Upsert(ctx, promptB)

		// Both should insert independently without interfering
		require.NoError(t, errA, "Project A prompt upsert should not error")
		require.NoError(t, errB, "Project B prompt upsert should not error")
		assert.True(t, savedA, "Project A prompt should be saved=true (new insert)")
		assert.True(t, savedB, "Project B prompt should be saved=true (new insert)")
	})

	t.Run("S5: re-upserting project A sync_id is idempotent (project B unaffected)", func(t *testing.T) {
		// Re-upsert project A — should be idempotent (saved=false), project B not touched
		promptA := &model.Prompt{
			SyncID:    syncIDA,
			Project:   "project-a",
			Content:   "Project A specific instruction",
			CreatedBy: "user@example.com",
			CreatedAt: now,
		}

		savedAAgain, errAAgain := repo.Upsert(ctx, promptA)

		require.NoError(t, errAAgain, "Re-upsert of project A prompt should not error")
		assert.False(t, savedAAgain, "Re-upsert of project A prompt should return saved=false")
	})
}

func TestPostgresPromptRepository_UpsertRejectsBlockedProject(t *testing.T) {
	pool, cleanup := startPostgresWithPrompts(t)
	defer cleanup()

	ctx := context.Background()
	promptRepo := NewPostgresPromptRepository(pool)
	blockRepo := NewPostgresProjectBlockRepository(pool)

	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Prompt Project",
		CanonicalProjectKey: "blocked-prompt-project",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "duplicate garbage project",
		Confirmation:        "blocked-prompt-project",
		ExportMarker:        "export-2026-07-06",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	saved, err := promptRepo.Upsert(ctx, &model.Prompt{
		SyncID:    "550e8400-e29b-41d4-a716-446655440073",
		Project:   "Blocked Prompt Project",
		Content:   "This prompt must not re-enter after block",
		CreatedBy: "user@example.com",
		CreatedAt: time.Now(),
	})

	require.ErrorIs(t, err, ErrProjectBlocked)
	assert.False(t, saved, "blocked prompt upsert should not report a saved row")

	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_prompts WHERE project = $1`, "Blocked Prompt Project").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "blocked prompt upsert must not create active prompt data")
}

package repository

import (
	"context"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresPromptRepository es la implementación de PromptRepository sobre PostgreSQL.
type postgresPromptRepository struct {
	db pgxQuerier
}

// NewPostgresPromptRepository crea la implementación real de PromptRepository.
func NewPostgresPromptRepository(pool *pgxpool.Pool) PromptRepository {
	return newPostgresPromptRepositoryWithQuerier(pool)
}

func newPostgresPromptRepositoryWithQuerier(db pgxQuerier) PromptRepository {
	return &postgresPromptRepository{db: db}
}

// Upsert inserta un nuevo prompt si el sync_id no existe todavía.
// Implementa el contrato de idempotencia: ON CONFLICT (sync_id) DO NOTHING.
//
//   - Si la fila se insertó → RowsAffected() == 1 → saved=true
//   - Si el sync_id ya existía → RowsAffected() == 0 → saved=false
//
// Este patrón es el mismo que se usa en ON CONFLICT DO NOTHING de PostgreSQL:
// no hay UPDATE, los prompts son inmutables una vez creados.
func (r *postgresPromptRepository) Upsert(ctx context.Context, p *model.Prompt) (bool, error) {
	if err := r.rejectBlockedProject(ctx, p.Project); err != nil {
		return false, err
	}
	const q = `
		INSERT INTO user_prompts (sync_id, project, content, created_by, created_at, synced_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (sync_id) DO NOTHING`

	tag, err := r.db.Exec(ctx, q,
		p.SyncID,
		p.Project,
		p.Content,
		p.CreatedBy,
		p.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("upsert prompt: %w", err)
	}

	// RowsAffected() == 1 → fila insertada (nueva)
	// RowsAffected() == 0 → conflicto en sync_id, DO NOTHING se activó
	return tag.RowsAffected() == 1, nil
}

func (r *postgresPromptRepository) rejectBlockedProject(ctx context.Context, project string) error {
	return checkProjectBlocked(ctx, r.db, project)
}

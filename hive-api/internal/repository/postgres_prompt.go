package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
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

// Upsert inserts a prompt when its sync_id is not stored yet.
//
// Prompts stay immutable: re-pushing the same sync_id changes no content, no
// author and no timestamps. `project` is the ONE exception, because the daemon
// is the sole authority on project identity: when its local migration rewrites
// a spelling ("Foo.Bar" -> "foo-bar") and re-pushes the row, the server has to
// accept that correction or the same prompt lives under two project names. See
// UpsertSession for the full rationale.
//
// The correction is conditional on the push NAMING the literal the row holds
// right now (`WHERE user_prompts.project = $6`, see Session.FromProject and
// UpsertSession). Without that precondition the conflict was not correcting a
// known row: it relocated whatever row that sync_id happened to hit, carrying it
// even out of a quarantine the request never names.
//
// Unlike the session correction, this one does NOT refresh synced_at: prompts
// have no incremental pull channel (nothing reads user_prompts), so there is no
// watermark to make them visible behind. If a ListPromptsSince is ever added,
// this branch has to start moving it — exactly as UpsertSession and
// applyReprojectMutation do — or the correction stays out of reach of the target
// project's pullers.
//
// The return value still means exactly "a new row was inserted". Since the
// conflict can run an UPDATE, RowsAffected() would be 1 for a correction too, so
// the distinction is made with `xmax = 0`, which is true only for the row this
// statement actually inserted. That keeps prompts_pushed from counting either an
// identical re-push or a correction. When the WHERE does not hold the conflict
// updates nothing and RETURNING yields no row: that is "not inserted", not an
// error.
func (r *postgresPromptRepository) Upsert(ctx context.Context, p *model.Prompt) (bool, error) {
	if err := r.rejectRelocationEnds(ctx, p); err != nil {
		return false, err
	}
	const q = `
		INSERT INTO user_prompts (sync_id, project, content, created_by, created_at, synced_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (sync_id) DO UPDATE
		  SET project = EXCLUDED.project
		  WHERE user_prompts.project = $6
		RETURNING (xmax = 0)`

	var inserted bool
	err := r.db.QueryRow(ctx, q,
		p.SyncID,
		p.Project,
		p.Content,
		p.CreatedBy,
		p.CreatedAt,
		relocationSource(p.Project, p.FromProject),
	).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("upsert prompt: %w", err)
	}

	return inserted, nil
}

func (r *postgresPromptRepository) rejectBlockedProject(ctx context.Context, project string) error {
	return checkProjectBlocked(ctx, r.db, project)
}

// rejectRelocationEnds is the prompt counterpart of the session check: a
// quarantine must hold on the project a row is moved OUT of, not only on the one
// it is written into.
func (r *postgresPromptRepository) rejectRelocationEnds(ctx context.Context, p *model.Prompt) error {
	if err := r.rejectBlockedProject(ctx, p.Project); err != nil {
		return err
	}
	if p.FromProject == "" || p.FromProject == p.Project {
		return nil
	}
	return r.rejectBlockedProject(ctx, p.FromProject)
}

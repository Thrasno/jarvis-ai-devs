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

// Upsert inserta un nuevo prompt si el sync_id no existe todavía.
//
// Los prompts siguen siendo inmutables: un re-push del mismo sync_id no cambia
// contenido, autor ni fechas. La ÚNICA excepción es `project`, porque el daemon
// es la autoridad sobre la identidad de proyecto: cuando su migración local
// reescribe la ortografía ("Foo.Bar" -> "foo-bar") y reenvía la fila, el
// servidor debe aceptar esa corrección o el mismo prompt queda bajo dos nombres
// de proyecto distintos. Ver UpsertSession para la nota completa.
//
// Esa corrección está condicionada a que el push NOMBRE el literal que la fila
// tiene ahora (`WHERE user_prompts.project = $6`, ver Session.FromProject y
// UpsertSession). Sin esa precondición el conflicto no corregía una fila
// conocida: reubicaba la fila que ese sync_id encontrase, sacándola incluso de
// una cuarentena que la request nunca nombra.
//
// A diferencia de la corrección de sesiones, esta NO refresca synced_at: los
// prompts no tienen canal de pull incremental (nada lee user_prompts), así que
// no hay watermark al que hacerlos visibles. Si algún día se añade un
// ListPromptsSince, esta rama debe empezar a moverlo — igual que UpsertSession y
// applyReprojectMutation — o la corrección quedará fuera del alcance de los
// pullers del proyecto destino.
//
// El valor devuelto sigue significando exactamente "se insertó una fila nueva":
// como el conflicto puede ejecutar un UPDATE, RowsAffected() valdría 1 también
// para una corrección, así que la distinción se hace con `xmax = 0`, que solo es
// verdadero para la fila realmente insertada por este statement. Así
// prompts_pushed no cuenta ni un re-push idéntico ni una corrección. Cuando el
// WHERE no se cumple el conflicto no actualiza nada y RETURNING no devuelve
// fila: eso es "no insertado", no un error.
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

package repository

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
)

// PromptRepository define las operaciones de base de datos para user-prompts.
//
// El contrato es deliberadamente simple: solo Upsert.
// Los prompts son inmutables una vez creados — si el daemon reenvía el mismo
// sync_id, el servidor lo ignora (ON CONFLICT DO NOTHING).
// No hay Update ni Delete en esta versión.
type PromptRepository interface {
	// Upsert inserta un nuevo prompt si el sync_id no existe todavía.
	// Si el sync_id ya existe (re-sync idempotente), no hace nada y devuelve saved=false.
	//
	// Retorna:
	//   - saved=true si se insertó una nueva fila
	//   - saved=false si el sync_id ya existía (conflicto ignorado)
	//   - error si hubo un fallo de base de datos
	Upsert(ctx context.Context, p *model.Prompt) (saved bool, err error)
}

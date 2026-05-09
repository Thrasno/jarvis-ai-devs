package model

import (
	"errors"
	"time"
)

// Session representa una sesión de trabajo en Hive.
//
// Los IDs de sesión tienen tres formas posibles:
//   - UUID generado por el daemon (sesiones explícitas via mem_session_start)
//   - 'manual-save-{project}' (sesión implícita: saves sin session_id explícito)
//   - 'legacy-pre-lifecycle-{project}' (sentinel de migración: memorias anteriores al lifecycle)
//
// Por eso la PK en Postgres es TEXT, no UUID.
type Session struct {
	// ID es la clave primaria. Puede ser un UUID, 'manual-save-{project}', o
	// 'legacy-pre-lifecycle-{project}'.
	ID string `json:"id" db:"id"`

	// SyncID es el UUID generado por el daemon antes del sync.
	// UNIQUE — garantiza idempotencia: el daemon puede reenviar sin duplicar.
	SyncID string `json:"sync_id" db:"sync_id"`

	Project   string `json:"project"   db:"project"`
	Directory string `json:"directory" db:"directory"`
	DevID     string `json:"dev_id"    db:"dev_id"`
	Client    string `json:"client"    db:"client"`

	StartedAt time.Time  `json:"started_at"          db:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"  db:"ended_at"`
	Summary   *string    `json:"summary,omitempty"   db:"summary"`
	SyncedAt  time.Time  `json:"synced_at"           db:"synced_at"`
	CreatedAt time.Time  `json:"created_at"          db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"          db:"updated_at"`
}

// Validate verifica que los campos mínimos requeridos estén presentes.
func (s *Session) Validate() error {
	if s.DevID == "" {
		return errors.New("dev_id is required")
	}
	if s.Client == "" {
		return errors.New("client is required")
	}
	if s.StartedAt.IsZero() {
		return errors.New("started_at must not be zero")
	}
	return nil
}

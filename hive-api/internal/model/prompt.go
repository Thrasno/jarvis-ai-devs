package model

import "time"

// Prompt representa un user-prompt sincronizado desde el daemon.
// Los prompts son instrucciones de usuario que el daemon almacena localmente
// y sincroniza al servidor para compartirlas entre sesiones y dispositivos.
type Prompt struct {
	ID      string `json:"id"`
	SyncID  string `json:"sync_id"`
	Project string `json:"project"`

	// FromProject is the same write-side precondition Session.FromProject
	// documents: the sync_id conflict branch moves `project` only when the
	// stored value equals it. It is never stored and never read back.
	FromProject string `json:"-"`

	Content   string     `json:"content"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	SyncedAt  *time.Time `json:"synced_at"`
}

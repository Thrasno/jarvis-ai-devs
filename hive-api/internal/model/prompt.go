package model

import "time"

// Prompt representa un user-prompt sincronizado desde el daemon.
// Los prompts son instrucciones de usuario que el daemon almacena localmente
// y sincroniza al servidor para compartirlas entre sesiones y dispositivos.
type Prompt struct {
	ID        string     `json:"id"`
	SyncID    string     `json:"sync_id"`
	Project   string     `json:"project"`
	Content   string     `json:"content"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	SyncedAt  *time.Time `json:"synced_at"`
}

package model

import "time"

// ErrorResponse es el envelope de error estándar de la API.
// TODOS los errores de la API devuelven este formato — nunca texto plano.
// Ejemplo de JSON: {"error": "invalid credentials"}
//
// Tener un formato de error consistente es crítico para los clientes
// (el daemon, apps frontend) — saben exactamente dónde está el mensaje.
type ErrorResponse struct {
	Error string `json:"error"`
}

// LoginResponse es la respuesta del POST /auth/login.
// Devuelve el token JWT y los datos básicos del usuario (sin password).
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserResponse `json:"user"`
}

// UserResponse es la representación pública de un usuario.
// Omite campos internos como Password (que ya tiene json:"-" en User,
// pero aquí lo hacemos explícito con un struct dedicado).
type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Level     UserLevel `json:"level"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// SyncResponse es la respuesta del POST /sync.
// Resume cuántas memorias se procesaron y devuelve las que el cliente no tenía.
type SyncResponse struct {
	// Pushed: cuántas memorias del cliente se guardaron (nuevas o actualizadas).
	Pushed int `json:"pushed"`

	// Pulled: memorias del servidor que el cliente no tenía todavía.
	// Puede ser un slice vacío [], nunca null.
	Pulled []*Memory `json:"pulled"`

	// Conflicts: memorias que el cliente intentó actualizar pero el servidor
	// tenía una versión más reciente (last-write-wins, servidor ganó).
	Conflicts int `json:"conflicts"`
}

// ListMemoriesResponse es la respuesta del GET /memories.
// Incluye los datos de paginación para que el cliente sepa cuántas páginas hay.
type ListMemoriesResponse struct {
	Memories []*Memory `json:"memories"`
	Total    int64     `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// SearchResponse es la respuesta del GET /memories/search.
type SearchResponse struct {
	Memories []*Memory `json:"memories"`
	Total    int64     `json:"total"`
	Query    string    `json:"query"`
	Limit    int       `json:"limit"`
}

// HealthResponse es la respuesta del GET /health.
type HealthResponse struct {
	Status  string `json:"status"`  // "ok" o "degraded"
	DB      string `json:"db"`      // "connected" o "unreachable"
	Version string `json:"version"` // hash del commit o tag de build
}

// AdminStatsResponse es la respuesta del GET /admin/stats.
type AdminStatsResponse struct {
	Users    UserStats    `json:"users"`
	Memories MemoryStats  `json:"memories"`
}

// UserStats agrupa las estadísticas de usuarios.
type UserStats struct {
	Total   int            `json:"total"`
	Active  int            `json:"active"`
	ByLevel map[string]int `json:"by_level"`
}

// MemoryStats agrupa las estadísticas de memorias.
type MemoryStats struct {
	Total        int64          `json:"total"`
	ByProject    []ProjectCount `json:"by_project"`
	ByCategory   []CategoryCount `json:"by_category"`
	LastSyncedAt *time.Time     `json:"last_synced_at"` // puntero: puede ser null si no hay memorias
}

// ProjectCount es un par proyecto → número de memorias.
type ProjectCount struct {
	Project string `json:"project"`
	Count   int64  `json:"count"`
}

// CategoryCount es un par categoría → número de memorias.
type CategoryCount struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

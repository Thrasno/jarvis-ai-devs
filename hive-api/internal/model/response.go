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
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
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

// SyncSessionPayload (response side) se usa en pulled_sessions.
// Reutiliza los campos de SyncSessionPayload del request (mismo wire format).
type SyncSessionResponse = SyncSessionPayload

// PullResult agrupa sesiones y memorias devueltas por un pull incremental.
// Las sesiones van ANTES que las memorias — el daemon receptor las inserta primero
// para satisfacer la FK memories.session_id → sessions(id).
type PullResult struct {
	Sessions []*Session
	Memories []*Memory
}

// SyncResponse es la respuesta del POST /sync.
// Resume cuántas memorias y prompts se procesaron y devuelve las que el cliente no tenía.
type SyncResponse struct {
	// Pushed: cuántas memorias del cliente se guardaron (nuevas o actualizadas).
	Pushed int `json:"pushed"`

	// Pulled: memorias del servidor que el cliente no tenía todavía.
	// Puede ser un slice vacío [], nunca null.
	Pulled []*Memory `json:"pulled"`

	// Conflicts: memorias que el cliente intentó actualizar pero el servidor
	// tenía una versión más reciente (last-write-wins, servidor ganó).
	Conflicts int `json:"conflicts"`

	// PromptsPushed: cuántos user-prompts se insertaron en esta sincronización.
	// Es 0 cuando el daemon no envía prompts (S9: backward-compat con daemons viejos).
	// Refleja el conteo real de upserts exitosos (S11).
	PromptsPushed int `json:"prompts_pushed"`

	// PulledSessions: sesiones del servidor que el cliente no tenía.
	// Procesadas por el daemon ANTES de las pulled memories para satisfacer la FK.
	PulledSessions []SyncSessionResponse `json:"pulled_sessions,omitempty"`

	NextMutationCursor *MutationCursor    `json:"next_mutation_cursor,omitempty"`
	PulledMutations    []MutationEnvelope `json:"pulled_mutations,omitempty"`
	CompatibilityMode  string             `json:"compatibility_mode,omitempty"`
}

const MutationProtocolVersion = 2

const CompatibilityModeLegacy = "legacy-row-state"

const CompatibilityModeMutationV2 = "mutation-sync-v2"

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
	Offset   int       `json:"offset"`
}

// HealthResponse es la respuesta del GET /health.
type HealthResponse struct {
	Status  string `json:"status"`  // "ok" o "degraded"
	DB      string `json:"db"`      // "connected" o "unreachable"
	Version string `json:"version"` // hash del commit o tag de build
}

// AdminStatsResponse es la respuesta del GET /admin/stats.
type AdminStatsResponse struct {
	Users    UserStats   `json:"users"`
	Memories MemoryStats `json:"memories"`
}

// UserStats agrupa las estadísticas de usuarios.
type UserStats struct {
	Total   int            `json:"total"`
	Active  int            `json:"active"`
	ByLevel map[string]int `json:"by_level"`
}

// MemoryStats agrupa las estadísticas de memorias.
type MemoryStats struct {
	Total        int64           `json:"total"`
	ByProject    []ProjectCount  `json:"by_project"`
	ByCategory   []CategoryCount `json:"by_category"`
	LastSyncedAt *time.Time      `json:"last_synced_at"` // puntero: puede ser null si no hay memorias
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

const (
	ProjectSyncHealthHealthy  = "healthy"
	ProjectSyncHealthDegraded = "degraded"
	ProjectSyncHealthUnknown  = "unknown"
)

type ProjectListResponse struct {
	Projects []ProjectSummary `json:"projects"`
	Total    int              `json:"total"`
}

type ProjectSummary struct {
	Name           string     `json:"name"`
	MemoryCount    int64      `json:"memoryCount"`
	SessionCount   int64      `json:"sessionCount"`
	LastActivityAt *time.Time `json:"lastActivityAt"`
	SyncHealth     string     `json:"syncHealth"`
}

type ProjectAggregate struct {
	Name              string
	MemoryCount       int64
	SessionCount      int64
	LastMemoryAt      *time.Time
	LastSessionAt     *time.Time
	LastSyncAt        *time.Time
	LatestSyncOutcome *SyncAttemptOutcome
}

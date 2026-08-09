package model

import (
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

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

// AdminUserResponse is the complete user-management projection.
type AdminUserResponse struct {
	ID         string         `json:"id"`
	Username   string         `json:"username"`
	Email      string         `json:"email"`
	Level      UserLevel      `json:"level"`
	IsActive   bool           `json:"is_active"`
	CreatedAt  time.Time      `json:"created_at"`
	SyncStatus UserSyncStatus `json:"sync_status"`
	LastSyncAt *time.Time     `json:"last_sync_at"`
}

// SyncSessionPayload (response side) se usa en pulled_sessions.
// Reutiliza los campos de SyncSessionPayload del request (mismo wire format).
type SyncSessionResponse = SyncSessionPayload

// PullResult agrupa sesiones y memorias devueltas por un pull incremental.
// Las sesiones van ANTES que las memorias — el daemon receptor las inserta primero
// para satisfacer la FK memories.session_id → sessions(id).
//
// MemoriesHasMore/SessionsHasMore + los next cursors implementan la paginación
// acotada del pull legado (PR 2a, design §2.2). El servidor SOLO expone la página
// actual + el indicador de continuación — la composición del drain completo
// (llamar repetidamente hasta hasMore=false) es responsabilidad exclusiva del
// daemon consumidor (PR 2b), no de este paquete.
type PullResult struct {
	Sessions []*Session
	Memories []*Memory

	MemoriesHasMore   bool
	NextPullCursor    *PullCursor
	SessionsHasMore   bool
	NextSessionCursor *PullCursor
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

	NextMutationCursor     *MutationCursor       `json:"next_mutation_cursor,omitempty"`
	PulledMutations        []MutationEnvelope    `json:"pulled_mutations,omitempty"`
	MutationResults        []MutationApplyResult `json:"mutation_results,omitempty"`
	CompatibilityMode      string                `json:"compatibility_mode,omitempty"`
	ProjectIdentityVersion string                `json:"project_identity_version,omitempty"`

	// SyncCapabilities names optional protocol features this server understands,
	// so a daemon can decide whether to use one instead of discovering the answer
	// from a rejected mutation.
	//
	// This is deliberately not project_identity_version. That field is a
	// strict-equality handshake — the daemon errors out when the server's value
	// differs from its own contract version — so announcing a new ability there
	// would break every daemon that has not been upgraded, in both directions. A
	// capability must degrade: an old daemon ignores an unknown list entry, and a
	// new daemon that does not find its capability simply does not use it.
	SyncCapabilities []string `json:"sync_capabilities,omitempty"`

	// Bounded legacy pull pagination (PR 2a, design §2.2). These fields cover the
	// two previously-unbounded legacy pull channels: Pulled (memories) and
	// PulledSessions. omitempty preserves backward compat — an old daemon that
	// doesn't understand these fields simply ignores them, and when the pull is
	// fully drained in one page (the common case) has_more is false/omitted so
	// old and new daemons see the same shape.
	PulledHasMore         bool        `json:"pulled_has_more,omitempty"`
	NextPullCursor        *PullCursor `json:"next_pull_cursor,omitempty"`
	PulledSessionsHasMore bool        `json:"pulled_sessions_has_more,omitempty"`
	NextSessionCursor     *PullCursor `json:"next_session_cursor,omitempty"`
}

const MutationProtocolVersion = 2

const CompatibilityModeLegacy = "legacy-row-state"

const CompatibilityModeMutationV2 = "mutation-sync-v2"

// SyncCapabilityReproject tells the daemon this server understands the
// reproject mutation op, and that sending one will move the memory rather than
// be rejected as an op nobody knows.
//
// The string itself is owned by the shared contract module, so the daemon that
// must declare it and the server that matches on it cannot drift apart.
const SyncCapabilityReproject = projectidentity.CapabilityReproject

// ServerSyncCapabilities is what this build advertises on every sync response.
func ServerSyncCapabilities() []string {
	return []string{SyncCapabilityReproject}
}

// mutationOpCapabilities maps each mutation op that a client may not understand
// to the capability it must declare before the server will send it. An op absent
// from this map is baseline: every client that speaks the mutation protocol at
// all can apply it, and it is never withheld.
var mutationOpCapabilities = map[MutationOp]string{
	MutationOpReproject: SyncCapabilityReproject,
}

// MutationOpCapability names the capability a client must declare before the
// server will send this op, or "" when the op is baseline and never withheld.
// It exists so a withhold can be reported in terms of the exact string the
// client failed to send, which is the whole diagnosis when the client sent a
// near-miss spelling.
func MutationOpCapability(op MutationOp) string {
	return mutationOpCapabilities[op]
}

// ClientUnderstandsMutationOp reports whether a client declaring these
// capabilities can apply this op. It is the pull-side gate: an event the client
// cannot apply must not enter its stream, because failing to apply one aborts
// its whole batch and strands its cursor.
//
// Absent capabilities mean "baseline only" — the honest reading of a request
// from a daemon that predates the field.
func ClientUnderstandsMutationOp(op MutationOp, capabilities []string) bool {
	required, gated := mutationOpCapabilities[op]
	if !gated {
		return true
	}
	for _, declared := range capabilities {
		if declared == required {
			return true
		}
	}
	return false
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
	SyncHealth     *string    `json:"syncHealth,omitempty"`
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

package model

import (
	"errors"
	"time"
)

// LoginRequest es el body del POST /auth/login.
// binding:"required" indica a Gin que el campo es obligatorio.
// Si falta, Gin devuelve automáticamente un 400 Bad Request.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
	DaemonID string `json:"daemon_id,omitempty"`
	Client   string `json:"client,omitempty"`
}

// CreateMemoryRequest es el body del POST /memories.
// Los campos sin binding:"required" son opcionales.
type CreateMemoryRequest struct {
	// SyncID es el UUID generado por el cliente (daemon) que identifica
	// esta memoria de forma única en todo el sistema.
	// binding:"required,uuid" valida que sea un UUID válido.
	SyncID string `json:"sync_id" binding:"required,uuid"`

	Project  string         `json:"project"  binding:"required,max=100"`
	TopicKey *string        `json:"topic_key"`
	Category MemoryCategory `json:"category" binding:"required"`
	Title    string         `json:"title"    binding:"required,max=500"`
	Content  string         `json:"content"  binding:"required"`

	// Tags y FilesAffected son opcionales — si no vienen en el JSON,
	// Go los inicializa como nil (que trataremos como array vacío).
	Tags          []string `json:"tags"`
	FilesAffected []string `json:"files_affected"`

	// SessionID es opcional. Cuando se omite, el service resuelve `manual-save-{project}`
	// vía SessionRepository.EnsureManualSaveSession para cumplir el FK NOT NULL en
	// memories.session_id (R2-CRIT-2 + paridad con el sync resolver).
	SessionID *string `json:"session_id,omitempty"`
}

// SyncRequest es el body del POST /sync.
// Contiene un batch de memorias a subir y el timestamp del último sync.
type SyncRequest struct {
	Project string `json:"project" binding:"required"`

	// Sessions es el batch de sesiones a enviar al servidor.
	// Opcional — daemons anteriores a Slice 4 no envían este campo.
	// Procesado ANTES de memories para satisfacer la FK memories.session_id.
	Sessions []SyncSessionPayload `json:"sessions" binding:"max=100,dive"`

	// Memories es el batch de memorias a enviar al servidor.
	// binding:"max=100" rechaza con 400 si vienen más de 100.
	// binding:"dive" le dice al validador que valide también
	// cada elemento del slice (no solo que el slice exista).
	Memories []SyncMemoryPayload `json:"memories" binding:"max=100,dive"`

	// Prompts es el batch de user-prompts a enviar al servidor.
	// Opcional — daemons antiguos no envían este campo (backward-compat).
	// binding:"max=100" rechaza más de 100 por request (S8).
	// binding:"dive" valida cada elemento del slice.
	Prompts []SyncPromptPayload `json:"prompts" binding:"max=100,dive"`

	// LastSync es opcional (puntero). Si es nil, el servidor devolverá
	// TODAS las memorias del proyecto en el pull. Si tiene valor,
	// solo devuelve las memorias más nuevas que esa fecha.
	LastSync *time.Time `json:"last_sync"`

	// Mutation sync v2 fields. Legacy clients omit these and keep the row-state path.
	ProtocolVersion int                `json:"protocol_version,omitempty"`
	MutationCursor  *MutationCursor    `json:"mutation_cursor,omitempty"`
	Mutations       []MutationEnvelope `json:"mutations,omitempty" binding:"max=100,dive"`

	// PullLimit bounds how many rows the legacy pull channels (pulled memories,
	// pulled sessions) return per request (PR 2a, design §2.2). This field is an
	// EXPLICIT opt-in into bounded pagination:
	//   - Omitted, 0, or negative → the server performs an UNBOUNDED legacy pull
	//     (no LIMIT clause, has_more=false, no cursor) — EXACTLY today's
	//     pre-pagination behavior. The current hive-daemon DOES rely on this: it
	//     has no pulled_has_more/next_pull_cursor handling and hardcodes
	//     PullHasMore=false, so capping the page here without the daemon knowing
	//     how to resume would silently strand rows past page 1 until the daemon
	//     is updated (PR 2b) to consume pagination.
	//   - Explicit positive value → clamped server-side via model.ClampPullLimit
	//     to [1, MaxPullLimit] and paginated with keyset cursors.
	PullLimit int `json:"pull_limit,omitempty"`

	// PullCursor resumes legacy memory pull pagination after a previous bounded
	// page (see SyncResponse.NextPullCursor). Nil means start from the beginning
	// of the current since-based window.
	PullCursor *PullCursor `json:"pull_cursor,omitempty"`

	// PullSessionCursor resumes legacy session pull pagination after a previous
	// bounded page (see SyncResponse.NextSessionCursor). Independent from
	// PullCursor — sessions and memories paginate separately.
	PullSessionCursor *PullCursor `json:"pull_session_cursor,omitempty"`

	AckSubject ProjectBlockAckSubject `json:"-"`
}

// SyncSessionPayload es el formato de sesión en el wire protocol de sync.
type SyncSessionPayload struct {
	ID        string     `json:"id"        binding:"required"`
	SyncID    string     `json:"sync_id"   binding:"required"`
	Project   string     `json:"project"   binding:"required"`
	Directory string     `json:"directory"`
	DevID     string     `json:"dev_id"    binding:"required"`
	Client    string     `json:"client"    binding:"required"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Summary   *string    `json:"summary"`
}

// SyncPromptPayload es la forma de cada prompt dentro de un SyncRequest.
// Refleja los campos que hive-daemon almacena localmente para user-prompts.
type SyncPromptPayload struct {
	// SyncID es el UUID generado por el daemon, garantiza idempotencia en el sync (S3).
	// binding:"required,uuid" valida que sea un UUID válido (S7).
	SyncID string `json:"sync_id" binding:"required,uuid"`

	// Project identifica a qué proyecto pertenece el prompt (S5).
	Project string `json:"project" binding:"required,max=100"`

	// Content es el texto del prompt. No puede estar vacío (S6) ni superar 50000 chars.
	// 50000 coincide con MaxObservationLength del daemon (mcp/tools.go).
	Content string `json:"content" binding:"required,max=50000"`

	// CreatedAt es cuándo se creó el prompt en el daemon (opcional).
	CreatedAt time.Time `json:"created_at"`
}

// SyncMemoryPayload es la forma de cada memoria dentro de un SyncRequest.
// Refleja exactamente los campos que hive-daemon almacena localmente.
// El servidor acepta esta forma y la adapta a su schema interno.
type SyncMemoryPayload struct {
	SyncID        string         `json:"sync_id"         binding:"required,uuid"`
	Project       string         `json:"project"         binding:"required"`
	TopicKey      *string        `json:"topic_key"`
	Category      MemoryCategory `json:"category"        binding:"required"`
	Title         string         `json:"title"           binding:"required,max=500"`
	Content       string         `json:"content"         binding:"required"`
	Tags          []string       `json:"tags"`
	FilesAffected []string       `json:"files_affected"`
	CreatedBy     string         `json:"created_by"      binding:"required"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	// SessionID is optional — absent on old daemons (backward-compat).
	// Server fills it via lazy manual-save-{project} fallback when empty.
	SessionID string `json:"session_id,omitempty"`
}

// SetLevelRequest es el body del POST /admin/users/:username/level.
type SetLevelRequest struct {
	Level UserLevel `json:"level" binding:"required,oneof=viewer member admin"`
}

type CreateUserRequest struct {
	Username          string    `json:"username" binding:"required,max=100"`
	Email             string    `json:"email" binding:"required,email,max=255"`
	Level             UserLevel `json:"level" binding:"required,oneof=viewer member admin"`
	TemporaryPassword string    `json:"temporary_password" binding:"required,min=8,max=72"`
}

type ResetTemporaryPasswordRequest struct {
	TemporaryPassword string `json:"temporary_password" binding:"required,min=8,max=72"`
}

const MaxTemporaryPasswordBytes = 72

var ErrTemporaryPasswordTooLong = errors.New("temporary password exceeds 72 bytes")

func ValidateTemporaryPasswordBytes(password string) error {
	if len([]byte(password)) > MaxTemporaryPasswordBytes {
		return ErrTemporaryPasswordTooLong
	}
	return nil
}

// ListMemoriesQuery son los query params del GET /memories.
// Usamos form:"..." en lugar de json:"..." porque vienen en la URL, no en el body.
// Ejemplo: GET /memories?project=jarvis-dev&limit=10&offset=0
type ListMemoriesQuery struct {
	Query    string `form:"query"`
	Project  string `form:"project"`
	Category string `form:"category"`
	From     string `form:"from"`
	Until    string `form:"until"`
	Limit    int    `form:"limit"  binding:"omitempty,min=1,max=100"`
	Offset   int    `form:"offset" binding:"omitempty,min=0"`
}

// SearchQuery son los query params del GET /memories/search.
type SearchQuery struct {
	Query    string `form:"query"   binding:"required"`
	Project  string `form:"project"`
	Category string `form:"category"`
	From     string `form:"from"`
	Until    string `form:"until"`
	Limit    int    `form:"limit"   binding:"omitempty,min=1,max=100"`
	Offset   int    `form:"offset"  binding:"omitempty,min=0"`
}

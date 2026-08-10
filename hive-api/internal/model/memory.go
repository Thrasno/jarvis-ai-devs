package model

import "time"

// MemoryCategory es el tipo de observación guardada.
// Usamos un tipo propio (no string plano) para que el compilador
// rechace categorías inválidas en tiempo de compilación.
type MemoryCategory string

const (
	CatBugfix         MemoryCategory = "bugfix"
	CatDecision       MemoryCategory = "decision"
	CatArchitecture   MemoryCategory = "architecture"
	CatDiscovery      MemoryCategory = "discovery"
	CatPattern        MemoryCategory = "pattern"
	CatConfig         MemoryCategory = "config"
	CatPreference     MemoryCategory = "preference"
	CatSessionSummary MemoryCategory = "session_summary"
)

// IsValid comprueba si la categoría es válida.
func (c MemoryCategory) IsValid() bool {
	switch c {
	case CatBugfix, CatDecision, CatArchitecture, CatDiscovery,
		CatPattern, CatConfig, CatPreference, CatSessionSummary:
		return true
	}
	return false
}

// Memory representa una observación o decisión guardada en Hive.
//
// Este struct es el "contrato" entre hive-daemon (local) y hive-api (cloud).
// Los campos marcados como "server-only" los establece el servidor —
// el cliente (daemon) no los envía, los recibe de vuelta.
//
// Compatibilidad con hive-daemon:
// Los campos base (SyncID, Project, TopicKey, Category, Title, Content,
// Tags, FilesAffected, CreatedBy, CreatedAt)
// son idénticos a los del daemon SQLite.
// Los campos nuevos (UpdatedAt, Origin, SyncedAt) son aditivos —
// Go ignora campos desconocidos al deserializar JSON, así que el daemon
// no se rompe al recibir una memoria con campos extra.
type Memory struct {
	// ID es el UUID primario generado por PostgreSQL.
	// Es distinto de SyncID — el ID solo existe en el servidor.
	ID string `json:"id"`

	// SyncID es el UUID generado por el daemon antes de sincronizar.
	// Es el puente entre la base de datos local y la nube.
	// Único globalmente — sirve como clave de idempotencia en el sync.
	SyncID string `json:"sync_id"`

	Project string `json:"project"`

	// TopicKey es la clave de agrupamiento/contexto de una memoria.
	// Es un puntero (*string) porque puede ser NULL en la base de datos.
	// Cuando TopicKey tiene valor, agrupa memorias relacionadas; cada guardado crea una fila nueva.
	// sync_id es la clave de idempotencia (Issue #119).
	// Cuando es nil, cada guardado crea una entrada nueva e inmutable.
	TopicKey *string `json:"topic_key,omitempty"`

	Category MemoryCategory `json:"category"`
	Title    string         `json:"title"`
	Content  string         `json:"content"`

	// Tags y FilesAffected son slices de strings.
	// En Go, []string es una lista de tamaño variable (como array en PHP).
	// Se almacenan como JSONB en PostgreSQL.
	// El valor por defecto de un slice en Go es nil, pero al serializar
	// a JSON queremos [] (array vacío), no null. Por eso usamos omitempty
	// solo en campos opcionales de verdad.
	Tags          []string `json:"tags"`
	FilesAffected []string `json:"files_affected"`

	CreatedBy string `json:"created_by"`
	// SessionID links this memory to a session. Set by the sync handler
	// (populated from payload.SessionID or lazy-created manual-save-{project}).
	// Nullable until Slice 4 T4.7 sets NOT NULL on the column.
	SessionID *string `json:"session_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Origin identifica qué daemon/usuario envió esta memoria al servidor.
	// Es server-only: el servidor lo establece al recibir el sync.
	// Puntero (*string) porque puede ser NULL para memorias creadas
	// directamente vía API (origin: "api").
	Origin *string `json:"origin,omitempty"`

	// SyncedAt es el momento en que el servidor recibió esta memoria.
	// Server-only: el cliente no lo envía, lo recibe de vuelta.
	SyncedAt time.Time `json:"synced_at"`

	// Tombstone metadata. Nil deleted_at means the memory is active.
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	DeletedBy    *string    `json:"deleted_by,omitempty"`
	DeleteReason *string    `json:"delete_reason,omitempty"`
	RestoredAt   *time.Time `json:"restored_at,omitempty"`
}

type MutationOp string

const (
	MutationOpCreate  MutationOp = "create"
	MutationOpUpdate  MutationOp = "update"
	MutationOpDelete  MutationOp = "delete"
	MutationOpRestore MutationOp = "restore"

	// MutationOpReproject carries the daemon's decision that a memory's project
	// literal changed. Every other op uses `project` to FIND its row and can
	// never change it, which is why the daemon's local identity migration
	// ("Foo.Bar" -> "foo-bar") had no way to reach the server at all.
	MutationOpReproject MutationOp = "reproject"
)

const MutationEntityMemory = "memory"

type MutationCursor struct {
	Sequence int64  `json:"sequence"`
	EventID  string `json:"event_id"`
}

// PullCursor is the keyset pagination cursor for the legacy (row-state) pull
// channels — pulled memories and pulled sessions. It mirrors the shape of
// MutationCursor but keys off (synced_at, sync_id), which is the ordering
// column pair used by PullSince and ListSessionsSince.
//
// SyncedAt + SyncID together form a strictly increasing, gap-free key when
// combined with `ORDER BY synced_at ASC, sync_id ASC` — synced_at alone is not
// unique enough to resume a page boundary when multiple rows share a timestamp.
type PullCursor struct {
	SyncedAt time.Time `json:"synced_at"`
	SyncID   string    `json:"sync_id"`
}

// IsZero reports whether the cursor has no position yet (start of the pull).
func (c PullCursor) IsZero() bool {
	return c.SyncedAt.IsZero() && c.SyncID == ""
}

// MaxPullLimit is the upper bound an explicit client pull_limit is clamped to.
const MaxPullLimit = 100

// UnboundedPullLimit is the sentinel value repositories interpret as "no LIMIT
// clause" — a single unbounded page, matching legacy pre-pagination behavior.
// It is returned by ClampPullLimit when the client did not explicitly opt into
// pagination (pull_limit absent, 0, or negative).
const UnboundedPullLimit = 0

// ClampPullLimit normalizes a client-supplied pull_limit.
//
// Backward-compat contract (PR 2a): pull_limit is an explicit opt-in into
// bounded pagination. A client that does not send it (or sends 0/negative)
// gets EXACTLY today's behavior — an unbounded pull with no LIMIT clause and
// has_more=false — because the current hive-daemon has no pulled_has_more /
// next_pull_cursor handling and would silently strand rows past page 1 if the
// server capped the page without the daemon knowing how to resume. Only when
// the client explicitly sends a positive pull_limit do we clamp it into
// [1, MaxPullLimit] and paginate with keyset cursors.
func ClampPullLimit(limit int) int {
	if limit <= 0 {
		return UnboundedPullLimit
	}
	if limit > MaxPullLimit {
		return MaxPullLimit
	}
	return limit
}

type TombstonePayload struct {
	DeletedAt time.Time `json:"deleted_at"`
	DeletedBy string    `json:"deleted_by,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

type MemoryPayload struct {
	SyncID        string         `json:"sync_id"`
	Project       string         `json:"project"`
	TopicKey      *string        `json:"topic_key,omitempty"`
	Category      MemoryCategory `json:"category"`
	Title         string         `json:"title"`
	Content       string         `json:"content"`
	Tags          []string       `json:"tags"`
	FilesAffected []string       `json:"files_affected"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	SessionID     string         `json:"session_id,omitempty"`
}

// ReprojectPayload names both ends of a project move.
//
// FromProject is not redundant with the row the server already has: it is the
// precondition. The server moves the row only if it currently holds
// FromProject, so a caller working from a stale idea of the row's project moves
// nothing instead of moving the wrong thing, and a replay after the move
// matches zero rows.
//
// ToProject duplicates the envelope's Project on purpose, so a journal entry
// read on its own still says where the memory went; the two must agree.
//
// It is the wire twin of hive-daemon's db.MutationReprojectPayload. The two
// modules ship separately, so each owns its own decoding of the same JSON shape;
// the tags are the contract between them.
type ReprojectPayload struct {
	FromProject string `json:"from_project"`
	ToProject   string `json:"to_project"`
}

type MutationEnvelope struct {
	EventID       string            `json:"event_id"`
	EntityType    string            `json:"entity_type"`
	EntitySyncID  string            `json:"entity_sync_id"`
	Project       string            `json:"project"`
	Op            MutationOp        `json:"op"`
	Sequence      int64             `json:"sequence"`
	OccurredAt    time.Time         `json:"occurred_at"`
	ActorID       string            `json:"actor_id,omitempty"`
	BaseUpdatedAt *time.Time        `json:"base_updated_at,omitempty"`
	Memory        *MemoryPayload    `json:"memory,omitempty"`
	Tombstone     *TombstonePayload `json:"tombstone,omitempty"`
	Reproject     *ReprojectPayload `json:"reproject,omitempty"`
}

type MutationApplyResult struct {
	EventID   string     `json:"event_id"`
	Op        MutationOp `json:"op"`
	Applied   bool       `json:"applied"`
	Duplicate bool       `json:"duplicate"`
	Rejected  bool       `json:"rejected"`
	Reason    string     `json:"reason,omitempty"`
	Sequence  int64      `json:"sequence,omitempty"`
}

type MutationBatch struct {
	Events []MutationEnvelope `json:"events"`
	Next   MutationCursor     `json:"next"`
}

// MemoryFilter agrupa los parámetros para filtrar y paginar memorias.
//
// En PHP pasarías un array asociativo o múltiples parámetros.
// En Go es idiomático crear un struct específico para esto —
// más legible y más fácil de extender.
type MemoryFilter struct {
	Project  string
	Category *MemoryCategory // puntero: nil = sin filtro de categoría

	// CreatedFrom and CreatedUntil filter by the canonical memory creation time.
	// Nil means the corresponding range bound is not applied.
	CreatedFrom  *time.Time
	CreatedUntil *time.Time

	// Paginación. Si Limit es 0, la capa de repositorio usará un default (20).
	Limit  int
	Offset int
}

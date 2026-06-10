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
)

const MutationEntityMemory = "memory"

type MutationCursor struct {
	Sequence int64  `json:"sequence"`
	EventID  string `json:"event_id"`
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

	// Paginación. Si Limit es 0, la capa de repositorio usará un default (20).
	Limit  int
	Offset int
}

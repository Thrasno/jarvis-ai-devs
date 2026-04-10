package model

import "time"

// LoginRequest es el body del POST /auth/login.
// binding:"required" indica a Gin que el campo es obligatorio.
// Si falta, Gin devuelve automáticamente un 400 Bad Request.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
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

	Confidence  *float32 `json:"confidence"`
	ImpactScore *float32 `json:"impact_score"`
}

// SyncRequest es el body del POST /sync.
// Contiene un batch de memorias a subir y el timestamp del último sync.
type SyncRequest struct {
	Project string `json:"project" binding:"required"`

	// Memories es el batch de memorias a enviar al servidor.
	// binding:"max=100" rechaza con 400 si vienen más de 100.
	// binding:"dive" le dice al validador que valide también
	// cada elemento del slice (no solo que el slice exista).
	Memories []SyncMemoryPayload `json:"memories" binding:"max=100,dive"`

	// LastSync es opcional (puntero). Si es nil, el servidor devolverá
	// TODAS las memorias del proyecto en el pull. Si tiene valor,
	// solo devuelve las memorias más nuevas que esa fecha.
	LastSync *time.Time `json:"last_sync"`
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
	Confidence    float32        `json:"confidence"`
	ImpactScore   float32        `json:"impact_score"`
}

// SetLevelRequest es el body del POST /admin/users/:username/level.
type SetLevelRequest struct {
	Level UserLevel `json:"level" binding:"required"`
}

// ListMemoriesQuery son los query params del GET /memories.
// Usamos form:"..." en lugar de json:"..." porque vienen en la URL, no en el body.
// Ejemplo: GET /memories?project=jarvis-dev&limit=10&offset=0
type ListMemoriesQuery struct {
	Project  string `form:"project"`
	Category string `form:"category"`
	Limit    int    `form:"limit"  binding:"omitempty,min=1,max=100"`
	Offset   int    `form:"offset" binding:"omitempty,min=0"`
}

// SearchQuery son los query params del GET /memories/search.
type SearchQuery struct {
	Query   string `form:"query"   binding:"required"`
	Project string `form:"project"`
	Limit   int    `form:"limit"   binding:"omitempty,min=1,max=100"`
	Offset  int    `form:"offset"  binding:"omitempty,min=0"`
}

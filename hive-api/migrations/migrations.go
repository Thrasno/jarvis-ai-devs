// Package migrations expone el SQL de migración embebido en el binario.
//
// Usamos go:embed para que el SQL viaje DENTRO del binario compilado.
// Esto significa que el servidor no necesita archivos externos para arrancar —
// basta con el binario. Es el enfoque 12-factor: self-contained deployments.
//
// ¿Por qué un paquete separado en lugar de embeber desde main.go?
// El directorio cmd/server/ está dos niveles por encima de migrations/,
// y go:embed no permite rutas con ".." — solo permite rutas relativas
// dentro del paquete o sus subdirectorios.
// Definiendo el embed aquí (en el paquete migrations/), la ruta "001_initial.sql"
// es válida y directa.
//
// Estrategia de migraciones:
// Cada migración es un archivo SQL independiente, idempotente (IF NOT EXISTS).
// main.go llama a RunMigrations por cada archivo en orden.
// En MVP futuro migraremos a golang-migrate para versiones incrementales.
package migrations

import _ "embed"

// InitialSQL es el contenido del script SQL de migración inicial (001).
// Se incrusta en el binario en tiempo de compilación.
//
//go:embed 001_initial.sql
var InitialSQL string

// UserPromptsSQL es el contenido de la migración 002 para la tabla user_prompts.
// Se ejecuta después de InitialSQL al arrancar el servidor.
//
//go:embed 002_user_prompts.sql
var UserPromptsSQL string

// SessionsSQL es el contenido de la migración 003: tabla sessions (con columnas
// directory, created_at, updated_at), backfill de sentinelas 'legacy-pre-lifecycle-{project}',
// columna memories.session_id y el flip final a NOT NULL con FK hacia sessions(id) (T4.7).
//
//go:embed 003_sessions.sql
var SessionsSQL string

// AuditLogsSQL es el contenido de la migración 004: tabla audit_logs con
// metadata JSONB e índices para filtros admin y orden determinístico.
//
//go:embed 004_audit_logs.sql
var AuditLogsSQL string

// MemoryMutationsSQL es la migración 005: tombstones en memories y journal
// de dominio memory_mutations. No reemplaza ni reutiliza audit_logs.
//
//go:embed 005_memory_mutations.sql
var MemoryMutationsSQL string

// DropTopicKeyUniqueConstraintSQL es la migración 006: elimina el índice UNIQUE
// en memories(project, topic_key) y lo reemplaza por un índice no único.
// topic_key pasa a ser una clave de agrupación, no de identidad (Issue #119).
//
//go:embed 006_drop_topic_key_unique_constraint.sql
var DropTopicKeyUniqueConstraintSQL string

// SyncAttemptLogsSQL is migration 007: dedicated production storage for daemon sync attempt audit logs.
//
//go:embed 007_sync_attempt_logs.sql
var SyncAttemptLogsSQL string

// ActivityFeedIndexSQL is migration 008: recency index for the global activity feed query.
//
//go:embed 008_activity_feed_index.sql
var ActivityFeedIndexSQL string

//go:embed 009_memory_discovery_indexes.sql
var MemoryDiscoveryIndexesSQL string

// PullCursorIndexesSQL is migration 010: composite (synced_at, sync_id) indexes on
// memories and sessions supporting keyset pagination for bounded legacy pull (PR 2a).
//
//go:embed 010_pull_cursor_indexes.sql
var PullCursorIndexesSQL string

// ProjectScopedPullCursorIndexesSQL is migration 011: project-leading composite
// indexes for the actual pull-cursor query shape, plus safe obsolete index cleanup.
//
//go:embed 011_project_scoped_pull_cursor_indexes.sql
var ProjectScopedPullCursorIndexesSQL string

// ProjectBlocksSQL is migration 012: durable project block tombstones and account-bound acknowledgements.
//
//go:embed 012_project_blocks.sql
var ProjectBlocksSQL string

// ProjectBlockAckSubjectsSQL is migration 013: account-bound ACK deliveries and ACK records.
//
//go:embed 013_project_block_ack_subjects.sql
var ProjectBlockAckSubjectsSQL string

//go:embed 014_user_security_version.sql
var UserSecurityVersionSQL string

//go:embed 015_sync_attempt_portal_users.sql
var SyncAttemptPortalUsersSQL string

//go:embed 016_sync_attempt_user_projection.sql
var SyncAttemptUserProjectionSQL string

// QuarantineContractSQL is migration 017. It retains legacy audit fields while
// preparing existing project block rows for generation-based lifecycle changes.
//
//go:embed 017_quarantine_contract.sql
var QuarantineContractSQL string

// DistributedQuarantineSQL is migration 018. It retains every generation
// command while project_blocks remains the mutable current-state head.
//
//go:embed 018_distributed_quarantine.sql
var DistributedQuarantineSQL string

// CanonicalProjectRegistrySQL is migration 019. The API backfills this additive
// registry through the shared Go canonicalizer after applying the schema.
//
//go:embed 019_canonical_project_identity.sql
var CanonicalProjectRegistrySQL string

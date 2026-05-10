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

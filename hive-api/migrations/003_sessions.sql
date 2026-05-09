-- Migración 003: tabla sessions + session_id en memories
-- Idempotente: todos los CREATE/ALTER usan IF NOT EXISTS o guards equivalentes
-- Compatible con PostgreSQL 15+

-- ============================================================
-- TABLA: sessions
-- ============================================================
--
-- Por qué TEXT en lugar de UUID para la PK:
-- Los IDs sentinel ('legacy-pre-lifecycle-{project}' y 'manual-save-{project}') son
-- strings literales, no UUIDs válidos. Usar TEXT como PK permite albergar ambos tipos
-- de ID en la misma columna sin necesitar una columna discriminadora extra.
-- El daemon genera UUIDs reales para sesiones explícitas (mem_session_start); esos
-- también son TEXT-compatibles. El campo sync_id sigue siendo UUID para el protocolo
-- de sync (idempotencia basada en UUID generado por el daemon).
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT         PRIMARY KEY,

    -- sync_id: generado por el daemon antes del sync (igual que memories.sync_id).
    -- UNIQUE garantiza que cada push es idempotente aunque el daemon reenvíe.
    sync_id     UUID         UNIQUE NOT NULL DEFAULT gen_random_uuid(),

    project     VARCHAR(100) NOT NULL,

    -- directory: directorio de trabajo activo cuando comenzó la sesión.
    -- DEFAULT '' permite backward-compat: sesiones sentinel y manual-save no tienen directorio.
    directory   TEXT         NOT NULL DEFAULT '',

    dev_id      VARCHAR(100) NOT NULL DEFAULT 'legacy',
    client      VARCHAR(50)  NOT NULL DEFAULT 'legacy',

    started_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    ended_at    TIMESTAMPTZ  NULL,
    summary     TEXT         NULL,

    -- synced_at: cuándo llegó este registro al servidor central.
    synced_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    -- created_at / updated_at: timestamps de auditoría.
    -- updated_at es actualizado por UpsertSession (Decision 12: LEAST(started_at) + updated_at=now()).
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Índices de performance para las queries más frecuentes
CREATE INDEX IF NOT EXISTS idx_sessions_project    ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_dev_id     ON sessions(dev_id);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_sync_id    ON sessions(sync_id);

-- ============================================================
-- BACKFILL: sentinel sessions para proyectos existentes
-- ============================================================
--
-- Para cada proyecto que ya tiene memorias, creamos una sesión sentinel con id
-- 'legacy-pre-lifecycle-{project}'. Esta sentinel representa todas las memorias
-- creadas antes de introducir el ciclo de vida de sesiones.
-- ON CONFLICT (id) DO NOTHING garantiza idempotencia: re-ejecutar esta migración
-- no duplica las sentinelas.
INSERT INTO sessions (id, project, dev_id, client, started_at, ended_at)
SELECT
    'legacy-pre-lifecycle-' || project AS id,
    project,
    'legacy' AS dev_id,
    'legacy' AS client,
    MIN(created_at)                    AS started_at,
    now()                              AS ended_at
FROM memories
GROUP BY project
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- COLUMNA: memories.session_id
-- ============================================================
--
-- Paso 1: agregar la columna nullable.
-- Se mantiene NULLABLE hasta Slice 4 (sync wire) — ver Decisión 6.
-- Razón: sin sync handler que pueble session_id en cada INSERT, enforzar NOT NULL
-- ahora rompería los pushes de memorias de daemons no actualizados. El SET NOT NULL
-- se aplica en la migración de Slice 4, una vez que el sync handler ya garantiza
-- que cada memoria llega con session_id resuelto.
ALTER TABLE memories ADD COLUMN IF NOT EXISTS session_id TEXT NULL;

-- Paso 2: backfill — asignar el sentinel correspondiente a todas las memorias
-- que aún no tienen session_id (idempotente: solo actualiza NULLs).
UPDATE memories
SET session_id = 'legacy-pre-lifecycle-' || project
WHERE session_id IS NULL;

-- ============================================================
-- T4.7 — Final NOT NULL flip (Decisión 6 / Slice 4)
-- ============================================================
--
-- Después del backfill, no debería haber NULLs en memories.session_id.
-- Esta migración es segura de ejecutar en producción porque:
--   1. El backfill anterior ya llenó los NULLs con sentinelas.
--   2. El sync handler (Slice 4) ya garantiza session_id en cada push nuevo.
--   3. SET NOT NULL en PostgreSQL es idempotente si la columna ya era NOT NULL.
--
-- Paso 3: convertir la columna en NOT NULL.
ALTER TABLE memories ALTER COLUMN session_id SET NOT NULL;

-- Paso 4: agregar FK constraint hacia sessions(id).
-- PostgreSQL no soporta `ADD CONSTRAINT IF NOT EXISTS` para FKs; el bloque DO/IF
-- consulta pg_constraint primero y solo agrega la FK si no existe — garantizando
-- idempotencia al re-ejecutar la migración.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_memories_session_id'
          AND conrelid = 'memories'::regclass
    ) THEN
        ALTER TABLE memories
            ADD CONSTRAINT fk_memories_session_id
            FOREIGN KEY (session_id) REFERENCES sessions(id);
    END IF;
END $$;

-- Paso 5 (Suspect-A) — índice sobre memories.session_id para satisfacer FR-D-2.
-- Sin este índice, queries del tipo `WHERE session_id = ?` son full table scans.
CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id);

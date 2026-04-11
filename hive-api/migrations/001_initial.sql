-- Migración inicial: schema completo de hive-api
-- Idempotente: todos los CREATE usan IF NOT EXISTS
-- Compatible con PostgreSQL 15+

-- ============================================================
-- TABLA: users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username    VARCHAR(100) UNIQUE NOT NULL,
    email       VARCHAR(255) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,        -- hash bcrypt
    level       VARCHAR(20)  NOT NULL DEFAULT 'member',
    is_active   BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Índice para login rápido por email (el caso de uso más frecuente)
CREATE INDEX IF NOT EXISTS idx_users_email     ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_username  ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_level     ON users(level);

-- ============================================================
-- TABLA: memories
-- ============================================================
CREATE TABLE IF NOT EXISTS memories (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- sync_id: generado por el daemon antes del sync.
    -- Es el puente entre la BD local (SQLite) y la nube (PostgreSQL).
    -- UNIQUE global — permite idempotencia en el sync.
    sync_id        UUID         UNIQUE NOT NULL,

    project        VARCHAR(100) NOT NULL,

    -- topic_key: nombre estable para memorias que se actualizan.
    -- NULL = memoria inmutable (cada guardado crea una nueva entrada).
    -- NOT NULL = memoria que puede ser actualizada con el mismo topic_key.
    topic_key      TEXT,

    category       VARCHAR(50)  NOT NULL,
    title          VARCHAR(500) NOT NULL,
    content        TEXT         NOT NULL,

    -- JSONB es más eficiente que TEXT[] para búsqueda y actualización parcial.
    -- Permite queries como: WHERE tags @> '["go","architecture"]'
    tags           JSONB        NOT NULL DEFAULT '[]',
    files_affected JSONB        NOT NULL DEFAULT '[]',

    created_by     VARCHAR(100) NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),

    -- origin: qué daemon envió esta memoria (formato "user@hostname").
    -- Útil para debugging cuando hay conflictos de sync.
    origin         VARCHAR(100),

    -- synced_at: cuándo llegó al servidor central.
    -- Distinto de created_at (cuándo la creó el daemon localmente).
    synced_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),

    confidence     REAL         NOT NULL DEFAULT 0,
    impact_score   REAL         NOT NULL DEFAULT 0,

    -- FTS: columna generada automáticamente por PostgreSQL.
    -- Se recalcula en cada INSERT/UPDATE, sin necesidad de triggers.
    -- Combina título (peso A = más importante) y contenido (peso B).
    -- El idioma 'simple' usa el stemmer castellano (comprende/comprendió → comprend).
    search_vector  TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')),   'A') ||
        setweight(to_tsvector('simple', coalesce(content, '')), 'B')
    ) STORED
);

-- Índice GIN para búsqueda FTS — sin este índice cada búsqueda hace full table scan
CREATE INDEX IF NOT EXISTS idx_memories_search_vector ON memories USING GIN(search_vector);

-- Índices de filtrado frecuente
CREATE INDEX IF NOT EXISTS idx_memories_project   ON memories(project);
CREATE INDEX IF NOT EXISTS idx_memories_category  ON memories(category);
CREATE INDEX IF NOT EXISTS idx_memories_synced_at ON memories(synced_at);
CREATE INDEX IF NOT EXISTS idx_memories_sync_id   ON memories(sync_id);

-- Unique parcial: solo una memoria con el mismo topic_key por proyecto.
-- "WHERE topic_key IS NOT NULL" = el constraint solo aplica cuando topic_key existe.
-- Las memorias con topic_key NULL no compiten entre sí (cada una es única por sync_id).
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_project_topic_key
    ON memories(project, topic_key)
    WHERE topic_key IS NOT NULL;

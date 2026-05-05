-- Migración 002: tabla de user-prompts
-- Idempotente: todos los CREATE usan IF NOT EXISTS
-- Compatible con PostgreSQL 15+

-- ============================================================
-- TABLA: user_prompts
-- ============================================================
-- Almacena los prompts de usuario sincronizados desde el daemon.
-- Un "prompt" es una instrucción personalizada que el usuario define
-- en su cliente local y que se sincroniza al servidor para compartirla
-- entre sesiones y dispositivos.
--
-- sync_id UNIQUE garantiza idempotencia: el daemon puede reenviar el
-- mismo prompt (misma sync_id) varias veces sin generar duplicados.
-- ON CONFLICT DO NOTHING en el upsert implementa este contrato.
CREATE TABLE IF NOT EXISTS user_prompts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- sync_id: generado por el daemon antes del sync.
    -- UNIQUE global — garantiza idempotencia (S3).
    sync_id    UUID UNIQUE NOT NULL,

    -- project: identifica a qué proyecto pertenece este prompt (S5).
    project    VARCHAR(100) NOT NULL,

    -- content: el texto del prompt. No puede estar vacío (S6).
    content    TEXT NOT NULL,

    -- created_by: quién creó el prompt (usuario del daemon).
    created_by VARCHAR(100) NOT NULL,

    -- created_at: cuándo se creó el prompt en el daemon.
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- synced_at: cuándo llegó al servidor central.
    -- NULL hasta que el servidor lo recibe; seteado explícitamente en el INSERT.
    synced_at  TIMESTAMPTZ NULL DEFAULT NULL
);

-- Índice para consultas por proyecto (S5: project isolation queries)
CREATE INDEX IF NOT EXISTS idx_user_prompts_project    ON user_prompts(project);

-- Índice para consultas por creador (útil para administración y auditoría)
CREATE INDEX IF NOT EXISTS idx_user_prompts_created_by ON user_prompts(created_by);

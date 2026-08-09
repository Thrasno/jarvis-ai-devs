package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startPostgresWithSessions inicia un contenedor PostgreSQL y ejecuta las tres migraciones
// (001 initial, 002 user_prompts, 003 sessions, 005 memory mutations). Retorna el pool y cleanup.
//
// R2-CRIT-5 — toda llamada que asume el schema completo (sessions table +
// memories.session_id NOT NULL + FK) DEBE usar este helper. `startPostgres` aplica
// solo la migración 001 para preservar el punto de partida que necesitan los tests
// de la propia migración 003 (que insertan memorias sin session_id antes de ejecutarla).
func startPostgresWithSessions(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	pool, cleanup := startPostgres(t)

	err := RunMigrations(pool, migrations.UserPromptsSQL)
	require.NoError(t, err, "failed to run migration 002")

	err = RunMigrations(pool, migrations.SessionsSQL)
	require.NoError(t, err, "failed to run migration 003")

	err = RunMigrations(pool, migrations.MemoryMutationsSQL)
	require.NoError(t, err, "failed to run migration 005")

	// 006: drop UNIQUE constraint on topic_key (Issue #119)
	err = RunMigrations(pool, migrations.DropTopicKeyUniqueConstraintSQL)
	require.NoError(t, err, "failed to run migration 006")

	err = RunMigrations(pool, migrations.ProjectBlocksSQL)
	require.NoError(t, err, "failed to run migration 012")
	err = RunMigrations(pool, migrations.QuarantineContractSQL)
	require.NoError(t, err, "failed to run migration 017")
	err = RunMigrations(pool, migrations.CanonicalProjectRegistrySQL)
	require.NoError(t, err, "failed to run migration 019")
	err = RunMigrations(pool, migrations.ReprojectMutationSQL)
	require.NoError(t, err, "failed to run migration 023")

	return pool, cleanup
}

// ─── T3.1 + T3.2 + T3.3: migration structure tests ──────────────────────────

// TestMigration003_SessionsTableExists verifica que la tabla sessions existe después de
// ejecutar la migración 003.
func TestMigration003_SessionsTableExists(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'sessions'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "sessions table should exist after migration 003")
}

// TestMigration003_SessionsColumns verifica que las columnas requeridas existen con
// los tipos correctos. TEXT PRIMARY KEY es intencional: los IDs sentinel
// ('manual-save-{project}', 'legacy-pre-lifecycle-{project}') no son UUIDs válidos,
// por lo que no podemos usar UUID como tipo de la PK.
func TestMigration003_SessionsColumns(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()

	cols := map[string]string{}
	rows, err := pool.Query(ctx, `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'sessions'
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var name, dtype string
		require.NoError(t, rows.Scan(&name, &dtype))
		cols[name] = dtype
	}
	require.NoError(t, rows.Err())

	// T4.0a — assert all 12 columns including directory, created_at, updated_at added in Slice 4
	required := []struct {
		col   string
		dtype string
	}{
		{"id", "text"},
		{"sync_id", "uuid"},
		{"project", "character varying"},
		{"directory", "text"},
		{"dev_id", "character varying"},
		{"client", "character varying"},
		{"started_at", "timestamp with time zone"},
		{"ended_at", "timestamp with time zone"},
		{"summary", "text"},
		{"synced_at", "timestamp with time zone"},
		{"created_at", "timestamp with time zone"},
		{"updated_at", "timestamp with time zone"},
	}

	for _, r := range required {
		t.Run("col_"+r.col, func(t *testing.T) {
			dtype, ok := cols[r.col]
			require.True(t, ok, "column %q should exist", r.col)
			assert.Equal(t, r.dtype, dtype, "column %q type mismatch", r.col)
		})
	}
}

// TestMigration003_Indexes verifica que los índices de performance existen.
func TestMigration003_Indexes(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT indexname FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'sessions'
	`)
	require.NoError(t, err)
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		found[name] = true
	}
	require.NoError(t, rows.Err())

	expected := []string{
		"idx_sessions_project",
		"idx_sessions_dev_id",
		"idx_sessions_started_at",
		"idx_sessions_sync_id",
	}
	for _, idx := range expected {
		t.Run(idx, func(t *testing.T) {
			assert.True(t, found[idx], "index %q should exist", idx)
		})
	}
}

// TestMigration003_MemoriesSessionIDColumn verifica que memories.session_id existe
// como columna NOT NULL después de aplicar la migración 003 completa (incluyendo T4.7
// que hace el flip final de NOT NULL — Decisión 6 / Slice 4).
func TestMigration003_MemoriesSessionIDColumn(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()

	var isNullable string
	err := pool.QueryRow(ctx, `
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'memories'
		  AND column_name = 'session_id'
	`).Scan(&isNullable)
	require.NoError(t, err, "memories.session_id column should exist")
	// T4.7: after Slice 4 migration, memories.session_id must be NOT NULL
	assert.Equal(t, "NO", isNullable, "memories.session_id must be NOT NULL after Slice 4 migration")
}

// TestMigration003_ExistingMemoriesBackfilled verifica que las memorias existentes
// (insertadas ANTES de aplicar la migración 003) quedan con session_id asignado
// al sentinel de su proyecto después del backfill.
func TestMigration003_ExistingMemoriesBackfilled(t *testing.T) {
	// Partimos de un Postgres con solo 001 + 002 aplicadas
	pool001, cleanup001 := startPostgres(t)
	defer cleanup001()

	ctx := context.Background()

	require.NoError(t, RunMigrations(pool001, migrations.UserPromptsSQL))

	// Insertar memorias en dos proyectos ANTES de la migración 003
	_, err := pool001.Exec(ctx, `
		INSERT INTO memories (sync_id, project, category, title, content, created_by)
		VALUES
		  ('aaaaaaaa-0000-0000-0000-000000000001', 'alpha', 'decision', 'Alpha memory', 'content', 'user'),
		  ('aaaaaaaa-0000-0000-0000-000000000002', 'alpha', 'bugfix',   'Alpha bugfix',  'content', 'user'),
		  ('bbbbbbbb-0000-0000-0000-000000000001', 'beta',  'pattern',  'Beta memory',   'content', 'user')
	`)
	require.NoError(t, err)

	// Aplicar migración 003 — debe backfillar session_id
	require.NoError(t, RunMigrations(pool001, migrations.SessionsSQL))

	// Verificar que todas las memorias tienen session_id NOT NULL con el sentinel correcto
	rows, err := pool001.Query(ctx, `SELECT project, session_id FROM memories ORDER BY project, sync_id`)
	require.NoError(t, err)
	defer rows.Close()

	type row struct{ project, sessionID string }
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.project, &r.sessionID))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())

	require.Len(t, got, 3)
	assert.Equal(t, "legacy-pre-lifecycle-alpha", got[0].sessionID)
	assert.Equal(t, "legacy-pre-lifecycle-alpha", got[1].sessionID)
	assert.Equal(t, "legacy-pre-lifecycle-beta", got[2].sessionID)
}

// TestMigration003_SentinelSessionsCreated verifica que por cada proyecto en memories,
// se crea un sentinel 'legacy-pre-lifecycle-{project}' en sessions con dev_id='legacy'.
func TestMigration003_SentinelSessionsCreated(t *testing.T) {
	pool001, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, RunMigrations(pool001, migrations.UserPromptsSQL))

	_, err := pool001.Exec(ctx, `
		INSERT INTO memories (sync_id, project, category, title, content, created_by, created_at)
		VALUES
		  ('cccccccc-0000-0000-0000-000000000001', 'proj-a', 'decision', 'M1', 'c', 'u', '2024-01-01'),
		  ('cccccccc-0000-0000-0000-000000000002', 'proj-b', 'bugfix',   'M2', 'c', 'u', '2024-02-01')
	`)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(pool001, migrations.SessionsSQL))

	rows, err := pool001.Query(ctx, `
		SELECT id, dev_id, client, ended_at IS NOT NULL
		FROM sessions
		WHERE id LIKE 'legacy-pre-lifecycle-%'
		ORDER BY id
	`)
	require.NoError(t, err)
	defer rows.Close()

	type sentinel struct {
		id     string
		devID  string
		client string
		ended  bool
	}
	var sentinels []sentinel
	for rows.Next() {
		var s sentinel
		require.NoError(t, rows.Scan(&s.id, &s.devID, &s.client, &s.ended))
		sentinels = append(sentinels, s)
	}
	require.NoError(t, rows.Err())

	require.Len(t, sentinels, 2)
	assert.Equal(t, "legacy-pre-lifecycle-proj-a", sentinels[0].id)
	assert.Equal(t, "legacy", sentinels[0].devID)
	assert.Equal(t, "legacy", sentinels[0].client)
	assert.True(t, sentinels[0].ended, "sentinel should have ended_at set")
	assert.Equal(t, "legacy-pre-lifecycle-proj-b", sentinels[1].id)
}

// TestMigration003_Idempotent verifica que ejecutar la migración dos veces no produce
// error ni duplicados.
func TestMigration003_Idempotent(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()

	// Segunda ejecución de la migración 003
	err := RunMigrations(pool, migrations.SessionsSQL)
	require.NoError(t, err, "running migration 003 twice must not error")

	// Sin filas extra en sessions (sin memorias previas no hay sentinels)
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no duplicate sentinel rows expected on empty DB")
}

// CRIT-1 — `ALTER TABLE ... ADD CONSTRAINT IF NOT EXISTS` is invalid Postgres
// syntax for FK constraints. This test seeds memories+sessions BEFORE running
// the migration twice, then asserts the FK exists exactly once after both
// runs.
func TestMigration003_FKConstraint_Idempotent(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, RunMigrations(pool, migrations.UserPromptsSQL))

	// Seed at least one memory so backfill exercises the path.
	_, err := pool.Exec(ctx, `
		INSERT INTO memories (sync_id, project, category, title, content, created_by)
		VALUES ('aaaaaaaa-1111-2222-3333-444455556666', 'idemp-proj', 'decision', 'M', 'C', 'u')`)
	require.NoError(t, err)

	// First run
	require.NoError(t, RunMigrations(pool, migrations.SessionsSQL))
	// Second run must not error — the FK constraint already exists.
	require.NoError(t, RunMigrations(pool, migrations.SessionsSQL),
		"second migration run must succeed; ADD CONSTRAINT IF NOT EXISTS for FKs is invalid PG syntax")

	// FK must exist exactly once.
	var fkCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_constraint
		WHERE conname = 'fk_memories_session_id'
		  AND conrelid = 'memories'::regclass
	`).Scan(&fkCount)
	require.NoError(t, err)
	assert.Equal(t, 1, fkCount, "FK fk_memories_session_id must exist exactly once after idempotent runs")
}

// ─── T3.4: model.Session tests ───────────────────────────────────────────────

// TestSession_Validation verifica las reglas de validación del modelo Session.
func TestSession_Validation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		session model.Session
		wantErr bool
	}{
		{
			name: "valid session",
			session: model.Session{
				ID:        "test-session-id",
				Project:   "test-project",
				DevID:     "dev@host",
				Client:    "claude-code",
				StartedAt: now,
			},
			wantErr: false,
		},
		{
			name: "valid session with optional ended_at",
			session: model.Session{
				ID:        "manual-save-project",
				Project:   "project",
				DevID:     "dev@host",
				Client:    "manual",
				StartedAt: now,
				EndedAt:   &now,
			},
			wantErr: false,
		},
		{
			name: "missing dev_id",
			session: model.Session{
				ID:        "test-session-id",
				Project:   "test-project",
				DevID:     "",
				Client:    "claude-code",
				StartedAt: now,
			},
			wantErr: true,
		},
		{
			name: "missing client",
			session: model.Session{
				ID:        "test-session-id",
				Project:   "test-project",
				DevID:     "dev@host",
				Client:    "",
				StartedAt: now,
			},
			wantErr: true,
		},
		{
			name: "zero started_at",
			session: model.Session{
				ID:        "test-session-id",
				Project:   "test-project",
				DevID:     "dev@host",
				Client:    "claude-code",
				StartedAt: time.Time{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.session.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ─── T3.5: repository tests ───────────────────────────────────────────────────

// TestPostgresSessionRepository_CreateSession verifica la creación básica de una sesión.
func TestPostgresSessionRepository_CreateSession(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	now := time.Now().UTC().Truncate(time.Second)
	session := &model.Session{
		ID:        "test-session-001",
		SyncID:    "d1e2f3a4-b5c6-7890-abcd-ef1234567890",
		Project:   "test-project",
		DevID:     "dev@host",
		Client:    "claude-code",
		StartedAt: now,
	}

	err := repo.CreateSession(ctx, session)
	require.NoError(t, err)

	// Verify it can be retrieved
	got, err := repo.GetSession(ctx, "test-session-001")
	require.NoError(t, err)
	assert.Equal(t, session.ID, got.ID)
	assert.Equal(t, session.DevID, got.DevID)
	assert.Equal(t, session.Client, got.Client)
	assert.Equal(t, session.Project, got.Project)
	assert.Nil(t, got.EndedAt, "newly created session should not be ended")
}

// TestPostgresSessionRepository_GetSession_NotFound verifica que GetSession devuelve
// ErrNotFound para IDs desconocidos.
func TestPostgresSessionRepository_GetSession_NotFound(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	_, err := repo.GetSession(ctx, "nonexistent-session-id")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestPostgresSessionRepository_EndSession verifica que EndSession actualiza ended_at y summary.
func TestPostgresSessionRepository_EndSession(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	// Create a session first
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.CreateSession(ctx, &model.Session{
		ID:        "end-test-session",
		SyncID:    "e2f3a4b5-c6d7-8901-bcde-f12345678901",
		Project:   "test-project",
		DevID:     "dev@host",
		Client:    "claude-code",
		StartedAt: now,
	}))

	summary := "Session ended successfully with great results"
	err := repo.EndSession(ctx, "end-test-session", summary)
	require.NoError(t, err)

	got, err := repo.GetSession(ctx, "end-test-session")
	require.NoError(t, err)
	require.NotNil(t, got.EndedAt, "ended_at should be set")
	require.NotNil(t, got.Summary)
	assert.Equal(t, summary, *got.Summary)
}

// TestPostgresSessionRepository_EndSession_NotFound verifica ErrNotFound para sesiones desconocidas.
func TestPostgresSessionRepository_EndSession_NotFound(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	err := repo.EndSession(ctx, "nonexistent-session", "summary")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestPostgresSessionRepository_EnsureManualSaveSession_CreatesRow verifica que la primera
// llamada a EnsureManualSaveSession crea la fila con id='manual-save-{project}'.
func TestPostgresSessionRepository_EnsureManualSaveSession_CreatesRow(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	sessionID, err := repo.EnsureManualSaveSession(ctx, "my-project")
	require.NoError(t, err)
	assert.Equal(t, "manual-save-my-project", sessionID)

	got, err := repo.GetSession(ctx, "manual-save-my-project")
	require.NoError(t, err)
	assert.Equal(t, "manual-save-my-project", got.ID)
	assert.Equal(t, "manual", got.Client)
	assert.Nil(t, got.EndedAt, "manual-save session must never be ended")
}

// TestPostgresSessionRepository_EnsureManualSaveSession_Idempotent verifica que llamadas
// repetidas devuelven el mismo ID sin crear filas duplicadas.
func TestPostgresSessionRepository_EnsureManualSaveSession_Idempotent(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	id1, err := repo.EnsureManualSaveSession(ctx, "idempotent-project")
	require.NoError(t, err)

	id2, err := repo.EnsureManualSaveSession(ctx, "idempotent-project")
	require.NoError(t, err)

	assert.Equal(t, id1, id2)

	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE id = 'manual-save-idempotent-project'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one row must exist")
}

// TestPostgresSessionRepository_ProjectSpellingsStayDistinct proves session
// reads and the lazy manual-save session both key on the literal spelling.
// Deriving the manual-save id from a folded key handed one spelling a session
// owned by another, which the caller's attribution check then rejected.
func TestPostgresSessionRepository_ProjectSpellingsStayDistinct(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	startedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	for i, project := range []string{" Foo_Bar ", "foo/bar"} {
		require.NoError(t, repo.CreateSession(ctx, &model.Session{
			ID:        fmt.Sprintf("variant-session-%d", i),
			SyncID:    fmt.Sprintf("f1000000-0000-0000-0000-%012d", i),
			Project:   project,
			DevID:     "tester",
			Client:    "test",
			StartedAt: startedAt.Add(time.Duration(i) * time.Minute),
		}))
	}

	unrelated, _, err := repo.ListSessionsSince(ctx, "FOO_BAR", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Empty(t, unrelated, "a spelling nobody stored is a different project")

	underscored, _, err := repo.ListSessionsSince(ctx, " Foo_Bar ", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, underscored, 1)
	assert.Equal(t, " Foo_Bar ", underscored[0].Project)

	slashed, _, err := repo.ListSessionsSince(ctx, "foo/bar", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, slashed, 1)
	assert.Equal(t, "foo/bar", slashed[0].Project)

	firstID, err := repo.EnsureManualSaveSession(ctx, " Foo_Bar ")
	require.NoError(t, err)
	secondID, err := repo.EnsureManualSaveSession(ctx, "foo/bar")
	require.NoError(t, err)
	assert.Equal(t, "manual-save- Foo_Bar ", firstID)
	assert.Equal(t, "manual-save-foo/bar", secondID)
}

// ─── T4.8: ListSessionsSince ─────────────────────────────────────────────────

// TestPostgresSessionRepository_ListSessionsSince verifica que ListSessionsSince devuelve
// las sesiones con synced_at >= since, ordenadas por (synced_at, sync_id) ASC.
func TestPostgresSessionRepository_ListSessionsSince(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert 3 sessions at staggered started_at values. We manipulate synced_at
	// directly so we can control the cutoff precisely.
	insertSess := func(id, syncID string, startedAt time.Time, syncedAt time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, 'test-proj', '', 'dev', 'claude-code', $3, $4)`,
			id, syncID, startedAt, syncedAt)
		require.NoError(t, err)
	}

	// Session 1: synced at T+0s (before cutoff)
	insertSess("sess-list-1", "a1000000-0000-0000-0000-000000000001", base, base)
	// Session 2: synced at T+60s (after cutoff)
	insertSess("sess-list-2", "a1000000-0000-0000-0000-000000000002", base.Add(time.Minute), base.Add(60*time.Second))
	// Session 3: synced at T+120s (after cutoff)
	insertSess("sess-list-3", "a1000000-0000-0000-0000-000000000003", base.Add(2*time.Minute), base.Add(120*time.Second))

	// Cutoff is base+30s — sessions 2 and 3 have synced_at after the cutoff.
	cutoff := base.Add(30 * time.Second)

	got, hasMore, err := repo.ListSessionsSince(ctx, "test-proj", cutoff, model.PullCursor{}, model.MaxPullLimit)
	require.NoError(t, err)
	require.Len(t, got, 2, "should return exactly 2 sessions after cutoff")
	assert.False(t, hasMore)

	// Ordered by (synced_at, sync_id) ASC, which matches started_at order here.
	assert.Equal(t, "sess-list-2", got[0].ID)
	assert.Equal(t, "sess-list-3", got[1].ID)
}

func TestPostgresSessionRepository_ListSessionsSince_FiltersBlockedProject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	sessionRepo := NewPostgresSessionRepository(pool)
	blockRepo := NewPostgresProjectBlockRepository(pool)
	syncedAt := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	insertSess := func(id, syncID, project string) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, $3, '', 'dev', 'hive-daemon', $4, $4)`,
			id, syncID, project, syncedAt)
		require.NoError(t, err)
	}

	insertSess("sess-blocked", "ab100000-0000-0000-0000-000000000001", "Blocked Project")
	insertSess("sess-open", "ab100000-0000-0000-0000-000000000002", "open-project")
	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Project",
		CanonicalProjectKey: "Blocked Project",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "duplicate",
		Confirmation:        "blocked-project",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	blocked, blockedHasMore, err := sessionRepo.ListSessionsSince(ctx, "Blocked Project", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Empty(t, blocked)
	assert.False(t, blockedHasMore)

	open, openHasMore, err := sessionRepo.ListSessionsSince(ctx, "open-project", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.False(t, openHasMore)
	assert.Equal(t, "sess-open", open[0].ID)
}

// TestPostgresSessionRepository_ListSessionsSince_IncludesExactWatermark verifica
// que el pull bounded no pierde sesiones cuyo synced_at es exactamente igual al
// watermark `since`; el cursor compuesto debe evitar duplicados al reanudar.
func TestPostgresSessionRepository_ListSessionsSince_IncludesExactWatermark(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	watermark := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	insertSess := func(id, syncID string, syncedAt time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, 'watermark-boundary-proj', '', 'dev', 'claude-code', $3, $3)`,
			id, syncID, syncedAt)
		require.NoError(t, err)
	}

	insertSess("sess-before-watermark", "a1100000-0000-0000-0000-000000000001", watermark.Add(-time.Second))
	insertSess("sess-at-watermark", "a1100000-0000-0000-0000-000000000002", watermark)
	insertSess("sess-after-watermark", "a1100000-0000-0000-0000-000000000003", watermark.Add(time.Second))

	page1, hasMore1, err := repo.ListSessionsSince(ctx, "watermark-boundary-proj", watermark, model.PullCursor{}, 1)
	require.NoError(t, err)
	require.True(t, hasMore1, "row after the exact-watermark session should require another page")
	require.Len(t, page1, 1)
	assert.Equal(t, "sess-at-watermark", page1[0].ID, "exact-watermark session must be included")

	cursor := model.PullCursor{SyncedAt: page1[0].SyncedAt, SyncID: page1[0].SyncID}
	page2, hasMore2, err := repo.ListSessionsSince(ctx, "watermark-boundary-proj", watermark, cursor, 1)
	require.NoError(t, err)
	assert.False(t, hasMore2)
	require.Len(t, page2, 1)
	assert.Equal(t, "sess-after-watermark", page2[0].ID, "cursor must resume after the exact-watermark session without duplicating it")
}

// TestPostgresSessionRepository_ListSessionsSince_ZeroCutoff verifica que una cutoff
// zero devuelve todas las sesiones.
func TestPostgresSessionRepository_ListSessionsSince_ZeroCutoff(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"sess-z-1", "sess-z-2"} {
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at)
			VALUES ($1, $2, 'zero-proj', '', 'dev', 'client', $3)`,
			id, fmt.Sprintf("b1000000-0000-0000-0000-%012d", i+1), base.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
	}

	got, _, err := repo.ListSessionsSince(ctx, "zero-proj", time.Time{}, model.PullCursor{}, model.MaxPullLimit)
	require.NoError(t, err)
	// At minimum the 2 inserted rows are returned (other tests also insert to same DB
	// but each test uses a fresh container so isolation is guaranteed)
	assert.GreaterOrEqual(t, len(got), 2)
}

// R2-CRIT-4 — ListSessionsSince must filter by project to prevent tenant data
// leak across daemons syncing distinct projects.
func TestPostgresSessionRepository_ListSessionsSince_FiltersByProject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	// Two sessions in project A, one in project B — all synced after the zero cutoff.
	insert := func(id, syncID, project string, startedAt time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, $3, '', 'dev', 'claude-code', $4, $4)`,
			id, syncID, project, startedAt)
		require.NoError(t, err)
	}
	insert("sess-A-1", "c1000000-0000-0000-0000-000000000001", "alpha", base)
	insert("sess-A-2", "c1000000-0000-0000-0000-000000000002", "alpha", base.Add(time.Minute))
	insert("sess-B-1", "c1000000-0000-0000-0000-000000000003", "beta", base.Add(2*time.Minute))

	gotAlpha, _, err := repo.ListSessionsSince(ctx, "alpha", time.Time{}, model.PullCursor{}, model.MaxPullLimit)
	require.NoError(t, err)
	require.Len(t, gotAlpha, 2, "alpha must return only its own sessions")
	for _, s := range gotAlpha {
		assert.Equal(t, "alpha", s.Project)
	}

	gotBeta, _, err := repo.ListSessionsSince(ctx, "beta", time.Time{}, model.PullCursor{}, model.MaxPullLimit)
	require.NoError(t, err)
	require.Len(t, gotBeta, 1)
	assert.Equal(t, "beta", gotBeta[0].Project)
}

// ─── CRITICAL FIX #1 — unbounded pull when the client does not opt in ────────
//
// TestPostgresSessionRepository_ListSessionsSince_UnboundedLimitReturnsAllRows
// verifies that limit<=0 (model.UnboundedPullLimit) performs a single unbounded
// pull — no LIMIT clause, all matching rows returned, hasMore always false.
// Mirrors the memory-channel contract in postgres_memory_test.go.
func TestPostgresSessionRepository_ListSessionsSince_UnboundedLimitReturnsAllRows(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	const totalRows = 150 // deliberately > model.MaxPullLimit (100)

	for i := 0; i < totalRows; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, 'unbounded-sess-test', '', 'dev', 'claude-code', $3, $3)`,
			fmt.Sprintf("sess-unbounded-%d", i),
			fmt.Sprintf("a9000000-0000-0000-0000-%012d", i),
			base.Add(time.Duration(i)*time.Second))
		require.NoError(t, err)
	}

	results, hasMore, err := repo.ListSessionsSince(ctx, "unbounded-sess-test", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.False(t, hasMore, "unbounded pull must always report hasMore=false")
	assert.Len(t, results, totalRows, "unbounded pull must return every matching row in a single page, uncapped by MaxPullLimit")
}

// ─── PR 2a fresh-review WARNING #2 — missing keyset pagination repository tests ──
//
// These tests were flagged as a gap by fresh-context review: ListSessionsSince's
// keyset pagination (limit+1 trim, cursor walk, synced_at tiebreak) had no
// direct repository-level coverage. Docker-gated; CI-deferred in environments
// without a Docker daemon.

// TestPostgresSessionRepository_ListSessionsSince_HasMoreTrimsToLimit verifies
// that when more rows exist than the requested page size, ListSessionsSince
// reports hasMore=true and trims the result to exactly `limit` rows.
func TestPostgresSessionRepository_ListSessionsSince_HasMoreTrimsToLimit(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	const totalRows = 5
	const pageLimit = 3

	for i := 0; i < totalRows; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, 'trim-sess-test', '', 'dev', 'claude-code', $3, $3)`,
			fmt.Sprintf("sess-trim-%d", i),
			fmt.Sprintf("d1000000-0000-0000-0000-%012d", i),
			base.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
	}

	results, hasMore, err := repo.ListSessionsSince(ctx, "trim-sess-test", time.Time{}, model.PullCursor{}, pageLimit)
	require.NoError(t, err)
	assert.True(t, hasMore, "5 rows with a page limit of 3 must report hasMore=true")
	assert.Len(t, results, pageLimit, "result must be trimmed to exactly the requested limit, not limit+1")
}

// TestPostgresSessionRepository_ListSessionsSince_FullCursorWalkVisitsEveryRowOnce
// walks the full backlog page by page using the returned cursor, and asserts no
// row is skipped or duplicated across the walk, and the walk terminates.
func TestPostgresSessionRepository_ListSessionsSince_FullCursorWalkVisitsEveryRowOnce(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	const totalRows = 11
	const pageLimit = 4

	wantIDs := make(map[string]bool, totalRows)
	for i := 0; i < totalRows; i++ {
		id := fmt.Sprintf("sess-walk-%d", i)
		wantIDs[id] = true
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, 'walk-sess-test', '', 'dev', 'claude-code', $3, $3)`,
			id,
			fmt.Sprintf("e1000000-0000-0000-0000-%012d", i),
			base.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
	}

	seen := make(map[string]int, totalRows)
	cursor := model.PullCursor{}
	pages := 0
	const maxPages = totalRows + 2 // safety bound so a broken cursor can't loop forever

	for {
		pages++
		require.LessOrEqual(t, pages, maxPages, "cursor walk did not terminate — possible infinite loop")

		page, hasMore, err := repo.ListSessionsSince(ctx, "walk-sess-test", time.Time{}, cursor, pageLimit)
		require.NoError(t, err)

		for _, s := range page {
			seen[s.ID]++
		}

		if !hasMore || len(page) == 0 {
			break
		}

		last := page[len(page)-1]
		cursor = model.PullCursor{SyncedAt: last.SyncedAt, SyncID: last.SyncID}
	}

	assert.Len(t, seen, totalRows, "every row must be visited exactly once across the full walk")
	for id := range wantIDs {
		assert.Equal(t, 1, seen[id], "session id %s must appear exactly once (no skip, no duplicate)", id)
	}
}

// TestPostgresSessionRepository_ListSessionsSince_TiebreakOnIdenticalSyncedAt
// verifies that two rows sharing the exact same synced_at are ordered
// deterministically by sync_id and that neither is skipped nor duplicated
// across a page boundary.
func TestPostgresSessionRepository_ListSessionsSince_TiebreakOnIdenticalSyncedAt(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	tie := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	syncIDLow := "f1000000-0000-0000-0000-000000000001"
	syncIDHigh := "f1000000-0000-0000-0000-000000000002"

	// Insert out of sync_id order on purpose, both sharing the exact same synced_at.
	for _, syncID := range []string{syncIDHigh, syncIDLow} {
		_, err := pool.Exec(ctx, `
			INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, synced_at)
			VALUES ($1, $2, 'tiebreak-sess-test', '', 'dev', 'claude-code', $3, $3)`,
			"sess-tie-"+syncID, syncID, tie)
		require.NoError(t, err)
	}

	// Page 1: limit=1 must return exactly the lower sync_id (tiebreak order) and
	// report hasMore=true because a second row with the same synced_at remains.
	page1, hasMore1, err := repo.ListSessionsSince(ctx, "tiebreak-sess-test", time.Time{}, model.PullCursor{}, 1)
	require.NoError(t, err)
	require.True(t, hasMore1, "second row with identical synced_at must not be silently dropped")
	require.Len(t, page1, 1)
	assert.Equal(t, syncIDLow, page1[0].SyncID, "tiebreak must order the lower sync_id first")

	// Page 2: resume from page 1's cursor — must return exactly the remaining row,
	// not skip it and not repeat page 1's row.
	cursor := model.PullCursor{SyncedAt: page1[0].SyncedAt, SyncID: page1[0].SyncID}
	page2, hasMore2, err := repo.ListSessionsSince(ctx, "tiebreak-sess-test", time.Time{}, cursor, 1)
	require.NoError(t, err)
	assert.False(t, hasMore2, "backlog is fully drained after the second row")
	require.Len(t, page2, 1)
	assert.Equal(t, syncIDHigh, page2[0].SyncID, "second page must return the remaining tied row, not repeat or skip it")
}

// R3-FIX-1 — UpsertSession on a manual-save-* row created lazily with dev_id='unknown'
// MUST refresh dev_id/client when a real daemon push arrives with attributed values,
// but MUST NOT downgrade an attributed row back to placeholder values.
func TestPostgresSessionRepository_UpsertSession_ManualSave_RefreshesDevID(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	// Step 1 — server lazily creates manual-save-alpha with placeholder values.
	id, err := repo.EnsureManualSaveSession(ctx, "alpha")
	require.NoError(t, err)
	require.Equal(t, "manual-save-alpha", id)

	// Capture initial started_at so we can assert LEAST() preservation later.
	var initialStartedAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT started_at FROM sessions WHERE id = $1`, id).Scan(&initialStartedAt))

	// Step 2 — daemon pushes its OWN manual-save row with attributed dev_id/client.
	// Use a started_at slightly LATER than the lazy row so LEAST() should keep
	// the earlier (initial) value.
	t0 := initialStartedAt.Add(5 * time.Minute)
	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID:        "manual-save-alpha",
		SyncID:    "11111111-2222-3333-4444-555555555555",
		Project:   "alpha",
		Directory: "",
		DevID:     "andres",
		Client:    "claude-code",
		StartedAt: t0,
	}))

	// Verify dev_id/client were refreshed AND started_at preserved as LEAST().
	var devID, client string
	var startedAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT dev_id, client, started_at FROM sessions WHERE id = $1`, "manual-save-alpha").
		Scan(&devID, &client, &startedAt))

	assert.Equal(t, "andres", devID, "dev_id MUST be refreshed from placeholder 'unknown' to attributed value")
	assert.Equal(t, "claude-code", client, "client MUST be refreshed from 'manual' to attributed value")
	assert.True(t, startedAt.Equal(initialStartedAt) || startedAt.Before(initialStartedAt),
		"started_at MUST keep LEAST(prev, t0) — preserve the earlier value")

	// Step 4 — a fresh lazy-creation arriving SECOND must NOT downgrade dev_id back
	// to 'unknown'. We simulate this by an UpsertSession with placeholder values.
	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID:        "manual-save-alpha",
		SyncID:    "11111111-2222-3333-4444-666666666666",
		Project:   "alpha",
		Directory: "",
		DevID:     "unknown",
		Client:    "manual",
		StartedAt: t0.Add(time.Hour),
	}))

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT dev_id, client FROM sessions WHERE id = $1`, "manual-save-alpha").
		Scan(&devID, &client))
	assert.Equal(t, "andres", devID, "MUST NOT downgrade attributed dev_id back to 'unknown'")
	assert.Equal(t, "claude-code", client, "MUST NOT downgrade attributed client back to 'manual'")
}

// R4-FIX-3 — UpsertSession on a manual-save-* row that is ALREADY attributed
// MUST NOT downgrade dev_id/client to the empty string when EXCLUDED carries
// "" instead of the placeholder defaults. Pre-fix the CASE-WHEN guard accepted
// any value other than 'unknown'/'manual', including empty, silently corrupting
// an attributed row.
func TestPostgresSessionRepository_UpsertSession_ManualSave_EmptyDevIDDoesNotDowngrade(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	// Step 1 — lazy-create + attribute the row to dev_id='andres', client='claude-code'.
	id, err := repo.EnsureManualSaveSession(ctx, "alpha")
	require.NoError(t, err)
	require.Equal(t, "manual-save-alpha", id)

	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID:        "manual-save-alpha",
		SyncID:    "ddddeeee-0000-0000-0000-000000000001",
		Project:   "alpha",
		DevID:     "andres",
		Client:    "claude-code",
		StartedAt: time.Now().UTC(),
	}))

	// Step 2 — push with EMPTY dev_id MUST NOT replace 'andres' with ''.
	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID:        "manual-save-alpha",
		SyncID:    "ddddeeee-0000-0000-0000-000000000002",
		Project:   "alpha",
		DevID:     "", // empty — must be rejected as a refresh source
		Client:    "claude-code",
		StartedAt: time.Now().UTC(),
	}))

	var devID, client string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT dev_id, client FROM sessions WHERE id = $1`, "manual-save-alpha").
		Scan(&devID, &client))
	assert.Equal(t, "andres", devID, "empty dev_id MUST NOT downgrade attributed value")
	assert.Equal(t, "claude-code", client, "client must remain attributed")

	// Step 3 — push with EMPTY client MUST NOT replace 'claude-code' with ''.
	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID:        "manual-save-alpha",
		SyncID:    "ddddeeee-0000-0000-0000-000000000003",
		Project:   "alpha",
		DevID:     "andres",
		Client:    "", // empty — must be rejected as a refresh source
		StartedAt: time.Now().UTC(),
	}))

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT dev_id, client FROM sessions WHERE id = $1`, "manual-save-alpha").
		Scan(&devID, &client))
	assert.Equal(t, "andres", devID)
	assert.Equal(t, "claude-code", client, "empty client MUST NOT downgrade attributed value")
}

// ─── T3.6: concurrent EnsureManualSaveSession ─────────────────────────────────

// TestPostgresSessionRepository_EnsureManualSaveSession_Concurrent verifica que N goroutines
// concurrentes llamando EnsureManualSaveSession con el mismo proyecto crean EXACTAMENTE UNA
// fila y no retornan errores. ON CONFLICT (id) DO NOTHING garantiza idempotencia.
func TestPostgresSessionRepository_EnsureManualSaveSession_Concurrent(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)

	const goroutines = 20
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = repo.EnsureManualSaveSession(ctx, "concurrent-project")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d returned an error", i)
	}

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE id = 'manual-save-concurrent-project'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one manual-save row must exist after concurrent inserts")
}

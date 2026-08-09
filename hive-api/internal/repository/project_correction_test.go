package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The daemon is the sole authority on project identity. When its local identity
// migration rewrites a row's project ("Foo.Bar" -> "foo-bar"), it re-pushes the
// same sync_id carrying the new literal. The server must accept that correction
// on the project column, and on nothing else: every other column keeps the
// idempotent "first write wins" semantics the sync protocol relies on.
//
// These tests run against real Postgres because the whole behaviour lives in an
// ON CONFLICT clause.

type sessionRow struct {
	ID        string
	SyncID    string
	Project   string
	Directory string
	DevID     string
	Client    string
	StartedAt time.Time
	EndedAt   *time.Time
	Summary   *string
	SyncedAt  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func readSessionRow(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id string) sessionRow {
	t.Helper()
	var row sessionRow
	err := pool.QueryRow(ctx, `
		SELECT id, sync_id::text, project, directory, dev_id, client,
		       started_at, ended_at, summary, synced_at, created_at, updated_at
		FROM sessions WHERE id = $1`, id).Scan(
		&row.ID, &row.SyncID, &row.Project, &row.Directory, &row.DevID, &row.Client,
		&row.StartedAt, &row.EndedAt, &row.Summary, &row.SyncedAt, &row.CreatedAt, &row.UpdatedAt)
	require.NoError(t, err)
	return row
}

// TestUpsertSession_RepushUnderCorrectedProjectMovesOnlyTheProject covers the
// regular (UUID-id) branch, which conflicts on sync_id — the branch the daemon
// uses for every real session.
func TestUpsertSession_RepushUnderCorrectedProjectMovesOnlyTheProject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	ended := started.Add(30 * time.Minute)
	summary := "original summary"

	original := &model.Session{
		ID:        "0a1e8400-e29b-41d4-a716-446655440001",
		SyncID:    "0a1e8400-e29b-41d4-a716-446655440101",
		Project:   "Foo.Bar",
		Directory: "/home/dev/foo.bar",
		DevID:     "dev-1",
		Client:    "claude",
		StartedAt: started,
		EndedAt:   &ended,
		Summary:   &summary,
	}
	require.NoError(t, repo.UpsertSession(ctx, original))
	before := readSessionRow(ctx, t, pool, original.ID)
	require.Equal(t, "Foo.Bar", before.Project)

	// The daemon folded the project locally and re-pushes the same sync_id.
	// Every other field also differs, and every other field must be ignored.
	corrected := &model.Session{
		ID:          original.ID,
		SyncID:      original.SyncID,
		Project:     "foo-bar",
		FromProject: "Foo.Bar",
		Directory:   "/somewhere/else",
		DevID:       "dev-2",
		Client:      "opencode",
		StartedAt:   started.Add(24 * time.Hour),
		EndedAt:     nil,
		Summary:     nil,
	}
	require.NoError(t, repo.UpsertSession(ctx, corrected))

	after := readSessionRow(ctx, t, pool, original.ID)
	assert.Equal(t, "foo-bar", after.Project, "the daemon's corrected project must win")
	assert.Equal(t, before.SyncID, after.SyncID)
	assert.Equal(t, before.Directory, after.Directory, "directory must not be touched by a re-push")
	assert.Equal(t, before.DevID, after.DevID)
	assert.Equal(t, before.Client, after.Client)
	assert.Equal(t, before.StartedAt, after.StartedAt)
	assert.Equal(t, before.EndedAt, after.EndedAt)
	assert.Equal(t, before.Summary, after.Summary)
	assert.Equal(t, before.SyncedAt, after.SyncedAt)
	assert.Equal(t, before.CreatedAt, after.CreatedAt)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)
}

// TestCreateSession_RepushUnderCorrectedProjectMovesOnlyTheProject covers the
// second sync_id-keyed conflict branch in the session repository.
func TestCreateSession_RepushUnderCorrectedProjectMovesOnlyTheProject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	original := &model.Session{
		ID:        "0b1e8400-e29b-41d4-a716-446655440001",
		SyncID:    "0b1e8400-e29b-41d4-a716-446655440101",
		Project:   "Foo.Bar",
		Directory: "/home/dev/foo.bar",
		DevID:     "dev-1",
		Client:    "claude",
		StartedAt: started,
	}
	require.NoError(t, repo.CreateSession(ctx, original))
	before := readSessionRow(ctx, t, pool, original.ID)

	corrected := *original
	corrected.Project = "foo-bar"
	corrected.FromProject = "Foo.Bar"
	corrected.Directory = "/somewhere/else"
	corrected.DevID = "dev-2"
	require.NoError(t, repo.CreateSession(ctx, &corrected))

	after := readSessionRow(ctx, t, pool, original.ID)
	assert.Equal(t, "foo-bar", after.Project)
	assert.Equal(t, before.Directory, after.Directory)
	assert.Equal(t, before.DevID, after.DevID)
	assert.Equal(t, before.StartedAt, after.StartedAt)
	assert.Equal(t, before.CreatedAt, after.CreatedAt)
}

// TestUpsertSession_RepushWithoutNamingTheCurrentProjectMovesNothing pins the
// precondition the correction branch was missing. A conflict on sync_id may only
// move `project` when the push NAMES the literal the row currently holds. Taking
// EXCLUDED.project unconditionally relocated whatever row the sync_id happened
// to hit, out of whatever project it happened to sit in — including a
// quarantined one the request never names and no block check ever sees.
//
// This mirrors applyReprojectMutation's `AND project = $3`: a stale or absent
// source matches nothing and moves nothing.
func TestUpsertSession_RepushWithoutNamingTheCurrentProjectMovesNothing(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	original := &model.Session{
		ID:        "0e1e8400-e29b-41d4-a716-446655440001",
		SyncID:    "0e1e8400-e29b-41d4-a716-446655440101",
		Project:   "Foo.Bar",
		DevID:     "dev-1",
		Client:    "claude",
		StartedAt: started,
	}
	require.NoError(t, repo.UpsertSession(ctx, original))

	cases := []struct {
		name        string
		fromProject string
	}{
		{"no source named", ""},
		{"a source the row does not hold", "some-other-project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			moved := *original
			moved.Project = "foo-bar"
			moved.FromProject = tc.fromProject
			require.NoError(t, repo.UpsertSession(ctx, &moved))

			after := readSessionRow(ctx, t, pool, original.ID)
			assert.Equal(t, "Foo.Bar", after.Project,
				"a push that does not name the row's current project must move nothing")
		})
	}
}

// TestUpsertSession_ARowInsideAQuarantineCannotBeMovedOut is the symmetric half
// of the memory guarantee. The target project of a session push is the request
// project, which the quarantine precheck already covers; the SOURCE was covered
// by nothing, so the very flow this change enables — the daemon folds
// "Foo.Bar" -> "foo-bar" and re-pushes — carried every quarantined session
// straight out of the quarantine.
func TestUpsertSession_ARowInsideAQuarantineCannotBeMovedOut(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	original := &model.Session{
		ID:        "0f1e8400-e29b-41d4-a716-446655440001",
		SyncID:    "0f1e8400-e29b-41d4-a716-446655440101",
		Project:   "Foo.Bar",
		DevID:     "dev-1",
		Client:    "claude",
		StartedAt: started,
	}
	require.NoError(t, repo.UpsertSession(ctx, original))
	blockProject(t, pool, "Foo.Bar", "Foo.Bar")

	moved := *original
	moved.Project = "foo-bar"
	moved.FromProject = "Foo.Bar"
	err := repo.UpsertSession(ctx, &moved)

	require.ErrorIs(t, err, ErrProjectBlocked)
	after := readSessionRow(ctx, t, pool, original.ID)
	assert.Equal(t, "Foo.Bar", after.Project, "a quarantined row must stay in its quarantine")
}

// TestUpsertSession_SentinelBranchesRefuseAProjectMove pins the deliberate
// asymmetry: the manual-save-* and legacy-pre-lifecycle-* branches conflict on
// `id`, and both ids embed the project literal ("manual-save-{project}"). A
// correct daemon that renames the project therefore produces a DIFFERENT id —
// a new row, never a conflict. Accepting EXCLUDED.project on those branches
// would add no correction path and would open one: a push of
// id="manual-save-A" carrying project="B" would move A's sentinel into B.
func TestUpsertSession_SentinelBranchesRefuseAProjectMove(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	cases := []struct {
		id           string
		firstSyncID  string
		secondSyncID string
	}{
		{"manual-save-Foo.Bar", "0d1e8400-e29b-41d4-a716-446655440001", "0d1e8400-e29b-41d4-a716-446655440002"},
		{"legacy-pre-lifecycle-Foo.Bar", "0d1e8400-e29b-41d4-a716-446655440003", "0d1e8400-e29b-41d4-a716-446655440004"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			require.NoError(t, repo.UpsertSession(ctx, &model.Session{
				ID:        tc.id,
				SyncID:    tc.firstSyncID,
				Project:   "Foo.Bar",
				DevID:     "dev-1",
				Client:    "claude",
				StartedAt: started,
			}))

			require.NoError(t, repo.UpsertSession(ctx, &model.Session{
				ID:        tc.id,
				SyncID:    tc.secondSyncID,
				Project:   "foo-bar",
				DevID:     "dev-1",
				Client:    "claude",
				StartedAt: started,
			}))

			after := readSessionRow(ctx, t, pool, tc.id)
			assert.Equal(t, "Foo.Bar", after.Project,
				"an id-keyed sentinel row must keep the project its id embeds")
		})
	}
}

// TestPromptUpsert_RepushUnderCorrectedProjectMovesOnlyTheProject covers the
// single prompt conflict branch, and pins that the corrected re-push is still
// NOT counted as a newly pushed prompt.
func TestPromptUpsert_RepushUnderCorrectedProjectMovesOnlyTheProject(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresPromptRepository(pool)
	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	const syncID = "0c1e8400-e29b-41d4-a716-446655440101"

	saved, err := repo.Upsert(ctx, &model.Prompt{
		SyncID:    syncID,
		Project:   "Foo.Bar",
		Content:   "original content",
		CreatedBy: "dev-1",
		CreatedAt: createdAt,
	})
	require.NoError(t, err)
	require.True(t, saved, "the first write is an insert")

	saved, err = repo.Upsert(ctx, &model.Prompt{
		SyncID:      syncID,
		Project:     "foo-bar",
		FromProject: "Foo.Bar",
		Content:     "rewritten content",
		CreatedBy:   "dev-2",
		CreatedAt:   createdAt.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.False(t, saved, "a correction is not a new prompt: prompts_pushed must not count it")

	var project, content, createdBy string
	var storedCreatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT project, content, created_by, created_at FROM user_prompts WHERE sync_id = $1`, syncID).
		Scan(&project, &content, &createdBy, &storedCreatedAt))

	assert.Equal(t, "foo-bar", project, "the daemon's corrected project must win")
	assert.Equal(t, "original content", content, "prompts stay immutable in every other column")
	assert.Equal(t, "dev-1", createdBy)
	assert.Equal(t, createdAt, storedCreatedAt.UTC())
}

// TestPromptUpsert_RepushWithoutNamingTheCurrentProjectMovesNothing is the
// prompt half of the from-project precondition. It also pins that a re-push
// which moves nothing is still not counted as a newly pushed prompt.
func TestPromptUpsert_RepushWithoutNamingTheCurrentProjectMovesNothing(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresPromptRepository(pool)
	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	cases := []struct {
		name        string
		syncID      string
		fromProject string
	}{
		{"no source named", "0e2e8400-e29b-41d4-a716-446655440101", ""},
		{"a source the row does not hold", "0e2e8400-e29b-41d4-a716-446655440102", "some-other-project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saved, err := repo.Upsert(ctx, &model.Prompt{
				SyncID:    tc.syncID,
				Project:   "Foo.Bar",
				Content:   "original content",
				CreatedBy: "dev-1",
				CreatedAt: createdAt,
			})
			require.NoError(t, err)
			require.True(t, saved)

			saved, err = repo.Upsert(ctx, &model.Prompt{
				SyncID:      tc.syncID,
				Project:     "foo-bar",
				FromProject: tc.fromProject,
				Content:     "rewritten content",
				CreatedBy:   "dev-2",
				CreatedAt:   createdAt,
			})
			require.NoError(t, err)
			assert.False(t, saved, "a re-push is never a new prompt")

			var project string
			require.NoError(t, pool.QueryRow(ctx,
				`SELECT project FROM user_prompts WHERE sync_id = $1`, tc.syncID).Scan(&project))
			assert.Equal(t, "Foo.Bar", project,
				"a push that does not name the row's current project must move nothing")
		})
	}
}

// TestPromptUpsert_ARowInsideAQuarantineCannotBeMovedOut mirrors the session
// guarantee: the source end of a relocation is checked, not just the target.
func TestPromptUpsert_ARowInsideAQuarantineCannotBeMovedOut(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresPromptRepository(pool)
	createdAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	const syncID = "0e3e8400-e29b-41d4-a716-446655440101"

	_, err := repo.Upsert(ctx, &model.Prompt{
		SyncID:    syncID,
		Project:   "Foo.Bar",
		Content:   "original content",
		CreatedBy: "dev-1",
		CreatedAt: createdAt,
	})
	require.NoError(t, err)
	blockProject(t, pool, "Foo.Bar", "Foo.Bar")

	_, err = repo.Upsert(ctx, &model.Prompt{
		SyncID:      syncID,
		Project:     "foo-bar",
		FromProject: "Foo.Bar",
		Content:     "rewritten content",
		CreatedBy:   "dev-2",
		CreatedAt:   createdAt,
	})

	require.ErrorIs(t, err, ErrProjectBlocked)
	var project string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT project FROM user_prompts WHERE sync_id = $1`, syncID).Scan(&project))
	assert.Equal(t, "Foo.Bar", project, "a quarantined row must stay in its quarantine")
}

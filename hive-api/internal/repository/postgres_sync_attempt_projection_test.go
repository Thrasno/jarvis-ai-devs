package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresSyncAttemptRepository_UserSyncProjectionUsesCanonicalCompletedAttempts(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttempts(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	const alice = "00000000-0000-0000-0000-000000000021"
	const bob = "00000000-0000-0000-0000-000000000022"
	const carol = "00000000-0000-0000-0000-000000000023"
	_, err := pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES
		($1, 'alice', 'alice@example.com', 'hash', true),
		($2, 'bob', 'bob@example.com', 'hash', false),
		($3, 'carol', 'carol@example.com', 'hash', true)`, alice, bob, carol)
	require.NoError(t, err)

	source := model.SyncAttemptPortalUserSourceAuthSubject
	_, err = repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "alice-success", DevID: "alice-device", Project: "project", StartedAt: now.Add(-2 * time.Hour), EndedAt: projectionTimePtr(now.Add(-2 * time.Hour)), Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: projectionStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "alice-failure", DevID: "alice-device", Project: "project", StartedAt: now.Add(-time.Hour), EndedAt: projectionTimePtr(now.Add(-time.Hour)), Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: projectionStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "bob-success", DevID: "bob-device", Project: "project", StartedAt: now.Add(-time.Hour), EndedAt: projectionTimePtr(now.Add(-time.Hour)), Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: projectionStringPtr(bob), PortalUserSource: &source},
		{AttemptID: "unresolved", DevID: "carol@example.com", Project: "project", StartedAt: now.Add(-time.Hour), EndedAt: projectionTimePtr(now.Add(-time.Hour)), Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "incomplete", DevID: "carol-device", Project: "project", StartedAt: now, Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: projectionStringPtr(carol), PortalUserSource: &source},
	})
	require.NoError(t, err)

	projection, err := repo.UserSyncProjection(ctx, now)

	require.NoError(t, err)
	rows := projection.Rows
	require.Len(t, rows, 3)
	byUser := userSyncProjectionRowsByID(rows)
	assert.Equal(t, model.SyncAttemptOutcomeFailure, *byUser[alice].LatestOutcome)
	assert.True(t, byUser[alice].LatestEndedAt.Equal(now.Add(-time.Hour)))
	assert.True(t, byUser[alice].LatestSuccessEndedAt.Equal(now.Add(-2*time.Hour)))
	assert.False(t, byUser[bob].IsActive)
	assert.Nil(t, byUser[carol].LatestEndedAt)
	assert.Nil(t, byUser[carol].LatestSuccessEndedAt)
}

func TestPostgresSyncAttemptRepository_UserSyncProjectionOrdersTiesAndRetainsFutureLatestAttempt(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttempts(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	const dana = "00000000-0000-0000-0000-000000000024"
	const erin = "00000000-0000-0000-0000-000000000025"
	_, err := pool.Exec(ctx, `INSERT INTO users (id, username, email, password) VALUES
		($1, 'dana', 'dana@example.com', 'hash'), ($2, 'erin', 'erin@example.com', 'hash')`, dana, erin)
	require.NoError(t, err)

	source := model.SyncAttemptPortalUserSourceAuthSubject
	tieTime := now.Add(-time.Hour)
	_, err = repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "tie-a", DevID: "dana-device", Project: "project", StartedAt: tieTime, EndedAt: &tieTime, Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: projectionStringPtr(dana), PortalUserSource: &source},
		{AttemptID: "tie-z", DevID: "dana-device", Project: "project", StartedAt: tieTime, EndedAt: &tieTime, Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: projectionStringPtr(dana), PortalUserSource: &source},
		{AttemptID: "erin-success", DevID: "erin-device", Project: "project", StartedAt: now.Add(-time.Hour), EndedAt: projectionTimePtr(now.Add(-time.Hour)), Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: projectionStringPtr(erin), PortalUserSource: &source},
		{AttemptID: "erin-future", DevID: "erin-device", Project: "project", StartedAt: now.Add(time.Hour), EndedAt: projectionTimePtr(now.Add(time.Hour)), Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: projectionStringPtr(erin), PortalUserSource: &source},
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE sync_attempt_logs SET ingested_at = $1 WHERE attempt_id IN ('tie-a', 'tie-z')`, now)
	require.NoError(t, err)

	projection, err := repo.UserSyncProjection(ctx, now)

	require.NoError(t, err)
	byUser := userSyncProjectionRowsByID(projection.Rows)
	assert.Equal(t, model.SyncAttemptOutcomeFailure, *byUser[dana].LatestOutcome)
	assert.True(t, byUser[erin].LatestEndedAt.Equal(now.Add(time.Hour)))
	assert.True(t, byUser[erin].LatestSuccessEndedAt.Equal(now.Add(-time.Hour)))
}

func userSyncProjectionRowsByID(rows []model.UserSyncProjectionRow) map[string]model.UserSyncProjectionRow {
	byID := make(map[string]model.UserSyncProjectionRow, len(rows))
	for _, row := range rows {
		byID[row.PortalUserID] = row
	}
	return byID
}

func projectionStringPtr(value string) *string { return &value }

func projectionTimePtr(value time.Time) *time.Time { return &value }

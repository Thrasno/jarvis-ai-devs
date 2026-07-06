package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func startPostgresWithProjectBlocks(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	pool, cleanup := startPostgresWithProjectSources(t)
	require.NoError(t, RunMigrations(pool, migrations.ProjectBlocksSQL), "failed to run project blocks migration")
	return pool, cleanup
}

func TestPostgresProjectBlockRepository_BlockStatusAndAck(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)

	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "duplicate garbage project",
		Confirmation:        "Jarvis Dev",
		ExportMarker:        "export-2026-07-05",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)
	require.Equal(t, "jarvis-dev", block.CanonicalProjectKey)
	require.NotEmpty(t, block.CommandID)
	require.NotEmpty(t, block.AckToken)

	status, err := repo.GetByCanonicalKey(ctx, "jarvis-dev")
	require.NoError(t, err)
	require.True(t, status.Blocked)
	require.Equal(t, "duplicate garbage project", status.Reason)

	subject := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}
	delivery, err := repo.EnsureAckDelivery(ctx, block, subject)
	require.NoError(t, err)

	clientSuppliedAppliedAt := time.Date(2000, 7, 5, 21, 0, 0, 0, time.UTC)
	beforeRecord := time.Now().UTC()
	ack, err := repo.RecordAck(ctx, model.ProjectBlockAck{
		CommandID:           block.CommandID,
		CanonicalProjectKey: "jarvis-dev",
		AckToken:            delivery.AckToken,
		Status:              model.ProjectBlockAckApplied,
		Warning:             "quarantined locally from stale client clock",
		AppliedAt:           clientSuppliedAppliedAt,
		AckSubject:          subject,
	})
	afterRecord := time.Now().UTC()
	require.NoError(t, err)
	require.Equal(t, model.ProjectBlockAckApplied, ack.Status)
	require.Equal(t, "quarantined locally from stale client clock", ack.Warning)
	require.NotEqual(t, clientSuppliedAppliedAt, ack.AppliedAt)
	require.WithinDuration(t, afterRecord, ack.AppliedAt, time.Second, "ack applied_at should be assigned near the server recording time")
	require.True(t, ack.AppliedAt.After(beforeRecord.Add(-time.Second)), "ack applied_at should not preserve a stale client timestamp")

	clientSuppliedAppliedAt = time.Date(2035, 7, 5, 21, 0, 0, 0, time.UTC)
	beforeRecord = time.Now().UTC()
	ack, err = repo.RecordAck(ctx, model.ProjectBlockAck{
		CommandID:           block.CommandID,
		CanonicalProjectKey: "jarvis-dev",
		AckToken:            delivery.AckToken,
		Status:              model.ProjectBlockAckApplied,
		Warning:             "quarantined locally from future client clock",
		AppliedAt:           clientSuppliedAppliedAt,
		AckSubject:          subject,
	})
	afterRecord = time.Now().UTC()
	require.NoError(t, err)
	require.Equal(t, model.ProjectBlockAckApplied, ack.Status)
	require.Equal(t, "quarantined locally from future client clock", ack.Warning)
	require.NotEqual(t, clientSuppliedAppliedAt, ack.AppliedAt)
	require.WithinDuration(t, afterRecord, ack.AppliedAt, time.Second, "ack applied_at should be assigned near the server recording time")
	require.True(t, ack.AppliedAt.After(beforeRecord.Add(-time.Second)), "ack applied_at should not preserve a future client timestamp")

	latest, err := repo.LatestAckForCommand(ctx, "jarvis-dev", block.CommandID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, block.CommandID, latest.CommandID)
	require.Equal(t, model.ProjectBlockAckApplied, latest.Status)
	require.Equal(t, ack.AppliedAt, latest.AppliedAt)
}

func TestPostgresProjectBlockRepository_ReblockRotatesCommandID(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	first, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "first reason",
		Confirmation:        "jarvis-dev",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)
	second, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "changed reason",
		Confirmation:        "jarvis-dev",
		ExportMarker:        "export-2",
		ActorUserID:         "admin-2",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.CommandID, second.CommandID)
}

func TestPostgresProjectBlockRepository_AckDeliveryIsSubjectBound(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "duplicate",
		Confirmation:        "jarvis-dev",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)
	subjectA := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}
	subjectB := model.ProjectBlockAckSubject{AuthSubject: "user-2", DaemonID: "daemon-2", Client: "hive-daemon"}

	deliveryA, err := repo.EnsureAckDelivery(ctx, block, subjectA)
	require.NoError(t, err)
	require.NotEmpty(t, deliveryA.AckToken)
	require.NotEqual(t, block.AckToken, deliveryA.AckToken)
	deliveryBAck, err := repo.EnsureAckDelivery(ctx, block, subjectB)
	require.NoError(t, err)
	require.NotEqual(t, deliveryA.AckToken, deliveryBAck.AckToken)
	deliveryAAgain, err := repo.EnsureAckDelivery(ctx, block, subjectA)
	require.NoError(t, err)
	require.Equal(t, deliveryA.AckToken, deliveryAAgain.AckToken)

	got, err := repo.GetAckDelivery(ctx, "jarvis-dev", block.CommandID, subjectA)
	require.NoError(t, err)
	require.Equal(t, deliveryA.AckToken, got.AckToken)
	require.Equal(t, subjectA, got.AckSubject)
}

func TestPostgresProjectBlockRepository_AckDeliveryIsAccountBound(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "duplicate",
		Confirmation:        "jarvis-dev",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)
	accountOnly := model.ProjectBlockAckSubject{AuthSubject: "user-1"}
	sameAccountWithDaemon := model.ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-2", Client: "hive-daemon"}
	differentAccount := model.ProjectBlockAckSubject{AuthSubject: "user-2", DaemonID: "daemon-1", Client: "hive-daemon"}

	delivery, err := repo.EnsureAckDelivery(ctx, block, accountOnly)
	require.NoError(t, err)
	sameAccountDelivery, err := repo.EnsureAckDelivery(ctx, block, sameAccountWithDaemon)
	require.NoError(t, err)
	require.Equal(t, delivery.AckToken, sameAccountDelivery.AckToken)
	differentAccountDelivery, err := repo.EnsureAckDelivery(ctx, block, differentAccount)
	require.NoError(t, err)
	require.NotEqual(t, delivery.AckToken, differentAccountDelivery.AckToken)

	got, err := repo.GetAckDelivery(ctx, "jarvis-dev", block.CommandID, sameAccountWithDaemon)
	require.NoError(t, err)
	require.Equal(t, delivery.AckToken, got.AckToken)
	require.Equal(t, accountOnly.AuthSubject, got.AckSubject.AuthSubject)
}

func TestPostgresMemoryRepository_GetByIDExcludesBlockedProject(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	blockRepo := NewPostgresProjectBlockRepository(pool)
	memoryRepo := NewPostgresMemoryRepository(pool)
	base := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	insertProjectSession(t, pool, "blocked-session", "Blocked Project", base, nil)

	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO memories (sync_id, project, category, title, content, created_by, created_at, updated_at, session_id)
		VALUES ('00000000-0000-0000-0000-000000000402', 'Blocked Project', 'decision', 'blocked memory', 'secret', 'tester', $1, $1, 'blocked-session')
		RETURNING id::text`, base).Scan(&id)
	require.NoError(t, err)

	_, err = blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Project",
		CanonicalProjectKey: "blocked-project",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-project",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	_, err = memoryRepo.GetByID(ctx, id)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresProjectRepository_ListAggregatesExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	blockRepo := NewPostgresProjectBlockRepository(pool)
	projectRepo := NewPostgresProjectRepository(pool)
	base := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)

	insertProjectSession(t, pool, "visible-session", "visible-project", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000301", "visible-project", "visible-session", base, base, nil)
	insertProjectSession(t, pool, "blocked-session", "Blocked Project", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000302", "Blocked Project", "blocked-session", base, base, nil)

	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Project",
		CanonicalProjectKey: "blocked-project",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "Blocked Project",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	got, err := projectRepo.ListAggregates(ctx)
	require.NoError(t, err)
	byName := projectAggregatesByName(got)
	require.Contains(t, byName, "visible-project")
	require.NotContains(t, byName, "Blocked Project")
}

func TestPostgresMemoryRepository_CountByProjectExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	blockRepo := NewPostgresProjectBlockRepository(pool)
	memoryRepo := NewPostgresMemoryRepository(pool)
	base := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	insertProjectSession(t, pool, "visible-count-session", "visible-count", base, nil)
	insertProjectSession(t, pool, "blocked-count-session", "Blocked Count", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000701", "visible-count", "visible-count-session", base, base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000702", "Blocked Count", "blocked-count-session", base, base, nil)
	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Count",
		CanonicalProjectKey: "blocked-count",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-count",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	counts, err := memoryRepo.CountByProject(ctx, model.MemoryFilter{})
	require.NoError(t, err)
	require.Equal(t, []model.ProjectCount{{Project: "visible-count", Count: 1}}, counts)
}

func TestPostgresMemoryRepository_PullSinceExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	blockRepo := NewPostgresProjectBlockRepository(pool)
	memoryRepo := NewPostgresMemoryRepository(pool)
	base := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	insertProjectSession(t, pool, "blocked-pull-session", "Blocked Pull", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000801", "Blocked Pull", "blocked-pull-session", base, base, nil)
	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Pull",
		CanonicalProjectKey: "blocked-pull",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-pull",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	memories, hasMore, err := memoryRepo.PullSince(ctx, "Blocked Pull", time.Time{}, nil, model.PullCursor{}, 10)
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Empty(t, memories)
}

func TestPostgresMemoryRepository_CountLiveActivityExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	blockRepo := NewPostgresProjectBlockRepository(pool)
	memoryRepo := NewPostgresMemoryRepository(pool)
	base := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	insertProjectSession(t, pool, "visible-live-session", "visible-live", base, nil)
	insertProjectSession(t, pool, "blocked-live-session", "Blocked Live", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000501", "visible-live", "visible-live-session", base, base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000502", "Blocked Live", "blocked-live-session", base.Add(time.Minute), base.Add(time.Minute), nil)
	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Live",
		CanonicalProjectKey: "blocked-live",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-live",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	count, newest, err := memoryRepo.CountLiveActivity(ctx, base.Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "00000000-0000-0000-0000-000000000501", newest)
}

func TestPostgresMemoryRepository_CountGrowthByMonthExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	blockRepo := NewPostgresProjectBlockRepository(pool)
	memoryRepo := NewPostgresMemoryRepository(pool)
	now := time.Now().UTC()
	insertProjectSession(t, pool, "visible-growth-session", "visible-growth", now, nil)
	insertProjectSession(t, pool, "blocked-growth-session", "Blocked Growth", now, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000601", "visible-growth", "visible-growth-session", now, now, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000602", "Blocked Growth", "blocked-growth-session", now, now, nil)
	_, err := blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Growth",
		CanonicalProjectKey: "blocked-growth",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-growth",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	growth, err := memoryRepo.CountGrowthByMonth(ctx, 1)
	require.NoError(t, err)
	require.Len(t, growth, 1)
	require.Equal(t, 1, growth[0].Value)
}

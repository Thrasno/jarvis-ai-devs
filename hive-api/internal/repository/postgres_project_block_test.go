package repository

import (
	"context"
	"fmt"
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
	require.NoError(t, RunMigrations(pool, migrations.QuarantineContractSQL), "failed to run quarantine contract migration")
	require.NoError(t, RunMigrations(pool, migrations.DistributedQuarantineSQL), "failed to run distributed quarantine migration")
	require.NoError(t, RunMigrations(pool, migrations.CanonicalProjectRegistrySQL), "failed to run canonical project registry migration")
	return pool, cleanup
}

func TestPostgresProjectBlockRepository_ReadsHistoricalLegacyActions(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	for _, action := range []string{"export_marker", model.ProjectBlockActionPurgeIntent} {
		t.Run(action, func(t *testing.T) {
			canonical := "legacy-" + action
			_, err := pool.Exec(ctx, `
				INSERT INTO project_blocks (project, canonical_project_key, action, reason, confirmation, export_marker, blocked)
				VALUES ($1, $2, $3, 'legacy row', $2, 'legacy export', true)`, canonical, canonical, action)
			require.NoError(t, err)
		})
	}

	for _, action := range []string{"export_marker", model.ProjectBlockActionPurgeIntent} {
		t.Run("read "+action, func(t *testing.T) {
			canonical := "legacy-" + action
			block, err := NewPostgresProjectBlockRepository(pool).GetByCanonicalKey(ctx, canonical)
			require.NoError(t, err)
			require.Equal(t, action, block.Action)
			require.Equal(t, canonical, block.Project)
			require.Equal(t, "legacy export", block.ExportMarker)
			require.EqualValues(t, 1, block.Generation)
		})
	}
}

// TestBlockedProjectPredicateBlocksOnlyTheSpellingItNames proves the read path
// hides exactly the projects an admin quarantined by their stored literal, and
// nothing else. The predicate used to over-approximate across an ASCII fold, so
// blocking "Foo.Bar" also hid "foo/bar" - a different project.
func TestBlockedProjectPredicateBlocksOnlyTheSpellingItNames(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	for i, project := range []string{"Foo.Bar", "foo/bar", "STRAßE", "visible"} {
		sessionID := "canonical-policy-" + project
		insertProjectSession(t, pool, sessionID, project, base, nil)
		insertProjectMemory(t, pool, fmt.Sprintf("00000000-0000-0000-0000-%012d", 900+i), project, sessionID, base, base, nil)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO project_blocks (project, canonical_project_key, action, reason, confirmation, export_marker, blocked)
		VALUES ('Foo.Bar', 'Foo.Bar', 'block', 'exact dotted spelling', 'Foo.Bar', '', true),
		       ('STRAßE', 'STRAßE', 'block', 'exact unicode spelling', 'STRAßE', '', true)`)
	require.NoError(t, err)

	repo := NewPostgresMemoryRepository(pool)
	listed, err := repo.List(ctx, model.MemoryFilter{Limit: 20})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"foo/bar", "visible"}, memoryProjects(listed),
		"only the two quarantined literals are hidden")

	for _, spelling := range []string{"Foo.Bar", "STRAßE"} {
		require.ErrorIs(t, checkProjectBlocked(ctx, pool, spelling), ErrProjectBlocked, spelling)
	}
	for _, spelling := range []string{"foo/bar", "foo-bar", "strasse", "visible"} {
		require.NoError(t, checkProjectBlocked(ctx, pool, spelling), spelling)
	}
}

func memoryProjects(memories []*model.Memory) []string {
	projects := make([]string, 0, len(memories))
	for _, memory := range memories {
		projects = append(projects, memory.Project)
	}
	return projects
}

func TestPostgresProjectBlockRepository_BlockStatusAndAck(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)

	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "Jarvis Dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "duplicate garbage project",
		Confirmation:        "Jarvis Dev",
		ExportMarker:        "export-2026-07-05",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)
	require.Equal(t, "Jarvis Dev", block.CanonicalProjectKey)
	require.NotEmpty(t, block.CommandID)
	require.NotEmpty(t, block.AckToken)

	status, err := repo.GetByCanonicalKey(ctx, "Jarvis Dev")
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
		CanonicalProjectKey: "Jarvis Dev",
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
		CanonicalProjectKey: "Jarvis Dev",
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

	latest, err := repo.LatestAckForCommand(ctx, "Jarvis Dev", block.CommandID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, block.CommandID, latest.CommandID)
	require.Equal(t, model.ProjectBlockAckApplied, latest.Status)
	require.Equal(t, ack.AppliedAt, latest.AppliedAt)
}

// TestPostgresProjectBlockRepository_KeyLookupsAreExact proves the block is
// found by the exact key it was created with, and by nothing else. A folded
// lookup returned another project's quarantine state.
func TestPostgresProjectBlockRepository_KeyLookupsAreExact(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             " Foo.Bar ",
		CanonicalProjectKey: " Foo.Bar ",
		Action:              model.ProjectBlockActionBlock,
		Reason:              "duplicate",
		Confirmation:        " Foo.Bar ",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	stored, err := repo.GetByCanonicalKey(ctx, " Foo.Bar ")
	require.NoError(t, err)
	require.Equal(t, " Foo.Bar ", stored.Project)
	require.Equal(t, " Foo.Bar ", stored.CanonicalProjectKey)

	_, err = repo.GetByCanonicalKey(ctx, "FOO_BAR")
	require.ErrorIs(t, err, ErrNotFound, "an equivalent-looking spelling is a different project")

	progress, err := repo.QuarantineProgress(ctx, " Foo.Bar ", block.Generation, "", 10)
	require.NoError(t, err)
	require.Equal(t, " Foo.Bar ", progress.CanonicalProjectKey)
	require.Equal(t, " Foo.Bar ", progress.Project)

	_, err = repo.QuarantineProgress(ctx, " Foo/Bar ", block.Generation, "", 10)
	require.ErrorIs(t, err, ErrNotFound, "an equivalent-looking spelling is a different project")
}

func TestPostgresProjectBlockRepository_ReblockRotatesCommandID(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	first, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "Jarvis Dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "first reason",
		Confirmation:        "jarvis-dev",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)
	second, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "Jarvis Dev",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "changed reason",
		Confirmation:        "jarvis-dev",
		ExportMarker:        "export-2",
		ActorUserID:         "admin-2",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.CommandID, second.CommandID)
}

func TestPostgresProjectBlockRepository_UnblockAdvancesGenerationAndReleasesCloud(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	first, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "jarvis-dev", ActorUserID: "admin-1"})
	require.NoError(t, err)
	require.True(t, first.Blocked)

	released, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Action: model.ProjectBlockActionUnblock, Reason: "restored", Confirmation: "jarvis-dev", ActorUserID: "admin-2"})
	require.NoError(t, err)
	require.False(t, released.Blocked)
	require.Equal(t, first.Generation+1, released.Generation)
	_, err = repo.GetByCanonicalKey(ctx, "Jarvis Dev")
	require.ErrorIs(t, err, ErrNotFound, "released projects must stop returning HTTP 423 immediately")

	var history int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM project_quarantine_commands WHERE canonical_project_key = 'jarvis-dev'").Scan(&history))
	require.Equal(t, 2, history)
}

func TestPostgresProjectBlockRepository_AckDeliveryIsSubjectBound(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "Jarvis Dev",
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

	got, err := repo.GetAckDelivery(ctx, "Jarvis Dev", block.CommandID, subjectA)
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
		CanonicalProjectKey: "Jarvis Dev",
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

	got, err := repo.GetAckDelivery(ctx, "Jarvis Dev", block.CommandID, sameAccountWithDaemon)
	require.NoError(t, err)
	require.Equal(t, delivery.AckToken, got.AckToken)
	require.Equal(t, accountOnly.AuthSubject, got.AckSubject.AuthSubject)
}

func TestPostgresProjectBlockRepository_QuarantineProgressUsesActiveAccountsAndCurrentGeneration(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "org-repo", ActorUserID: "admin-1"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES
		('00000000-0000-0000-0000-000000000011', 'Zoe', 'zoe@example.com', 'hash', true),
		('00000000-0000-0000-0000-000000000012', 'ada', 'ada@example.com', 'hash', true),
		('00000000-0000-0000-0000-000000000013', 'inactive', 'inactive@example.com', 'hash', false)`)
	require.NoError(t, err)
	for _, subject := range []string{"00000000-0000-0000-0000-000000000011", "00000000-0000-0000-0000-000000000013"} {
		delivery, err := repo.EnsureAckDelivery(ctx, block, model.ProjectBlockAckSubject{AuthSubject: subject})
		require.NoError(t, err)
		_, err = repo.RecordAck(ctx, model.ProjectBlockAck{CommandID: block.CommandID, CanonicalProjectKey: block.CanonicalProjectKey, AckToken: delivery.AckToken, AckSubject: model.ProjectBlockAckSubject{AuthSubject: subject}, Status: model.ProjectBlockAckApplied})
		require.NoError(t, err)
	}

	progress, err := repo.QuarantineProgress(ctx, "org-repo", block.Generation, "", 10)
	require.NoError(t, err)
	require.Equal(t, model.QuarantineProgressTotals{Active: 2, Acknowledged: 1, Pending: 1}, progress.Totals)
	require.Len(t, progress.Progress, 2)
	require.Equal(t, model.QuarantineProgressRow{Username: "ada", State: "pending"}, progress.Progress[0])
	require.Equal(t, "Zoe", progress.Progress[1].Username)
	require.Equal(t, model.ProjectBlockAckApplied, progress.Progress[1].State)
	require.NotNil(t, progress.Progress[1].AcknowledgedAt)
	require.NotContains(t, progress.Progress[1].Username, "@")
}

func TestPostgresProjectBlockRepository_QuarantineProgressCollapsesDuplicateAcknowledgementsAndPagesSafely(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "org-repo", ActorUserID: "admin-1"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES
		('00000000-0000-0000-0000-000000000021', 'Ada', 'ada@example.com', 'hash', true),
		('00000000-0000-0000-0000-000000000022', 'Bea', 'bea@example.com', 'hash', true),
		('00000000-0000-0000-0000-000000000023', 'Cyd', 'cyd@example.com', 'hash', true)`)
	require.NoError(t, err)

	ada := model.ProjectBlockAckSubject{AuthSubject: "00000000-0000-0000-0000-000000000021"}
	delivery, err := repo.EnsureAckDelivery(ctx, block, ada)
	require.NoError(t, err)
	_, err = repo.RecordAck(ctx, model.ProjectBlockAck{CommandID: block.CommandID, CanonicalProjectKey: block.CanonicalProjectKey, AckToken: delivery.AckToken, AckSubject: ada, Status: model.ProjectBlockAckFailed})
	require.NoError(t, err)
	_, err = repo.RecordAck(ctx, model.ProjectBlockAck{CommandID: block.CommandID, CanonicalProjectKey: block.CanonicalProjectKey, AckToken: delivery.AckToken, AckSubject: ada, Status: model.ProjectBlockAckApplied})
	require.NoError(t, err)

	first, err := repo.QuarantineProgress(ctx, block.CanonicalProjectKey, block.Generation, "", 1)
	require.NoError(t, err)
	require.Equal(t, model.QuarantineProgressTotals{Active: 3, Acknowledged: 1, Pending: 2}, first.Totals)
	require.Equal(t, []model.QuarantineProgressRow{{Username: "Ada", State: model.ProjectBlockAckApplied, AcknowledgedAt: first.Progress[0].AcknowledgedAt}}, first.Progress)
	require.NotNil(t, first.Progress[0].AcknowledgedAt)
	require.NotEmpty(t, first.NextCursor)

	second, err := repo.QuarantineProgress(ctx, block.CanonicalProjectKey, block.Generation, first.NextCursor, 1)
	require.NoError(t, err)
	require.Equal(t, model.QuarantineProgressTotals{Active: 3, Acknowledged: 1, Pending: 2}, second.Totals)
	require.Equal(t, []model.QuarantineProgressRow{{Username: "Bea", State: "pending"}}, second.Progress)

	_, err = repo.QuarantineProgress(ctx, "other-repo", block.Generation, first.NextCursor, 1)
	require.Error(t, err)
	_, err = repo.QuarantineProgress(ctx, block.CanonicalProjectKey, block.Generation+1, first.NextCursor, 1)
	require.Error(t, err)
}

func TestPostgresProjectBlockRepository_QuarantineProgressKeepsOlderGenerationConsistentAfterNewGeneration(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	first, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionBlock, Reason: "first", Confirmation: "org-repo", ActorUserID: "admin-1"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES ('00000000-0000-0000-0000-000000000031', 'Ada', 'ada@example.com', 'hash', true)`)
	require.NoError(t, err)
	subject := model.ProjectBlockAckSubject{AuthSubject: "00000000-0000-0000-0000-000000000031"}
	delivery, err := repo.EnsureAckDelivery(ctx, first, subject)
	require.NoError(t, err)
	_, err = repo.RecordAck(ctx, model.ProjectBlockAck{CommandID: first.CommandID, CanonicalProjectKey: first.CanonicalProjectKey, AckToken: delivery.AckToken, AckSubject: subject, Status: model.ProjectBlockAckApplied})
	require.NoError(t, err)

	second, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionUnblock, Reason: "release", Confirmation: "org-repo", ActorUserID: "admin-1"})
	require.NoError(t, err)
	require.Equal(t, first.Generation+1, second.Generation)

	older, err := repo.QuarantineProgress(ctx, first.CanonicalProjectKey, first.Generation, "", 10)
	require.NoError(t, err)
	require.Equal(t, first.Generation, older.Generation)
	require.Equal(t, model.QuarantineProgressTotals{Active: 1, Acknowledged: 1, Pending: 0}, older.Totals)
	require.Equal(t, model.ProjectBlockAckApplied, older.Progress[0].State)

	current, err := repo.QuarantineProgress(ctx, second.CanonicalProjectKey, second.Generation, "", 10)
	require.NoError(t, err)
	require.Equal(t, second.Generation, current.Generation)
	require.Equal(t, model.QuarantineProgressTotals{Active: 1, Acknowledged: 0, Pending: 1}, current.Totals)
	require.Equal(t, "pending", current.Progress[0].State)
}

func TestPostgresProjectBlockRepository_ListQuarantinesDerivesCurrentGenerationOutcome(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	block, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "org-repo", ActorUserID: "admin-1"})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES ('00000000-0000-0000-0000-000000000041', 'Ada', 'ada@example.com', 'hash', true)`)
	require.NoError(t, err)
	subject := model.ProjectBlockAckSubject{AuthSubject: "00000000-0000-0000-0000-000000000041"}
	delivery, err := repo.EnsureAckDelivery(ctx, block, subject)
	require.NoError(t, err)
	_, err = repo.RecordAck(ctx, model.ProjectBlockAck{CommandID: block.CommandID, CanonicalProjectKey: block.CanonicalProjectKey, AckToken: delivery.AckToken, AckSubject: subject, Status: model.ProjectBlockAckApplied})
	require.NoError(t, err)

	summaries, err := repo.ListQuarantines(ctx)

	require.NoError(t, err)
	require.Equal(t, []model.QuarantineSummary{{
		Project: "Org/Repo", CanonicalProjectKey: "org-repo", Generation: block.Generation,
		Action: model.ProjectBlockActionBlock, State: model.ProjectBlockAckApplied, TransitionedAt: block.BlockedAt,
	}}, summaries)
	for _, summary := range summaries {
		require.NotEmpty(t, summary.Project)
		require.NotEmpty(t, summary.State)
	}
}

func TestPostgresTxManager_ReadOnlyRepeatableReadKeepsAdminSnapshotDuringConcurrentTransition(t *testing.T) {
	pool, cleanup := startPostgresWithProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectBlockRepository(pool)
	first, err := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionBlock, Reason: "first", Confirmation: "org-repo", ActorUserID: "admin-1"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES ('00000000-0000-0000-0000-000000000042', 'Ada', 'ada@example.com', 'hash', true)`)
	require.NoError(t, err)

	var before, after model.QuarantineProgressResponse
	var summaries []model.QuarantineSummary
	err = NewPostgresTxManager(pool).ReadOnlyRepeatableRead(ctx, func(ctx context.Context, repos TxRepositories) error {
		var snapshotErr error
		before, snapshotErr = repos.ProjectBlocks.QuarantineProgress(ctx, first.CanonicalProjectKey, first.Generation, "", 10)
		if snapshotErr != nil {
			return snapshotErr
		}

		_, mutationErr := repo.BlockProject(ctx, model.ProjectBlockCreate{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Action: model.ProjectBlockActionUnblock, Reason: "release", Confirmation: "org-repo", ActorUserID: "admin-2"})
		if mutationErr != nil {
			return mutationErr
		}

		after, snapshotErr = repos.ProjectBlocks.QuarantineProgress(ctx, first.CanonicalProjectKey, first.Generation, "", 10)
		if snapshotErr != nil {
			return snapshotErr
		}
		summaries, snapshotErr = repos.ProjectBlocks.ListQuarantines(ctx)
		return snapshotErr
	})

	require.NoError(t, err)
	require.Equal(t, first.Generation, before.Generation)
	require.Equal(t, before, after)
	require.Equal(t, []model.QuarantineSummary{{
		Project: "Org/Repo", CanonicalProjectKey: "org-repo", Generation: first.Generation,
		Action: model.ProjectBlockActionBlock, State: model.ProjectBlockProgressPending, TransitionedAt: first.BlockedAt,
	}}, summaries)

	current, err := repo.ListQuarantines(ctx)
	require.NoError(t, err)
	require.Len(t, current, 1)
	require.Equal(t, first.Generation+1, current[0].Generation)
	require.Equal(t, model.ProjectBlockActionUnblock, current[0].Action)
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
		CanonicalProjectKey: "Blocked Project",
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
		CanonicalProjectKey: "Blocked Project",
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
		CanonicalProjectKey: "Blocked Count",
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
		CanonicalProjectKey: "Blocked Pull",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-pull",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	memories, hasMore, err := memoryRepo.PullSince(ctx, "blocked/pull", time.Time{}, nil, model.PullCursor{}, 10)
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
		CanonicalProjectKey: "Blocked Live",
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
		CanonicalProjectKey: "Blocked Growth",
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

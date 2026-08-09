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

// This file is the guarantee that hive-api never folds a project spelling.
//
// The rule used to be enforced by a source scanner. A scanner asks how a query
// is spelled, so it is only ever as good as the spellings someone thought of:
// two hardening rounds later it still could not see lower(coalesce(project,''))
// or regexp_replace(btrim(project), ...), and the fold it exists to prevent was
// spelled with both. The tests here ask what a query DOES. A fold written in a
// way nobody anticipated still returns the wrong rows, and still fails.
//
// Two properties, one table each:
//
//   - a project-scoped path answers for the exact literal its rows carry, and
//     for nothing else. Every read is asked twice: once with the stored
//     spelling, once with the spelling a fold would produce from it.
//   - a quarantine applies to the exact literal an admin blocked, and to
//     nothing else.
//
// The tables name every path. A path added without a row in one of them is a
// path nobody proved, and the gap is visible in review rather than in a
// dashboard reading another project's memories.

const (
	// storedProject and foldedProject are the same project to a human and two
	// different projects to this module. foldedProject is what the deleted fold
	// produced from storedProject: lowercased, separators collapsed to '-'.
	storedProject = "Foo.Bar"
	foldedProject = "foo-bar"

	// The non-ASCII pair. Case folding is locale- and encoding-dependent above
	// ASCII, so a fold that looked harmless on "Foo.Bar" can still merge or
	// split these two.
	storedUnicodeProject = "Straße.Zwei"
	foldedUnicodeProject = "straße-zwei"
)

// foldProbes are the spellings a read must NOT answer for, given rows stored
// under stored.
//
// One probe is not enough, and finding that out is why this list exists: the
// full fold differs from the stored spelling in case AND separators AND
// surrounding space, so asking only for "foo-bar" leaves a query folding just
// the case — lower(project) = lower($1) — passing, because "foo.bar" and
// "foo-bar" still differ. Each probe therefore isolates ONE dimension a fold
// can collapse, and the last combines them.
func foldProbes(stored string) []string {
	switch stored {
	case storedProject:
		return []string{
			"foo.bar",   // case
			"FOO.BAR",   // case, the other way
			"Foo-Bar",   // separator
			"Foo_Bar",   // separator, another spelling of it
			" Foo.Bar ", // surrounding whitespace
			"foo-bar",   // all of them: the fold that shipped
		}
	case storedUnicodeProject:
		return []string{
			"straße.zwei",   // case, non-ASCII
			"STRASSE.ZWEI",  // upper-casing ß expands it to SS
			"strasse.zwei",  // the transliteration an unaccent-style fold produces
			"Straße-Zwei",   // separator
			" Straße.Zwei ", // surrounding whitespace
			"straße-zwei",   // all of them
		}
	}
	t := stored
	panic("no fold probes defined for project spelling " + t)
}

// projectScopedRead is one path that answers a question about a named project.
//
// count returns how many rows the path attributes to that project: for a path
// that takes a project argument, the size of what it returns; for a path that
// reports on every project at once, the part of its output carrying that name.
type projectScopedRead struct {
	name  string
	count func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int
}

// projectScopedReads is every read path in this module whose answer depends on
// which project it is asked about.
//
// Deliberately absent, because they are not project-scoped:
// GetBySyncID, EndSession, GetSession, DeleteOlderThan, UserSyncProjection, and
// the whole user/audit surface. Write paths are covered by the quarantine table
// below, which is the only project-identity decision a write makes.
// GetByCanonicalKey needs a block to exist before it can answer, so it is
// asserted in TestQuarantineAppliesOnlyToTheBlockedLiteral instead.
func projectScopedReads() []projectScopedRead {
	return []projectScopedRead{
		{
			name: "MemoryRepository.PullSince",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memories, _, err := NewPostgresMemoryRepository(pool).PullSince(ctx, project, time.Time{}, nil, model.PullCursor{}, 0)
				require.NoError(t, err)
				return len(memories)
			},
		},
		{
			name: "MemoryRepository.List",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memories, err := NewPostgresMemoryRepository(pool).List(ctx, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return len(memories)
			},
		},
		{
			name: "MemoryRepository.Count",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				total, err := NewPostgresMemoryRepository(pool).Count(ctx, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return int(total)
			},
		},
		{
			name: "MemoryRepository.Search",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memories, err := NewPostgresMemoryRepository(pool).Search(ctx, scopeSearchTerm, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return len(memories)
			},
		},
		{
			name: "MemoryRepository.CountSearch",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				total, err := NewPostgresMemoryRepository(pool).CountSearch(ctx, scopeSearchTerm, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return int(total)
			},
		},
		{
			name: "MemoryRepository.CountByProject",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				counts, err := NewPostgresMemoryRepository(pool).CountByProject(ctx, model.MemoryFilter{})
				require.NoError(t, err)
				total := 0
				for _, count := range counts {
					if count.Project == project {
						total += int(count.Count)
					}
				}
				return total
			},
		},
		{
			name: "MemoryRepository.ListMemoryMutations",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				batch, err := NewPostgresMemoryRepository(pool).ListMemoryMutations(ctx, project, model.MutationCursor{}, 100)
				require.NoError(t, err)
				return len(batch.Events)
			},
		},
		{
			name: "MemoryRepository.ListActivityFeed",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				feed, err := NewPostgresMemoryRepository(pool).ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 100})
				require.NoError(t, err)
				rows := 0
				for _, row := range feed {
					if row.Project == project {
						rows++
					}
				}
				return rows
			},
		},
		{
			name: "MemoryRepository.ProjectExists",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				exists, err := NewPostgresMemoryRepository(pool).ProjectExists(ctx, project)
				require.NoError(t, err)
				return boolCount(exists)
			},
		},
		{
			name: "SessionRepository.ListSessionsSince",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				sessions, _, err := NewPostgresSessionRepository(pool).ListSessionsSince(ctx, project, time.Time{}, model.PullCursor{}, 0)
				require.NoError(t, err)
				return len(sessions)
			},
		},
		{
			name: "SyncAttemptRepository.ListForSummary",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				records, err := NewPostgresSyncAttemptRepository(pool).ListForSummary(ctx, model.SyncAttemptSummaryFilter{Project: project})
				require.NoError(t, err)
				return len(records)
			},
		},
		{
			name: "SyncAttemptRepository.SyncHealthByProject",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				rows, err := NewPostgresSyncAttemptRepository(pool).SyncHealthByProject(ctx, 24*time.Hour)
				require.NoError(t, err)
				return countHealthRows(rows, project)
			},
		},
		{
			name: "SyncAttemptRepository.ProjectSyncHealth",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				projection, err := NewPostgresSyncAttemptRepository(pool).ProjectSyncHealth(ctx)
				require.NoError(t, err)
				return countHealthRows(projection.Rows, project)
			},
		},
		{
			name: "ProjectRepository.ListAggregates",
			count: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
				require.NoError(t, err)
				matches := 0
				for _, aggregate := range aggregates {
					if aggregate.Name == project {
						matches++
					}
				}
				return matches
			},
		},
	}
}

// TestEveryProjectScopedReadAnswersOnlyForTheStoredLiteral is the behavioural
// replacement for the source scanner.
//
// Each path is asked for the stored spelling and must answer, then asked for
// the spelling a fold produces from it and must answer with nothing. A fold
// anywhere between the caller and the row breaks the second half, no matter how
// it is written, where it lives, or whether this repository contains its source
// at all.
func TestEveryProjectScopedReadAnswersOnlyForTheStoredLiteral(t *testing.T) {
	pool, cleanup := startPostgresWithEveryMigration(t)
	defer cleanup()

	ctx := context.Background()
	seedProjectScopeFixture(t, ctx, pool)

	for _, stored := range []string{storedProject, storedUnicodeProject} {
		for _, path := range projectScopedReads() {
			t.Run(path.name+"/"+stored, func(t *testing.T) {
				require.Positive(t, path.count(t, ctx, pool, stored),
					"%s found nothing for the spelling its rows are stored under; the fixture or the path is wrong", path.name)
				for _, probe := range foldProbes(stored) {
					require.Zero(t, path.count(t, ctx, pool, probe),
						"%s answered for %q with rows stored as %q: something between the caller and the row folds project identity",
						path.name, probe, stored)
				}
			})
		}
	}
}

// quarantineConsumer is one path whose result depends on whether a project is
// blocked. rowsFor reports how many rows it attributes to the named project.
type quarantineConsumer struct {
	name    string
	rowsFor func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int
}

// quarantineReaders is every read path that applies the block predicate.
//
// ListMemoryMutations and ListForSummary are absent on purpose. They carry no
// block predicate, so asserting one here would pin behaviour the module does
// not have. Their project scoping is still proved in the table above. Each is
// absent for a different reason, and the difference matters:
//
//   - ListMemoryMutations SHOULD NOT return rows for a quarantined project, and
//     nothing in the query stops it. It is safe today only because it has
//     exactly one production caller — syncService.pushWithRepos in
//     internal/service/sync.go — and that function calls precheckBlockedProjects
//     on its first line, aborting the whole push before the pull ever runs.
//     The guard therefore lives in the caller, not in the query. A second caller
//     that reaches this method without running that precheck first will hand a
//     daemon the mutation journal of a quarantined project. If you are adding
//     such a caller, you have two options: precheck the project the same way
//     pushWithRepos does, or push the predicate down into the query
//     (unblockedProjectPredicate, as ListSessionsSince does) and add
//     ListMemoryMutations to the table below.
//     TestSync_Push_BlockedProjectNeverReachesTheMutationJournal in
//     internal/service pins the existing caller's guard.
//
//   - ListForSummary is intentionally not quarantined. It reads sync-attempt
//     telemetry, not user memory content: the record that a project attempted to
//     sync, and whether it succeeded. Blocking a project is a reason to stop
//     serving its content, not a reason to erase the operational history of the
//     block taking effect. Do not add a predicate here.
//
// CountLiveActivity and CountGrowthByMonth are block consumers but report
// totals, not per-project rows, so the shape here cannot express them.
// TestQuarantineRemovesTheBlockedProjectFromAggregateCounts covers them.
func quarantineReaders() []quarantineConsumer {
	return []quarantineConsumer{
		{
			name: "MemoryRepository.List",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memories, err := NewPostgresMemoryRepository(pool).List(ctx, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return len(memories)
			},
		},
		{
			name: "MemoryRepository.Count",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				total, err := NewPostgresMemoryRepository(pool).Count(ctx, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return int(total)
			},
		},
		{
			name: "MemoryRepository.Search",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memories, err := NewPostgresMemoryRepository(pool).Search(ctx, scopeSearchTerm, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return len(memories)
			},
		},
		{
			name: "MemoryRepository.CountSearch",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				total, err := NewPostgresMemoryRepository(pool).CountSearch(ctx, scopeSearchTerm, model.MemoryFilter{Project: project})
				require.NoError(t, err)
				return int(total)
			},
		},
		{
			name: "MemoryRepository.PullSince",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memories, _, err := NewPostgresMemoryRepository(pool).PullSince(ctx, project, time.Time{}, nil, model.PullCursor{}, 0)
				require.NoError(t, err)
				return len(memories)
			},
		},
		{
			name: "MemoryRepository.GetByID",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				memory, err := NewPostgresMemoryRepository(pool).GetByID(ctx, memoryIDForProject(t, ctx, pool, project))
				if err != nil {
					require.ErrorIs(t, err, ErrNotFound)
					return 0
				}
				return boolCount(memory != nil)
			},
		},
		{
			name: "MemoryRepository.CountByProject",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				counts, err := NewPostgresMemoryRepository(pool).CountByProject(ctx, model.MemoryFilter{})
				require.NoError(t, err)
				total := 0
				for _, count := range counts {
					if count.Project == project {
						total += int(count.Count)
					}
				}
				return total
			},
		},
		{
			name: "MemoryRepository.ListActivityFeed",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				feed, err := NewPostgresMemoryRepository(pool).ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{Limit: 100})
				require.NoError(t, err)
				rows := 0
				for _, row := range feed {
					if row.Project == project {
						rows++
					}
				}
				return rows
			},
		},
		{
			name: "SessionRepository.ListSessionsSince",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				sessions, _, err := NewPostgresSessionRepository(pool).ListSessionsSince(ctx, project, time.Time{}, model.PullCursor{}, 0)
				require.NoError(t, err)
				return len(sessions)
			},
		},
		{
			name: "SyncAttemptRepository.SyncHealthByProject",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				rows, err := NewPostgresSyncAttemptRepository(pool).SyncHealthByProject(ctx, 24*time.Hour)
				require.NoError(t, err)
				return countHealthRows(rows, project)
			},
		},
		{
			name: "SyncAttemptRepository.ProjectSyncHealth",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				projection, err := NewPostgresSyncAttemptRepository(pool).ProjectSyncHealth(ctx)
				require.NoError(t, err)
				return countHealthRows(projection.Rows, project)
			},
		},
		{
			name: "ProjectRepository.ListAggregates",
			rowsFor: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) int {
				aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
				require.NoError(t, err)
				matches := 0
				for _, aggregate := range aggregates {
					if aggregate.Name == project {
						matches++
					}
				}
				return matches
			},
		},
	}
}

// quarantineWriters is every write path that runs checkProjectBlocked. Each
// returns the error the write produced for that project.
func quarantineWriters() []struct {
	name  string
	write func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error
} {
	return []struct {
		name  string
		write func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error
	}{
		{
			name: "MemoryRepository.Create",
			write: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error {
				_, err := NewPostgresMemoryRepository(pool).Create(ctx, newScopeMemory(project, uniqueSyncID(t, ctx, pool)))
				return err
			},
		},
		{
			name: "MemoryRepository.Upsert",
			write: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error {
				_, _, err := NewPostgresMemoryRepository(pool).Upsert(ctx, newScopeMemory(project, uniqueSyncID(t, ctx, pool)))
				return err
			},
		},
		{
			name: "SessionRepository.CreateSession",
			write: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error {
				return NewPostgresSessionRepository(pool).CreateSession(ctx, newScopeSession(project, uniqueSyncID(t, ctx, pool)))
			},
		},
		{
			name: "SessionRepository.UpsertSession",
			write: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error {
				return NewPostgresSessionRepository(pool).UpsertSession(ctx, newScopeSession(project, uniqueSyncID(t, ctx, pool)))
			},
		},
		{
			name: "SessionRepository.EnsureManualSaveSession",
			write: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error {
				_, err := NewPostgresSessionRepository(pool).EnsureManualSaveSession(ctx, project)
				return err
			},
		},
		{
			name: "PromptRepository.Upsert",
			write: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) error {
				_, err := NewPostgresPromptRepository(pool).Upsert(ctx, &model.Prompt{
					SyncID:    uniqueSyncID(t, ctx, pool),
					Project:   project,
					Content:   "scope probe",
					CreatedBy: "tester",
					CreatedAt: scopeBaseTime,
				})
				return err
			},
		},
	}
}

// TestQuarantineAppliesOnlyToTheBlockedLiteral is the block-predicate half of
// the same guarantee.
//
// An admin blocks the stored spelling. Every path that consults a block must
// then hide that project and leave every sibling — genuinely different projects
// that a fold would merge with it — completely alone. A fold in the predicate
// quarantines a project nobody named, which is the defect that shipped.
func TestQuarantineAppliesOnlyToTheBlockedLiteral(t *testing.T) {
	pool, cleanup := startPostgresWithEveryMigration(t)
	defer cleanup()

	ctx := context.Background()
	seedQuarantineFixture(t, ctx, pool)

	siblings := quarantineSiblings()
	readers := quarantineReaders()
	before := make(map[string]map[string]int, len(readers))
	for _, reader := range readers {
		counts := map[string]int{storedProject: reader.rowsFor(t, ctx, pool, storedProject)}
		require.Positive(t, counts[storedProject], "%s must see the project before it is blocked", reader.name)
		for _, sibling := range siblings {
			counts[sibling] = reader.rowsFor(t, ctx, pool, sibling)
			require.Positive(t, counts[sibling],
				"%s must see sibling %q before anything is blocked", reader.name, sibling)
		}
		before[reader.name] = counts
	}

	_, err := NewPostgresProjectBlockRepository(pool).BlockProject(ctx, model.ProjectBlockCreate{
		Project:      storedProject,
		Action:       "block",
		Reason:       "scope probe",
		Confirmation: storedProject,
	})
	require.NoError(t, err)

	for _, reader := range readers {
		t.Run("read/"+reader.name, func(t *testing.T) {
			require.Zero(t, reader.rowsFor(t, ctx, pool, storedProject),
				"%s still reads a quarantined project", reader.name)
			for _, sibling := range siblings {
				require.Equal(t, before[reader.name][sibling], reader.rowsFor(t, ctx, pool, sibling),
					"%s changed what it reports for %q, which nobody blocked: the predicate folds the spelling it was given",
					reader.name, sibling)
			}
		})
	}

	for _, writer := range quarantineWriters() {
		t.Run("write/"+writer.name, func(t *testing.T) {
			require.ErrorIs(t, writer.write(t, ctx, pool, storedProject), ErrProjectBlocked,
				"%s wrote into a quarantined project", writer.name)
			for _, sibling := range siblings {
				require.NoError(t, writer.write(t, ctx, pool, sibling),
					"%s refused a write to %q, which nobody blocked", writer.name, sibling)
			}
		})
	}

	t.Run("read/ProjectBlockRepository.GetByCanonicalKey", func(t *testing.T) {
		blocks := NewPostgresProjectBlockRepository(pool)
		block, err := blocks.GetByCanonicalKey(ctx, storedProject)
		require.NoError(t, err)
		require.Equal(t, storedProject, block.CanonicalProjectKey,
			"the block is stored and found under the exact literal the admin supplied")

		for _, sibling := range siblings {
			_, err = blocks.GetByCanonicalKey(ctx, sibling)
			require.ErrorIs(t, err, ErrNotFound,
				"GetByCanonicalKey resolved a block for %q, which nobody blocked", sibling)
		}
	})
}

// TestQuarantineRemovesTheBlockedProjectFromAggregateCounts covers the two
// block consumers that report a total rather than per-project rows.
//
// They cannot say which project a number came from, so the assertion is the
// size of the change: blocking a project must remove exactly that project's
// memories from each total and leave the sibling's alone. A folded predicate
// removes both and overshoots.
func TestQuarantineRemovesTheBlockedProjectFromAggregateCounts(t *testing.T) {
	pool, cleanup := startPostgresWithEveryMigration(t)
	defer cleanup()

	ctx := context.Background()
	seedQuarantineFixture(t, ctx, pool)
	repo := NewPostgresMemoryRepository(pool)

	seeded := 1 + len(quarantineSiblings())
	liveBefore, _, err := repo.CountLiveActivity(ctx, scopeBaseTime.Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, seeded, liveBefore, "the fixture seeds one memory per project")
	require.Equal(t, seeded, totalGrowth(t, ctx, repo))

	_, err = NewPostgresProjectBlockRepository(pool).BlockProject(ctx, model.ProjectBlockCreate{
		Project:      storedProject,
		Action:       "block",
		Reason:       "scope probe",
		Confirmation: storedProject,
	})
	require.NoError(t, err)

	liveAfter, _, err := repo.CountLiveActivity(ctx, scopeBaseTime.Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, seeded-1, liveAfter,
		"CountLiveActivity must drop the blocked project's memory and keep every sibling's")
	require.Equal(t, seeded-1, totalGrowth(t, ctx, repo),
		"CountGrowthByMonth must drop the blocked project's memory and keep every sibling's")
}

func totalGrowth(t *testing.T, ctx context.Context, repo MemoryRepository) int {
	t.Helper()
	months, err := repo.CountGrowthByMonth(ctx, 24)
	require.NoError(t, err)
	total := 0
	for _, month := range months {
		total += month.Value
	}
	return total
}

// --- fixture ---

const scopeSearchTerm = "quarkbait"

var (
	scopeBaseTime       = time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	scopeWriteSessionID = "scope-session-0"
)

// startPostgresWithEveryMigration applies the migration set the server applies.
// These tests are about what a deployed database does, so they run against the
// schema a deployed database has, not a hand-picked subset of it.
func startPostgresWithEveryMigration(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	pool, cleanup := startPostgres(t)
	for _, sql := range migrations.Ordered() {
		require.NoError(t, RunMigrations(pool, sql))
	}
	return pool, cleanup
}

// seedProjectScopeFixture stores rows under the two stored spellings only.
//
// The folded spellings are deliberately left empty. That is what makes a folded
// read's empty answer mean something: the only rows a fold could possibly
// return for "foo-bar" are the ones stored under "Foo.Bar", so any non-empty
// answer is the leak itself. Requiring the stored spelling to answer separately
// rules out the other explanation, a query that returns nothing to anyone.
func seedProjectScopeFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for index, project := range []string{storedProject, storedUnicodeProject} {
		seedProjectRows(t, ctx, pool, project, index)
	}
}

// quarantineSiblings are the projects that must survive a block on
// storedProject: one per fold dimension, because a predicate that folds only
// some of them is still a predicate that quarantines a project nobody named.
//
// A single sibling is not enough, and the reason is worth keeping. With only
// "foo-bar" here, a predicate folding case and collapsing separators still
// passed: that fold keeps '.', so "Foo.Bar" became "foo.bar" and never met
// "foo-bar". The sibling that catches a fold is the one differing from the
// blocked spelling in exactly what that fold collapses.
func quarantineSiblings() []string { return foldProbes(storedProject) }

// seedQuarantineFixture stores rows under the blocked spelling and under every
// sibling. All must exist: whether blocking one leaves the others untouched is
// unanswerable if the others are absent.
func seedQuarantineFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for index, project := range append([]string{storedProject}, quarantineSiblings()...) {
		seedProjectRows(t, ctx, pool, project, index)
	}
}

func seedProjectRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string, index int) {
	t.Helper()

	userID := scopeUUID(index, 1)
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, password, level, is_active)
		VALUES ($1, $2, $3, 'hash', 'member', true)`,
		userID, "scope-user-"+scopeSuffix(index), "scope-"+scopeSuffix(index)+"@example.com")
	require.NoError(t, err)

	sessionID := "scope-session-" + scopeSuffix(index)
	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (id, sync_id, project, dev_id, client, started_at, created_at, updated_at, synced_at)
		VALUES ($1, $2, $3, 'tester', 'test', $4, $4, $4, $4)`,
		sessionID, scopeUUID(index, 2), project, scopeBaseTime)
	require.NoError(t, err)

	memorySyncID := scopeUUID(index, 3)
	_, err = pool.Exec(ctx, `
		INSERT INTO memories (sync_id, project, category, title, content, created_by, created_at, updated_at, synced_at, session_id)
		VALUES ($1, $2, 'decision', $3, $4, 'tester', $5, $5, $5, $6)`,
		memorySyncID, project, "scope "+scopeSearchTerm, "content "+scopeSearchTerm, scopeBaseTime, sessionID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO user_prompts (sync_id, project, content, created_by, created_at, synced_at)
		VALUES ($1, $2, 'scope prompt', 'tester', $3, $3)`,
		scopeUUID(index, 4), project, scopeBaseTime)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO memory_mutations (event_id, entity_type, entity_sync_id, project, op, occurred_at, actor_id)
		VALUES ($1, 'memory', $2, $3, 'create', $4, 'tester')`,
		scopeUUID(index, 5), memorySyncID, project, scopeBaseTime)
	require.NoError(t, err)

	// Sync health projections need a recent attempt bound to an active portal
	// user; the health window is relative to now.
	_, err = pool.Exec(ctx, `
		INSERT INTO sync_attempt_logs (attempt_id, source_dev_id, project, started_at, ended_at, outcome, portal_user_id)
		VALUES ($1, 'tester', $2, now(), now(), 'success', $3)`,
		"scope-attempt-"+scopeSuffix(index), project, userID)
	require.NoError(t, err)

	require.NoError(t, RegisterProjectIdentity(ctx, pool, project, "", scopeBaseTime))
}

func newScopeMemory(project, syncID string) *model.Memory {
	return &model.Memory{
		SyncID:    syncID,
		Project:   project,
		Category:  model.CatDecision,
		Title:     "scope write",
		Content:   "scope write",
		CreatedBy: "tester",
		CreatedAt: scopeBaseTime,
		UpdatedAt: scopeBaseTime,
		SessionID: &scopeWriteSessionID,
	}
}

func newScopeSession(project, syncID string) *model.Session {
	return &model.Session{
		ID:        "scope-write-" + syncID,
		SyncID:    syncID,
		Project:   project,
		DevID:     "tester",
		Client:    "test",
		StartedAt: scopeBaseTime,
	}
}

// uniqueSyncID mints a sync id no fixture row uses, so a write probe can never
// collide with seeded data or with an earlier probe.
func uniqueSyncID(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id))
	return id
}

func memoryIDForProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, project string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id::text FROM memories WHERE project = $1 ORDER BY created_at LIMIT 1`, project).Scan(&id))
	return id
}

func countHealthRows(rows []model.ProjectSyncHealthRow, project string) int {
	matches := 0
	for _, row := range rows {
		if row.Project == project {
			matches++
		}
	}
	return matches
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func scopeSuffix(index int) string {
	return string(rune('0' + index))
}

func scopeUUID(index, slot int) string {
	return "0000000" + scopeSuffix(index) + "-0000-0000-0000-00000000000" + string(rune('0'+slot))
}

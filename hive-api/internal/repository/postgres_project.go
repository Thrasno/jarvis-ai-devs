package repository

import (
	"context"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresProjectRepository struct {
	db pgxQuerier
}

func NewPostgresProjectRepository(pool *pgxpool.Pool) ProjectRepository {
	return newPostgresProjectRepositoryWithQuerier(pool)
}

func newPostgresProjectRepositoryWithQuerier(db pgxQuerier) ProjectRepository {
	return &postgresProjectRepository{db: db}
}

func (r *postgresProjectRepository) ListAggregates(ctx context.Context) ([]model.ProjectAggregate, error) {
	// Rows group on the literal project spelling they carry. The API never
	// derives a key, so two spellings are one project only when they are equal.
	//
	// The display name is then resolved through project_identities, which is
	// keyed by that same literal. The join is therefore exact equality: it can
	// only attach a name to a project, never merge two of them. Grouping stays on
	// the row literal so the join cannot influence which rows belong together.
	//
	// A project with no registry row falls back to the literal its rows carry.
	q := fmt.Sprintf(`
WITH memory_rows AS (
    SELECT %s AS project_key, created_at, updated_at, deleted_at, restored_at
    FROM memories WHERE %s
),
session_rows AS (
    SELECT %s AS project_key, started_at, ended_at
    FROM sessions WHERE %s
),
sync_rows AS (
    SELECT %s AS project_key, outcome, started_at, ended_at, ingested_at, id
    FROM sync_attempt_logs WHERE %s
),
projects AS (
    SELECT project_key FROM memory_rows
    UNION
    SELECT project_key FROM session_rows
    UNION
    SELECT project_key FROM sync_rows
),
memory_agg AS (
    SELECT
        project_key,
        COUNT(*) FILTER (WHERE deleted_at IS NULL) AS memory_count,
        MAX(GREATEST(created_at, updated_at, COALESCE(deleted_at, '-infinity'::timestamptz), COALESCE(restored_at, '-infinity'::timestamptz))) AS last_memory_at
    FROM memory_rows
    GROUP BY project_key
),
session_agg AS (
    SELECT
        project_key,
        COUNT(*) AS session_count,
        MAX(COALESCE(ended_at, started_at)) AS last_session_at
    FROM session_rows
    GROUP BY project_key
),
latest_sync AS (
    SELECT DISTINCT ON (project_key)
        project_key,
        outcome,
        COALESCE(ended_at, started_at) AS last_sync_at
    FROM sync_rows
    ORDER BY project_key, COALESCE(ended_at, started_at) DESC, ingested_at DESC, id DESC
)
SELECT
    COALESCE(NULLIF(i.remote_spelling, ''), i.first_spelling, p.project_key),
    COALESCE(m.memory_count, 0),
    COALESCE(s.session_count, 0),
    m.last_memory_at,
    s.last_session_at,
    ls.last_sync_at,
    ls.outcome
FROM projects p
LEFT JOIN project_identities i ON i.project_key = p.project_key
LEFT JOIN memory_agg m ON m.project_key = p.project_key
LEFT JOIN session_agg s ON s.project_key = p.project_key
LEFT JOIN latest_sync ls ON ls.project_key = p.project_key
ORDER BY COALESCE(NULLIF(i.remote_spelling, ''), i.first_spelling, p.project_key)`,
		"memories.project", unblockedProjectPredicate("memories.project"),
		"sessions.project", unblockedProjectPredicate("sessions.project"),
		"sync_attempt_logs.project", unblockedProjectPredicate("sync_attempt_logs.project"))

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, wrapPgError(err, "List project aggregates")
	}
	defer rows.Close()

	records := []model.ProjectAggregate{}
	for rows.Next() {
		var record model.ProjectAggregate
		var outcome *string
		if err := rows.Scan(
			&record.Name,
			&record.MemoryCount,
			&record.SessionCount,
			&record.LastMemoryAt,
			&record.LastSessionAt,
			&record.LastSyncAt,
			&outcome,
		); err != nil {
			return nil, wrapPgError(err, "Scan project aggregate row")
		}
		if outcome != nil {
			parsed := model.SyncAttemptOutcome(*outcome)
			record.LatestSyncOutcome = &parsed
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project aggregate rows: %w", err)
	}
	return records, nil
}

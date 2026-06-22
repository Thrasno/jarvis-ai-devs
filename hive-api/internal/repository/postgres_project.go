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
	const q = `
WITH projects AS (
    SELECT project FROM memories
    UNION
    SELECT project FROM sessions
    UNION
    SELECT project FROM sync_attempt_logs
),
memory_agg AS (
    SELECT
        project,
        COUNT(*) FILTER (WHERE deleted_at IS NULL) AS memory_count,
        MAX(GREATEST(created_at, updated_at, COALESCE(deleted_at, '-infinity'::timestamptz), COALESCE(restored_at, '-infinity'::timestamptz))) AS last_memory_at
    FROM memories
    GROUP BY project
),
session_agg AS (
    SELECT
        project,
        COUNT(*) AS session_count,
        MAX(COALESCE(ended_at, started_at)) AS last_session_at
    FROM sessions
    GROUP BY project
),
latest_sync AS (
    SELECT DISTINCT ON (project)
        project,
        outcome,
        COALESCE(ended_at, started_at) AS last_sync_at
    FROM sync_attempt_logs
    ORDER BY project, COALESCE(ended_at, started_at) DESC, ingested_at DESC, id DESC
)
SELECT
    p.project,
    COALESCE(m.memory_count, 0),
    COALESCE(s.session_count, 0),
    m.last_memory_at,
    s.last_session_at,
    ls.last_sync_at,
    ls.outcome
FROM projects p
LEFT JOIN memory_agg m ON m.project = p.project
LEFT JOIN session_agg s ON s.project = p.project
LEFT JOIN latest_sync ls ON ls.project = p.project
ORDER BY p.project`

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

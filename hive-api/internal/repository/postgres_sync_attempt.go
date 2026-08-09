package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresSyncAttemptRepository struct {
	db   pgxQuerier
	pool *pgxpool.Pool
}

func NewPostgresSyncAttemptRepository(pool *pgxpool.Pool) SyncAttemptRepository {
	return &postgresSyncAttemptRepository{db: pool, pool: pool}
}

func newPostgresSyncAttemptRepositoryWithQuerier(db pgxQuerier) SyncAttemptRepository {
	return &postgresSyncAttemptRepository{db: db}
}

func (r *postgresSyncAttemptRepository) UpsertBatch(ctx context.Context, attempts []model.SyncAttemptLog) (model.SyncAttemptStoreResult, error) {
	if r.pool == nil {
		return r.upsertBatch(ctx, r.db, attempts)
	}
	result := model.SyncAttemptStoreResult{}
	for _, attempt := range attempts {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return result, wrapPgError(err, "begin sync attempt write")
		}
		stored, err := r.upsertBatch(ctx, tx, []model.SyncAttemptLog{attempt})
		if err != nil {
			_ = tx.Rollback(ctx)
			return result, err
		}
		if err := tx.Commit(ctx); err != nil {
			return result, wrapPgError(err, "commit sync attempt write")
		}
		result.AcceptedIDs = append(result.AcceptedIDs, stored.AcceptedIDs...)
		result.DuplicateIDs = append(result.DuplicateIDs, stored.DuplicateIDs...)
	}
	return result, nil
}

func (r *postgresSyncAttemptRepository) upsertBatch(ctx context.Context, db pgxQuerier, attempts []model.SyncAttemptLog) (model.SyncAttemptStoreResult, error) {
	result := model.SyncAttemptStoreResult{}
	for _, attempt := range attempts {
		syncCounts, err := json.Marshal(nonNilIntMap(attempt.SyncCounts))
		if err != nil {
			return result, fmt.Errorf("marshal sync attempt counts: %w", err)
		}
		metadata, err := json.Marshal(nonNilStringMap(attempt.Metadata))
		if err != nil {
			return result, fmt.Errorf("marshal sync attempt metadata: %w", err)
		}

		const q = `
INSERT INTO sync_attempt_logs
	    (attempt_id, source_dev_id, project, client, daemon_id, started_at, ended_at, outcome,
	     http_status, error_code, error_message, request_id, sync_counts, metadata, portal_user_id, portal_user_source)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (source_dev_id, attempt_id) DO NOTHING`
		tag, err := db.Exec(ctx, q,
			attempt.AttemptID, attempt.DevID, attempt.Project, attempt.Client, attempt.DaemonID,
			attempt.StartedAt, attempt.EndedAt, string(attempt.Outcome), attempt.HTTPStatus,
			attempt.ErrorCode, attempt.ErrorMessage, attempt.RequestID, syncCounts, metadata, attempt.PortalUserID, attempt.PortalUserSource,
		)
		if err != nil {
			return result, wrapPgError(err, "Upsert sync attempt")
		}
		if tag.RowsAffected() == 0 {
			result.DuplicateIDs = append(result.DuplicateIDs, attempt.AttemptID)
			continue
		}
		result.AcceptedIDs = append(result.AcceptedIDs, attempt.AttemptID)
	}
	return result, nil
}

func (r *postgresSyncAttemptRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM sync_attempt_logs WHERE COALESCE(ended_at, started_at) < $1`, cutoff)
	if err != nil {
		return 0, wrapPgError(err, "Delete old sync attempts")
	}
	return tag.RowsAffected(), nil
}

func (r *postgresSyncAttemptRepository) ListForSummary(ctx context.Context, filter model.SyncAttemptSummaryFilter) ([]model.SyncAttemptSummaryRecord, error) {
	const q = `
SELECT source_dev_id, project, COALESCE(client, ''), COALESCE(daemon_id, ''), started_at, outcome, error_code
FROM sync_attempt_logs
WHERE started_at >= $1
  AND ($2 = '' OR project = $2)
  AND ($3 = '' OR source_dev_id = $3)
  AND ($4 = '' OR client = $4)
  AND ($5 = '' OR daemon_id = $5)
  AND ($6 = '' OR outcome = $6)
  AND ($7 = '' OR error_code = $7)
ORDER BY started_at DESC`
	rows, err := r.db.Query(ctx, q, filter.Since, filter.Project, filter.DevID, filter.Client, filter.DaemonID, filter.Outcome, filter.ErrorCode)
	if err != nil {
		return nil, wrapPgError(err, "List sync attempts for summary")
	}
	defer rows.Close()

	records := []model.SyncAttemptSummaryRecord{}
	for rows.Next() {
		var record model.SyncAttemptSummaryRecord
		var outcome string
		if err := rows.Scan(&record.DevID, &record.Project, &record.Client, &record.DaemonID, &record.StartedAt, &outcome, &record.ErrorCode); err != nil {
			return nil, wrapPgError(err, "Scan sync attempt summary row")
		}
		record.Outcome = model.SyncAttemptOutcome(outcome)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "Iterate sync attempt summary rows")
	}
	return records, nil
}

// SyncHealthByProject returns per-project sync health for the given window.
func (r *postgresSyncAttemptRepository) SyncHealthByProject(ctx context.Context, window time.Duration) ([]model.ProjectSyncHealthRow, error) {
	since := time.Now().UTC().Add(-window)

	q := fmt.Sprintf(`
		SELECT p.project, last.outcome, p.contributors, last.started_at FROM (
		  SELECT project, COUNT(DISTINCT source_dev_id) AS contributors
		  FROM sync_attempt_logs
		  WHERE project <> '' AND started_at >= $1
		    AND %s
		  GROUP BY project
		) p
		JOIN LATERAL (
		  SELECT outcome, started_at FROM sync_attempt_logs s
		  WHERE s.project = p.project AND s.started_at >= $1
		  ORDER BY started_at DESC LIMIT 1
		) last ON true
		ORDER BY
		  CASE
		    WHEN last.outcome = 'failure' THEN 0
		    WHEN last.outcome = 'success' THEN 2
		    ELSE 1
		  END,
		  last.started_at DESC,
		  p.project ASC`, unblockedProjectPredicate("sync_attempt_logs.project"))

	rows, err := r.db.Query(ctx, q, since)
	if err != nil {
		return nil, wrapPgError(err, "SyncHealthByProject")
	}
	defer rows.Close()

	result := []model.ProjectSyncHealthRow{}
	for rows.Next() {
		var row model.ProjectSyncHealthRow
		var outcome string
		if err := rows.Scan(&row.Project, &outcome, &row.ContributorCount, &row.LastActivityAt); err != nil {
			return nil, wrapPgError(err, "scan SyncHealthByProject row")
		}
		row.LastOutcome = model.SyncAttemptOutcome(outcome)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate SyncHealthByProject rows")
	}
	return result, nil
}

func (r *postgresSyncAttemptRepository) ProjectSyncHealth(ctx context.Context) (model.ProjectSyncHealthProjection, error) {
	q := fmt.Sprintf(`
		WITH latest AS (
			SELECT DISTINCT ON (s.project, s.portal_user_id)
				s.project, s.outcome, COALESCE(s.ended_at, s.started_at) AS activity_at
			FROM sync_attempt_logs s
			JOIN users u ON u.id = s.portal_user_id AND u.is_active = true
			WHERE s.project <> '' AND s.portal_user_id IS NOT NULL AND %s
			ORDER BY s.project, s.portal_user_id, COALESCE(s.ended_at, s.started_at) DESC,
				s.ingested_at DESC, s.attempt_id DESC, s.id DESC
		), health AS (
			SELECT project,
				CASE WHEN BOOL_OR(outcome = 'failure') THEN 'failure' ELSE 'success' END AS outcome,
				COUNT(*)::int AS contributors, MAX(activity_at) AS last_activity_at
			FROM latest GROUP BY project
		)
		SELECT project, outcome, contributors, last_activity_at,
			COUNT(*) OVER (), COUNT(*) FILTER (WHERE outcome = 'failure') OVER ()
		FROM health
		ORDER BY CASE WHEN outcome = 'failure' THEN 0 ELSE 1 END, last_activity_at DESC, project`, unblockedProjectPredicate("s.project"))
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return model.ProjectSyncHealthProjection{}, wrapPgError(err, "ProjectSyncHealth")
	}
	defer rows.Close()
	projection := model.ProjectSyncHealthProjection{Rows: []model.ProjectSyncHealthRow{}}
	for rows.Next() {
		var row model.ProjectSyncHealthRow
		var outcome string
		if err := rows.Scan(&row.Project, &outcome, &row.ContributorCount, &row.LastActivityAt, &projection.Total, &projection.Degraded); err != nil {
			return model.ProjectSyncHealthProjection{}, wrapPgError(err, "scan ProjectSyncHealth row")
		}
		row.LastOutcome = model.SyncAttemptOutcome(outcome)
		projection.Rows = append(projection.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return model.ProjectSyncHealthProjection{}, wrapPgError(err, "iterate ProjectSyncHealth rows")
	}
	return projection, nil
}

func (r *postgresSyncAttemptRepository) UserSyncProjection(ctx context.Context, now time.Time) (model.UserSyncProjection, error) {
	const q = `
		WITH completed_attempts AS (
			SELECT portal_user_id, ended_at, outcome,
				ROW_NUMBER() OVER (
					PARTITION BY portal_user_id
					ORDER BY ended_at DESC, ingested_at DESC, attempt_id DESC, id DESC
				) AS latest_rank,
				MAX(ended_at) FILTER (WHERE outcome = 'success' AND ended_at <= $1) OVER (
					PARTITION BY portal_user_id
				) AS latest_success_ended_at
			FROM sync_attempt_logs
			WHERE portal_user_id IS NOT NULL
				AND ended_at IS NOT NULL
		)
		SELECT u.id::text, u.is_active, a.ended_at, a.outcome, a.latest_success_ended_at
		FROM users u
		LEFT JOIN completed_attempts a ON a.portal_user_id = u.id AND a.latest_rank = 1
		ORDER BY u.id`

	rows, err := r.db.Query(ctx, q, now.UTC())
	if err != nil {
		return model.UserSyncProjection{}, wrapPgError(err, "UserSyncProjection")
	}
	defer rows.Close()

	projection := model.UserSyncProjection{Rows: []model.UserSyncProjectionRow{}}
	for rows.Next() {
		var row model.UserSyncProjectionRow
		var outcome *string
		if err := rows.Scan(&row.PortalUserID, &row.IsActive, &row.LatestEndedAt, &outcome, &row.LatestSuccessEndedAt); err != nil {
			return model.UserSyncProjection{}, wrapPgError(err, "scan UserSyncProjection row")
		}
		if outcome != nil {
			value := model.SyncAttemptOutcome(*outcome)
			row.LatestOutcome = &value
		}
		projection.Rows = append(projection.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return model.UserSyncProjection{}, wrapPgError(err, "iterate UserSyncProjection rows")
	}
	return projection, nil
}

func nonNilIntMap(values map[string]int) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	return values
}

func nonNilStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

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
	db pgxQuerier
}

func NewPostgresSyncAttemptRepository(pool *pgxpool.Pool) SyncAttemptRepository {
	return newPostgresSyncAttemptRepositoryWithQuerier(pool)
}

func newPostgresSyncAttemptRepositoryWithQuerier(db pgxQuerier) SyncAttemptRepository {
	return &postgresSyncAttemptRepository{db: db}
}

func (r *postgresSyncAttemptRepository) UpsertBatch(ctx context.Context, attempts []model.SyncAttemptLog) (model.SyncAttemptStoreResult, error) {
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
     http_status, error_code, error_message, request_id, sync_counts, metadata)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (source_dev_id, attempt_id) DO NOTHING`
		tag, err := r.db.Exec(ctx, q,
			attempt.AttemptID, attempt.DevID, attempt.Project, attempt.Client, attempt.DaemonID,
			attempt.StartedAt, attempt.EndedAt, string(attempt.Outcome), attempt.HTTPStatus,
			attempt.ErrorCode, attempt.ErrorMessage, attempt.RequestID, syncCounts, metadata,
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

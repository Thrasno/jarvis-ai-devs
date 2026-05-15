package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresAuditRepository struct {
	db pgxQuerier
}

func NewPostgresAuditRepository(pool *pgxpool.Pool) AuditRepository {
	return newPostgresAuditRepositoryWithQuerier(pool)
}

func newPostgresAuditRepositoryWithQuerier(db pgxQuerier) AuditRepository {
	return &postgresAuditRepository{db: db}
}

func (r *postgresAuditRepository) Insert(ctx context.Context, entry *model.AuditEntry) error {
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Now().UTC()
	}
	if entry.Metadata == nil {
		entry.Metadata = model.AuditMetadata{}
	}
	entry.Metadata = model.SanitizeAuditMetadata(entry.Action, entry.Metadata)

	metadataJSON, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}

	const q = `
		INSERT INTO audit_logs
			(occurred_at, actor_user_id, project, action, outcome, entry_count, reason_code, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, occurred_at`

	err = r.db.QueryRow(ctx, q,
		entry.OccurredAt,
		entry.ActorUserID,
		entry.Project,
		entry.Action,
		entry.Outcome,
		entry.EntryCount,
		entry.ReasonCode,
		metadataJSON,
	).Scan(&entry.ID, &entry.OccurredAt)
	return wrapPgError(err, "Insert audit log")
}

func (r *postgresAuditRepository) List(ctx context.Context, filter model.AuditFilter) ([]*model.AuditEntry, error) {
	filter = filter.Normalize()
	where, args := buildAuditFilterWhere(filter, 3)
	args = append([]interface{}{filter.Limit, filter.Offset}, args...)

	q := fmt.Sprintf(`SELECT id, occurred_at, actor_user_id, project, action, outcome,
						 entry_count, reason_code, metadata
					FROM audit_logs
					WHERE 1=1 %s
					ORDER BY occurred_at DESC, id DESC
					LIMIT $1 OFFSET $2`, where)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, wrapPgError(err, "List audit logs")
	}
	defer rows.Close()

	return scanAuditRows(rows)
}

func (r *postgresAuditRepository) Count(ctx context.Context, filter model.AuditFilter) (int64, error) {
	where, args := buildAuditFilterWhere(filter.Normalize(), 1)
	q := fmt.Sprintf(`SELECT COUNT(*) FROM audit_logs WHERE 1=1 %s`, where)

	var count int64
	err := r.db.QueryRow(ctx, q, args...).Scan(&count)
	return count, wrapPgError(err, "Count audit logs")
}

func buildAuditFilterWhere(filter model.AuditFilter, startIdx int) (string, []interface{}) {
	clauses := []string{}
	args := []interface{}{}
	argIdx := startIdx

	add := func(sql string, value interface{}) {
		clauses = append(clauses, fmt.Sprintf(sql, argIdx))
		args = append(args, value)
		argIdx++
	}

	if filter.Project != nil && *filter.Project != "" {
		add("project = $%d", *filter.Project)
	}
	if filter.ActorUserID != nil && *filter.ActorUserID != "" {
		add("actor_user_id = $%d", *filter.ActorUserID)
	}
	if filter.Action != nil && *filter.Action != "" {
		add("action = $%d", *filter.Action)
	}
	if filter.Outcome != nil && *filter.Outcome != "" {
		add("outcome = $%d", *filter.Outcome)
	}
	if filter.Since != nil {
		add("occurred_at >= $%d", *filter.Since)
	}
	if filter.Until != nil {
		add("occurred_at <= $%d", *filter.Until)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func scanAuditRows(rows pgx.Rows) ([]*model.AuditEntry, error) {
	entries := []*model.AuditEntry{}
	for rows.Next() {
		entry, err := scanAuditRow(rows)
		if err != nil {
			return nil, wrapPgError(err, "scan audit row")
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanAuditRow(row pgx.Row) (*model.AuditEntry, error) {
	entry := &model.AuditEntry{}
	var metadataRaw []byte

	err := row.Scan(
		&entry.ID,
		&entry.OccurredAt,
		&entry.ActorUserID,
		&entry.Project,
		&entry.Action,
		&entry.Outcome,
		&entry.EntryCount,
		&entry.ReasonCode,
		&metadataRaw,
	)
	if err != nil {
		return nil, err
	}
	if len(metadataRaw) > 0 {
		if err := json.Unmarshal(metadataRaw, &entry.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal audit metadata: %w", err)
		}
	}
	if entry.Metadata == nil {
		entry.Metadata = model.AuditMetadata{}
	}
	return entry, nil
}

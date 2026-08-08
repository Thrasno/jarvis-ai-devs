package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/projectkey"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresProjectBlockRepository struct {
	db pgxQuerier
}

func NewPostgresProjectBlockRepository(pool *pgxpool.Pool) ProjectBlockRepository {
	return newPostgresProjectBlockRepositoryWithQuerier(pool)

}

func newPostgresProjectBlockRepositoryWithQuerier(db pgxQuerier) ProjectBlockRepository {
	return &postgresProjectBlockRepository{db: db}
}

func (r *postgresProjectBlockRepository) BlockProject(ctx context.Context, create model.ProjectBlockCreate) (*model.ProjectBlock, error) {
	create.CanonicalProjectKey = projectkey.Canonicalize(create.CanonicalProjectKey)
	if create.CanonicalProjectKey == "" {
		create.CanonicalProjectKey = projectkey.Canonicalize(create.Project)
	}
	const q = `
	INSERT INTO project_blocks (project, canonical_project_key, action, reason, confirmation, export_marker, actor_user_id, blocked, blocked_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $3 <> 'unblock', now())
	ON CONFLICT (canonical_project_key) DO UPDATE SET
    project = EXCLUDED.project,
    action = EXCLUDED.action,
    reason = EXCLUDED.reason,
    confirmation = EXCLUDED.confirmation,
	    export_marker = EXCLUDED.export_marker,
	    actor_user_id = EXCLUDED.actor_user_id,
	    command_id = gen_random_uuid(),
	    generation = project_blocks.generation + 1,
	    blocked = EXCLUDED.blocked,
    blocked_at = now(),
    updated_at = now()
		RETURNING id::text, command_id::text, ack_token, project, canonical_project_key, action, generation, reason, confirmation, export_marker,
	          COALESCE(actor_user_id, ''), blocked, blocked_at, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, create.Project, create.CanonicalProjectKey, create.Action, create.Reason, create.Confirmation, create.ExportMarker, create.ActorUserID)
	return scanProjectBlock(row)
}

func (r *postgresProjectBlockRepository) GetByCanonicalKey(ctx context.Context, canonicalProjectKey string) (*model.ProjectBlock, error) {
	canonicalProjectKey = projectkey.Canonicalize(canonicalProjectKey)
	const q = `
		SELECT id::text, command_id::text, ack_token, project, canonical_project_key, action, generation, reason, confirmation, export_marker,
       COALESCE(actor_user_id, ''), blocked, blocked_at, created_at, updated_at
FROM project_blocks
	WHERE canonical_project_key = $1 AND blocked = true`
	block, err := scanProjectBlock(r.db.QueryRow(ctx, q, canonicalProjectKey))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, wrapPgError(err, "get project block")
	}
	return block, nil
}

func (r *postgresProjectBlockRepository) ListInboxCommands(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error) {
	if !subject.Valid() {
		return nil, ErrNotFound
	}
	const ensureDeliveries = `
		INSERT INTO project_block_ack_deliveries (command_id, canonical_project_key, ack_auth_subject, ack_daemon_id, ack_client)
		SELECT command_id, canonical_project_key, $1, $2, $3
		FROM project_quarantine_commands
		ON CONFLICT (command_id, canonical_project_key, ack_auth_subject) DO UPDATE SET
			ack_daemon_id = EXCLUDED.ack_daemon_id,
			ack_client = EXCLUDED.ack_client,
			updated_at = now()`
	if _, err := r.db.Exec(ctx, ensureDeliveries, subject.AuthSubject, subject.DaemonID, subject.Client); err != nil {
		return nil, wrapPgError(err, "ensure project quarantine inbox deliveries")
	}
	const q = `
		SELECT c.command_id::text, d.ack_token, c.project, c.canonical_project_key, c.reason, c.action, c.generation, c.blocked_at
		FROM project_quarantine_commands c
		JOIN project_block_ack_deliveries d
		  ON d.command_id = c.command_id
		 AND d.canonical_project_key = c.canonical_project_key
		WHERE d.ack_auth_subject = $1
		ORDER BY c.canonical_project_key ASC, c.generation ASC`
	rows, err := r.db.Query(ctx, q, subject.AuthSubject)
	if err != nil {
		return nil, wrapPgError(err, "list project quarantine inbox")
	}
	defer rows.Close()
	commands := make([]model.ProjectBlockCommand, 0)
	for rows.Next() {
		var command model.ProjectBlockCommand
		if err := rows.Scan(&command.CommandID, &command.AckToken, &command.Project, &command.CanonicalProjectKey, &command.Reason, &command.Action, &command.Generation, &command.BlockedAt); err != nil {
			return nil, wrapPgError(err, "scan project quarantine inbox")
		}
		commands = append(commands, command)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate project quarantine inbox")
	}
	return commands, nil
}

func (r *postgresProjectBlockRepository) RecordAck(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error) {
	ack.CanonicalProjectKey = projectkey.Canonicalize(ack.CanonicalProjectKey)
	ack.AppliedAt = time.Now().UTC()
	const q = `
			INSERT INTO project_block_acks (command_id, canonical_project_key, ack_token, ack_auth_subject, ack_daemon_id, ack_client, status, warning, applied_at)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (command_id, canonical_project_key, ack_auth_subject) DO UPDATE SET
			    ack_token = EXCLUDED.ack_token,
			    ack_daemon_id = EXCLUDED.ack_daemon_id,
			    ack_client = EXCLUDED.ack_client,
			    status = EXCLUDED.status,
		    warning = EXCLUDED.warning,
		    applied_at = EXCLUDED.applied_at,
	    updated_at = now()
		RETURNING command_id::text, canonical_project_key, ack_token, ack_auth_subject, ack_daemon_id, ack_client, status, warning, applied_at`
	row := r.db.QueryRow(ctx, q, ack.CommandID, ack.CanonicalProjectKey, ack.AckToken, ack.AckSubject.AuthSubject, ack.AckSubject.DaemonID, ack.AckSubject.Client, ack.Status, ack.Warning, ack.AppliedAt)
	if err := row.Scan(&ack.CommandID, &ack.CanonicalProjectKey, &ack.AckToken, &ack.AckSubject.AuthSubject, &ack.AckSubject.DaemonID, &ack.AckSubject.Client, &ack.Status, &ack.Warning, &ack.AppliedAt); err != nil {
		return model.ProjectBlockAck{}, wrapPgError(err, "record project block ack")
	}
	return ack, nil
}

func (r *postgresProjectBlockRepository) EnsureAckDelivery(ctx context.Context, block *model.ProjectBlock, subject model.ProjectBlockAckSubject) (model.ProjectBlockCommand, error) {
	if block == nil || !subject.Valid() {
		return model.ProjectBlockCommand{}, ErrNotFound
	}
	const q = `
			INSERT INTO project_block_ack_deliveries (command_id, canonical_project_key, ack_auth_subject, ack_daemon_id, ack_client)
			VALUES ($1::uuid, $2, $3, $4, $5)
			ON CONFLICT (command_id, canonical_project_key, ack_auth_subject) DO UPDATE SET
			    ack_daemon_id = EXCLUDED.ack_daemon_id,
			    ack_client = EXCLUDED.ack_client,
			    updated_at = now()
			RETURNING ack_token`
	block.CanonicalProjectKey = projectkey.Canonicalize(block.CanonicalProjectKey)
	cmd := block.Command()
	if err := r.db.QueryRow(ctx, q, block.CommandID, block.CanonicalProjectKey, subject.AuthSubject, subject.DaemonID, subject.Client).Scan(&cmd.AckToken); err != nil {
		return model.ProjectBlockCommand{}, wrapPgError(err, "ensure project block ack delivery")
	}
	return cmd, nil
}

func (r *postgresProjectBlockRepository) GetAckDelivery(ctx context.Context, canonicalProjectKey, commandID string, subject model.ProjectBlockAckSubject) (*model.ProjectBlockAckDelivery, error) {
	canonicalProjectKey = projectkey.Canonicalize(canonicalProjectKey)
	const q = `
			SELECT command_id::text, canonical_project_key, ack_token, ack_auth_subject, ack_daemon_id, ack_client
			FROM project_block_ack_deliveries
			WHERE canonical_project_key = $1 AND command_id = $2::uuid AND ack_auth_subject = $3`
	delivery := &model.ProjectBlockAckDelivery{}
	err := r.db.QueryRow(ctx, q, canonicalProjectKey, commandID, subject.AuthSubject).Scan(&delivery.CommandID, &delivery.CanonicalProjectKey, &delivery.AckToken, &delivery.AckSubject.AuthSubject, &delivery.AckSubject.DaemonID, &delivery.AckSubject.Client)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, wrapPgError(err, "get project block ack delivery")
	}
	return delivery, nil
}

func (r *postgresProjectBlockRepository) LatestAckForCommand(ctx context.Context, canonicalProjectKey, commandID string) (*model.ProjectBlockAck, error) {
	canonicalProjectKey = projectkey.Canonicalize(canonicalProjectKey)
	const q = `
		SELECT command_id::text, canonical_project_key, ack_token, ack_auth_subject, ack_daemon_id, ack_client, status, warning, applied_at
	FROM project_block_acks
	WHERE canonical_project_key = $1 AND command_id = $2::uuid
	ORDER BY updated_at DESC, created_at DESC
	LIMIT 1`
	ack := &model.ProjectBlockAck{}
	err := r.db.QueryRow(ctx, q, canonicalProjectKey, commandID).Scan(&ack.CommandID, &ack.CanonicalProjectKey, &ack.AckToken, &ack.AckSubject.AuthSubject, &ack.AckSubject.DaemonID, &ack.AckSubject.Client, &ack.Status, &ack.Warning, &ack.AppliedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, wrapPgError(err, "latest project block ack for command")
	}
	return ack, nil
}

func (r *postgresProjectBlockRepository) QuarantineProgress(ctx context.Context, canonicalProjectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error) {
	if limit < 1 || limit > 100 {
		return model.QuarantineProgressResponse{}, ErrNotFound
	}
	canonicalProjectKey = projectkey.Canonicalize(canonicalProjectKey)
	if canonicalProjectKey == "" || generation < 1 {
		return model.QuarantineProgressResponse{}, ErrNotFound
	}
	cursor, err := model.DecodeQuarantineCursor(after, canonicalProjectKey, generation)
	if err != nil {
		return model.QuarantineProgressResponse{}, err
	}
	const commandQuery = `SELECT project, action FROM project_quarantine_commands WHERE canonical_project_key = $1 AND generation = $2`
	result := model.QuarantineProgressResponse{CanonicalProjectKey: canonicalProjectKey, Generation: generation, Progress: []model.QuarantineProgressRow{}}
	if err := r.db.QueryRow(ctx, commandQuery, canonicalProjectKey, generation).Scan(&result.Project, &result.Action); err != nil {
		if err == pgx.ErrNoRows {
			return model.QuarantineProgressResponse{}, ErrNotFound
		}
		return model.QuarantineProgressResponse{}, wrapPgError(err, "load quarantine command")
	}
	const q = `
		WITH latest_ack AS (
			SELECT DISTINCT ON (a.ack_auth_subject) a.ack_auth_subject, a.status, a.applied_at
			FROM project_block_acks a
			JOIN project_quarantine_commands c ON c.command_id = a.command_id
			WHERE c.canonical_project_key = $1 AND c.generation = $2
			ORDER BY a.ack_auth_subject, a.applied_at DESC, a.updated_at DESC,
				CASE a.status WHEN 'applied' THEN 3 WHEN 'failed' THEN 2 ELSE 1 END DESC
		), relation AS (
			SELECT u.id::text AS user_id, u.username, lower(u.username) AS username_key, a.status, a.applied_at
			FROM users u LEFT JOIN latest_ack a ON a.ack_auth_subject = u.id::text
			WHERE u.is_active = true
		), counted AS (
			SELECT *, count(*) OVER () AS active, count(status) OVER () AS acknowledged FROM relation
		)
		SELECT md5(user_id) AS cursor_id, username, COALESCE(status, 'pending'), applied_at, active, acknowledged
		FROM counted
		WHERE ($3 = '' OR (username_key, username, md5(user_id)) > ($3, $4, $5))
		ORDER BY username_key ASC, username ASC, md5(user_id) ASC LIMIT $6`
	rows, err := r.db.Query(ctx, q, canonicalProjectKey, generation, strings.ToLower(cursor.Username), cursor.Username, cursor.CursorID, limit+1)
	if err != nil {
		return model.QuarantineProgressResponse{}, wrapPgError(err, "load quarantine progress")
	}
	defer rows.Close()
	var last model.QuarantineCursor
	for rows.Next() {
		var cursorID, username, state string
		var acknowledgedAt *time.Time
		var active, acknowledged int
		if err := rows.Scan(&cursorID, &username, &state, &acknowledgedAt, &active, &acknowledged); err != nil {
			return model.QuarantineProgressResponse{}, wrapPgError(err, "scan quarantine progress")
		}
		if result.Totals.Active == 0 {
			result.Totals = model.QuarantineProgressTotals{Active: active, Acknowledged: acknowledged, Pending: active - acknowledged}
		}
		if len(result.Progress) == limit {
			result.NextCursor = last.Encode()
			continue
		}
		result.Progress = append(result.Progress, model.QuarantineProgressRow{Username: username, State: state, AcknowledgedAt: acknowledgedAt})
		last = model.QuarantineCursor{CanonicalProjectKey: canonicalProjectKey, Generation: generation, Username: username, CursorID: cursorID}
	}
	if err := rows.Err(); err != nil {
		return model.QuarantineProgressResponse{}, wrapPgError(err, "iterate quarantine progress")
	}
	return result, nil
}

func (r *postgresProjectBlockRepository) ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error) {
	const q = `WITH current_commands AS (
			SELECT DISTINCT ON (canonical_project_key) project, canonical_project_key, generation, action, blocked_at, command_id
			FROM project_quarantine_commands
			ORDER BY canonical_project_key, generation DESC
		)
		SELECT COALESCE(NULLIF(i.remote_spelling, ''), i.first_spelling, c.project), c.canonical_project_key, c.generation, c.action,
			COALESCE((
				SELECT a.status
				FROM project_block_acks a
				JOIN users u ON u.id::text = a.ack_auth_subject AND u.is_active = true
				WHERE a.command_id = c.command_id
				ORDER BY a.applied_at DESC, a.updated_at DESC,
					CASE a.status WHEN 'applied' THEN 3 WHEN 'failed' THEN 2 ELSE 1 END DESC
				LIMIT 1
			), 'pending') AS state,
			c.blocked_at
		FROM current_commands c
		LEFT JOIN project_identities i ON i.project_key = c.canonical_project_key
		ORDER BY c.canonical_project_key ASC
		LIMIT 100`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, wrapPgError(err, "list quarantines")
	}
	defer rows.Close()
	result := make([]model.QuarantineSummary, 0)
	for rows.Next() {
		var summary model.QuarantineSummary
		if err := rows.Scan(&summary.Project, &summary.CanonicalProjectKey, &summary.Generation, &summary.Action, &summary.State, &summary.TransitionedAt); err != nil {
			return nil, wrapPgError(err, "scan quarantine summary")
		}
		result = append(result, summary)
	}
	return result, rows.Err()
}

func scanProjectBlock(row pgx.Row) (*model.ProjectBlock, error) {
	block := &model.ProjectBlock{}
	err := row.Scan(&block.ID, &block.CommandID, &block.AckToken, &block.Project, &block.CanonicalProjectKey, &block.Action, &block.Generation, &block.Reason,
		&block.Confirmation, &block.ExportMarker, &block.ActorUserID, &block.Blocked, &block.BlockedAt, &block.CreatedAt, &block.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return block, nil
}

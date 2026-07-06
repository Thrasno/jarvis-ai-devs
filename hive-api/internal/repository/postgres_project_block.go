package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
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
	const q = `
INSERT INTO project_blocks (project, canonical_project_key, action, reason, confirmation, export_marker, actor_user_id, blocked, blocked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, true, now())
ON CONFLICT (canonical_project_key) DO UPDATE SET
    project = EXCLUDED.project,
    action = EXCLUDED.action,
    reason = EXCLUDED.reason,
    confirmation = EXCLUDED.confirmation,
	    export_marker = EXCLUDED.export_marker,
	    actor_user_id = EXCLUDED.actor_user_id,
	    command_id = gen_random_uuid(),
	    blocked = true,
    blocked_at = now(),
    updated_at = now()
	RETURNING id::text, command_id::text, ack_token, project, canonical_project_key, action, reason, confirmation, export_marker,
	          COALESCE(actor_user_id, ''), blocked, blocked_at, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, create.Project, create.CanonicalProjectKey, create.Action, create.Reason, create.Confirmation, create.ExportMarker, create.ActorUserID)
	return scanProjectBlock(row)
}

func (r *postgresProjectBlockRepository) GetByCanonicalKey(ctx context.Context, canonicalProjectKey string) (*model.ProjectBlock, error) {
	const q = `
	SELECT id::text, command_id::text, ack_token, project, canonical_project_key, action, reason, confirmation, export_marker,
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

func (r *postgresProjectBlockRepository) RecordAck(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error) {
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
	cmd := block.Command()
	if err := r.db.QueryRow(ctx, q, block.CommandID, block.CanonicalProjectKey, subject.AuthSubject, subject.DaemonID, subject.Client).Scan(&cmd.AckToken); err != nil {
		return model.ProjectBlockCommand{}, wrapPgError(err, "ensure project block ack delivery")
	}
	return cmd, nil
}

func (r *postgresProjectBlockRepository) GetAckDelivery(ctx context.Context, canonicalProjectKey, commandID string, subject model.ProjectBlockAckSubject) (*model.ProjectBlockAckDelivery, error) {
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

func scanProjectBlock(row pgx.Row) (*model.ProjectBlock, error) {
	block := &model.ProjectBlock{}
	err := row.Scan(&block.ID, &block.CommandID, &block.AckToken, &block.Project, &block.CanonicalProjectKey, &block.Action, &block.Reason,
		&block.Confirmation, &block.ExportMarker, &block.ActorUserID, &block.Blocked, &block.BlockedAt, &block.CreatedAt, &block.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return block, nil
}

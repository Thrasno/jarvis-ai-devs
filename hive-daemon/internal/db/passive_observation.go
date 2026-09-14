package db

import (
	"context"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

// SavePassiveObservation persists a raw observation captured by the
// subagent-stop hook. session_id, project, and source may be empty strings
// when the hook cannot resolve them. sync_id is stored as NULL; it is reserved
// for future Hive sync integration and has no value at capture time.
func (d *DB) SavePassiveObservation(ctx context.Context, sessionID, project, source, content string) error {
	const q = `
INSERT INTO passive_observations (session_id, project, source, content)
VALUES (?, ?, ?, ?)`

	if _, err := d.sqlDB.ExecContext(ctx, q, sessionID, project, source, content); err != nil {
		return fmt.Errorf("save passive observation: %w", err)
	}
	return nil
}

// SavePassiveObservationWithSession atomically materializes a regular session
// and persists its attributed passive observation.
func (d *DB) SavePassiveObservationWithSession(ctx context.Context, in models.PassiveObservationWrite) error {
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save passive observation with session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	session, err := d.ensureSessionInTx(ctx, tx, in.Session, reopenForWrite)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO passive_observations (session_id, project, source, content)
VALUES (?, ?, ?, ?)`, in.Session.ID, session.Project, in.Source, in.Content); err != nil {
		return fmt.Errorf("save passive observation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save passive observation with session: %w", err)
	}
	return nil
}

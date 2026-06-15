package db

import (
	"context"
	"fmt"
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
